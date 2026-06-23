package gigachat //nolint:testpackage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestGenerateContent_EmitsGenAiSpan(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth"):
			_, _ = w.Write([]byte(`{"access_token":"t","expires_at":99999999999999}`))
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = w.Write([]byte(`{"id":"file-1"}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"кот на фото"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := &Client{
		httpClient: &http.Client{Timeout: 2 * time.Second},
		cfg:        config{Model: "GigaChat-Pro", Scope: "S", AuthKey: "k"},
		authURL:    srv.URL + "/oauth",
		baseURL:    srv.URL,
	}

	out, err := c.GenerateContent(context.Background(), "опиши", []byte("img"))
	require.NoError(t, err)
	assert.Equal(t, "кот на фото", out)

	ended := sr.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, "gen_ai gigachat", ended[0].Name())
	attrs := map[string]string{}
	for _, kv := range ended[0].Attributes() {
		if kv.Value.Type() == attribute.STRING {
			attrs[string(kv.Key)] = kv.Value.AsString()
		}
	}
	assert.Equal(t, "кот на фото", attrs["gen_ai.output"])
}
