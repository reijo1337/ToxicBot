package telegram_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_reaction"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/telegram"
	"github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

const reactionJSON = `"message_reaction":{"chat":{"id":-100,"type":"supergroup"},"message_id":42,"user":{"id":7,"first_name":"Alice"},"date":1700000000,"old_reaction":[],"new_reaction":[{"type":"emoji","emoji":"👍"}]}`

type reactionFunc func(context.Context, int, *telebot.MessageReaction) error

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func (f reactionFunc) Handle(ctx context.Context, id int, event *telebot.MessageReaction) error {
	return f(ctx, id, event)
}

type request struct {
	Offset  int      `json:"offset"`
	Timeout int      `json:"timeout"`
	Allowed []string `json:"allowed_updates"`
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(body))

	assert.NoError(t, err)
}

type observedPoller struct {
	inner   telebot.Poller
	entered chan struct{}
}

func (p *observedPoller) Poll(bot *telebot.Bot, dest chan telebot.Update, stop chan struct{}) {
	close(p.entered)
	p.inner.Poll(bot, dest, stop)
}

func startBot(
	t *testing.T,
	server *httptest.Server,
	poller *telegram.Poller,
	setup func(*telebot.Bot),
) func() {
	t.Helper()

	observed := &observedPoller{
		inner:   poller,
		entered: make(chan struct{}),
	}

	bot, err := telebot.NewBot(
		telebot.Settings{
			Offline: true,
			Token:   "test",
			URL:     server.URL,
			Poller:  observed,
			Client:  poller.Client(server.Client()),
		},
	)

	require.NoError(t, err)

	setup(bot)
	done := make(chan struct{})

	go func() {
		bot.Start()
		close(done)
	}()
	<-observed.entered

	var once sync.Once
	stop := func() {
		once.Do(func() {
			stopped := make(chan struct{})

			go func() {
				bot.Stop()
				close(stopped)
			}()

			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Error("Bot.Stop did not return")
			}

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("Bot.Start did not return")
			}
		})
	}

	t.Cleanup(stop)

	return stop
}

// spec: REACT-005
// spec: REACT-007
func TestPoller_MixedJSONAndSubscriptions(t *testing.T) {
	t.Parallel()

	requests := make(chan request)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bottest/getUpdates", r.URL.Path)

		var req request

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		select {
		case requests <- req:
		case <-r.Context().Done():
			return
		}

		if calls.Add(1) == 1 {
			writeJSON(
				t,
				w,
				`{"ok":true,"result":[{"update_id":10,"message":{"message_id":40,"chat":{"id":-100,"type":"supergroup"},"from":{"id":8},"text":"hello"}},{"update_id":11,`+reactionJSON+`},{"update_id":12,"message":{"message_id":41,"chat":{"id":-100,"type":"supergroup"},"from":{"id":8},"new_chat_members":[{"id":9}]}},{"update_id":13,"message_reaction_count":{"chat":{"id":-100},"message_id":42,"reactions":[{"type":{"type":"emoji","emoji":"👍"},"total_count":1}]}},{"update_id":14,"future_update":{}},{"update_id":15,"chat_member":{"chat":{"id":-100}}}]}`,
			)

			return
		}

		<-r.Context().Done()
	}))

	t.Cleanup(server.Close)

	history := chathistory.NewBuffer(100)
	poller := telegram.NewPoller(10*time.Second, on_reaction.New(history), logger.New())
	messages := make(chan string, 1)
	joins := make(chan int64, 1)
	stop := startBot(t, server, poller, func(bot *telebot.Bot) {
		bot.Handle(
			telebot.OnText,
			func(c telebot.Context) error {
				messages <- c.Text()

				return nil
			},
		)
		bot.Handle(
			telebot.OnUserJoined,
			func(c telebot.Context) error {
				joins <- c.Message().UserJoined.ID

				return nil
			},
		)
	})

	first := receive(t, requests)

	assert.Equal(t, 1, first.Offset)
	assert.Equal(t, 10, first.Timeout)
	assert.ElementsMatch(
		t,
		[]string{
			"message",
			"edited_message",
			"channel_post",
			"edited_channel_post",
			"callback_query",
			"inline_query",
			"chosen_inline_result",
			"shipping_query",
			"pre_checkout_query",
			"poll",
			"poll_answer",
			"my_chat_member",
			"chat_join_request",
			"message_reaction",
		},
		first.Allowed,
	)
	assert.Equal(t, "hello", receive(t, messages))
	assert.Equal(t, int64(9), receive(t, joins))
	assert.Equal(t, 16, receive(t, requests).Offset)

	stop()
	entries := history.Get(-100)

	require.Len(t, entries, 1)
	assert.Equal(t, 11, entries[0].Reaction.UpdateID)
	assert.Equal(t, int32(2), calls.Load())
}

// spec: REACT-008
func TestPoller_StopInterruptsRaw(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)

		assert.NoError(t, err)

		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))

	t.Cleanup(server.Close)

	poller := telegram.NewPoller(
		time.Minute,
		reactionFunc(func(context.Context, int, *telebot.MessageReaction) error {
			return nil
		}),
		logger.New(),
	)

	stop := startBot(t, server, poller, func(*telebot.Bot) {})
	receive(t, started)
	stop()
	receive(t, cancelled)
}

// spec: REACT-008
func TestPoller_DirectStopInterruptsRawWithWrappedClient(t *testing.T) {
	t.Parallel()

	poller := telegram.NewPoller(
		time.Minute,
		reactionFunc(func(context.Context, int, *telebot.MessageReaction) error {
			return nil
		}),
		logger.New(),
	)

	stop, done := make(chan struct{}), make(chan struct{})
	cancelled := make(chan bool, 1)
	client := poller.Client(
		&http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				close(stop)
				timer := time.NewTimer(200 * time.Millisecond)
				defer timer.Stop()

				select {
				case <-r.Context().Done():
					cancelled <- true
					return nil, r.Context().Err()
				case <-timer.C:
					cancelled <- false
					return nil, errors.New("direct Poll stop did not cancel Raw")
				}
			}),
		},
	)

	bot, err := telebot.NewBot(
		telebot.Settings{
			Offline: true,
			URL:     "http://telegram.invalid",
			Client:  client,
		},
	)

	require.NoError(t, err)

	go func() {
		poller.Poll(bot, make(chan telebot.Update), stop)
		close(done)
	}()
	receive(t, done)

	assert.True(t, receive(t, cancelled))
}

// spec: REACT-008
func TestPoller_StopInterruptsDestinationWait(t *testing.T) {
	t.Parallel()

	accepted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(
			t,
			w,
			`{"ok":true,"result":[{"update_id":1,`+reactionJSON+`},{"update_id":2,"message":{"text":"blocked"}}]}`,
		)
	}))

	t.Cleanup(server.Close)

	bot, err := telebot.NewBot(
		telebot.Settings{
			Offline: true,
			URL:     server.URL,
			Client:  server.Client(),
		},
	)

	require.NoError(t, err)

	poller := telegram.NewPoller(
		time.Second,
		reactionFunc(
			func(context.Context, int, *telebot.MessageReaction) error {
				close(accepted)

				return nil
			},
		),
		logger.New(),
	)

	stop, done := make(chan struct{}), make(chan struct{})

	go func() {
		poller.Poll(bot, make(chan telebot.Update), stop)
		close(done)
	}()
	receive(t, accepted)
	close(stop)
	receive(t, done)
}

// spec: REACT-008
func TestPoller_RetryPausesAfterNetworkAnd429(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		body  string
		delay time.Duration
	}{
		"network": {
			delay: time.Second,
		},
		"telegram retry_after": {
			body:  `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 2","parameters":{"retry_after":2}}`,
			delay: 2 * time.Second,
		},
		"invalid response": {
			body:  `not-json`,
			delay: time.Second,
		},
		"missing update ID": {
			body:  `{"ok":true,"result":[{"update_id":10,"message":{"text":"unacknowledged"}},{"message":{"text":"missing ID"}}]}`,
			delay: time.Second,
		},
		"invalid update ID": {
			body:  `{"ok":true,"result":[{"update_id":"bad","message":{"text":"bad ID"}}]}`,
			delay: time.Second,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			attempts := make(chan time.Time)

			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, err := io.Copy(io.Discard, r.Body)

					assert.NoError(t, err)

					select {
					case attempts <- time.Now():
					case <-r.Context().Done():
						return
					}

					if calls.Add(1) != 1 {
						<-r.Context().Done()

						return
					}

					if tt.body != "" {
						writeJSON(t, w, tt.body)

						return
					}

					hijacker, ok := w.(http.Hijacker)
					if !assert.True(t, ok) {
						return
					}

					conn, _, err := hijacker.Hijack()
					if assert.NoError(t, err) {
						assert.NoError(t, conn.Close())
					}
				}),
			)

			t.Cleanup(server.Close)

			stop := startBot(
				t,
				server,
				telegram.NewPoller(
					time.Minute,
					reactionFunc(
						func(context.Context, int, *telebot.MessageReaction) error {
							return nil
						},
					),
					logger.New(),
				),
				func(*telebot.Bot) {},
			)

			first, second := receive(t, attempts), receive(t, attempts)

			assert.GreaterOrEqual(t, second.Sub(first), tt.delay)

			stop()
		})
	}
}

// spec: REACT-008
func TestPoller_StopInterruptsRetryDelay(t *testing.T) {
	t.Parallel()

	responded := make(chan struct{})

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeJSON(
			t,
			w,
			`{"ok":false,"error_code":429,"description":"retry later","parameters":{"retry_after":60}}`,
		)
		close(responded)
	}))

	t.Cleanup(server.Close)

	stop := startBot(
		t,
		server,
		telegram.NewPoller(
			time.Minute,
			reactionFunc(func(context.Context, int, *telebot.MessageReaction) error {
				return nil
			}),
			logger.New(),
		),
		func(*telebot.Bot) {},
	)

	receive(t, responded)

	select {
	case <-time.After(25 * time.Millisecond):
	case <-t.Context().Done():
		t.Fatal("test cancelled before retry wait")
	}

	stop()

	assert.Equal(t, int32(1), calls.Load())
}

type recoveringStore struct {
	mu      sync.Mutex
	loads   int
	entries []chathistory.Entry
}

func (s *recoveringStore) Load(context.Context, int64) ([]chathistory.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.loads == 1 {
		return nil, errors.New("temporary database read error")
	}

	return []chathistory.Entry{
		{
			ID:   40,
			Text: "already stored",
		},
	}, nil
}

func (s *recoveringStore) Save(_ context.Context, _ int64, entries []chathistory.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = entries

	return nil
}

// spec: REACT-011
func TestPoller_RetriesFromFailedLoadWithoutRedeliveringPrefix(t *testing.T) {
	t.Parallel()

	requests := make(chan request)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		select {
		case requests <- req:
		case <-r.Context().Done():
			return
		}

		switch calls.Add(1) {
		case 1:
			writeJSON(
				t,
				w,
				`{"ok":true,"result":[{"update_id":10,"message":{"text":"prefix"}},{"update_id":11,`+reactionJSON+`},{"update_id":12,"message":{"text":"suffix"}}]}`,
			)
		case 2:
			writeJSON(
				t,
				w,
				`{"ok":true,"result":[{"update_id":11,`+reactionJSON+`},{"update_id":12,"message":{"text":"suffix"}}]}`,
			)
		default:
			<-r.Context().Done()
		}
	}))

	t.Cleanup(server.Close)

	store := &recoveringStore{}
	history := chathistory.NewBuffer(
		100,
		chathistory.WithStore(store),
		chathistory.WithLogger(logger.New()),
	)

	texts := make(chan string, 1)
	stop := startBot(
		t,
		server,
		telegram.NewPoller(time.Second, on_reaction.New(history), logger.New()),
		func(bot *telebot.Bot) {
			bot.Handle(
				telebot.OnText,
				func(c telebot.Context) error {
					texts <- c.Text()

					return nil
				},
			)
		},
	)

	assert.Equal(t, 1, receive(t, requests).Offset)
	assert.Equal(t, "prefix", receive(t, texts))

	first := time.Now()

	assert.Equal(t, 11, receive(t, requests).Offset)
	assert.GreaterOrEqual(t, time.Since(first), 900*time.Millisecond)
	assert.Equal(t, "suffix", receive(t, texts))
	assert.Equal(t, 13, receive(t, requests).Offset)

	stop()

	assert.Empty(t, texts)

	entries := history.Get(-100)

	require.Len(t, entries, 2)
	assert.Equal(t, "already stored", entries[0].Text)
	require.NotNil(t, entries[1].Reaction)
	assert.Equal(t, 11, entries[1].Reaction.UpdateID)
}

type unavailableChatStore struct {
	mu    sync.Mutex
	saved map[int64][]chathistory.Entry
}

func (*unavailableChatStore) Load(_ context.Context, chatID int64) ([]chathistory.Entry, error) {
	if chatID == -100 {
		return nil, errors.New("decode chat history: damaged gob")
	}

	return nil, nil
}

func (s *unavailableChatStore) Save(
	_ context.Context,
	chatID int64,
	entries []chathistory.Entry,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[chatID] = entries

	return nil
}

// spec: HIST-032
// spec: REACT-011
func TestPoller_PersistentHistoryErrorDoesNotBlockOtherChats(t *testing.T) {
	t.Parallel()

	requests := make(chan request)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		select {
		case requests <- req:
		case <-r.Context().Done():
			return
		}

		if calls.Add(1) <= 3 {
			writeJSON(
				t,
				w,
				`{"ok":true,"result":[{"update_id":10,"future_update":{}},{"update_id":11,`+reactionJSON+`},{"update_id":12,"message":{"chat":{"id":-200},"text":"other chat"}},{"update_id":13,"message_reaction":{"chat":{"id":-200},"message_id":1,"user":{"id":8},"date":1700000000,"new_reaction":[{"type":"emoji","emoji":"👍"}]}}]}`,
			)

			return
		}

		<-r.Context().Done()
	}))

	t.Cleanup(server.Close)

	store := &unavailableChatStore{
		saved: make(map[int64][]chathistory.Entry),
	}

	history := chathistory.NewBuffer(
		100,
		chathistory.WithStore(store),
		chathistory.WithLogger(logger.New()),
	)

	texts := make(chan string, 1)
	stop := startBot(
		t,
		server,
		telegram.NewPoller(time.Second, on_reaction.New(history), logger.New()),
		func(bot *telebot.Bot) {
			bot.Handle(
				telebot.OnText,
				func(c telebot.Context) error {
					texts <- c.Text()

					return nil
				},
			)
		},
	)

	assert.Equal(t, 1, receive(t, requests).Offset)

	first := time.Now()

	assert.Equal(t, 11, receive(t, requests).Offset)

	second := time.Now()

	assert.GreaterOrEqual(t, second.Sub(first), 900*time.Millisecond)
	assert.Equal(t, 11, receive(t, requests).Offset)
	assert.GreaterOrEqual(t, time.Since(second), 1900*time.Millisecond)
	assert.Equal(t, "other chat", receive(t, texts))
	assert.Equal(t, 14, receive(t, requests).Offset)

	stop()
	store.mu.Lock()
	defer store.mu.Unlock()

	assert.NotContains(t, store.saved, int64(-100), "unreadable history must never be overwritten")
	require.Len(t, store.saved[-200], 1)
	assert.Equal(t, 13, store.saved[-200][0].Reaction.UpdateID)
}

// spec: REACT-012
func TestPoller_MalformedUpdateDoesNotBlockBatch(t *testing.T) {
	t.Parallel()

	requests := make(chan request)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		select {
		case requests <- req:
		case <-r.Context().Done():
			return
		}

		if calls.Add(1) == 1 {
			writeJSON(
				t,
				w,
				`{"ok":true,"result":[{"update_id":10,"message":{"text":"before"}},{"update_id":11,"message_reaction":{"message_id":"bad"}},{"update_id":12,"message":{"text":"after"}}]}`,
			)

			return
		}

		<-r.Context().Done()
	}))

	t.Cleanup(server.Close)

	texts := make(chan string, 2)
	stop := startBot(
		t,
		server,
		telegram.NewPoller(time.Second, on_reaction.New(chathistory.NewBuffer(100)), logger.New()),
		func(bot *telebot.Bot) {
			bot.Handle(
				telebot.OnText,
				func(c telebot.Context) error {
					texts <- c.Text()

					return nil
				},
			)
		},
	)

	assert.Equal(t, 1, receive(t, requests).Offset)
	assert.ElementsMatch(
		t,
		[]string{"before", "after"},
		[]string{receive(t, texts), receive(t, texts)},
	)
	assert.Equal(t, 13, receive(t, requests).Offset)

	stop()
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for poller")
		var zero T
		return zero
	}
}
