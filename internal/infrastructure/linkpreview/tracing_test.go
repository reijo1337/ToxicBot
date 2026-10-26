package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// spec: LINK-024
//
//nolint:paralleltest // sets the global OTel provider; other parallel tests start afterwards
func TestLoader_RealSpansNeverCaptureSecrets(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(
		func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) },
	)
	loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().
			Set("Location", "http://user:LOCATION_SECRET@other.example/LOCATION_PATH?token=LOCATION_QUERY")
		w.WriteHeader(http.StatusFound)
	}))
	_, err := loader.Page(t.Context(), "http://example.com/REQUEST_SECRET?token=QUERY_SECRET")
	require.Error(t, err)
	loader.dialContext = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("NETWORK_SECRET http://example.com/NETWORK_PATH?token=NETWORK_QUERY")
	}
	_, err = loader.Image(t.Context(), "http://example.com/IMAGE_SECRET?token=IMAGE_QUERY")
	require.Error(t, err)
	loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) { return nil, errors.New("DNS_SECRET") }
	_, err = loader.Page(t.Context(), "http://example.com/DNS_PATH?token=DNS_QUERY")
	require.Error(t, err)
	ended := recorder.Ended()
	require.Len(t, ended, 3)
	for _, span := range ended {
		attributes := make(map[string]any)
		for _, value := range span.Attributes() {
			attributes[string(value.Key)] = value.Value.AsInterface()
		}
		assert.Equal(t, testHostname, attributes["server.address"])
		assert.NotEmpty(t, attributes["linkpreview.stage"])
		assert.NotEmpty(t, attributes["linkpreview.reason"])
		assert.True(t, span.EndTime().After(span.StartTime()))
		diagnostic := fmt.Sprint(attributes, span.Events(), span.Status(), span.Name())
		for _, secret := range []string{"SECRET", "QUERY", "PATH", "http://", "https://", "user:", "token="} {
			assert.NotContains(t, diagnostic, secret)
		}
	}
}
