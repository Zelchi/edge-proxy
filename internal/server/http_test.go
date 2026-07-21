package server

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func TestNewConfiguresInputTimeouts(t *testing.T) {
	srv := New(":443", http.NotFoundHandler())

	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Fatalf("ReadTimeout = %s, want 30s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0 for streaming responses", srv.WriteTimeout)
	}
}

func TestNewHTTP3ConfiguresQUICAndAltSvc(t *testing.T) {
	http3Server := NewHTTP3(":443", http.NotFoundHandler(), &tls.Config{})

	if http3Server.TLSConfig == nil {
		t.Fatal("TLSConfig is nil")
	}
	if len(http3Server.TLSConfig.NextProtos) != 1 || http3Server.TLSConfig.NextProtos[0] != http3.NextProtoH3 {
		t.Fatalf("NextProtos = %q, want %q", http3Server.TLSConfig.NextProtos, http3.NextProtoH3)
	}
	altSvc, err := HTTP3AltSvc(":443")
	if err != nil {
		t.Fatal(err)
	}
	if altSvc != `h3=":443"; ma=2592000` {
		t.Fatalf("Alt-Svc = %q", altSvc)
	}
}
