package tagger

import (
	"context"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

const (
	chatID = int64(100)
	userID = int64(200)
	nick   = "Валера"
	tyChe  = "Ты чё"
)

func newTagHandler(t *testing.T) (*Handler, *MockmessageGenerator) {
	t.Helper()

	ctrl := gomock.NewController(t)
	gen := NewMockmessageGenerator(ctrl)

	h := newHandler(
		context.Background(),
		gen,
		nil,
		nil,
		newRegistry(t, newMemStorage()),
		nil,
		nil,
		zeroRandom{},
		nil,
		&fakeSettings{},
		tagFrom,
		tagTo,
	)

	return h, gen
}

// spec: TAG-010
func TestBuildTag_PassesChatHistoryAndTarget(t *testing.T) {
	t.Parallel()

	history := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "обсуждаем пиво",
		},
	}
	h, gen := newTagHandler(t)

	target := message.TagTarget{UserID: userID, Author: "пользователь #200", Nickname: nick}
	gen.EXPECT().GetMessageTextForTag(gomock.Any(), history, float32(0.7), target).
		Return(message.GenerationResult{Message: "пиво ты и то не осилишь", Strategy: message.AiGenerationStrategy})
	h.buildTag(chatID, userID, nick, 0.7, history)
}

// spec: TAG-010
func TestBuildTag_StripsDuplicatedLeadingNickname(t *testing.T) {
	t.Parallel()

	h, gen := newTagHandler(t)
	gen.EXPECT().
		GetMessageTextForTag(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(message.GenerationResult{Message: "валера, ты чё затих?", Strategy: message.AiGenerationStrategy})

	text := h.buildTag(chatID, userID, nick, 1, nil)

	assert.Equal(t, "[Валера](tg://user?id=200), Ты чё затих?", text)
}

func TestStripLeadingNickname(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ text, nick, want string }{
		"кличка с запятой":    {"Валера, ты чё", nick, tyChe},
		"кличка с тире":       {"Валера — ты чё", nick, tyChe},
		"другой регистр":      {"ВАЛЕРА! ты чё", nick, tyChe},
		"нет клички в начале": {"Ты чё, Валера", nick, "Ты чё, Валера"},
		"кличка внутри слова": {"Валерка, ты чё", nick, "Валерка, ты чё"},
		"только кличка":       {"Валера!", nick, "Валера!"},
		"пустая кличка":       {"Валера, привет", "", "Валера, привет"},
		"текст короче клички": {"Вал", nick, "Вал"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, stripLeadingNickname(c.text, c.nick))
		})
	}
}

// spec: TAG-004
func TestBuildTag_FormatsMentionWithNicknameAndText(t *testing.T) {
	t.Parallel()

	h, gen := newTagHandler(t)
	gen.EXPECT().
		GetMessageTextForTag(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(message.GenerationResult{Message: "чего молчишь", Strategy: message.ByListGenerationStrategy})

	text := h.buildTag(chatID, userID, nick, 1, nil)

	assert.Equal(t, "[Валера](tg://user?id=200), чего молчишь", text)
}

// spec: TAG-011
// spec: TAG-012
func TestBuildTag_IdentifiesSelectedUserInsteadOfLastAuthor(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		username   string
		firstName  string
		wantAuthor string
	}{
		"username":    {username: "target", firstName: "Алексей", wantAuthor: "@target"},
		"first name":  {firstName: "Алексей", wantAuthor: "Алексей"},
		"id fallback": {wantAuthor: "пользователь #200"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			history := []chathistory.Entry{{ID: 1, Author: "@other", Text: "я тебя выключу"}}
			h, gen := newTagHandler(t)
			ctx := context.Background()
			require.NoError(t, h.addUser(ctx, chatID, &telebot.User{ID: userID, Username: "old"}))
			require.NoError(t, h.addUser(ctx, chatID, &telebot.User{ID: 300, Username: "other"}))
			require.NoError(t, h.addUser(
				ctx,
				chatID,
				&telebot.User{ID: userID, Username: tc.username, FirstName: tc.firstName},
			))

			var target message.TagTarget
			gen.EXPECT().GetMessageTextForTag(
				gomock.Any(), history, float32(1), gomock.Any(),
			).DoAndReturn(func(_ context.Context, _ []chathistory.Entry, _ float32, value message.TagTarget) message.GenerationResult {
				target = value
				return message.GenerationResult{
					Message:  "Чего притих?",
					Strategy: message.AiGenerationStrategy,
				}
			})

			text := h.buildTag(chatID, userID, nick, 1, history)

			assert.Equal(
				t,
				message.TagTarget{UserID: userID, Author: tc.wantAuthor, Nickname: nick},
				target,
			)
			assert.Equal(t, "[Валера](tg://user?id=200), Чего притих?", text)
			assert.Equal(t, []int64{userID, 300}, h.users.Users(chatID))
		})
	}
}
