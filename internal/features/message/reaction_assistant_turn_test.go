package message

import (
	"context"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reactionPromptRecorder struct {
	messages []LLMMessage
}

func (r *reactionPromptRecorder) Chat(_ context.Context, messages ...LLMMessage) (string, error) {
	r.messages = append([]LLMMessage(nil), messages...)

	return "Это проверочный ответ.", nil
}

// spec: GEN-050
func TestGenerator_ReactionsIdentifyDistinctBareAssistantTurns(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	history := []chathistory.Entry{
		{
			ID:     40,
			Time:   at,
			Author: "@user",
			Text:   "earlier user message",
		},
		{
			ID:      42,
			Time:    at.Add(time.Second),
			Author:  "@bot",
			Text:    "BOT_TEXT_A",
			FromBot: true,
		},
		{
			ID:      44,
			Time:    at.Add(2 * time.Second),
			Author:  "@bot",
			Text:    "BOT_TEXT_B",
			FromBot: true,
		},
		{
			ID:     45,
			Time:   at.Add(3 * time.Second),
			Author: "@user",
			Text:   "current user message",
		},
		reactionEntry(900, 42, at.Add(4*time.Second)),
		reactionEntry(901, 44, at.Add(5*time.Second)),
	}

	recorder := &reactionPromptRecorder{}
	generator := &Generator{
		ai:           recorder,
		systemPrompt: DefaultSystemPromptBase(),
	}

	result := generator.GetMessageTextWithHistory(context.Background(), history, 1, true)

	require.Equal(t, AiGenerationStrategy, result.Strategy)

	first := append([]LLMMessage(nil), recorder.messages...)

	require.Len(t, first, 7)
	assert.Contains(t, first[5].Content, `message_id="42"`)
	assert.Contains(t, first[5].Content, `target_assistant_turn="1"`)
	assert.Contains(t, first[6].Content, `message_id="44"`)
	assert.Contains(t, first[6].Content, `target_assistant_turn="2"`)
	assert.Equal(t, LLMMessage{
		Role:    RoleAssistant,
		Name:    "@bot",
		Content: "BOT_TEXT_A",
	}, first[2])
	assert.Equal(t, LLMMessage{
		Role:    RoleAssistant,
		Name:    "@bot",
		Content: "BOT_TEXT_B",
	}, first[3])
	assert.NotContains(t, first[5].Content, "BOT_TEXT")
	assert.NotContains(t, first[6].Content, "BOT_TEXT")
	assert.Contains(t, first[0].Content, "target_assistant_turn")
	assert.Contains(t, first[0].Content, "начиная с 1")

	history[1].ID, history[2].ID = history[2].ID, history[1].ID
	result = generator.GetMessageTextWithHistory(context.Background(), history, 1, true)

	require.Equal(t, AiGenerationStrategy, result.Strategy)
	assert.NotEqual(
		t,
		first,
		recorder.messages,
		"swapping bot target IDs must change the generated prompt",
	)
	assert.Contains(t, recorder.messages[5].Content, `target_assistant_turn="2"`)
	assert.Contains(t, recorder.messages[6].Content, `target_assistant_turn="1"`)
}

// spec: GEN-050
// spec: GEN-051
func TestBuildChatCompletions_ReactionTurnCountsOnlyEmittedAssistants(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	history := []chathistory.Entry{
		reactionEntry(900, 39, at),
		{
			ID:      39,
			Time:    at.Add(time.Second),
			Author:  "@bot",
			Text:    "SKIPPED",
			FromBot: true,
		},
		{
			ID:     40,
			Time:   at.Add(2 * time.Second),
			Author: "@user",
			Text:   "user message",
		},
		{
			ID:      42,
			Time:    at.Add(3 * time.Second),
			Author:  "@bot",
			Text:    "VISIBLE",
			FromBot: true,
		},
		reactionEntry(901, 42, at.Add(4*time.Second)),
		reactionEntry(902, 999, at.Add(5*time.Second)),
	}

	msgs := buildChatCompletions("SYS", history, replyGeneration)

	require.Len(t, msgs, 6)
	assert.NotContains(
		t,
		msgs[1].Content,
		"target_assistant_turn",
		"skipped bot text is not an assistant turn",
	)
	assert.Equal(t, "VISIBLE", msgs[3].Content)
	assert.Contains(t, msgs[4].Content, `target_assistant_turn="1"`)
	assert.NotContains(
		t,
		msgs[5].Content,
		"target_assistant_turn",
		"absent bot text is not an assistant turn",
	)
}

// spec: GEN-050
// spec: GEN-054
func TestBuildTagCompletions_ReactionNeverInventsAssistantTurn(t *testing.T) {
	t.Parallel()

	history := []chathistory.Entry{
		{
			ID:       40,
			SenderID: 200,
			Author:   "@target",
			Text:     "own message",
		},
		{
			ID:      42,
			Author:  "@bot",
			Text:    "BOT_SECRET",
			FromBot: true,
		},
		reactionEntry(900, 42, time.Time{}),
	}

	msgs := buildTagCompletions("SYS", history, testTagTarget)

	assert.Contains(t, msgs[1].Content, `target_from_bot="true"`)
	assert.NotContains(t, msgs[1].Content, "target_assistant_turn")
	assert.NotContains(t, msgs[1].Content, "BOT_SECRET")
}

// spec: GEN-050
func TestBuildChatCompletions_FilteredPhotoCannotInventAssistantTurn(t *testing.T) {
	t.Parallel()

	event := reactionEntry(900, 42, time.Time{})
	event.Reaction.TargetKnown = true
	event.Reaction.TargetFromBot = true
	event.Reaction.TargetAuthor = "@bot"

	const photo = "<photo><caption>caption</caption><vision_description>description</vision_description></photo>"
	history := []chathistory.Entry{
		{
			ID:           43,
			Author:       "@user",
			Text:         photo,
			PreFormatted: true,
		},
		event,
	}

	msgs := buildChatCompletions("SYS", history, replyGeneration)

	require.Len(t, msgs, 3)
	assert.Contains(t, msgs[1].Content, photo)
	assert.Contains(t, msgs[2].Content, `target_from_bot="true"`)
	assert.NotContains(t, msgs[2].Content, "target_assistant_turn")
}
