package tls

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/acme/autocert"
)

type Manager struct {
	autocert            *autocert.Manager
	fallbackCertificate *fallbackCertificate
}

type fallbackCertificate struct {
	certFile string
	keyFile  string

	mu          sync.Mutex
	certificate *tls.Certificate
	certState   fileState
	keyState    fileState
}

type fileState struct {
	modTime int64
	size    int64
}

func NewManager(certs string, domains []string, fallbackCertsDir string) (*Manager, error) {
	manager := &Manager{
		autocert: &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domains...),
			Cache:      autocert.DirCache(certs),
		},
	}
	if fallbackCertsDir != "" {
		fallback, err := newFallbackCertificate(
			filepath.Join(fallbackCertsDir, "fullchain.pem"),
			filepath.Join(fallbackCertsDir, "privkey.pem"),
		)
		if err != nil {
			return nil, fmt.Errorf("load fallback certificate: %w", err)
		}
		manager.fallbackCertificate = fallback
	}
	return manager, nil
}

func (m *Manager) TLSConfig() *tls.Config {
	config := m.autocert.TLSConfig()
	config.MinVersion = tls.VersionTLS12
	if m.fallbackCertificate != nil {
		autocertGetCertificate := config.GetCertificate
		config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			certificate, err := autocertGetCertificate(hello)
			if err == nil && certificate != nil {
				return certificate, nil
			}
			return m.fallbackCertificate.load(), nil
		}
	}
	return config
}

func newFallbackCertificate(certFile, keyFile string) (*fallbackCertificate, error) {
	fallback := &fallbackCertificate{certFile: certFile, keyFile: keyFile}
	if err := fallback.reload(); err != nil {
		return nil, err
	}
	return fallback, nil
}

// load returns the latest valid certificate. If a certificate is being replaced
// and its files are momentarily inconsistent, the previous certificate remains
// available until the new pair can be loaded successfully.
func (f *fallbackCertificate) load() *tls.Certificate {
	certState, keyState, err := certificateStates(f.certFile, f.keyFile)
	if err != nil {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.certificate
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if certState == f.certState && keyState == f.keyState {
		return f.certificate
	}
	if err := f.reload(); err != nil {
		return f.certificate
	}
	return f.certificate
}

func (f *fallbackCertificate) reload() error {
	beforeCertState, beforeKeyState, err := certificateStates(f.certFile, f.keyFile)
	if err != nil {
		return err
	}
	certificate, err := tls.LoadX509KeyPair(f.certFile, f.keyFile)
	if err != nil {
		return err
	}
	certState, keyState, err := certificateStates(f.certFile, f.keyFile)
	if err != nil {
		return err
	}
	if beforeCertState != certState || beforeKeyState != keyState {
		return fmt.Errorf("fallback certificate files changed while loading")
	}
	f.certificate = &certificate
	f.certState = certState
	f.keyState = keyState
	return nil
}

func certificateStates(certFile, keyFile string) (fileState, fileState, error) {
	certInfo, err := os.Stat(certFile)
	if err != nil {
		return fileState{}, fileState{}, err
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		return fileState{}, fileState{}, err
	}
	return fileState{modTime: certInfo.ModTime().UnixNano(), size: certInfo.Size()}, fileState{modTime: keyInfo.ModTime().UnixNano(), size: keyInfo.Size()}, nil
}

func (m *Manager) HTTPHandler(next http.Handler) http.Handler {
	return m.autocert.HTTPHandler(next)
}
