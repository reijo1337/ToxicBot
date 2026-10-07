package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// spec: GEN-036
func TestGenerator_TagDoesNotMarkAnotherUsersMessageAsTrigger(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	aiMock := NewMockai(ctrl)
	rnd := NewMockrandomizer(ctrl)
	rnd.EXPECT().Float32().Return(float32(0))
	filter := NewMockmeaningfullFilter(ctrl)
	var captured []LLMMessage
	aiMock.EXPECT().Chat(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, msgs ...LLMMessage) (string, error) {
			captured = msgs
			return "Сидишь тихо, будто слова кончились.", nil
		})
	g := &Generator{ai: aiMock, r: rnd, meaningfullFilter: filter, systemPrompt: "SYS"}
	history := []chathistory.Entry{
		{ID: 1, Time: time.Unix(1, 0), Author: "@target", Text: "обсуждаем пиво"},
		{ID: 2, Time: time.Unix(2, 0), Author: "@other", Text: "я тебя выключу"},
	}
	result := g.GetMessageTextForTag(context.Background(), history, 1, "пинг @target")

	require.Equal(t, AiGenerationStrategy, result.Strategy)
	require.Len(t, captured, 3)
	assert.Contains(t, captured[0].Content, "пинг @target")
	assert.Equal(t, "@target", captured[1].Name)
	assert.Equal(t, "@other", captured[2].Name)
	assert.Contains(t, captured[2].Content, "я тебя выключу")
	for _, msg := range captured[1:] {
		assert.NotContains(t, msg.Content, `now="true"`)
	}
}

// spec: GEN-037
func TestGenerator_TagIgnoresMeaningfulnessOfLastHistoryEntry(t *testing.T) {
	t.Parallel()

	cases := map[string]chathistory.Entry{
		"short reply": {ID: 2, Author: "@other", Text: "ок"},
		"empty reply": {ID: 2, Author: "@other"},
		"bot reply":   {ID: 2, Author: "@bot", Text: "Ответ бота.", FromBot: true},
	}
	for name, last := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			aiMock := NewMockai(ctrl)
			aiMock.EXPECT().Chat(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return("Чего притих?", nil)
			rnd := NewMockrandomizer(ctrl)
			rnd.EXPECT().Float32().Return(float32(0))
			g := &Generator{
				ai:                aiMock,
				r:                 rnd,
				meaningfullFilter: NewMockmeaningfullFilter(ctrl),
				systemPrompt:      "SYS",
			}
			history := []chathistory.Entry{{ID: 1, Author: "@target", Text: "обсуждаем пиво"}, last}

			result := g.GetMessageTextForTag(context.Background(), history, 1, "пинг @target")

			assert.Equal(t, AiGenerationStrategy, result.Strategy)
			assert.Equal(t, "Чего притих?", result.Message)
		})
	}
}

// spec: GEN-001
// spec: GEN-003
// spec: GEN-004
// spec: GEN-037
func TestGenerator_TagFallsBackToList(t *testing.T) {
	t.Parallel()

	t.Run("empty history", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		rnd := NewMockrandomizer(ctrl)
		rnd.EXPECT().Intn(1).Return(0)
		g := &Generator{r: rnd, ai: NewMockai(ctrl), messages: []string{"Готовая фраза."}}

		result := g.GetMessageTextForTag(context.Background(), nil, 1, "пинг @target")

		assert.Equal(t, ByListGenerationStrategy, result.Strategy)
		assert.Equal(t, "Готовая фраза.", result.Message)
	})
	t.Run("probability rejects AI", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		rnd := NewMockrandomizer(ctrl)
		rnd.EXPECT().Float32().Return(float32(0.75))
		rnd.EXPECT().Intn(1).Return(0)
		g := &Generator{r: rnd, ai: NewMockai(ctrl), messages: []string{"Готовая фраза."}}

		result := g.GetMessageTextForTag(
			context.Background(),
			[]chathistory.Entry{{Author: "@other", Text: "ок"}},
			0.5,
			"пинг @target",
		)

		assert.Equal(t, ByListGenerationStrategy, result.Strategy)
		assert.Equal(t, "Готовая фраза.", result.Message)
	})
	t.Run("AI error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		rnd := NewMockrandomizer(ctrl)
		rnd.EXPECT().Float32().Return(float32(0))
		rnd.EXPECT().Intn(1).Return(0)
		aiMock := NewMockai(ctrl)
		aiErr := errors.New("LLM unavailable")
		aiMock.EXPECT().Chat(gomock.Any(), gomock.Any(), gomock.Any()).Return("", aiErr)
		log := NewMocklogger(ctrl)
		log.EXPECT().WithError(gomock.Any(), aiErr).Return(context.Background())
		log.EXPECT().Warn(gomock.Any(), gomock.Any())
		g := &Generator{r: rnd, ai: aiMock, logger: log, messages: []string{"Готовая фраза."}}

		result := g.GetMessageTextForTag(
			context.Background(),
			[]chathistory.Entry{{Author: "@other", Text: "ок"}},
			1,
			"пинг @target",
		)

		assert.Equal(t, ByListGenerationStrategy, result.Strategy)
		assert.Equal(t, "Готовая фраза.", result.Message)
	})
}
