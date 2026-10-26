package bulling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/linkcontext"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

type enrichmentFunc func(context.Context, int64, linkcontext.Reference) chathistory.Link

func (f enrichmentFunc) Enrich(
	ctx context.Context,
	chatID int64,
	ref linkcontext.Reference,
) chathistory.Link {
	return f(ctx, chatID, ref)
}

func setLink(t *testing.T, env *attributionEnv, result chathistory.Link) {
	t.Helper()
	env.contexts = env.contexts[:1]
	env.contexts[0].msg.Text = "😀 почитай тут и https://second.example/"
	env.contexts[0].msg.Entities = telebot.Entities{
		{Type: telebot.EntityTextLink, Offset: 3, Length: 11, URL: "https://example.com/article"},
	}
	env.handler.links = enrichmentFunc(
		func(_ context.Context, chatID int64, ref linkcontext.Reference) chathistory.Link {
			assert.Equal(t, int64(7), chatID)
			assert.Equal(
				t,
				linkcontext.Reference{URL: "https://example.com/article", Anchor: "почитай тут"},
				ref,
			)
			return result
		},
	)
}

// spec: BULL-015
func TestHandle_LinkEnrichedOnSkippedReaction(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	link := chathistory.Link{
		URL:                "https://example.com/article",
		Anchor:             "почитай тут",
		Title:              "Статья",
		PreviewDescription: "Обложка",
	}
	setLink(t, env, link)
	env.handler.settingsProvider = stubSettings{
		s: &chatsettings.Settings{ThresholdCount: 100, ThresholdTime: time.Minute},
	}
	require.NoError(t, env.handler.Handle(env.contexts[0]))
	entries := env.history.Get(7)
	require.Len(t, entries, 1)
	assert.Equal(t, link, entries[0].Link)
	assert.Equal(t, int64(101), entries[0].SenderID)
	assert.Equal(t, env.contexts[0].msg.Text, entries[0].Text)
	assert.False(t, entries[0].PreFormatted)

	env.handler.settingsProvider = stubSettings{
		s: &chatsettings.Settings{ThresholdCount: 1, ThresholdTime: time.Minute},
	}
	env.handler.cooldown["7:101"] = time.Now().Add(time.Hour)
	require.NoError(t, env.handler.Handle(env.contexts[0]))
	assert.Len(t, env.history.Get(7), 2)
}

// spec: BULL-016
// spec: BULL-017
func TestHandle_LinkCurrentReplyAndSendFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		link        chathistory.Link
		sendErr     error
		wantEntries int
	}{
		{name: "success", link: chathistory.Link{URL: "https://example.com/article", Title: "Статья"}, wantEntries: 2},
		{name: "send_failed", link: chathistory.Link{URL: "https://example.com/article", Description: "Описание"}, sendErr: errors.New("send"), wantEntries: 1},
		{name: "enrichment_failed", wantEntries: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newAttributionEnv(t)
			setLink(t, env, tc.link)
			env.generator.EXPECT().
				GetMessageTextWithHistory(gomock.Any(), gomock.Any(), float32(0), false).
				DoAndReturn(func(_ context.Context, entries []chathistory.Entry, _ float32, _ bool) message.GenerationResult {
					require.Len(t, entries, 1)
					assert.Equal(t, tc.link, entries[0].Link)
					assert.Equal(t, int64(101), entries[0].SenderID)
					return message.GenerationResult{Message: "reply"}
				})
			env.replier.EXPECT().
				Reply(env.contexts[0].msg, "reply").
				Return(&telebot.Message{ID: 10}, tc.sendErr)
			require.ErrorIs(t, env.handler.Handle(env.contexts[0]), tc.sendErr)
			entries := env.history.Get(7)
			require.Len(t, entries, tc.wantEntries)
			assert.Equal(t, tc.link, entries[0].Link)
		})
	}
}

// spec: BULL-015
// spec: BULL-016
func TestHandle_SlowLinkKeepsTriggerSnapshotAndCooldown(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.handler.settingsProvider = stubSettings{s: &chatsettings.Settings{
		ThresholdCount: 1, ThresholdTime: time.Minute, Cooldown: time.Hour,
	}}
	env.contexts = env.contexts[:1]
	first := env.contexts[0]
	first.msg.Text = "https://example.com/"
	started, release := make(chan struct{}), make(chan struct{})
	env.handler.links = enrichmentFunc(
		func(_ context.Context, _ int64, _ linkcontext.Reference) chathistory.Link {
			close(started)
			<-release
			return chathistory.Link{URL: "https://example.com/", Title: "Article"}
		},
	)
	var generated [][]chathistory.Entry
	env.generator.EXPECT().GetMessageTextWithHistory(gomock.Any(), gomock.Any(), float32(0), false).
		DoAndReturn(func(_ context.Context, entries []chathistory.Entry, _ float32, _ bool) message.GenerationResult {
			generated = append(generated, entries)
			return message.GenerationResult{Message: "reply"}
		}).
		AnyTimes()
	env.replier.EXPECT().
		Reply(gomock.Any(), "reply").
		Return(&telebot.Message{ID: 10}, nil).
		AnyTimes()
	finished := make(chan error, 1)
	go func() { finished <- env.handler.Handle(first) }()
	<-started
	second := &attributionContext{fakeCtx: &fakeCtx{
		chat: first.chat, sender: first.sender, bot: first.bot,
		msg: &telebot.Message{ID: 2, Text: "later message", Unixtime: first.msg.Unixtime + 1},
	}}
	secondErr := env.handler.Handle(second)
	close(release)
	require.NoError(t, secondErr)
	require.NoError(t, <-finished)
	require.Len(t, generated, 1, "the later message must observe the already reserved cooldown")
	require.Len(
		t,
		generated[0],
		1,
		"a later turn must not become the target of the original response",
	)
	assert.Equal(t, first.msg.ID, generated[0][0].ID)
	assert.Equal(t, "Article", generated[0][0].Link.Title)
	assert.Len(t, env.history.Get(7), 3)
}
