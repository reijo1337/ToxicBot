package bulling

import (
	"context"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

type stubSettings struct{ s *chatsettings.Settings }

func (st stubSettings) GetForChat(_ context.Context, _ int64) (*chatsettings.Settings, error) {
	return st.s, nil
}

type fakeCtx struct {
	telebot.Context

	store  map[string]interface{}
	chat   *telebot.Chat
	sender *telebot.User
	msg    *telebot.Message
	bot    *telebot.Bot
}

func (c *fakeCtx) Chat() *telebot.Chat       { return c.chat }
func (c *fakeCtx) Sender() *telebot.User     { return c.sender }
func (c *fakeCtx) Message() *telebot.Message { return c.msg }
func (c *fakeCtx) Bot() *telebot.Bot         { return c.bot }
func (c *fakeCtx) Set(k string, v interface{}) {
	if c.store == nil {
		c.store = map[string]interface{}{}
	}
	c.store[k] = v
}
func (c *fakeCtx) Get(k string) interface{} { return c.store[k] }

//nolint:paralleltest // sets global OTel tracer provider / mutates package state; must run serially
func TestHandle_EmitsBullingSpanOnSkip(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))

	ctrl := gomock.NewController(t)
	gen := NewMockmessageGenerator(ctrl)
	statIncer := NewMockstatIncer(ctrl)
	history := NewMockhistoryBuffer(ctrl)
	replier := NewMockbotReplier(ctrl)

	// Skip path: high ThresholdCount means a single message never meets the
	// threshold, so the handler records to history and returns.
	history.EXPECT().Add(int64(7), gomock.Any())

	h, err := New(context.Background(), gen, statIncer, stubSettings{s: &chatsettings.Settings{
		ThresholdCount: 100,
		ThresholdTime:  time.Minute,
		Cooldown:       time.Hour,
		AIChance:       1.0,
	}}, history, replier, "@bot")
	require.NoError(t, err)

	ctx := &fakeCtx{
		chat:   &telebot.Chat{ID: 7},
		sender: &telebot.User{ID: 9, Username: "alice"},
		msg:    &telebot.Message{ID: 1, Unixtime: time.Now().Unix(), Text: "привет"},
		bot:    &telebot.Bot{Me: &telebot.User{ID: 1, Username: "bot"}},
	}
	require.NoError(t, h.Handle(ctx))

	var found bool
	for _, s := range sr.Ended() {
		if s.Name() == "bulling" {
			found = true
			attrs := map[string]string{}
			for _, kv := range s.Attributes() {
				if kv.Value.Type() == attribute.STRING {
					attrs[string(kv.Key)] = kv.Value.AsString()
				}
			}
			assert.Equal(t, "skip", attrs["outcome"])
		}
	}
	assert.True(t, found, "bulling span must be emitted")
}
