package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gopkg.in/telebot.v3"
)

// fakeTC is a telebot.Context stub backed by a real store map so Set/Get work.
type fakeTC struct {
	telebot.Context

	store map[string]interface{}
}

func newFakeTC() *fakeTC { return &fakeTC{store: map[string]interface{}{}} }

func (c *fakeTC) Set(key string, v interface{}) { c.store[key] = v }
func (c *fakeTC) Get(key string) interface{}    { return c.store[key] }

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestStartHandlerSpan_ParentsOffStashedRoot(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)

	tc := newFakeTC()
	rootCtx, root := Tracer().Start(context.Background(), "on_text")
	StashRootContext(tc, rootCtx)

	_, child := StartHandlerSpan(tc, "bulling")
	child.End()
	root.End()

	ended := sr.Ended()
	require.Len(t, ended, 2)
	// child ends first; assert it parents off root.
	assert.Equal(t, "bulling", ended[0].Name())
	assert.Equal(t, root.SpanContext().SpanID(), ended[0].Parent().SpanID())
}
