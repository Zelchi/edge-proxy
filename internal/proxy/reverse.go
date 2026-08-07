package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type upstreamContextKey struct{}

func UpstreamFromRequest(r *http.Request) string {
	upstream, _ := r.Context().Value(upstreamContextKey{}).(string)
	return upstream
}

func NewReverseProxy(target string, preserveHost bool) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse upstream %q: %w", target, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("upstream %q must use http or https", target)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("upstream %q requires a host", target)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("upstream %q must not contain credentials, a query, or a fragment", target)
	}

	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			*r = *r.WithContext(context.WithValue(r.Context(), upstreamContextKey{}, u.Host))
			originalHost := r.Host
			originalScheme := "http"
			if r.TLS != nil {
				originalScheme = "https"
			}

			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.URL.Path, r.URL.RawPath = joinURLPath(u, r.URL)
			if !preserveHost {
				r.Host = u.Host
			}

			if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				r.Header.Set("X-Forwarded-For", clientIP)
				r.Header.Set("X-Real-IP", clientIP)
			} else {
				r.Header.Del("X-Forwarded-For")
				r.Header.Del("X-Real-IP")
			}

			r.Header.Set("X-Forwarded-Host", originalHost)
			r.Header.Set("X-Forwarded-Proto", originalScheme)
		},

		FlushInterval: 10 * time.Millisecond,
		Transport:     DefaultTransport(),

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[PROXY ERROR] Backend %s falhou: %v", target, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}, nil

}

func joinURLPath(base, request *url.URL) (path, rawPath string) {
	if base.RawPath == "" && request.RawPath == "" {
		return singleJoiningSlash(base.Path, request.Path), ""
	}

	basePath := base.EscapedPath()
	requestPath := request.EscapedPath()
	path = singleJoiningSlash(base.Path, request.Path)
	rawPath = singleJoiningSlash(basePath, requestPath)
	return path, rawPath
}

func singleJoiningSlash(first, second string) string {
	firstSlash := strings.HasSuffix(first, "/")
	secondSlash := strings.HasPrefix(second, "/")
	switch {
	case firstSlash && secondSlash:
		return first + second[1:]
	case !firstSlash && !secondSlash:
		return first + "/" + second
	default:
		return first + second
	}
}
