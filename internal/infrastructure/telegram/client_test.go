package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type contextBody struct {
	ctx    context.Context
	text   *strings.Reader
	closed bool
}

func (b *contextBody) Read(p []byte) (int, error) {
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	default:
		return b.text.Read(p)
	}
}

func (b *contextBody) Close() error {
	b.closed = true

	return nil
}

func requestUpdates(t *testing.T, client *http.Client) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://telegram.invalid/bottest/getUpdates",
		nil,
	)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

func TestClient_ClonesBaseAndPreservesResponseLifetime(t *testing.T) {
	t.Parallel()

	pollContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller := &Poller{
		pollContext: pollContext,
	}

	var body *contextBody
	baseTransport := transportFunc(func(r *http.Request) (*http.Response, error) {
		body = &contextBody{
			ctx:  r.Context(),
			text: strings.NewReader("complete response body"),
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       body,
		}, nil
	})

	redirectError := errors.New("redirect policy")
	base := &http.Client{
		Timeout:   3 * time.Second,
		Transport: baseTransport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return redirectError
		},
	}

	client := poller.Client(base)

	require.NotSame(t, base, client)
	assert.Equal(t, base.Timeout, client.Timeout)
	require.ErrorIs(t, client.CheckRedirect(nil, nil), redirectError)
	assert.IsType(t, baseTransport, base.Transport)

	response, err := requestUpdates(t, client)

	require.NoError(t, err)

	data, err := io.ReadAll(response.Body)

	require.NoError(t, err)
	assert.Equal(t, "complete response body", string(data))

	select {
	case <-body.ctx.Done():
		t.Fatal("context cancelled before response body was closed")
	default:
	}

	require.NoError(t, response.Body.Close())
	assert.True(t, body.closed)

	select {
	case <-body.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("body Close did not clean up context")
	}
}

func TestClient_CancellationStaysAttachedAfterHeaders(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{"poll stopped": "poll", "original request cancelled": "request"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pollContext, pollCancel := context.WithCancel(context.Background())
			defer pollCancel()
			requestContext, requestCancel := context.WithCancel(context.Background())
			defer requestCancel()
			poller := &Poller{
				pollContext: pollContext,
			}

			var capturedRequest *http.Request
			client := poller.Client(
				&http.Client{
					Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
						capturedRequest = r

						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body: &contextBody{
								ctx:  r.Context(),
								text: strings.NewReader("unread"),
							},
						}, nil
					}),
				},
			)

			req, err := http.NewRequestWithContext(
				requestContext,
				http.MethodPost,
				"http://telegram.invalid/bottest/getUpdates",
				nil,
			)

			require.NoError(t, err)

			response, err := client.Do(req)

			require.NoError(t, err)

			if source == "poll" {
				pollCancel()
			} else {
				requestCancel()
			}

			select {
			case <-capturedRequest.Context().Done():
			case <-time.After(time.Second):
				t.Fatal("request did not observe cancellation")
			}

			_, err = io.ReadAll(response.Body)

			require.ErrorIs(t, err, context.Canceled)
			require.NoError(t, response.Body.Close())
		})
	}
}

type waitingBody struct {
	ctx context.Context
}

func (b *waitingBody) Read([]byte) (int, error) {
	<-b.ctx.Done()

	return 0, b.ctx.Err()
}

func (b *waitingBody) Close() error {
	return nil
}

func TestClient_PreservesTimeoutWhileReadingBody(t *testing.T) {
	t.Parallel()

	poller := &Poller{
		pollContext: context.Background(),
	}

	client := poller.Client(
		&http.Client{
			Timeout: 20 * time.Millisecond,
			Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: &waitingBody{
						ctx: r.Context(),
					},
				}, nil
			}),
		},
	)

	response, err := requestUpdates(t, client)

	require.NoError(t, err)

	_, err = io.ReadAll(response.Body)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, response.Body.Close())
}

func TestClient_LeavesOtherMethodsUntouched(t *testing.T) {
	t.Parallel()

	pollContext, cancel := context.WithCancel(context.Background())
	cancel()
	poller := &Poller{
		pollContext: pollContext,
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"http://telegram.invalid/bottest/sendMessage",
		nil,
	)

	require.NoError(t, err)

	client := poller.Client(
		&http.Client{
			Transport: transportFunc(func(actual *http.Request) (*http.Response, error) {
				assert.Same(t, req, actual)

				select {
				case <-actual.Context().Done():
					t.Error("poll stop affected another Telegram method")
				default:
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ordinary method")),
				}, nil
			}),
		},
	)

	response, err := client.Transport.RoundTrip(req)

	require.NoError(t, err)

	data, err := io.ReadAll(response.Body)

	require.NoError(t, err)
	assert.Equal(t, "ordinary method", string(data))
	require.NoError(t, response.Body.Close())
}

func TestClient_CleansUpOnTransportError(t *testing.T) {
	t.Parallel()

	poller := &Poller{
		pollContext: context.Background(),
	}

	want := errors.New("transport failure")

	var capturedRequest *http.Request
	client := poller.Client(
		&http.Client{
			Transport: transportFunc(
				func(r *http.Request) (*http.Response, error) {
					capturedRequest = r

					return nil, want
				},
			),
		},
	)

	response, err := requestUpdates(t, client)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}

	require.ErrorIs(t, err, want)

	select {
	case <-capturedRequest.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("failed request context was not cleaned up")
	}
}

func TestClient_DefaultTimeoutAndTransport(t *testing.T) {
	t.Parallel()

	poller := &Poller{}
	client := poller.Client(nil)

	assert.Equal(t, time.Minute, client.Timeout)
	assert.NotNil(t, client.Transport)
}
