package tls

import (
	"crypto/tls"
	"net/http"

	"golang.org/x/crypto/acme/autocert"
)

type Manager struct {
	autocert *autocert.Manager
}

func NewManager(certs string, domains []string) *Manager {
	return &Manager{
		autocert: &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domains...),
			Cache:      autocert.DirCache(certs),
		},
	}
}

func (m *Manager) TLSConfig() *tls.Config {
	config := m.autocert.TLSConfig()
	config.MinVersion = tls.VersionTLS12
	return config
}

func (m *Manager) HTTPHandler(next http.Handler) http.Handler {
	return m.autocert.HTTPHandler(next)
}
