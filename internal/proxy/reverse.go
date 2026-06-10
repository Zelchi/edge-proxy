package proxy

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func NewReverseProxy(target string) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil {
		panic(err)
	}

	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			originalHost := r.Host

			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host

			r.Host = originalHost
			r.Header.Set("X-Forwarded-Host", originalHost)

			if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
					clientIP = prior + ", " + clientIP
				}
				r.Header.Set("X-Forwarded-For", clientIP)
			}

			if r.TLS != nil {
				r.Header.Set("X-Forwarded-Proto", "https")
			} else {
				r.Header.Set("X-Forwarded-Proto", "http")
			}
		},

		FlushInterval: 10 * time.Millisecond,
		Transport:     DefaultTransport(),

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[PROXY ERROR] Backend %s falhou: %v", target, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
}
