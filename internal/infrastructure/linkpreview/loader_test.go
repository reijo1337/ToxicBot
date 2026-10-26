package linkpreview

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"
)

const testHostname = "example.com"

func newServerLoader(t *testing.T, handler http.Handler) *Loader {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	loader := New(nil)
	loader.lookupIP = func(_ context.Context, network, host string) ([]netip.Addr, error) {
		if network != "ip" || !strings.HasSuffix(host, ".") {
			return nil, errors.New("DNS name must be absolute")
		}
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	loader.dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "93.184.216.34:80" && address != "93.184.216.34:443" {
			return nil, fmt.Errorf("unpinned dial %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	return loader
}

// spec: LINK-004
// spec: LINK-005
// spec: LINK-006
// spec: LINK-022
func TestPage_OpenGraphHeadOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, html string
		want       Metadata
	}{
		{
			"fields",
			`<HTML><HEAD><META CONTENT="A &amp; B" PROPERTY="OG:TITLE"><meta name="og:description" content="Описание &quot;вещи&quot;"><meta property="og:title" content="ignored"></HEAD><body><meta property="og:description" content="body"></body>`,
			Metadata{Title: "A & B", Description: `Описание "вещи"`},
		},
		{
			"title",
			`<head><meta property="og:title" content=""><meta content="Title" property="og:title"></head>`,
			Metadata{Title: "Title"},
		},
		{
			"description",
			`<head><meta property="og:description" content="Description"></head>`,
			Metadata{Description: "Description"},
		},
		{
			"image",
			`<head><meta property="og:image" content="/first.png"><meta property="og:image" content="/second.png"></head>`,
			Metadata{ImageURL: "http://example.com/first.png"},
		},
		{
			"no_og",
			`<head><title>Not OG</title><meta name="description" content="Not OG"><meta http-equiv="refresh" content="0; url=http://other.example/"></head><body><script>fetch("http://other.example")</script><meta property="og:title" content="body"></body>`,
			Metadata{},
		},
		{
			"implicit_body",
			`<head><meta property="og:title" content="Title"><div><meta property="og:description" content="body">`,
			Metadata{Title: "Title"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = io.WriteString(w, tt.html)
				}),
			)
			got, err := loader.Page(t.Context(), "http://example.com/article")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

// spec: LINK-004
func TestPage_LeadingUTF8BOM(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"text/html", "text/html; charset=utf-8"} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", contentType)
					_, _ = io.WriteString(
						w,
						"\ufeff"+`<head><meta property="og:title" content="Привет"><meta property="og:description" content="Описание"><meta property="og:image" content="/preview.png"></head>`,
					)
				}),
			)
			got, err := loader.Page(t.Context(), "http://example.com/article")
			require.NoError(t, err)
			assert.Equal(t, Metadata{
				Title:       "Привет",
				Description: "Описание",
				ImageURL:    "http://example.com/preview.png",
			}, got)
		})
	}
}

// spec: LINK-006
func TestPage_NonLeadingBOMStillStartsBody(t *testing.T) {
	t.Parallel()
	loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<head>\ufeff"+`<meta property="og:title" content="body">`)
	}))
	got, err := loader.Page(t.Context(), "http://example.com/article")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// spec: LINK-004
func TestPage_DecodesCharsetAndBoundsText(t *testing.T) {
	t.Parallel()
	body, err := charmap.Windows1251.NewEncoder().
		String(`<head><meta charset="windows-1251"><meta property="og:title" content="Привет"><meta property="og:description" content="` + strings.Repeat("я", 510) + `"></head>`)
	require.NoError(t, err)
	loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, body)
	}))
	got, err := loader.Page(t.Context(), "http://example.com/")
	require.NoError(t, err)
	assert.Equal(t, "Привет", got.Title)
	assert.Equal(t, strings.Repeat("я", 500), got.Description)
}

// spec: LINK-007
// spec: LINK-021
func TestPage_ResolveImageAgainstRedirectAndBase(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, base, want string }{
		{"final_page", "", "http://other.example/final/image.png"},
		{"relative_base", `<base href="../assets/">`, "http://other.example/assets/image.png"},
		{
			"absolute_base",
			`<base href="https://images.example/photos/">`,
			"https://images.example/photos/image.png",
		},
		{"invalid_base", `<base href="http://127.0.0.1/">`, "http://other.example/final/image.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Host == testHostname {
						w.Header().Set("Location", "http://other.example/final/page")
						w.WriteHeader(http.StatusFound)
						return
					}
					w.Header().Set("Content-Type", "text/html")
					_, _ = io.WriteString(
						w,
						`<head><meta property="og:image" content="image.png">`+tt.base+`<meta property="og:url" content="http://canonical.example/"></head>`,
					)
				}),
			)
			got, err := loader.Page(t.Context(), "http://example.com/start")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.ImageURL)
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

// spec: LINK-011
// spec: LINK-021
func TestLoader_RejectsUnsafeTargetsBeforeDial(t *testing.T) {
	t.Parallel()
	targets := []string{
		"http://127.0.0.1/",
		"http://169.254.169.254/",
		"http://172.17.0.1/",
		"http://10.0.0.1/",
		"http://100.64.0.1/",
		"http://192.0.2.1/",
		"http://198.18.0.1/",
		"http://224.0.0.1/",
		"http://240.1.1.1/",
		"http://0.0.0.0/",
		"http://[::1]/",
		"http://[fc00::1]/",
		"http://[fe80::1]/",
		"http://[::ffff:127.0.0.1]/",
		"http://[64:ff9b::808:808]/",
		"http://[2002:808:808::1]/",
		"http://[2001:db8::1]/",
		"http://[2001::1]/",
		"http://[ff02::1]/",
		"http://[::]/",
		"http://[fe80::1%25en0]/",
		"http://localhost/",
		"http://name.localhost/",
		"http://service.local/",
		"http://jaeger/",
		"http://service.internal/",
		"http://name.home.arpa/",
		"http://example.com:8080/",
		"https://example.com:80/",
		"ftp://example.com/",
		"http://user:password@example.com/",
		"http://example.com/" + strings.Repeat("a", 2048),
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			loader := New(nil)
			var calls atomic.Int32
			loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
				calls.Add(1)
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			loader.dialContext = func(context.Context, string, string) (net.Conn, error) {
				t.Error("unsafe connection attempted")
				return nil, errors.New("unexpected dial")
			}
			_, err := loader.Page(t.Context(), target)
			require.Error(t, err)
			_, err = loader.Image(t.Context(), target)
			require.Error(t, err)
		})
	}
}

// spec: LINK-011
func TestLoader_DNSIsValidatedAndPinned(t *testing.T) {
	t.Parallel()
	t.Run("mixed_answers", func(t *testing.T) {
		t.Parallel()
		loader := New(nil)
		loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("10.0.0.1"),
			}, nil
		}
		loader.dialContext = func(context.Context, string, string) (net.Conn, error) {
			t.Error("mixed DNS answer dialed")
			return nil, errors.New("unexpected dial")
		}
		_, err := loader.Page(t.Context(), "http://example.com/")
		require.Error(t, err)
	})
	t.Run("no_second_lookup", func(t *testing.T) {
		t.Parallel()
		loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<head><meta property="og:title" content="Pinned"></head>`)
		}))
		var lookups atomic.Int32
		loader.lookupIP = func(_ context.Context, _, host string) ([]netip.Addr, error) {
			require.Equal(t, "example.com.", host)
			if lookups.Add(1) > 1 {
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		got, err := loader.Page(t.Context(), "http://example.com/")
		require.NoError(t, err)
		assert.Equal(t, "Pinned", got.Title)
		assert.EqualValues(t, 1, lookups.Load())
	})
}

// spec: LINK-020
// spec: LINK-021
//
//nolint:paralleltest // checks process environment proxy configuration
func TestLoader_RequestsAreAnonymousAndIgnoreProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	var requests []http.Header
	loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, int64(0), r.ContentLength)
		assert.Equal(t, 1, r.ProtoMajor)
		requests = append(requests, r.Header.Clone())
		w.Header().Set("Set-Cookie", "bot_secret=must_not_copy")
		if r.Host == testHostname {
			w.Header().Set("Location", "http://other.example/redirect")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<head><meta property="og:title" content="anonymous"></head>`)
	}))
	got, err := loader.Page(t.Context(), "http://example.com/secret-path?token=secret-query")
	require.NoError(t, err)
	assert.Equal(t, "anonymous", got.Title)
	require.Len(t, requests, 2)
	for _, header := range requests {
		for _, key := range []string{"Authorization", "Cookie", "Referer", "Origin", "Traceparent", "Tracestate", "Baggage"} {
			assert.Empty(t, header.Get(key), key)
		}
		assert.Equal(t, "identity", header.Get("Accept-Encoding"))
		assert.NotEmpty(t, header.Get("User-Agent"))
	}
}

// spec: LINK-021
// spec: LINK-012
func TestLoader_RedirectPolicyAndPerHopQuota(t *testing.T) {
	t.Parallel()
	for _, location := range []string{"http://127.0.0.1/", "http://other.example:8080/", "ftp://other.example/", "http://user:pass@other.example/"} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					w.Header().Set("Location", location)
					w.WriteHeader(http.StatusFound)
				}),
			)
			_, err := loader.Page(t.Context(), "http://example.com/")
			require.Error(t, err)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
	t.Run("four_requests_max", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "/again")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		_, err := loader.Page(t.Context(), "http://example.com/")
		require.Error(t, err)
		assert.EqualValues(t, 4, calls.Load())
	})
	t.Run("quota_on_redirect", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "http://other.example/again")
			w.WriteHeader(http.StatusFound)
		}))
		var hosts []string
		loader.allowHTTP = func(host string) bool { hosts = append(hosts, host); return host == testHostname }
		_, err := loader.Page(t.Context(), "http://example.com/")
		require.Error(t, err)
		assert.Equal(t, []string{testHostname, "other.example"}, hosts)
		assert.EqualValues(t, 1, calls.Load())
	})
}

// spec: LINK-012
// spec: LINK-026
// spec: LINK-027
func TestLoader_RejectsResponsesWithoutRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		body    string
	}{
		{"429", 429, nil, ""},
		{"500", 500, nil, ""},
		{"101", 101, nil, ""},
		{
			"compressed",
			200,
			map[string]string{"Content-Type": "text/html", "Content-Encoding": "gzip"},
			"not-gzip",
		},
		{
			"compressed_redirect",
			302,
			map[string]string{"Content-Encoding": "gzip", "Location": "http://other.example/"},
			"not-gzip",
		},
		{
			"wrong_type",
			200,
			map[string]string{"Content-Type": "application/json"},
			`{"title":"no"}`,
		},
		{
			"large_headers",
			200,
			map[string]string{
				"Content-Type": "text/html",
				"X-Padding":    strings.Repeat("a", 33*1024),
			},
			"",
		},
		{
			"large_html",
			200,
			map[string]string{"Content-Type": "text/html"},
			`<head><!--` + strings.Repeat("x", 1024*1024) + `--></head>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, http.MethodGet, r.Method)
					for key, value := range tt.headers {
						w.Header().Set(key, value)
					}
					w.WriteHeader(tt.status)
					_, _ = io.WriteString(w, tt.body)
				}),
			)
			_, err := loader.Page(t.Context(), "http://example.com/")
			require.Error(t, err)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

// spec: LINK-012
func TestImage_ValidatesBytesAndGeometry(t *testing.T) {
	t.Parallel()
	var pngBody, jpegBody bytes.Buffer
	require.NoError(t, png.Encode(&pngBody, image.NewRGBA(image.Rect(0, 0, 2, 3))))
	require.NoError(t, jpeg.Encode(&jpegBody, image.NewRGBA(image.Rect(0, 0, 2, 3)), nil))
	huge := append([]byte(nil), pngBody.Bytes()...)
	huge[16] = 0x7f
	huge[17] = 0xff
	huge[18] = 0xff
	huge[19] = 0xff
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	tests := []struct {
		name, contentType string
		body              []byte
		valid             bool
	}{
		{
			"png",
			"image/png",
			pngBody.Bytes(),
			true,
		},
		{"jpeg", "image/jpeg", jpegBody.Bytes(), true},
		{"mime_spoof", "image/png", []byte("<svg></svg>"), false},
		{"gif", "image/gif", []byte("GIF89a"), false},
		{"truncated", "image/png", pngBody.Bytes()[:12], false},
		{"geometry", "image/png", huge, false},
		{
			"large",
			"image/png",
			append(pngBody.Bytes(), bytes.Repeat([]byte("a"), 5*1024*1024)...),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", tt.contentType)
					_, _ = w.Write(tt.body)
				}),
			)
			got, err := loader.Image(t.Context(), "http://example.com/image")
			if !tt.valid {
				require.Error(t, err)
				if tt.name == "geometry" {
					var failure *Error
					require.ErrorAs(t, err, &failure)
					assert.Equal(t, "image_geometry", failure.Reason)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.body, got)
		})
	}
}

// spec: LINK-025
// spec: LINK-021
func TestLoader_TLSVerificationAndDowngrade(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, testHostname, r.TLS.ServerName)
		w.Header().Set("Location", "http://other.example/downgrade")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	loader := New(nil)
	loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	loader.dialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	_, err := loader.Page(t.Context(), "https://example.com/")
	require.Error(t, err)
	assert.Zero(t, calls.Load())
	loader.tlsRoots = x509.NewCertPool()
	loader.tlsRoots.AddCert(srv.Certificate())
	_, err = loader.Page(t.Context(), "https://example.com/")
	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load())
	_, err = loader.Page(t.Context(), "https://wrong.example/")
	require.Error(t, err)
	assert.EqualValues(t, 1, calls.Load())
}

// spec: LINK-012
func TestLoader_HTTPConcurrencyAndCanceledWait(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{}, 5)
	release := make(chan struct{})
	loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<head></head>")
	}))
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() { _, _ = loader.Page(t.Context(), "http://example.com/") })
	}
	for range 4 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("four HTTP slots did not become active")
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	_, err := loader.Page(ctx, "http://example.com/")
	require.Error(t, err)
	select {
	case <-entered:
		t.Error("fifth HTTP request exceeded concurrency limit")
	default:
	}
	close(release)
	group.Wait()
}

// spec: LINK-012
func TestPage_StopsAfterHeadAndKeepsFieldsOnLimit(t *testing.T) {
	t.Parallel()
	t.Run("head_closes_before_body", func(t *testing.T) {
		t.Parallel()
		bodyStopped := make(chan struct{})
		loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(
				w,
				`<head><!--`+strings.Repeat(
					"x",
					1100,
				)+`--><meta property="og:title" content="Ready"></head>`,
			)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
			close(bodyStopped)
		}))
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		got, err := loader.Page(ctx, "http://example.com/")
		require.NoError(t, err)
		assert.Equal(t, "Ready", got.Title)
		select {
		case <-bodyStopped:
		case <-time.After(time.Second):
			t.Fatal("body not closed after head")
		}
	})
	t.Run("partial_fields", func(t *testing.T) {
		t.Parallel()
		loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(
				w,
				`<head><meta property="og:title" content="Ready"><!--`+strings.Repeat(
					"x",
					1024*1024,
				)+`--></head>`,
			)
		}))
		got, err := loader.Page(t.Context(), "http://example.com/")
		require.Error(t, err)
		assert.Equal(t, "Ready", got.Title)
	})
}

// spec: LINK-011
// spec: LINK-021
// spec: LINK-026
// spec: LINK-027
func TestImage_UsesSameRedirectDNSAndResponseGuards(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, location, encoding string
		status                   int
	}{
		{name: "redirect_private", location: "http://10.0.0.1/image", status: 302},
		{name: "compressed", encoding: "br", status: 200},
		{name: "rate_limit", status: 429},
		{name: "server_error", status: 503},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assert.Equal(t, http.MethodGet, r.Method)
					w.Header().Set("Location", tt.location)
					w.Header().Set("Content-Encoding", tt.encoding)
					w.Header().Set("Content-Type", "image/png")
					w.WriteHeader(tt.status)
				}),
			)
			_, err := loader.Image(t.Context(), "http://example.com/image")
			require.Error(t, err)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
	t.Run("quota_applies_to_image", func(t *testing.T) {
		t.Parallel()
		loader := newServerLoader(
			t,
			http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { t.Error("image bypassed quota") },
			),
		)
		loader.allowHTTP = func(string) bool { return false }
		_, err := loader.Image(t.Context(), "http://example.com/image")
		require.Error(t, err)
	})
}

// spec: LINK-011
func TestLoader_DNSFailuresStopBeforeConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		addresses []netip.Addr
		err       error
	}{
		{name: "empty"},
		{name: "error", err: errors.New("dns-secret-path")},
		{name: "mapped_private", addresses: []netip.Addr{netip.MustParseAddr("::ffff:172.17.0.1")}},
		{name: "special_ipv6", addresses: []netip.Addr{netip.MustParseAddr("2001:20::1")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loader := New(nil)
			loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) { return tt.addresses, tt.err }
			loader.dialContext = func(context.Context, string, string) (net.Conn, error) {
				t.Error("dial after invalid DNS")
				return nil, errors.New("unexpected dial")
			}
			_, err := loader.Page(t.Context(), "http://example.com/")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "dns-secret-path")
		})
	}
}

// spec: LINK-012
func TestLoader_BoundsDNSAndRequestWaits(t *testing.T) {
	t.Parallel()
	t.Run("dns_budget", func(t *testing.T) {
		t.Parallel()
		loader := New(nil)
		loader.lookupIP = func(ctx context.Context, _, _ string) ([]netip.Addr, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.LessOrEqual(t, time.Until(deadline), time.Second)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
		defer cancel()
		started := time.Now()
		_, err := loader.Page(ctx, "http://example.com/")
		require.Error(t, err)
		assert.Less(t, time.Since(started), time.Second)
	})
	t.Run("request_budget", func(t *testing.T) {
		t.Parallel()
		loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		}))
		started := time.Now()
		_, err := loader.Page(t.Context(), "http://example.com/")
		require.Error(t, err)
		assert.Less(t, time.Since(started), 4*time.Second)
	})
}

// spec: LINK-027
func TestLoader_ConnectionFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	loader := New(nil)
	loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("1.1.1.1"),
		}, nil
	}
	var dials atomic.Int32
	loader.dialContext = func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("network failure at /secret?token=secret")
	}
	_, err := loader.Page(t.Context(), "http://example.com/secret?token=secret")
	require.Error(t, err)
	assert.EqualValues(t, 1, dials.Load())
	assert.NotContains(t, err.Error(), "secret")
}

// spec: LINK-011
func TestLoader_PublicTargetsKeepHostAndRequestPath(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, target, host, path string }{
		{
			"standard_port",
			"http://EXAMPLE.com:80/path%2Fkept?token=query#fragment",
			testHostname,
			"/path%2Fkept?token=query",
		},
		{"not_local_suffix", "http://example.notlocalhost/", "example.notlocalhost", "/"},
		{"ipv4_literal", "http://93.184.216.34/", "93.184.216.34", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, tt.host, r.Host)
					assert.Equal(t, tt.path, r.RequestURI)
					w.Header().Set("Content-Type", "text/html")
					_, _ = io.WriteString(
						w,
						`<head><meta property="og:title" content="Public"></head>`,
					)
				}),
			)
			if tt.name == "ipv4_literal" {
				loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
					t.Error("IP literal must not query DNS")
					return nil, errors.New("unexpected DNS")
				}
			}
			got, err := loader.Page(t.Context(), tt.target)
			require.NoError(t, err)
			assert.Equal(t, "Public", got.Title)
		})
	}
}

// spec: LINK-011
func TestLoader_RejectsIANAReservedIPv6Space(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"2000::1", "2001:1000::1", "3000::1", "3ffe::1"} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			loader := New(nil)
			loader.dialContext = func(context.Context, string, string) (net.Conn, error) {
				t.Error("reserved IPv6 address dialed")
				return nil, errors.New("unexpected dial")
			}
			_, err := loader.Page(t.Context(), "http://["+address+"]/")
			require.Error(t, err)
		})
	}
}

// spec: LINK-011
func TestLoader_PublicIPv6DNSAnswersRemainUsable(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"2001:4860:4860::8888", "2410::1", "2606:4700:4700::1111", "2a02:6b8::1"} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/html")
					_, _ = io.WriteString(
						w,
						`<head><meta property="og:title" content="IPv6"></head>`,
					)
				}),
			)
			loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr(address)}, nil
			}
			dial := loader.dialContext
			loader.dialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
				assert.Equal(t, "["+address+"]:80", target)
				return dial(ctx, network, "93.184.216.34:80")
			}
			got, err := loader.Page(t.Context(), "http://example.com/")
			require.NoError(t, err)
			assert.Equal(t, "IPv6", got.Title)
		})
	}
}

// spec: LINK-011
func TestLoader_LocalNamesNeverReachDNS(t *testing.T) {
	t.Parallel()
	for _, hostname := range []string{"localhost", "test.localhost", "test.local", "test.internal", "home.arpa", "test.home.arpa", "jaeger"} {
		t.Run(hostname, func(t *testing.T) {
			t.Parallel()
			loader := New(nil)
			loader.lookupIP = func(context.Context, string, string) ([]netip.Addr, error) {
				t.Error("local name reached DNS")
				return nil, errors.New("unexpected DNS")
			}
			_, err := loader.Page(t.Context(), "http://"+hostname+"/")
			require.Error(t, err)
		})
	}
}

// spec: LINK-004
// spec: LINK-012
func TestPage_ShortReadyHeadDoesNotWaitForBody(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"text/html", "text/html; charset=utf-8"} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()
			loader := newServerLoader(
				t,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", contentType)
					_, _ = io.WriteString(
						w,
						`<head><meta property="og:title" content="Ready"></head>`,
					)
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
					<-r.Context().Done()
				}),
			)
			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
			defer cancel()
			got, err := loader.Page(ctx, "http://example.com/")
			require.NoError(t, err)
			assert.Equal(t, "Ready", got.Title)
		})
	}
}

// spec: LINK-012
func TestPage_KeepsShortPrefixMetadataOnReadFailure(t *testing.T) {
	t.Parallel()
	loader := newServerLoader(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<head><meta property="og:title" content="Ready">`)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Millisecond)
	defer cancel()
	got, err := loader.Page(ctx, "http://example.com/")
	require.Error(t, err)
	assert.Equal(t, "Ready", got.Title)
}
