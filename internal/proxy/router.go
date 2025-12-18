package proxy

import (
	"net/http"
	"net/http/httputil"
)

type Router struct {
	routes map[string]*httputil.ReverseProxy
}

func NewRouter() *Router {
	return &Router{routes: make(map[string]*httputil.ReverseProxy)}
}

func (r *Router) Add(host string, proxy *httputil.ReverseProxy) {
	r.routes[host] = proxy
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if p, ok := r.routes[req.Host]; ok {
		p.ServeHTTP(w, req)
		return
	}
	http.NotFound(w, req)
}
