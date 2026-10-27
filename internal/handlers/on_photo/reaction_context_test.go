package on_photo

import (
	"context"
	"math/rand"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	pkglogger "github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

// spec: GEN-050
// spec: PHOTO-028
func TestDropBotEntries_ReactionRetainsLateBotTargetWithoutText(t *testing.T) {
	t.Parallel()

	event := chathistory.Entry{
		Reaction: &chathistory.Reaction{
			UpdateID:  900,
			MessageID: 42,
			ActorID:   200,
		},
	}

	history := []chathistory.Entry{
		event,
		{
			ID:       42,
			SenderID: 300,
			Author:   "@bot",
			FromBot:  true,
			Text:     "SECRET",
		},
	}

	got := dropBotEntries(history)

	require.Len(t, got, 1)
	require.NotNil(t, got[0].Reaction)
	assert.True(t, got[0].Reaction.TargetKnown)
	assert.Equal(t, "@bot", got[0].Reaction.TargetAuthor)
	assert.Equal(t, int64(300), got[0].Reaction.TargetSenderID)
	assert.True(t, got[0].Reaction.TargetFromBot)
	assert.Empty(t, got[0].Text)
	assert.False(t, event.Reaction.TargetKnown, "filter must not mutate the snapshot")
}

// spec: GEN-050
// spec: PHOTO-028
func TestHandle_PhotoCarriesReactionSnapshot(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	env.handler.r = rand.New(zeroSource{})
	env.handler.logger = pkglogger.New()
	env.setupPhotoPipeline("На изображении кот")
	msg := photoMessage(50, "смотри", replyToBotMessage())
	event := chathistory.Entry{
		Reaction: &chathistory.Reaction{
			UpdateID:  900,
			MessageID: 42,
			ActorID:   200,
		},
	}

	env.history.EXPECT().Get(testChatID).Return([]chathistory.Entry{event})
	env.generator.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), float32(1), true, gomock.Any()).
		DoAndReturn(func(_ context.Context, history []chathistory.Entry, _ float32, _ bool, _ string) message.GenerationResult {
			require.Len(t, history, 2)
			assert.Equal(t, event.Reaction, history[0].Reaction)
			assert.Equal(t, msg.ID, history[1].ID)
			assert.Contains(t, history[1].Text, "На изображении кот")

			return message.GenerationResult{
				Message: "reply",
			}
		})
	env.replier.EXPECT().Reply(msg, "reply").Return(&telebot.Message{
		ID: 51,
	}, nil)
	env.history.EXPECT().AddAll(testChatID, gomock.Any(), gomock.Any())

	require.NoError(t, env.handler.Handle(newCtx(msg, goodSender())))
}

// spec: GEN-052
// spec: PHOTO-028
func TestHandle_ReactionHistoryDoesNotEnableSkippedPhoto(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	env.handler.logger = pkglogger.New()
	history := chathistory.NewBuffer(10)
	event := chathistory.Entry{
		Reaction: &chathistory.Reaction{
			UpdateID:  900,
			MessageID: 42,
			ActorID:   200,
		},
	}

	history.Add(testChatID, event)
	env.handler.history = history
	env.settings.EXPECT().GetForChat(gomock.Any(), testChatID).
		Return(&chatsettings.Settings{
			PhotoReactChance: 0,
		}, nil)

	require.NoError(t, env.handler.Handle(newCtx(photoMessage(50, "caption", nil), goodSender())))
	assert.Equal(t, []chathistory.Entry{event}, history.Get(testChatID))
}
