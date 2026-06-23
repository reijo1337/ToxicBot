package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gopkg.in/telebot.v3"
)

type fakeTC struct {
	telebot.Context

	store map[string]interface{}
	upd   telebot.Update
}

func (c *fakeTC) Set(key string, v interface{}) {
	if c.store == nil {
		c.store = map[string]interface{}{}
	}
	c.store[key] = v
}
func (c *fakeTC) Get(key string) interface{} { return c.store[key] }
func (c *fakeTC) Update() telebot.Update     { return c.upd }
func (c *fakeTC) Chat() *telebot.Chat        { return &telebot.Chat{ID: 7} }
func (c *fakeTC) Sender() *telebot.User      { return &telebot.User{ID: 9} }

type stubSub struct{ slug string }

func (s stubSub) Slug() string               { return s.slug }
func (stubSub) Handle(telebot.Context) error { return nil }

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestDispatcher_StartsRootSpanNamedByEndpoint(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	h := New(telebot.OnText, stubSub{slug: "bulling"})
	require.NoError(t, h.Handle(&fakeTC{upd: telebot.Update{ID: 123}}))

	ended := sr.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, "on_text", ended[0].Name())
}
