package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"github.com/quic-go/quic-go/http3"
)

const DefaultHTTP3Address = ":443"

func NewHTTP3(addr string, handler http.Handler, tlsConfig *tls.Config) *http3.Server {
	return &http3.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
	}
}

func HTTP3AltSvc(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("extract HTTP/3 port from %q: %w", addr, err)
	}
	return fmt.Sprintf(`h3=":%s"; ma=2592000`, port), nil
}
