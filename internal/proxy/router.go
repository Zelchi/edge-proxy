package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

type Router struct {
	mu     sync.Mutex
	routes atomic.Value // routingTable
}

type routingTable struct {
	routes          map[string]*httputil.ReverseProxy
	fallback        Fallback
	redirectToHTTPS bool
}

type Fallback struct {
	Host       string
	StatusCode int
}

func NewRouter() *Router {
	router := &Router{}
	router.routes.Store(routingTable{routes: make(map[string]*httputil.ReverseProxy)})
	return router
}

func (r *Router) Add(host string, proxy *httputil.ReverseProxy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	routes := r.copyRoutes()
	routes[host] = proxy
	r.routes.Store(routes)
}

func (r *Router) Clear() {
	r.Replace(nil)
}

// Replace atomically publishes a complete routing table. Requests already being
// proxied continue with their selected route, while new requests use this table.
func (r *Router) Replace(routes map[string]*httputil.ReverseProxy) {
	r.ReplaceWithOptions(routes, Fallback{}, false)
}

// ReplaceWithFallback atomically publishes a complete routing table and the
// fallback used for unknown hosts.
func (r *Router) ReplaceWithFallback(routes map[string]*httputil.ReverseProxy, fallback Fallback) {
	r.ReplaceWithOptions(routes, fallback, false)
}

// ReplaceWithOptions atomically publishes all request-routing behavior.
func (r *Router) ReplaceWithOptions(routes map[string]*httputil.ReverseProxy, fallback Fallback, redirectToHTTPS bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	next := make(map[string]*httputil.ReverseProxy, len(routes))
	for host, proxy := range routes {
		next[normalizeHost(host)] = proxy
	}
	fallback.Host = normalizeHost(fallback.Host)
	r.routes.Store(routingTable{
		routes:          next,
		fallback:        fallback,
		redirectToHTTPS: redirectToHTTPS,
	})
}

func (r *Router) copyRoutes() map[string]*httputil.ReverseProxy {
	current := r.routes.Load().(routingTable).routes
	next := make(map[string]*httputil.ReverseProxy, len(current)+1)
	for host, proxy := range current {
		next[host] = proxy
	}
	return next
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	table := r.routes.Load().(routingTable)
	if p, ok := table.routes[normalizeHost(req.Host)]; ok {
		p.ServeHTTP(w, req)
		return
	}

	serveUnknownHost(w, req, table.fallback)
}

func (r *Router) HasRoute(host string) bool {
	table := r.routes.Load().(routingTable)
	_, ok := table.routes[normalizeHost(host)]
	return ok
}

// ServeHTTPRedirect handles a plaintext HTTP request using the same atomic
// routing snapshot as HTTPS requests.
func (r *Router) ServeHTTPRedirect(w http.ResponseWriter, req *http.Request) {
	table := r.routes.Load().(routingTable)
	if _, ok := table.routes[normalizeHost(req.Host)]; !ok {
		serveUnknownHost(w, req, table.fallback)
		return
	}
	if !table.redirectToHTTPS {
		http.NotFound(w, req)
		return
	}
	http.Redirect(w, req, "https://"+req.Host+req.URL.RequestURI(), http.StatusMovedPermanently)
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.Trim(host, "[]")
	host = strings.ToLower(host)
	return strings.TrimRight(host, ".")
}

func isACMEChallenge(path string) bool {
	return strings.HasPrefix(path, "/.well-known/acme-challenge/")
}

func serveUnknownHost(w http.ResponseWriter, req *http.Request, fallback Fallback) {
	if fallback.Host != "" && normalizeHost(req.Host) != normalizeHost(fallback.Host) && !isACMEChallenge(req.URL.Path) {
		redirectToFallback(w, req, fallback)
		return
	}
	http.NotFound(w, req)
}

func redirectToFallback(w http.ResponseWriter, req *http.Request, fallback Fallback) {
	status := fallback.StatusCode
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		status = http.StatusPermanentRedirect
	}

	target := url.URL{
		Scheme:   "https",
		Host:     fallback.Host,
		Path:     req.URL.Path,
		RawPath:  req.URL.RawPath,
		RawQuery: req.URL.RawQuery,
	}
	http.Redirect(w, req, target.String(), status)
}
