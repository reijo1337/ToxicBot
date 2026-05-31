package deepseek

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestChat_EmitsGenAiSpanWithIO(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(
			[]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"Иди отсюда."}}]}`),
		)
	}))
	defer srv.Close()

	c := newClientForTest(t, srv)
	_, err := c.Chat(context.Background(),
		message.LLMMessage{Role: message.RoleSystem, Content: "BE TOXIC"},
		message.LLMMessage{Role: message.RoleUser, Content: "hi"},
	)
	require.NoError(t, err)

	ended := sr.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, "gen_ai deepseek", ended[0].Name())

	attrs := map[string]string{}
	for _, kv := range ended[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	assert.Equal(t, "Иди отсюда.", attrs["gen_ai.output"])
	assert.Equal(t, "stop", attrs["gen_ai.finish_reason"])
	assert.Contains(t, attrs["gen_ai.input"], "hi")
}
