package on_user_left

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

type fakeCtx struct {
	telebot.Context

	store map[string]interface{}
}

func (c *fakeCtx) Chat() *telebot.Chat                     { return &telebot.Chat{ID: 1} }
func (c *fakeCtx) Sender() *telebot.User                   { return &telebot.User{ID: 2} }
func (c *fakeCtx) Reply(interface{}, ...interface{}) error { return nil }
func (c *fakeCtx) Set(k string, v interface{}) {
	if c.store == nil {
		c.store = map[string]interface{}{}
	}
	c.store[k] = v
}
func (c *fakeCtx) Get(k string) interface{} { return c.store[k] }

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestHandle_EmitsOnUserLeftSpan(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	ctrl := gomock.NewController(t)
	statIncer := NewMockstatIncer(ctrl)
	statIncer.EXPECT().Inc(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	h := New(context.Background(), statIncer)
	require.NoError(t, h.Handle(&fakeCtx{}))

	ended := sr.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, "on_user_left", ended[0].Name())
	attrs := map[string]string{}
	for _, kv := range ended[0].Attributes() {
		if kv.Value.Type() == attribute.STRING {
			attrs[string(kv.Key)] = kv.Value.AsString()
		}
	}
	assert.Equal(t, "react", attrs["outcome"])
}
