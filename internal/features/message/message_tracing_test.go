package message

import (
	"context"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
)

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestGenerator_WithHistory_EmitsDecisionAndSanitizeSpans(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	ctrl := gomock.NewController(t)
	aiMock := NewMockai(ctrl)
	aiMock.EXPECT().Chat(gomock.Any(), gomock.Any(), gomock.Any()).Return("ответ", nil)

	g := &Generator{ai: aiMock, systemPrompt: "SYS"}
	history := []chathistory.Entry{{ID: 1, Author: "@a", Text: "нечто"}}
	res := g.GetMessageTextWithHistory(context.Background(), history, 0.0, true)

	require.Equal(t, AiGenerationStrategy, res.Strategy)
	names := map[string]bool{}
	for _, s := range sr.Ended() {
		names[s.Name()] = true
	}
	assert.True(t, names["decision"], "decision span must be emitted")
	assert.True(t, names["sanitize"], "sanitize span must be emitted")
}
