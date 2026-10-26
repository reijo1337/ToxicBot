package on_photo

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	pkglogger "github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

type zeroSource struct{}

func (zeroSource) Int63() int64 { return 0 }
func (zeroSource) Seed(int64)   {}

// spec: PHOTO-023
// spec: HIST-022
func TestHandle_PhotoSenderIsIndependentOfForwardOrigin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		sender         *telebot.User
		senderChat     *telebot.Chat
		originalSender *telebot.User
		originalChat   *telebot.Chat
		wantSenderID   int64
		wantOrigin     string
	}{
		{
			name: "user_forward", sender: &telebot.User{ID: 101, FirstName: "Alex"},
			originalSender: &telebot.User{ID: 303, Username: "original"}, wantSenderID: 101, wantOrigin: "переслано от @original",
		},
		{
			name: "channel_forward", sender: &telebot.User{ID: 202, FirstName: "Alex"},
			originalChat: &telebot.Chat{ID: -303, Username: "source"}, wantSenderID: 202, wantOrigin: "переслано из канала @source",
		},
		{
			name: "sender_chat", sender: &telebot.User{ID: 101, FirstName: "Alex"},
			senderChat: &telebot.Chat{ID: -10}, wantOrigin: "скинул картинку в чат",
		},
		{name: "bot", sender: &telebot.User{ID: 101, IsBot: true}, wantOrigin: "скинул картинку в чат"},
		{name: "zero_id", sender: &telebot.User{FirstName: "Alex"}, wantOrigin: "скинул картинку в чат"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			env.handler.r = rand.New(zeroSource{})
			env.handler.logger = pkglogger.New()
			history := chathistory.NewBuffer(10)
			env.handler.history = history
			env.setupPhotoPipeline("На изображении телефон другого человека")
			msg := photoMessage(50, "чужой телефон", replyToBotMessage())
			msg.SenderChat = tc.senderChat
			msg.OriginalSender = tc.originalSender
			msg.OriginalChat = tc.originalChat
			env.generator.EXPECT().
				GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), float32(1), true, gomock.Any()).
				DoAndReturn(func(_ context.Context, entries []chathistory.Entry, _ float32, _ bool, _ string) message.GenerationResult {
					require.Len(t, entries, 1)
					assert.Equal(t, tc.wantSenderID, entries[0].SenderID)
					assert.Contains(t, entries[0].Text, tc.wantOrigin)
					return message.GenerationResult{Message: "reply"}
				})
			env.replier.EXPECT().Reply(msg, "reply").Return(&telebot.Message{ID: 51}, nil)
			require.NoError(t, env.handler.Handle(newCtx(msg, tc.sender)))
			entries := history.Get(testChatID)
			require.Len(t, entries, 2)
			assert.Equal(t, tc.wantSenderID, entries[0].SenderID)
			assert.Contains(t, entries[0].Text, tc.wantOrigin)
			assert.True(t, entries[0].PreFormatted)
			assert.Equal(t, 999, entries[0].ReplyToID)
			assert.True(t, entries[1].FromBot)
			assert.Zero(t, entries[1].SenderID)
		})
	}
}

// spec: PHOTO-023
// spec: HIST-022
func TestHandle_FailedPhotoReplyDoesNotCreateAuthorship(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.handler.r = rand.New(zeroSource{})
	env.handler.logger = pkglogger.New()
	history := chathistory.NewBuffer(10)
	env.handler.history = history
	env.setupPhotoPipeline("описание")
	msg := photoMessage(50, "", replyToBotMessage())
	env.generator.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), float32(1), true, gomock.Any()).
		Return(message.GenerationResult{Message: "reply"})
	env.replier.EXPECT().Reply(msg, "reply").Return(nil, errors.New("send"))
	require.Error(t, env.handler.Handle(newCtx(msg, goodSender())))
	assert.Empty(t, history.Get(testChatID))
}

// spec: HIST-022
func TestHandle_MissingPhotoSenderDoesNotCreateAuthorship(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	history := chathistory.NewBuffer(10)
	env.handler.history = history
	require.NoError(t, env.handler.Handle(newCtx(photoMessage(50, "", nil), nil)))
	assert.Empty(t, history.Get(testChatID))
}
