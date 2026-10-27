package on_reaction

import (
	"context"
	"fmt"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/telebot.v3"
)

type historyWriter interface {
	AddReaction(ctx context.Context, chatID int64, entry chathistory.Entry) error
}

type Handler struct {
	history historyWriter
}

func New(history historyWriter) *Handler {
	return &Handler{
		history: history,
	}
}

func (h *Handler) Handle(ctx context.Context, updateID int, event *telebot.MessageReaction) error {
	ctx, root := tracing.Tracer().Start(ctx, "telegram.message_reaction", trace.WithNewRoot())
	defer root.End()
	ctx, span := tracing.Tracer().Start(ctx, "on_reaction")
	defer span.End()

	outcome := "skip"
	attrs := []attribute.KeyValue{attribute.Int("telegram.update_id", updateID)}
	defer func() {
		attrs = append(attrs, attribute.String("outcome", outcome))
		root.SetAttributes(attrs...)
		span.SetAttributes(attrs...)
	}()

	if event == nil {
		return nil
	}

	attrs = append(attrs, attribute.Int("message.id", event.MessageID))
	if event.Chat != nil {
		attrs = append(attrs, attribute.Int64("chat.id", event.Chat.ID))
	}

	if event.User != nil {
		attrs = append(attrs, attribute.Int64("user.id", event.User.ID))
	}

	if event.ActorChat != nil || event.User == nil || event.User.ID <= 0 ||
		event.User.IsBot || event.Chat == nil || event.Chat.ID == 0 || event.MessageID <= 0 {
		return nil
	}

	old, next := knownReactions(event.OldReaction), knownReactions(event.NewReaction)
	added, removed := difference(next, old), difference(old, next)
	attrs = append(
		attrs,
		attribute.Int("reaction.added_count", len(added)),
		attribute.Int("reaction.removed_count", len(removed)),
	)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}

	err := h.history.AddReaction(ctx, event.Chat.ID, chathistory.Entry{
		Time: time.Unix(event.DateUnixtime, 0),
		Reaction: &chathistory.Reaction{
			UpdateID:  updateID,
			MessageID: event.MessageID,
			ActorID:   event.User.ID,
			Actor: message.SanitizeAuthor(
				event.User.Username, event.User.FirstName, event.User.ID, false,
			),
			Added:   added,
			Removed: removed,
		},
	})
	if err != nil {
		outcome = "history_error"
		root.SetStatus(codes.Error, "history unavailable")
		span.SetStatus(codes.Error, "history unavailable")

		return fmt.Errorf("add reaction: %w", err)
	}

	outcome = "recorded"

	return nil
}

func knownReactions(values []telebot.Reaction) []chathistory.ReactionType {
	var result []chathistory.ReactionType
	seen := make(map[chathistory.ReactionType]struct{}, len(values))

	for _, value := range values {
		reaction := chathistory.ReactionType{
			Type: value.Type,
		}

		switch value.Type {
		case "emoji":
			if value.Emoji == "" {
				continue
			}
			reaction.Value = value.Emoji
		case "custom_emoji":
			if value.CustomEmoji == "" {
				continue
			}
			reaction.Value = value.CustomEmoji
		case "paid":
		default:
			continue
		}

		if _, exists := seen[reaction]; exists {
			continue
		}

		seen[reaction] = struct{}{}
		result = append(result, reaction)
	}

	return result
}

func difference(values, subtract []chathistory.ReactionType) []chathistory.ReactionType {
	other := make(map[chathistory.ReactionType]struct{}, len(subtract))

	for _, value := range subtract {
		other[value] = struct{}{}
	}

	var result []chathistory.ReactionType

	for _, value := range values {
		if _, exists := other[value]; !exists {
			result = append(result, value)
		}
	}

	return result
}
