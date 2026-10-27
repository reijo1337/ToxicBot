package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultClientTimeout = time.Minute

type pollTransport struct {
	base   http.RoundTripper
	poller *Poller
}

type responseBody struct {
	body    io.ReadCloser
	cleanup func()
}

// Client clones the supplied client and binds getUpdates to the active Poll
// lifetime. Other Telegram methods retain the client's original behavior.
func (p *Poller) Client(base *http.Client) *http.Client {
	client := http.Client{
		Timeout: defaultClientTimeout,
	}
	if base != nil {
		client = *base
	}

	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	client.Transport = &pollTransport{
		base:   transport,
		poller: p,
	}

	return &client
}

func (t *pollTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(req.URL.Path, "/getUpdates") {
		return t.base.RoundTrip(req)
	}

	pollContext := t.poller.currentContext()
	if pollContext == nil {
		return t.base.RoundTrip(req)
	}

	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(pollContext, cancel)
	cleanup := func() {
		stop()
		cancel()
	}

	response, err := t.base.RoundTrip(req.Clone(ctx))
	if err != nil || response == nil || response.Body == nil {
		cleanup()

		return response, err
	}

	response.Body = &responseBody{
		body:    response.Body,
		cleanup: cleanup,
	}

	return response, nil
}

func (b *responseBody) Read(p []byte) (int, error) {
	return b.body.Read(p)
}

func (b *responseBody) Close() error {
	err := b.body.Close()
	b.cleanup()

	return err
}
