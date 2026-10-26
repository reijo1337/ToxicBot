package linkpreview

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxConcurrentHTTP = 4
	maxRedirects      = 3
	requestTimeout    = 3 * time.Second
	dnsTimeout        = time.Second
	tlsTimeout        = 2 * time.Second
	maxHeaderBytes    = 32 * 1024
	userAgent         = "ToxicBot-LinkPreview/1.0"
)

// Metadata contains independent, untrusted OpenGraph fields from the page head.
type Metadata struct{ Title, Description, ImageURL string }

// Error carries only fixed diagnostics; upstream errors and URLs are never retained.
type Error struct{ Stage, Reason string }

func (e *Error) Error() string { return "link preview " + e.Stage + ": " + e.Reason }

// Loader fetches anonymous resources from validated public HTTP(S) destinations.
type Loader struct {
	allowHTTP   func(string) bool
	lookupIP    func(context.Context, string, string) ([]netip.Addr, error)
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsRoots    *x509.CertPool
	slots       chan struct{}
}

// New shares the HTTP concurrency bound and invokes allowHTTP before each HTTP hop.
// A nil callback permits requests without applying an additional rate limit.
func New(allowHTTP func(string) bool) *Loader {
	return &Loader{
		allowHTTP:   allowHTTP,
		lookupIP:    net.DefaultResolver.LookupNetIP,
		dialContext: (&net.Dialer{Timeout: requestTimeout}).DialContext,
		// Four active requests may hold tokens; additional callers wait in their own context.
		slots: make(chan struct{}, maxConcurrentHTTP),
	}
}

// Page reads only bounded HTML head metadata, without following content instructions.
func (l *Loader) Page(ctx context.Context, rawURL string) (Metadata, error) {
	ctx, span := tracing.Tracer().Start(ctx, "linkpreview.page")
	defer span.End()
	//nolint:bodyclose // closeResponse closes the body, cancels its request, and releases the HTTP slot together.
	response, target, closeResponse, err := l.fetch(ctx, rawURL, "text/html", span)
	if err != nil {
		return Metadata{}, finish(span, "page", err)
	}
	defer closeResponse()
	metadata, err := parseHead(response.Body, response.Header.Get("Content-Type"), target)
	if err != nil {
		return metadata, finish(span, "page", err)
	}
	return metadata, nil
}

// Image returns only bounded PNG or JPEG data with acceptable decoded dimensions.
func (l *Loader) Image(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, span := tracing.Tracer().Start(ctx, "linkpreview.image")
	defer span.End()
	//nolint:bodyclose // closeResponse closes the body, cancels its request, and releases the HTTP slot together.
	response, _, closeResponse, err := l.fetch(ctx, rawURL, "image/png, image/jpeg", span)
	if err != nil {
		return nil, finish(span, "image", err)
	}
	defer closeResponse()
	data, err := readImage(response.Body, response.Header.Get("Content-Type"))
	if err != nil {
		return nil, finish(span, "image", err)
	}
	span.SetAttributes(attribute.Int("linkpreview.bytes", len(data)))
	return data, nil
}

func (l *Loader) fetch(
	ctx context.Context,
	rawURL, accept string,
	span trace.Span,
) (*http.Response, *url.URL, func(), error) {
	target, err := ParseURL(rawURL)
	if err != nil {
		return nil, nil, nil, err
	}
	for redirects := 0; ; redirects++ {
		span.SetAttributes(
			attribute.String("server.address", target.Hostname()),
			attribute.Int("linkpreview.redirects", redirects),
		)
		response, closeResponse, hopErr := l.hop(ctx, target, accept)
		if hopErr != nil {
			return nil, nil, nil, hopErr
		}
		span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
		if encoding := strings.TrimSpace(
			response.Header.Get("Content-Encoding"),
		); encoding != "" &&
			!strings.EqualFold(encoding, "identity") {
			closeResponse()
			return nil, nil, nil, errors.New("content_encoding")
		}
		if response.StatusCode == http.StatusOK {
			return response, target, closeResponse, nil
		}
		closeResponse()
		if !redirectStatus(response.StatusCode) {
			return nil, nil, nil, errors.New("http_status")
		}
		if redirects >= maxRedirects {
			return nil, nil, nil, errors.New("redirect_limit")
		}
		location, parseErr := url.Parse(response.Header.Get("Location"))
		if parseErr != nil || response.Header.Get("Location") == "" {
			return nil, nil, nil, errors.New("redirect_url")
		}
		next, checkErr := ParseURL(target.ResolveReference(location).String())
		if checkErr != nil {
			return nil, nil, nil, checkErr
		}
		if target.Scheme == schemeHTTPS && next.Scheme != schemeHTTPS {
			return nil, nil, nil, errors.New("https_downgrade")
		}
		target = next
	}
}

func (l *Loader) hop(
	ctx context.Context,
	target *url.URL,
	accept string,
) (*http.Response, func(), error) {
	select {
	case l.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, errors.New("timeout")
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	release := func() { cancel(); <-l.slots }
	address, err := l.resolve(requestCtx, target.Hostname())
	if err != nil {
		release()
		return nil, nil, err
	}
	if l.allowHTTP != nil && !l.allowHTTP(target.Hostname()) {
		release()
		return nil, nil, errors.New("http_quota")
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return l.dialContext(ctx, network, destination(target, address))
		},
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    l.tlsRoots,
			NextProtos: []string{"http/1.1"},
		},
		TLSHandshakeTimeout:    tlsTimeout,
		ResponseHeaderTimeout:  requestTimeout,
		MaxResponseHeaderBytes: maxHeaderBytes,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		ForceAttemptHTTP2:      false,
		TLSNextProto:           make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target.String(), nil)
	if err != nil {
		release()
		return nil, nil, errors.New("invalid_url")
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Encoding", "identity")
	// RoundTrip provides no cookie jar, automatic redirect, or URL-bearing client error wrapper.
	response, err := transport.RoundTrip(request)
	if err != nil {
		transport.CloseIdleConnections()
		release()
		return nil, nil, requestFailure(err)
	}
	closeResponse := func() { _ = response.Body.Close(); transport.CloseIdleConnections(); release() }
	return response, closeResponse, nil
}

func (l *Loader) resolve(ctx context.Context, hostname string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(hostname); err == nil {
		if !publicAddress(addr) {
			return netip.Addr{}, errors.New("blocked_address")
		}
		return addr.Unmap(), nil
	}
	dnsCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()
	addresses, err := l.lookupIP(dnsCtx, "ip", hostname+".")
	if err != nil {
		return netip.Addr{}, errors.New("dns")
	}
	if len(addresses) == 0 {
		return netip.Addr{}, errors.New("dns")
	}
	for _, addr := range addresses {
		if !publicAddress(addr) {
			return netip.Addr{}, errors.New("blocked_address")
		}
	}
	return addresses[0].Unmap(), nil
}

func redirectStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func requestFailure(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("timeout")
	}
	var certificateError *tls.CertificateVerificationError
	if errors.As(err, &certificateError) {
		return errors.New("tls")
	}
	return errors.New("network")
}

func finish(span trace.Span, stage string, err error) error {
	safe := &Error{Stage: stage, Reason: err.Error()}
	span.SetAttributes(
		attribute.String("linkpreview.stage", stage),
		attribute.String("linkpreview.reason", safe.Reason),
	)
	span.SetStatus(codes.Error, safe.Reason)
	return safe
}
