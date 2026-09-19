package gigachat //nolint:testpackage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()

	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cfg: config{
			AuthKey: "test-auth-key",
			Scope:   "GIGACHAT_API_PERS",
			Model:   "GigaChat-Pro",
			Timeout: 5 * time.Second,
		},
		token:   "test-token",
		tokenEx: time.Now().Add(time.Hour),
	}
}

// spec: LLM-009
func TestClient_GenerateContent_Success(t *testing.T) {
	t.Parallel()

	prompt := "describe this image"
	imageBytes := []byte("fake-image-data")
	expectedText := "На картинке изображён кот на столе."

	mux := http.NewServeMux()

	mux.HandleFunc("POST /files", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		err := r.ParseMultipartForm(10 << 20) //nolint
		if !assert.NoError(t, err) {
			return
		}

		file, _, err := r.FormFile("file")
		if !assert.NoError(t, err) {
			return
		}
		defer file.Close() //nolint

		data, err := io.ReadAll(file)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, imageBytes, data)
		assert.Equal(t, "general", r.FormValue("purpose")) //nolint

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(uploadResponse{ID: "file-123"}) //nolint
	})

	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}

		var req chatRequest
		err = json.Unmarshal(body, &req)
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, "GigaChat-Pro", req.Model)
		if !assert.Len(t, req.Messages, 1) {
			return
		}

		msg := req.Messages[0]
		assert.Equal(t, "user", msg.Role)
		assert.Equal(t, prompt, msg.Content)
		assert.Equal(t, []string{"file-123"}, msg.Attachments)

		resp := chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: expectedText}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t)

	result, err := client.generateContentWithBaseURL(
		context.Background(), prompt, imageBytes, server.URL,
	)

	require.NoError(t, err)
	assert.Equal(t, expectedText, result)
}

// spec: LLM-010
func TestClient_Chat_HTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`)) //nolint
	}))
	defer server.Close()

	client := newTestClient(t)

	result, err := client.chatWithBaseURL(
		context.Background(), "test-token", "prompt", "file-123", server.URL,
	)

	assert.Empty(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat API error (status 500)")
}

// spec: LLM-011
func TestClient_Chat_EmptyChoices(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{Choices: []choice{}}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint
	}))
	defer server.Close()

	client := newTestClient(t)

	result, err := client.chatWithBaseURL(
		context.Background(), "test-token", "prompt", "file-123", server.URL,
	)

	assert.Empty(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices in response")
}

// spec: LLM-011
func TestClient_Chat_EmptyContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: ""}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint
	}))
	defer server.Close()

	client := newTestClient(t)

	result, err := client.chatWithBaseURL(
		context.Background(), "test-token", "prompt", "file-123", server.URL,
	)

	assert.Empty(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty content in response")
}

// spec: LLM-014
func TestClient_Upload_Error(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad file"}`)) //nolint
	}))
	defer server.Close()

	client := newTestClient(t)

	result, err := client.uploadImageWithBaseURL(
		context.Background(), "test-token", []byte("image"), server.URL,
	)

	assert.Empty(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload error (status 400)")
}

// spec: LLM-012
func TestClient_GetToken_Success(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(30 * time.Minute).UnixMilli()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Basic test-auth-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("RqUID")) //nolint

		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "scope=GIGACHAT_API_PERS", string(body))

		resp := tokenResponse{
			AccessToken: "new-access-token",
			ExpiresAt:   expiresAt,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint
	}))
	defer server.Close()

	client := &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cfg: config{
			AuthKey: "test-auth-key",
			Scope:   "GIGACHAT_API_PERS",
		},
	}

	token, err := client.getTokenWithURL(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, "new-access-token", token)
	assert.Equal(t, "new-access-token", client.token)
}

// spec: LLM-012
func TestClient_GetToken_Cached(t *testing.T) {
	t.Parallel()

	client := &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cfg: config{
			AuthKey: "test-auth-key",
			Scope:   "GIGACHAT_API_PERS",
		},
		token:   "cached-token",
		tokenEx: time.Now().Add(time.Hour),
	}

	token, err := client.getToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "cached-token", token)
}

// spec: LLM-013
func TestNew_MissingAuthKey(t *testing.T) {
	t.Setenv("GIGACHAT_AUTH_KEY", "placeholder")
	require.NoError(t, os.Unsetenv("GIGACHAT_AUTH_KEY"))

	client, err := New()

	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "failed to parse config")
}
