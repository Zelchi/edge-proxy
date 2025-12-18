package proxy

import (
	"net/http"
	"time"
)

func DefaultTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        200,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}
}
