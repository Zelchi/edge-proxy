package server

import (
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

func New(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		// Keep this unset so long-lived streamed responses are not cut off.
		WriteTimeout:   0,
		IdleTimeout:    idleTimeout,
		MaxHeaderBytes: 1 << 20,
	}
}
