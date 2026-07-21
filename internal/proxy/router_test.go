package proxy

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestRouterReplaceDoesNotBlockInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	slowProxy := testProxy(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(http.NoBody),
		}, nil
	}))

	router := NewRouter()
	router.Replace(map[string]*httputil.ReverseProxy{"old.example": slowProxy})

	requestDone := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, "http://old.example/", nil)
		req.Host = "old.example"
		router.ServeHTTP(httptest.NewRecorder(), req)
		close(requestDone)
	}()

	<-started
	replaced := make(chan struct{})
	go func() {
		router.Replace(map[string]*httputil.ReverseProxy{})
		close(replaced)
	}()

	select {
	case <-replaced:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("replacing routes waited for an in-flight proxy request")
	}

	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("in-flight proxy request did not complete")
	}
}

func TestRouterRedirectsUnknownHostToFallback(t *testing.T) {
	router := NewRouter()
	router.ReplaceWithFallback(nil, Fallback{Host: "fallback.example"})

	req := httptest.NewRequest(http.MethodGet, "https://unknown.example/docs?lang=pt", nil)
	req.Host = "unknown.example"
	response := httptest.NewRecorder()

	router.ServeHTTP(response, req)

	if response.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusPermanentRedirect)
	}
	if location := response.Header().Get("Location"); location != "https://fallback.example/docs?lang=pt" {
		t.Fatalf("Location = %q, want fallback URL with path and query", location)
	}
}

func TestRouterDoesNotRedirectACMEChallengeOrFallbackHost(t *testing.T) {
	router := NewRouter()
	router.ReplaceWithFallback(nil, Fallback{Host: "fallback.example"})

	for _, requestURL := range []string{
		"https://unknown.example/.well-known/acme-challenge/token",
		"https://fallback.example/",
	} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, requestURL, nil)
		router.ServeHTTP(response, req)

		if response.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want %d", requestURL, response.Code, http.StatusNotFound)
		}
	}
}

func TestRouterHTTPRedirectUsesRoutingSnapshot(t *testing.T) {
	router := NewRouter()
	router.ReplaceWithOptions(
		map[string]*httputil.ReverseProxy{"app.example": {}},
		Fallback{Host: "fallback.example"},
		true,
	)

	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://app.example/docs?lang=pt", nil)
	req.Host = "app.example"
	router.ServeHTTPRedirect(response, req)

	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMovedPermanently)
	}
	if location := response.Header().Get("Location"); location != "https://app.example/docs?lang=pt" {
		t.Fatalf("Location = %q, want HTTPS redirect", location)
	}
}

func TestRouterNormalizesHostsForLookup(t *testing.T) {
	router := NewRouter()
	router.Replace(map[string]*httputil.ReverseProxy{"APP.EXAMPLE.": {}})

	if !router.HasRoute("app.example:443") {
		t.Fatal("route with port and normalized casing was not found")
	}
}

func TestNewReverseProxyRejectsInvalidTarget(t *testing.T) {
	for _, target := range []string{
		"://invalid",
		"ftp://upstream.example",
		"http:///missing-host",
	} {
		if _, err := NewReverseProxy(target, false); err == nil {
			t.Errorf("NewReverseProxy(%q) returned no error", target)
		}
	}
}

func TestReverseProxyRebuildsForwardedHeaders(t *testing.T) {
	proxy, err := NewReverseProxy("http://upstream.example:8080", false)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://public.example/path", nil)
	req.Host = "public.example"
	req.RemoteAddr = "203.0.113.10:4567"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-For", "spoofed")
	req.Header.Set("X-Real-IP", "spoofed")
	req.Header.Set("X-Forwarded-Proto", "http")

	proxy.Director(req)

	if req.Header.Get("X-Forwarded-For") != "203.0.113.10" {
		t.Fatalf("X-Forwarded-For = %q", req.Header.Get("X-Forwarded-For"))
	}
	if req.Header.Get("X-Real-IP") != "203.0.113.10" {
		t.Fatalf("X-Real-IP = %q", req.Header.Get("X-Real-IP"))
	}
	if req.Header.Get("X-Forwarded-Host") != "public.example" {
		t.Fatalf("X-Forwarded-Host = %q", req.Header.Get("X-Forwarded-Host"))
	}
	if req.Header.Get("X-Forwarded-Proto") != "https" {
		t.Fatalf("X-Forwarded-Proto = %q", req.Header.Get("X-Forwarded-Proto"))
	}
	if req.Host != "upstream.example:8080" {
		t.Fatalf("Host = %q", req.Host)
	}
}

func TestReverseProxyCanPreserveOriginalHost(t *testing.T) {
	proxy, err := NewReverseProxy("http://upstream.example:8080", true)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://public.example/path", nil)
	req.Host = "public.example"

	proxy.Director(req)

	if req.Host != "public.example" {
		t.Fatalf("Host = %q, want original public host", req.Host)
	}
}

func TestReverseProxyPrefixesUpstreamBasePath(t *testing.T) {
	proxy, err := NewReverseProxy("http://upstream.example/api", false)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://public.example/v1/items?active=true", nil)

	proxy.Director(req)

	if req.URL.Path != "/api/v1/items" {
		t.Fatalf("path = %q, want /api/v1/items", req.URL.Path)
	}
	if req.URL.RawQuery != "active=true" {
		t.Fatalf("query = %q, want client query", req.URL.RawQuery)
	}
}

func testProxy(transport http.RoundTripper) *httputil.ReverseProxy {
	target, err := url.Parse("http://upstream.example")
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	return proxy
}
