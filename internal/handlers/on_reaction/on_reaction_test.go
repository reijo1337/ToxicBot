package on_reaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gopkg.in/telebot.v3"
)

type recordedHistory struct {
	chatID  int64
	entries []chathistory.Entry
	err     error
}

func (h *recordedHistory) AddReaction(
	_ context.Context,
	chatID int64,
	entry chathistory.Entry,
) error {
	if h.err != nil {
		return h.err
	}

	h.chatID = chatID
	h.entries = append(h.entries, entry)

	return nil
}

func personalReaction() *telebot.MessageReaction {
	return &telebot.MessageReaction{
		Chat: &telebot.Chat{
			ID: -100,
		},
		MessageID: 42,
		User: &telebot.User{
			ID:       7,
			Username: "alice",
		},
		DateUnixtime: 1700000000,
	}
}

// spec: REACT-001
// spec: REACT-002
// spec: REACT-003
func TestHandle_RecordsSetChanges(t *testing.T) {
	t.Parallel()

	up := telebot.Reaction{
		Type:  "emoji",
		Emoji: "👍",
	}

	down := telebot.Reaction{
		Type:  "emoji",
		Emoji: "👎",
	}

	unchanged := telebot.Reaction{
		Type:  "emoji",
		Emoji: "🔥",
	}

	tests := map[string]struct {
		old, next      []telebot.Reaction
		added, removed []chathistory.ReactionType
	}{
		"add": {
			next: []telebot.Reaction{up},
			added: []chathistory.ReactionType{
				{
					Type:  "emoji",
					Value: "👍",
				},
			},
		},
		"remove without prior event": {
			old: []telebot.Reaction{up},
			removed: []chathistory.ReactionType{
				{
					Type:  "emoji",
					Value: "👍",
				},
			},
		},
		"replace with unchanged": {
			old:  []telebot.Reaction{up, unchanged},
			next: []telebot.Reaction{down, unchanged},
			added: []chathistory.ReactionType{
				{
					Type:  "emoji",
					Value: "👎",
				},
			},
			removed: []chathistory.ReactionType{
				{
					Type:  "emoji",
					Value: "👍",
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			history := &recordedHistory{}
			event := personalReaction()
			event.OldReaction, event.NewReaction = tt.old, tt.next

			require.NoError(t, New(history).Handle(context.Background(), 123, event))
			require.Len(t, history.entries, 1)
			assert.Equal(t, int64(-100), history.chatID)
			assert.Equal(t, chathistory.Entry{
				Time: time.Unix(1700000000, 0),
				Reaction: &chathistory.Reaction{
					UpdateID:  123,
					MessageID: 42,
					ActorID:   7,
					Actor:     "@alice",
					Added:     tt.added,
					Removed:   tt.removed,
				},
			}, history.entries[0])
		})
	}
}

// spec: REACT-004
func TestHandle_SetOrderAndDuplicatesDoNotRecord(t *testing.T) {
	t.Parallel()

	event := personalReaction()
	up := telebot.Reaction{
		Type:  "emoji",
		Emoji: "👍",
	}

	custom := telebot.Reaction{
		Type:        "custom_emoji",
		CustomEmoji: "123",
	}

	event.OldReaction = []telebot.Reaction{up, custom, up}
	event.NewReaction = []telebot.Reaction{custom, up, custom}
	history := &recordedHistory{}

	require.NoError(t, New(history).Handle(context.Background(), 1, event))
	assert.Empty(t, history.entries)
}

// spec: REACT-005
// spec: REACT-006
func TestHandle_SkipsUntrustedEvents(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*telebot.MessageReaction){
		"anonymous": func(e *telebot.MessageReaction) {
			e.User = nil
		},
		"actor chat": func(e *telebot.MessageReaction) {
			e.ActorChat = &telebot.Chat{
				ID: -200,
			}
		},
		"missing chat": func(e *telebot.MessageReaction) {
			e.Chat = nil
		},
		"zero chat": func(e *telebot.MessageReaction) {
			e.Chat.ID = 0
		},
		"zero actor": func(e *telebot.MessageReaction) {
			e.User.ID = 0
		},
		"negative actor": func(e *telebot.MessageReaction) {
			e.User.ID = -1
		},
		"bot": func(e *telebot.MessageReaction) {
			e.User.IsBot = true
		},
		"zero target": func(e *telebot.MessageReaction) {
			e.MessageID = 0
		},
		"negative target": func(e *telebot.MessageReaction) {
			e.MessageID = -1
		},
	}

	for name, invalidate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			event := personalReaction()
			event.NewReaction = []telebot.Reaction{
				{
					Type:  "emoji",
					Emoji: "👍",
				},
			}

			invalidate(event)
			history := &recordedHistory{}

			require.NoError(t, New(history).Handle(context.Background(), 1, event))
			assert.Empty(t, history.entries)
		})
	}
}

// spec: REACT-006
func TestHandle_TypedKnownReactionsSurviveUnknown(t *testing.T) {
	t.Parallel()

	event := personalReaction()
	event.NewReaction = []telebot.Reaction{
		{
			Type:  "future",
			Emoji: "fake",
		},
		{
			Type:  "emoji",
			Emoji: "👍",
		},
		{
			Type:        "custom_emoji",
			CustomEmoji: "123",
		},
		{
			Type: "paid",
		},
		{
			Type: "emoji",
		},
		{
			Type: "custom_emoji",
		},
		{
			Type:  "paid",
			Emoji: "ignored",
		},
	}

	history := &recordedHistory{}

	require.NoError(t, New(history).Handle(context.Background(), 1, event))
	require.Len(t, history.entries, 1)
	assert.Equal(
		t,
		[]chathistory.ReactionType{
			{
				Type:  "emoji",
				Value: "👍",
			},
			{
				Type:  "custom_emoji",
				Value: "123",
			},
			{
				Type: "paid",
			},
		},
		history.entries[0].Reaction.Added,
	)
}

// spec: REACT-009
func TestHandle_OnlyHistoryIsChanged(t *testing.T) {
	t.Parallel()

	history := chathistory.NewBuffer(100)
	history.Add(-100, chathistory.Entry{
		ID:       42,
		SenderID: 9,
		Author:   "@target",
		Text:     "original",
	})
	event := personalReaction()
	event.NewReaction = []telebot.Reaction{
		{
			Type:  "emoji",
			Emoji: "👍",
		},
	}

	require.NoError(t, New(history).Handle(context.Background(), 123, event))

	entries := history.Get(-100)

	require.Len(t, entries, 2)
	assert.Equal(
		t,
		chathistory.Entry{
			ID:       42,
			SenderID: 9,
			Author:   "@target",
			Text:     "original",
		},
		entries[0],
	)
	assert.Zero(t, entries[1].ID)
	assert.Zero(t, entries[1].SenderID)
	assert.Empty(t, entries[1].Text)
	assert.Empty(t, entries[1].Author)
	assert.False(t, entries[1].FromBot)
}

//nolint:paralleltest // verifies the process-global OpenTelemetry provider
func TestHandle_SpansOnlyExposeSafeMetadata(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)

	t.Cleanup(
		func() {
			otel.SetTracerProvider(previous)

			require.NoError(t, provider.Shutdown(context.Background()))
		},
	)

	event := personalReaction()
	event.User.Username = "secret-name"
	event.NewReaction = []telebot.Reaction{
		{
			Type:  "emoji",
			Emoji: "secret-reaction",
		},
	}

	history := &recordedHistory{
		err: errors.New("secret-store-error"),
	}

	err := New(history).Handle(context.Background(), 123, event)

	require.ErrorIs(t, err, history.err)

	spans := recorder.Ended()

	require.Len(t, spans, 2)
	assert.Equal(t, "on_reaction", spans[0].Name())
	assert.Equal(t, "telegram.message_reaction", spans[1].Name())
	assert.Equal(t, spans[1].SpanContext().SpanID(), spans[0].Parent().SpanID())
	assert.False(t, spans[1].Parent().IsValid())

	want := []attribute.KeyValue{
		attribute.Int("telegram.update_id", 123), attribute.Int64("chat.id", -100),
		attribute.Int64("user.id", 7), attribute.Int("message.id", 42),
		attribute.Int("reaction.added_count", 1), attribute.Int("reaction.removed_count", 0),
		attribute.String("outcome", "history_error"),
	}

	for _, span := range spans {
		assert.ElementsMatch(t, want, span.Attributes())
		assert.Empty(t, span.Events())
		assert.Equal(t, "history unavailable", span.Status().Description)
	}
}
