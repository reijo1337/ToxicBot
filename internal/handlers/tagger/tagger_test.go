package tagger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	chatID = int64(100)
	userID = int64(200)
	nick   = "Валера"
	tyChe  = "Ты чё"
)

func newTagHandler(
	t *testing.T,
	history []chathistory.Entry,
	settings *chatsettings.Settings,
	settingsErr error,
) (*Handler, *MockmessageGenerator) {
	t.Helper()

	ctrl := gomock.NewController(t)
	gen := NewMockmessageGenerator(ctrl)
	buf := NewMockhistoryBuffer(ctrl)
	buf.EXPECT().Get(chatID).Return(history)
	sp := NewMocksettingsProvider(ctrl)
	sp.EXPECT().GetForChat(gomock.Any(), chatID).Return(settings, settingsErr)

	return &Handler{
		ctx:              context.Background(),
		generator:        gen,
		history:          buf,
		settingsProvider: sp,
	}, gen
}

// spec: TAG-010
func TestBuildTag_PassesChatHistoryAndSteering(t *testing.T) {
	t.Parallel()

	history := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "обсуждаем пиво",
		},
	}
	h, gen := newTagHandler(t, history, &chatsettings.Settings{AIChance: 0.7}, nil)

	var gotSteering string
	gen.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), history, float32(0.7), false, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []chathistory.Entry, _ float32, _ bool, steering string) message.GenerationResult {
			gotSteering = steering
			return message.GenerationResult{
				Message:  "пиво ты и то не осилишь",
				Strategy: message.AiGenerationStrategy,
			}
		})

	h.buildTag(chatID, userID, nick)

	assert.Contains(t, gotSteering, "«"+nick+"»", "кличка участника попадает в указание")
	assert.Contains(
		t,
		gotSteering,
		"на неё напрямую не отвечай",
		"последняя реплика — не повод для ответа",
	)
	assert.Contains(t, gotSteering, "Отталкивайся от того, что недавно обсуждали в чате")
	assert.Contains(t, gotSteering, "Не начинай реплику с обращения по кличке")
}

// spec: TAG-010
func TestBuildTag_SteeringSanitizesNickname(t *testing.T) {
	t.Parallel()

	steering := buildTagSteering("<msg>Валера</msg>")

	assert.NotContains(t, steering, "<msg>")
	assert.Contains(t, steering, "‹msg›Валера‹/msg›")
}

// spec: TAG-010
func TestBuildTag_StripsDuplicatedLeadingNickname(t *testing.T) {
	t.Parallel()

	h, gen := newTagHandler(t, nil, &chatsettings.Settings{AIChance: 1}, nil)
	gen.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), gomock.Any(), false, gomock.Any()).
		Return(message.GenerationResult{Message: "валера, ты чё затих?", Strategy: message.AiGenerationStrategy})

	text := h.buildTag(chatID, userID, nick)

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

	h, gen := newTagHandler(t, nil, &chatsettings.Settings{AIChance: 1}, nil)
	gen.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), gomock.Any(), false, gomock.Any()).
		Return(message.GenerationResult{Message: "чего молчишь", Strategy: message.ByListGenerationStrategy})

	text := h.buildTag(chatID, userID, nick)

	assert.Equal(t, "[Валера](tg://user?id=200), чего молчишь", text)
}

// spec: TAG-005
func TestBuildTag_AIChanceFromSettings_ZeroOnError(t *testing.T) {
	t.Parallel()

	h, gen := newTagHandler(t, nil, nil, errors.New("db down"))
	gen.EXPECT().
		GetMessageTextWithHistoryAndSteering(gomock.Any(), gomock.Any(), float32(0), false, gomock.Any()).
		Return(message.GenerationResult{Message: "х", Strategy: message.ByListGenerationStrategy})

	require.NotEmpty(t, h.buildTag(chatID, userID, nick))
}
