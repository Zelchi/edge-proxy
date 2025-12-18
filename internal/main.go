package main

import (
	"log"
	"net/http"

	"edge-proxy/server/internal/config"
	"edge-proxy/server/internal/proxy"
	"edge-proxy/server/internal/server"
	edgetls "edge-proxy/server/internal/tls"
)

func main() {

	cfg, err := config.Load("config.yml")
	if err != nil {
		log.Fatal(err)
	}

	// ACME / TLS manager
	tlsManager := edgetls.NewManager(
		cfg.TLS.CacheDir,
		cfg.TLS.Domains,
	)

	// Router
	router := proxy.NewRouter()
	for _, r := range cfg.Routes {
		router.Add(r.Host, proxy.NewReverseProxy(r.Upstream))
	}

	// Rate limit
	limiter := proxy.NewLimiter(10, 20)
	handler := limiter.Middleware(router)

	// HTTP -> HTTPS redirect
	go func() {
		log.Fatal(
			http.ListenAndServe(
				cfg.HTTP.Address,
				tlsManager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
				})),
			),
		)
	}()

	// HTTPS
	srv := server.New(cfg.HTTPS.Address, handler)
	srv.TLSConfig = tlsManager.TLSConfig()

	log.Fatal(srv.ListenAndServeTLS("", ""))
}
