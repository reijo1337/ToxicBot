package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/telebot.v3"
)

// rootCtxKey is the telebot.Context store key under which the dispatcher
// stashes the root span context so parallel sub-handlers can parent their own
// spans off it.
const rootCtxKey = "tracing.rootctx"

// StashRootContext stores goCtx (carrying the root span) on the telebot
// context. Called once by the dispatcher before fan-out.
func StashRootContext(tc telebot.Context, goCtx context.Context) {
	tc.Set(rootCtxKey, goCtx)
}

// rootContext returns the stashed root context, or context.Background() when
// the dispatcher started no root span (tracing disabled or a directly
// registered handler).
func rootContext(tc telebot.Context) context.Context {
	if v, ok := tc.Get(rootCtxKey).(context.Context); ok && v != nil {
		return v
	}
	return context.Background()
}

// StartHandlerSpan opens a child span named after the handler slug, parented
// off the dispatcher's root span. Returns the span context (to thread into the
// generator / LLM clients) and the span (End it via defer).
func StartHandlerSpan(tc telebot.Context, slug string) (context.Context, trace.Span) {
	//nolint:spancheck // span is returned to and ended by the caller
	return Tracer().Start(rootContext(tc), slug)
}

// EndpointName maps a telebot endpoint to a readable span/operation name.
func EndpointName(endpoint string) string {
	switch endpoint {
	case telebot.OnText:
		return "on_text"
	case telebot.OnSticker:
		return "on_sticker"
	case telebot.OnVoice:
		return "on_voice"
	case telebot.OnPhoto:
		return "on_photo"
	case telebot.OnUserJoined:
		return "on_user_joined"
	case telebot.OnUserLeft:
		return "on_user_left"
	default:
		return endpoint
	}
}

// UpdateAttrs extracts root-span attributes from the incoming update.
func UpdateAttrs(tc telebot.Context) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.Int("telegram.update_id", tc.Update().ID)}
	if c := tc.Chat(); c != nil {
		attrs = append(attrs, attribute.Int64("chat.id", c.ID))
	}
	if u := tc.Sender(); u != nil {
		attrs = append(attrs, attribute.Int64("user.id", u.ID))
	}
	return attrs
}
