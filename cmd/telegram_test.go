package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

type pollingTransportFunc func(*http.Request) (*http.Response, error)

func (f pollingTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// spec: REACT-008
//
//nolint:paralleltest // controls scheduling to reproduce the SDK's delayed stop watcher
func TestTelegramPoller_StopBeforeSDKCancellationWatcher(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	stopped := make(chan struct{})
	cancelled := make(chan bool, 1)

	var bot *telebot.Bot
	client := &http.Client{
		Transport: pollingTransportFunc(func(r *http.Request) (*http.Response, error) {
			go func() {
				bot.Stop()
				close(stopped)
			}()
			timer := time.NewTimer(200 * time.Millisecond)
			defer timer.Stop()

			select {
			case <-r.Context().Done():
				cancelled <- true
				return nil, r.Context().Err()
			case <-timer.C:
				cancelled <- false
				return nil, errors.New("request was not cancelled by Stop")
			}
		}),
	}

	poller := newTelegramPoller(time.Minute, chathistory.NewBuffer(100), logger.New())

	var err error
	bot, err = telebot.NewBot(telebot.Settings{
		Offline: true,
		Token:   "test",
		URL:     "http://example.invalid",
		Client:  poller.Client(client),
		Poller:  poller,
	})

	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		bot.Start()
		close(done)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Bot.Stop did not return")
	}

	assert.True(t, <-cancelled, "Raw request outlived Bot.Stop cancellation")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Bot.Start did not return")
	}
}

// spec: REACT-007
// spec: REACT-009
func TestTelegramPoller_MainWiring(t *testing.T) {
	t.Parallel()

	history := chathistory.NewBuffer(100)

	var calls, active atomic.Int32

	var overlapping atomic.Bool
	polled := make(chan struct{})

	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bottest/getUpdates", r.URL.Path)

		current := active.Add(1)
		defer active.Add(-1)
		if current > 1 {
			overlapping.Store(true)
		}

		var req struct {
			Allowed []string `json:"allowed_updates"`
		}

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Contains(t, req.Allowed, "message_reaction")
		assert.NotContains(t, req.Allowed, "message_reaction_count")
		assert.NotContains(t, req.Allowed, "chat_member")

		if calls.Add(1) == 1 {
			_, err := w.Write([]byte(`{
  "ok": true,
  "result": [
    {"update_id": 1, "message": {
      "message_id": 10, "chat": {"id": -100, "type": "supergroup"},
      "from": {"id": 7}, "text": "ordinary"
    }},
    {"update_id": 2, "message_reaction": {
      "chat": {"id": -100, "type": "supergroup"}, "message_id": 10,
      "user": {"id": 8, "first_name": "Alice"}, "date": 1700000000,
      "old_reaction": [], "new_reaction": [{"type": "emoji", "emoji": "👍"}]
    }}
  ]
}`))

			assert.NoError(t, err)

			return
		}

		once.Do(func() {
			close(polled)
		})
		<-r.Context().Done()
	}))

	t.Cleanup(server.Close)

	poller := newTelegramPoller(time.Second, history, logger.New())
	bot, err := telebot.NewBot(telebot.Settings{
		Offline: true,
		Token:   "test",
		URL:     server.URL,
		Client:  poller.Client(server.Client()),
		Poller:  poller,
	})

	require.NoError(t, err)

	text := make(chan string, 1)
	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		text <- c.Text()

		return nil
	})
	done := make(chan struct{})

	go func() {
		bot.Start()
		close(done)
	}()

	select {
	case <-polled:
	case <-time.After(5 * time.Second):
		t.Fatal("poller was not wired")
	}

	bot.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bot did not stop")
	}

	assert.False(t, overlapping.Load())
	assert.Equal(t, int32(2), calls.Load())

	select {
	case got := <-text:
		assert.Equal(t, "ordinary", got)
	case <-time.After(time.Second):
		t.Fatal("ordinary message lost")
	}

	assert.Empty(t, text)

	entries := history.Get(-100)

	require.Len(t, entries, 1)
	assert.Equal(t, 2, entries[0].Reaction.UpdateID)
	assert.Zero(t, entries[0].SenderID)
}
