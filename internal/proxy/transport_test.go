package proxy

import (
	"testing"
	"time"
)

func TestDefaultTransportTimeouts(t *testing.T) {
	transport := DefaultTransport()

	if transport.DialContext == nil {
		t.Fatal("DialContext is nil")
	}
	if transport.TLSHandshakeTimeout != 5*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %s, want 5s", transport.TLSHandshakeTimeout)
	}
	if transport.ResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %s, want 30s", transport.ResponseHeaderTimeout)
	}
	if transport.ExpectContinueTimeout != time.Second {
		t.Fatalf("ExpectContinueTimeout = %s, want 1s", transport.ExpectContinueTimeout)
	}
	if transport.MaxIdleConnsPerHost != 50 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 50", transport.MaxIdleConnsPerHost)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = false")
	}
}
