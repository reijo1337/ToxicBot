package gigachat //nolint:testpackage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec: LLM-016
func TestClient_GetToken_DeadlineCancelsWaitForConcurrentOAuth(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"shared-token","expires_at":99999999999999}`)
	}))
	defer server.Close()
	defer unblock()

	client := newTestClient(t)
	client.token = ""
	firstDone := make(chan error, 1)
	go func() { _, err := client.getTokenWithURL(t.Context(), server.URL); firstDone <- err }()
	<-started

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	waiterDone := make(chan error, 1)
	go func() { _, err := client.getTokenWithURL(ctx, server.URL); waiterDone <- err }()
	select {
	case err := <-waiterDone:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Error("token waiter outlived its deadline while another OAuth request held ownership")
		unblock()
		<-waiterDone
	}

	unblock()
	require.NoError(t, <-firstDone)
	token, err := client.getTokenWithURL(t.Context(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, "shared-token", token)
	assert.EqualValues(t, 1, requests.Load())
}

// spec: LLM-017
func TestClient_Chat_DeadlineCancelsRetryBackoff(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := client.chatWithBaseURL(ctx, "test-token", "prompt", "file-123", server.URL)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, result)
	assert.Less(t, time.Since(started), time.Second)
	assert.EqualValues(t, 1, requests.Load())
}

// spec: LLM-017
func TestWaitForRetry_CancellationDoesNotAdvanceBackoff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := time.Now()
		done := make(chan error, 1)
		go func() { done <- waitForRetry(ctx, 3*time.Second) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		require.ErrorIs(t, <-done, context.Canceled)
		assert.Equal(t, time.Duration(0), time.Since(started))
	})
}

// spec: LLM-017
func TestWaitForRetry_RespectsUncanceledDelay(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		require.NoError(t, waitForRetry(t.Context(), 2*time.Second))
		assert.Equal(t, 2*time.Second, time.Since(started))
	})
}

// spec: LLM-012
func TestClient_GetToken_CanceledOwnerReleasesAuthorization(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-release
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"replacement-token","expires_at":99999999999999}`)
	}))
	defer server.Close()
	defer close(release)
	client := newTestClient(t)
	client.token = ""
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() { _, err := client.getTokenWithURL(ctx, server.URL); firstDone <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	nextCtx, nextCancel := context.WithTimeout(t.Context(), time.Second)
	defer nextCancel()
	token, err := client.getTokenWithURL(nextCtx, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "replacement-token", token)
	assert.EqualValues(t, 2, requests.Load())
}
