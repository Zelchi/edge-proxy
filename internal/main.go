package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy"
	"edge-proxy/internal/server"
	"edge-proxy/internal/tls"

	"golang.org/x/time/rate"
)

func main() {
	log.Println("[BOOT] Carregando configuração inicial...")
	cfg, err := config.Load("config.yml")
	if err != nil {
		log.Fatal("[FATAL] Falha ao carregar config.yml:", err)
	}
	configSnapshot := config.NewSnapshot(cfg)

	tlsManager, err := tls.NewManager(
		cfg.TLS.CertsDir,
		cfg.TLS.Domains,
		cfg.TLS.CertsFallback,
	)
	if err != nil {
		log.Fatal("[FATAL] Falha ao inicializar TLS:", err)
	}
	log.Println("[BOOT] TLS Manager inicializado.")

	router := proxy.NewRouter()
	loadRoutes := func(cfg *config.Config) error {
		routes := make(map[string]*httputil.ReverseProxy, len(cfg.Routes))
		for _, r := range cfg.Routes {
			log.Printf("[ROUTES] Adicionando rota: host=%s -> upstream=%s", r.Host, r.Upstream)
			reverseProxy, err := proxy.NewReverseProxy(r.Upstream, r.PreserveHost)
			if err != nil {
				return fmt.Errorf("criar rota para %s: %w", r.Host, err)
			}
			routes[r.Host] = reverseProxy
		}
		router.ReplaceWithOptions(routes, proxy.Fallback{
			Host:       cfg.Fallback.Host,
			StatusCode: cfg.Fallback.StatusCode,
		}, cfg.HTTP.RedirectToHTTPS)
		log.Println("[ROUTES] Rotas recarregadas.")
		return nil
	}
	if err := loadRoutes(cfg); err != nil {
		log.Fatal("[FATAL] Falha ao configurar rotas:", err)
	}

	limiter := proxy.NewLimiter(rate.Limit(cfg.RateLimit.RequestsPerSecond), cfg.RateLimit.Burst)
	metrics := &proxy.Metrics{}
	applicationHandler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		router.ServeHTTP(w, r)
	}))
	dashboardHandler := server.WithDashboard(cfg.Dashboard.Host, metrics, applicationHandler)
	handler := proxy.WithMetrics(metrics, proxy.WithAccessLog(slog.Default(), server.WithHealth(dashboardHandler)))

	httpsHandler := handler
	http3Server := server.NewHTTP3(server.DefaultHTTP3Address, handler, tlsManager.TLSConfig())
	altSvc, err := server.HTTP3AltSvc(server.DefaultHTTP3Address)
	if err != nil {
		log.Fatal("[FATAL] Falha ao configurar HTTP/3:", err)
	}
	httpsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Alt-Svc", altSvc)
		handler.ServeHTTP(w, r)
	})
	go func() {
		log.Printf("[HTTP/3] Servidor HTTP/3 ouvindo em UDP %s", server.DefaultHTTP3Address)
		if err := http3Server.ListenAndServe(); err != nil {
			log.Printf("[HTTP/3] Servidor encerrado: %v", err)
		}
	}()

	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == server.HealthPath {
			handler.ServeHTTP(w, r)
			return
		}
		if server.IsDashboardHost(cfg.Dashboard.Host, r.Host) {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		router.ServeHTTPRedirect(w, r)
	})

	go func() {
		if err := config.WatchConfig(context.Background(), "config.yml", func() {
			log.Println("[WATCHER] Mudança detectada em config.yml, recarregando...")
			newCfg, err := config.Load("config.yml")
			if err != nil {
				log.Println("[WATCHER] Falha ao recarregar config.yml:", err)
				return
			}
			if err := config.ValidateReload(configSnapshot.Load(), newCfg); err != nil {
				log.Println("[WATCHER] Configuração exige reinício:", err)
				return
			}
			if err := loadRoutes(newCfg); err != nil {
				log.Println("[WATCHER] Falha ao aplicar rotas:", err)
				return
			}
			configSnapshot.Store(newCfg)
		}); err != nil {
			log.Println("[WATCHER] Encerrado:", err)
		}
	}()

	go func() {
		log.Printf("[HTTP] Servidor HTTP ouvindo em %s", cfg.HTTP.Address)
		log.Fatal(
			http.ListenAndServe(
				cfg.HTTP.Address,
				tlsManager.HTTPHandler(httpHandler),
			),
		)
	}()

	if cfg.HTTPS.Address != "" {
		log.Printf("[HTTPS] Servidor HTTPS ouvindo em %s", cfg.HTTPS.Address)
		srv := server.New(cfg.HTTPS.Address, httpsHandler)
		srv.TLSConfig = tlsManager.TLSConfig()
		log.Fatal(srv.ListenAndServeTLS("", ""))
	} else {
		log.Println("[HTTPS] Configuração HTTPS ausente ou endereço vazio. HTTPS não será iniciado.")
		select {}
	}
}
