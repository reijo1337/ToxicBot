package on_photo

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/linkcontext"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	pkglogger "github.com/reijo1337/ToxicBot/pkg/logger"
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

// spec: PHOTO-024
// spec: PHOTO-025
func TestHandle_CaptionLinkRetainsPhotoAndPersistenceRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		link        chathistory.Link
		sendErr     error
		wantEntries int
	}{
		{name: "success", link: chathistory.Link{URL: "https://example.com/", Anchor: "почитай тут", Title: "Сайт", PreviewDescription: "Обложка книги"}, wantEntries: 2},
		{name: "preview_failed", link: chathistory.Link{URL: "https://example.com/", Anchor: "почитай тут", Title: "Сайт"}, wantEntries: 2},
		{name: "page_failed", wantEntries: 2},
		{name: "send_failed", link: chathistory.Link{URL: "https://example.com/", Title: "Сайт"}, sendErr: errors.New("send"), wantEntries: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			env.handler.r = rand.New(zeroSource{})
			env.handler.logger = pkglogger.New()
			history := chathistory.NewBuffer(10)
			env.handler.history = history
			env.setupPhotoPipeline("На изображении кот")
			msg := photoMessage(50, "😀 почитай тут", replyToBotMessage())
			msg.CaptionEntities = telebot.Entities{
				{Type: telebot.EntityTextLink, Offset: 3, Length: 11, URL: "https://example.com/"},
			}
			env.handler.links = enrichmentFunc(
				func(_ context.Context, chatID int64, ref linkcontext.Reference) chathistory.Link {
					assert.Equal(t, testChatID, chatID)
					assert.Equal(
						t,
						linkcontext.Reference{URL: "https://example.com/", Anchor: "почитай тут"},
						ref,
					)
					return tc.link
				},
			)
			env.generator.EXPECT().
				GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), float32(1), true, gomock.Any()).
				DoAndReturn(func(_ context.Context, entries []chathistory.Entry, _ float32, _ bool, _ string) message.GenerationResult {
					require.Len(t, entries, 1)
					assert.Equal(t, tc.link, entries[0].Link)
					assert.Contains(t, entries[0].Text, "На изображении кот")
					assert.NotContains(t, entries[0].Text, "Обложка книги")
					assert.Equal(t, int64(7), entries[0].SenderID)
					assert.True(t, entries[0].PreFormatted)
					return message.GenerationResult{Message: "reply"}
				})
			env.replier.EXPECT().Reply(msg, "reply").Return(&telebot.Message{ID: 51}, tc.sendErr)
			require.ErrorIs(t, env.handler.Handle(newCtx(msg, goodSender())), tc.sendErr)
			entries := history.Get(testChatID)
			require.Len(t, entries, tc.wantEntries)
			if len(entries) > 0 {
				assert.Equal(t, tc.link, entries[0].Link)
			}
		})
	}
}

// spec: PHOTO-025
func TestHandle_SkippedPhotoDoesNotEnrichCaption(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.settings.EXPECT().
		GetForChat(gomock.Any(), testChatID).
		Return(&chatsettings.Settings{PhotoReactChance: 0}, nil)
	env.handler.links = enrichmentFunc(
		func(_ context.Context, _ int64, _ linkcontext.Reference) chathistory.Link {
			t.Error("skipped photo must not start enrichment")
			return chathistory.Link{}
		},
	)
	require.NoError(
		t,
		env.handler.Handle(newCtx(photoMessage(50, "https://example.com/", nil), goodSender())),
	)
}

// spec: PHOTO-024
func TestHandle_CaptionLinkKeepsTriggerSnapshot(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.handler.r = rand.New(zeroSource{})
	env.handler.logger = pkglogger.New()
	history := chathistory.NewBuffer(10)
	env.handler.history = history
	env.setupPhotoPipeline("На изображении кот")
	msg := photoMessage(50, "https://example.com/", replyToBotMessage())
	env.handler.links = enrichmentFunc(
		func(_ context.Context, _ int64, _ linkcontext.Reference) chathistory.Link {
			history.Add(
				testChatID,
				chathistory.Entry{
					ID:     60,
					Text:   "later message",
					Author: "@bob",
					Time:   msg.Time().Add(time.Second),
				},
			)
			return chathistory.Link{URL: "https://example.com/", Title: "Article"}
		},
	)
	env.generator.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), float32(1), true, gomock.Any()).
		DoAndReturn(func(_ context.Context, entries []chathistory.Entry, _ float32, _ bool, _ string) message.GenerationResult {
			require.Len(t, entries, 1, "later message must not replace the photo as current turn")
			assert.Equal(t, msg.ID, entries[0].ID)
			assert.Equal(t, "Article", entries[0].Link.Title)
			return message.GenerationResult{Message: "reply"}
		})
	env.replier.EXPECT().Reply(msg, "reply").Return(&telebot.Message{ID: 51}, nil)
	require.NoError(t, env.handler.Handle(newCtx(msg, goodSender())))
	assert.Len(t, history.Get(testChatID), 3)
}
