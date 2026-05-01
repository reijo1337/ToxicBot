package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reqBody mirrors the on-the-wire JSON. Only the fields we assert on are
// declared; unknown fields are dropped by the decoder.
type reqBody struct {
	Model    string       `json:"model"`
	Messages []reqMessage `json:"messages"`
}

type reqMessage struct {
	Role    string `json:"role"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
}

func newClientForTest(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	sdk := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(srv.URL),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(2*time.Second),
	)
	return &Client{sdk: sdk, model: "deepseek-chat"}
}

func TestChat_PutsNameOnUserAndAssistant_ButNotSystem(t *testing.T) {
	t.Parallel()

	var got reqBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		raw, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		if !assert.NoError(t, json.Unmarshal(raw, &got)) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi back"}}]}`))
	}))
	defer srv.Close()

	c := newClientForTest(t, srv)
	out, err := c.Chat(
		context.Background(),
		message.LLMMessage{Role: message.RoleSystem, Content: "BE TOXIC"},
		message.LLMMessage{
			Role:    message.RoleUser,
			Name:    "@alice",
			Content: `<msg time="2026-05-01T14:32">hi</msg>`,
		},
		message.LLMMessage{
			Role:    message.RoleAssistant,
			Name:    "@toxic_bot",
			Content: `<msg time="2026-05-01T14:33" reply_to="@alice">prev reply</msg>`,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "hi back", out)

	require.Len(t, got.Messages, 3)
	assert.Equal(t, "system", got.Messages[0].Role)
	assert.Empty(t, got.Messages[0].Name, "system message must not carry a name")
	assert.Equal(t, "BE TOXIC", got.Messages[0].Content)

	assert.Equal(t, "user", got.Messages[1].Role)
	assert.Equal(t, "@alice", got.Messages[1].Name)
	assert.Equal(
		t,
		`<msg time="2026-05-01T14:32">hi</msg>`,
		got.Messages[1].Content,
	) // sanity: original content passes through

	assert.Equal(t, "assistant", got.Messages[2].Role)
	assert.Equal(t, "@toxic_bot", got.Messages[2].Name)
}

func TestChat_RejectsEmptyInput(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatalf("server should not be called when no messages are provided")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newClientForTest(t, srv)
	_, err := c.Chat(context.Background())
	require.Error(t, err)
}

func TestChat_WrapsHTTPErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer srv.Close()

	c := newClientForTest(t, srv)
	_, err := c.Chat(context.Background(),
		message.LLMMessage{Role: message.RoleUser, Content: "hi"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deepseek chat",
		"errors must be wrapped with a context-friendly prefix")
}
