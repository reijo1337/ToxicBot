package bulling

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

type zeroSource struct{}

func (zeroSource) Int63() int64 { return 0 }
func (zeroSource) Seed(int64)   {}

type attributionContext struct {
	*fakeCtx

	notifyErr error
}

func (c *attributionContext) Notify(telebot.ChatAction) error { return c.notifyErr }

type attributionEnv struct {
	handler   *Handler
	generator *MockmessageGenerator
	replier   *MockbotReplier
	history   *chathistory.Buffer
	contexts  []*attributionContext
}

func newAttributionEnv(t *testing.T) *attributionEnv {
	t.Helper()
	ctrl := gomock.NewController(t)
	env := &attributionEnv{
		generator: NewMockmessageGenerator(ctrl),
		replier:   NewMockbotReplier(ctrl),
		history:   chathistory.NewBuffer(100),
	}
	stat := NewMockstatIncer(ctrl)
	stat.EXPECT().Inc(gomock.Any(), int64(7), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	var err error
	env.handler, err = New(
		context.Background(),
		env.generator,
		stat,
		stubSettings{s: &chatsettings.Settings{
			ThresholdCount: 1, ThresholdTime: time.Minute,
		}},
		env.history,
		env.replier,
		"@bot",
	)
	require.NoError(t, err)
	env.handler.r = rand.New(zeroSource{})
	for i, senderID := range []int64{101, 202} {
		env.contexts = append(env.contexts, &attributionContext{fakeCtx: &fakeCtx{
			chat:   &telebot.Chat{ID: 7},
			sender: &telebot.User{ID: senderID, FirstName: "Alex"},
			msg:    &telebot.Message{ID: i + 1, Unixtime: time.Now().Unix(), Text: "my text"},
			bot:    &telebot.Bot{Me: &telebot.User{ID: 999, Username: "bot"}},
		}})
	}
	return env
}

func (env *attributionEnv) expectGeneration(t *testing.T) {
	t.Helper()
	for _, ctx := range env.contexts {
		env.generator.EXPECT().
			GetMessageTextWithHistory(gomock.Any(), gomock.Any(), float32(0), false).
			DoAndReturn(func(_ context.Context, entries []chathistory.Entry, _ float32, _ bool) message.GenerationResult {
				assert.Equal(t, ctx.sender.ID, entries[len(entries)-1].SenderID)
				return message.GenerationResult{Message: "reply"}
			})
	}
}

func (env *attributionEnv) handleSenders(t *testing.T, wantErr error) {
	t.Helper()
	for _, ctx := range env.contexts {
		require.ErrorIs(t, env.handler.Handle(ctx), wantErr)
	}
}

func (env *attributionEnv) assertUserAuthorship(t *testing.T, wantEntries int) {
	t.Helper()
	entries := env.history.Get(7)
	require.Len(t, entries, wantEntries)
	var users []chathistory.Entry
	for _, entry := range entries {
		if entry.FromBot {
			assert.Zero(t, entry.SenderID)
			continue
		}
		users = append(users, entry)
	}
	require.Len(t, users, 2)
	assert.Equal(t, int64(101), users[0].SenderID)
	assert.Equal(t, int64(202), users[1].SenderID)
	assert.Equal(t, users[0].Author, users[1].Author)
}

// spec: BULL-014
func TestHandle_SameNameSendersOnSkip(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.handler.settingsProvider = stubSettings{
		s: &chatsettings.Settings{ThresholdCount: 100, ThresholdTime: time.Minute},
	}
	env.handleSenders(t, nil)
	env.assertUserAuthorship(t, 2)
}

// spec: BULL-014
func TestHandle_SameNameSendersOnSuccess(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.expectGeneration(t)
	for _, ctx := range env.contexts {
		env.replier.EXPECT().
			Reply(ctx.msg, "reply").
			Return(&telebot.Message{ID: 100 + ctx.msg.ID}, nil)
	}
	env.handleSenders(t, nil)
	env.assertUserAuthorship(t, 4)
}

// spec: BULL-014
func TestHandle_SameNameSendersOnReplyError(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.expectGeneration(t)
	sendErr := errors.New("send")
	env.replier.EXPECT().Reply(gomock.Any(), "reply").Return(nil, sendErr).Times(2)
	env.handleSenders(t, sendErr)
	env.assertUserAuthorship(t, 2)
}

// spec: BULL-014
func TestHandle_SameNameSendersOnNotifyError(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.expectGeneration(t)
	notifyErr := errors.New("notify")
	for _, ctx := range env.contexts {
		ctx.notifyErr = notifyErr
	}
	env.handleSenders(t, notifyErr)
	assert.Empty(t, env.history.Get(7))
}

// spec: HIST-022
func TestHandle_UnconfirmedTextSenderNeverGetsPersonalAuthorship(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		sender      *telebot.User
		senderChat  *telebot.Chat
		wantEntries int
	}{
		{name: "channel", sender: &telebot.User{ID: 101, FirstName: "Alex"}, senderChat: &telebot.Chat{ID: -10}, wantEntries: 1},
		{name: "bot", sender: &telebot.User{ID: 101, IsBot: true}, wantEntries: 1},
		{name: "missing", wantEntries: 0},
		{name: "zero_id", sender: &telebot.User{FirstName: "Alex"}, wantEntries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			history := chathistory.NewBuffer(10)
			h, err := New(
				context.Background(),
				NewMockmessageGenerator(ctrl),
				NewMockstatIncer(ctrl),
				stubSettings{
					s: &chatsettings.Settings{ThresholdCount: 100, ThresholdTime: time.Minute},
				},
				history,
				NewMockbotReplier(ctrl),
				"@bot",
			)
			require.NoError(t, err)
			ctx := &fakeCtx{
				chat:   &telebot.Chat{ID: 7},
				sender: tc.sender,
				msg: &telebot.Message{
					ID:         1,
					Text:       "Alex sent this",
					SenderChat: tc.senderChat,
					Unixtime:   time.Now().Unix(),
				},
				bot: &telebot.Bot{Me: &telebot.User{ID: 999, Username: "bot"}},
			}
			require.NoError(t, h.Handle(ctx))
			entries := history.Get(7)
			require.Len(t, entries, tc.wantEntries)
			for _, entry := range entries {
				assert.Zero(t, entry.SenderID)
			}
		})
	}
}
