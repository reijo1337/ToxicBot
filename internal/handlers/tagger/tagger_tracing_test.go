package tagger

import (
	"context"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
)

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestBuildTag_EmitsTimerRootSpan(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	ctrl := gomock.NewController(t)
	gen := NewMockmessageGenerator(ctrl)
	settings := NewMocksettingsProvider(ctrl)
	settings.EXPECT().GetForChat(gomock.Any(), int64(100)).
		Return(&chatsettings.Settings{AIChance: 0.5}, nil)
	gen.EXPECT().GetMessageText(gomock.Any(), prompt, float32(0.5)).
		Return(message.GenerationResult{Message: "сосунок", Strategy: message.AiGenerationStrategy})

	h := &Handler{ctx: context.Background(), generator: gen, settingsProvider: settings}
	text := h.buildTag(100, 200, "ник")
	assert.Contains(t, text, "сосунок")

	ended := sr.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, "tagger", ended[0].Name())
	attrs := map[string]string{}
	for _, kv := range ended[0].Attributes() {
		if kv.Value.Type() == attribute.STRING {
			attrs[string(kv.Key)] = kv.Value.AsString()
		}
	}
	assert.Equal(t, "timer", attrs["trigger"])
}
