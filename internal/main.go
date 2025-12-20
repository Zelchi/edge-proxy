package main

import (
	"log"
	"net/http"

	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy"
	"edge-proxy/internal/server"
	"edge-proxy/internal/tls"
)

func main() {

	// Load configs
	cfg, err := config.Load("config.yml")
	if err != nil {
		log.Fatal(err)
	}

	// ACME / TLS manager
	tlsManager := tls.NewManager(
		cfg.TLS.CertsDir,
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
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.HTTP.RedirectToHTTPS {
			http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// HTTPS
	srv := server.New(cfg.HTTPS.Address, handler)
	srv.TLSConfig = tlsManager.TLSConfig()

	log.Fatal(srv.ListenAndServeTLS("", ""))

	// Start servers
	go func() {
		log.Fatal(
			http.ListenAndServe(
				cfg.HTTP.Address,
				tlsManager.HTTPHandler(httpHandler),
			),
		)
	}()
}
