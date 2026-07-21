package tls

import (
	cryptotls "crypto/tls"
	"testing"
)

func TestTLSConfigPreservesAutocertProtocols(t *testing.T) {
	manager := NewManager(t.TempDir(), []string{"app.example"})
	config := manager.TLSConfig()

	if config.MinVersion != cryptotls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", config.MinVersion)
	}
	if config.GetCertificate == nil {
		t.Fatal("GetCertificate is nil")
	}
	if !containsProtocol(config.NextProtos, "h2") || !containsProtocol(config.NextProtos, "http/1.1") {
		t.Fatalf("NextProtos = %q, want HTTP/2 and HTTP/1.1", config.NextProtos)
	}
}

func containsProtocol(protocols []string, wanted string) bool {
	for _, protocol := range protocols {
		if protocol == wanted {
			return true
		}
	}
	return false
}
