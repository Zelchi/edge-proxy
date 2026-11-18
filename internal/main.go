package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"edge-proxy/internal/config"
	"edge-proxy/internal/protocol/hostroute"
	"edge-proxy/internal/proxy"
	"edge-proxy/internal/server"
	"edge-proxy/internal/stream"
	"edge-proxy/internal/tls"
	"edge-proxy/internal/turnproxy"

	"golang.org/x/time/rate"
)

type streamServer interface {
	Serve(context.Context) error
	Close() error
}

func main() {
	log.Println("[BOOT] Carregando configuração inicial...")
	cfg, err := config.Load("config.yml")
	if err != nil {
		log.Fatal("[FATAL] Falha ao carregar config.yml:", err)
	}
	configSnapshot := config.NewSnapshot(cfg)

	tlsManager, err := tls.NewManager(
		cfg.HTTPS.TLS.CertsDir,
		cfg.HTTPS.TLS.Domains,
		cfg.HTTPS.TLS.CertsFallback,
	)
	if err != nil {
		log.Fatal("[FATAL] Falha ao inicializar TLS:", err)
	}
	log.Println("[BOOT] TLS Manager inicializado.")

	router := proxy.NewRouter()
	loadRoutes := func(current *config.Config) error {
		routes := make(map[string]*httputil.ReverseProxy, len(current.HTTP.Routes))
		for _, route := range current.HTTP.Routes {
			log.Printf("[ROUTES] Adicionando rota: host=%s -> upstream=%s", route.Host, route.Upstream)
			reverseProxy, err := proxy.NewReverseProxy(route.Upstream, route.PreserveHost)
			if err != nil {
				return fmt.Errorf("criar rota para %s: %w", route.Host, err)
			}
			routes[route.Host] = reverseProxy
		}
		router.ReplaceWithOptions(routes, proxy.Fallback{
			Host: current.HTTP.Fallback,
		}, current.HTTP.RedirectToHTTPS)
		log.Println("[ROUTES] Rotas HTTP recarregadas.")
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
	dashboardHandler := server.WithDashboard(cfg.HTTP.Dashboard, metrics, applicationHandler)
	handler := proxy.WithMetrics(metrics, proxy.WithAccessLog(slog.Default(), server.WithHealth(dashboardHandler)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	streamListeners, err := startStreamListeners(cfg, metrics.StreamMetrics(), slog.Default())
	if err != nil {
		log.Fatal("[FATAL] Falha ao iniciar listeners TCP/UDP:", err)
	}
	streamServers := startStreamServers(ctx, stop, streamListeners, slog.Default())

	// HTTP/3 keeps its existing UDP :443 endpoint. TCP/UDP stream listeners are
	// independent and are created from the multiprotocol configuration above.
	httpsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		altSvc, err := server.HTTP3AltSvc(server.DefaultHTTP3Address)
		if err == nil {
			w.Header().Add("Alt-Svc", altSvc)
		}
		handler.ServeHTTP(w, r)
	})
	http3Server := server.NewHTTP3(server.DefaultHTTP3Address, handler, tlsManager.TLSConfig())

	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == server.HealthPath {
			handler.ServeHTTP(w, r)
			return
		}
		current := configSnapshot.Load()
		if server.IsDashboardHost(current.HTTP.Dashboard, r.Host) {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		router.ServeHTTPRedirectWithPolicy(w, r, current.HTTP.RedirectToHTTPS)
	})

	httpServer := server.New(cfg.HTTP.Address, tlsManager.HTTPHandler(httpHandler))
	var httpsServer *http.Server
	serverErrors := make(chan error, 3)
	var servers sync.WaitGroup
	servers.Add(1)
	go func() {
		defer servers.Done()
		log.Printf("[HTTP] Servidor HTTP ouvindo em %s", cfg.HTTP.Address)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("HTTP: %w", err)
		}
	}()
	if cfg.HTTPS.Address != "" {
		httpsServer = server.New(cfg.HTTPS.Address, httpsHandler)
		httpsServer.TLSConfig = tlsManager.TLSConfig()
		servers.Add(1)
		go func() {
			defer servers.Done()
			log.Printf("[HTTPS] Servidor HTTPS ouvindo em %s", cfg.HTTPS.Address)
			if err := httpsServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- fmt.Errorf("HTTPS: %w", err)
			}
		}()
	} else {
		log.Println("[HTTPS] Configuração HTTPS ausente ou endereço vazio. HTTPS não será iniciado.")
	}
	servers.Add(1)
	go func() {
		defer servers.Done()
		log.Printf("[HTTP/3] Servidor HTTP/3 ouvindo em UDP %s", server.DefaultHTTP3Address)
		if err := http3Server.ListenAndServe(); err != nil {
			if ctx.Err() == nil {
				serverErrors <- fmt.Errorf("HTTP/3: %w", err)
			}
		}
	}()

	go func() {
		if err := config.WatchConfig(ctx, "config.yml", func() {
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
		}); err != nil && ctx.Err() == nil {
			log.Println("[WATCHER] Encerrado:", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("[SHUTDOWN] Sinal de encerramento recebido.")
	case err := <-serverErrors:
		log.Println("[FATAL] Servidor encerrado inesperadamente:", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, listener := range streamListeners {
		if err := listener.Close(); err != nil {
			log.Printf("[SHUTDOWN] Falha ao fechar listener: %v", err)
		}
	}
	_ = httpServer.Shutdown(shutdownCtx)
	if httpsServer != nil {
		_ = httpsServer.Shutdown(shutdownCtx)
	}
	_ = http3Server.Shutdown(shutdownCtx)
	streamServers.Wait()
	servers.Wait()
}

func startStreamServers(ctx context.Context, stop context.CancelFunc, listeners []streamServer, logger *slog.Logger) *sync.WaitGroup {
	var servers sync.WaitGroup
	servers.Add(len(listeners))
	for _, listener := range listeners {
		go func(listener streamServer) {
			defer servers.Done()
			if err := listener.Serve(ctx); err != nil && ctx.Err() == nil {
				logger.Error("stream listener stopped", "error", err)
				stop()
			}
		}(listener)
	}
	return &servers
}

func startStreamListeners(cfg *config.Config, metrics *proxy.StreamMetrics, logger *slog.Logger) ([]streamServer, error) {
	listeners := make([]streamServer, 0, 2)
	closeListeners := func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	if cfg.TCP.Address != "" {
		router, upstreams, err := buildTCPRouter(cfg.TCP)
		if err != nil {
			closeListeners()
			return nil, fmt.Errorf("configurar TCP: %w", err)
		}
		listenerMetrics := metrics.Register("tcp", "", cfg.TCP.Address, upstreams)
		listener, err := server.NewTCPListener(cfg.TCP.Address, router, cfg.TCP.MaxConnections, listenerMetrics, logger)
		if err != nil {
			closeListeners()
			return nil, fmt.Errorf("abrir TCP: %w", err)
		}
		listeners = append(listeners, listener)
		log.Printf("[TCP] Servidor TCP ouvindo em %s", cfg.TCP.Address)
	}
	if cfg.UDP.Address != "" {
		upstream, upstreams, err := buildUDPUpstream(cfg.UDP)
		if err != nil {
			closeListeners()
			return nil, fmt.Errorf("configurar UDP: %w", err)
		}
		listenerMetrics := metrics.Register("udp", "", cfg.UDP.Address, upstreams)
		listener, err := server.NewUDPListener(cfg.UDP.Address, upstream, cfg.UDP.MaxSessions, listenerMetrics, logger)
		if err != nil {
			closeListeners()
			return nil, fmt.Errorf("abrir UDP: %w", err)
		}
		listeners = append(listeners, listener)
		log.Printf("[UDP] Servidor UDP ouvindo em %s", cfg.UDP.Address)
	}
	turnListeners, err := startTURNProxyListeners(cfg, metrics, logger)
	if err != nil {
		closeListeners()
		return nil, err
	}
	listeners = append(listeners, turnListeners...)
	return listeners, nil
}

func startTURNProxyListeners(cfg *config.Config, metrics *proxy.StreamMetrics, logger *slog.Logger) ([]streamServer, error) {
	if cfg.TURN.PublicIP == "" {
		return nil, nil
	}
	key := []byte(os.Getenv("LUNA_TURN_PROXY_SECRET"))
	if len(key) < 32 {
		return nil, errors.New("LUNA_TURN_PROXY_SECRET must contain at least 32 characters when turn proxying is enabled")
	}
	publicIP := net.ParseIP(cfg.TURN.PublicIP).To4()
	if publicIP == nil {
		return nil, errors.New("turn.public_ip must be an IPv4 address")
	}
	_, portText, err := net.SplitHostPort(cfg.TURN.TCPAddress)
	if err != nil {
		return nil, fmt.Errorf("parse turn.tcp_address: %w", err)
	}
	controlPort, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("parse turn control port: %w", err)
	}

	listeners := make([]streamServer, 0, cfg.TURN.RelayMaxPort-cfg.TURN.RelayMinPort+3)
	closeListeners := func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	tcpRouter := turnproxy.TCPRouter{
		Upstream:    cfg.TURN.ControlUpstream,
		PublicIP:    publicIP,
		PublicPort:  controlPort,
		ProxySecret: key,
	}
	tcpMetrics := metrics.Register("turn-tcp", "", cfg.TURN.TCPAddress, []string{cfg.TURN.ControlUpstream})
	tcpListener, err := server.NewTCPListener(cfg.TURN.TCPAddress, tcpRouter, cfg.TURN.MaxSessions, tcpMetrics, logger)
	if err != nil {
		return nil, fmt.Errorf("open TURN TCP proxy listener: %w", err)
	}
	listeners = append(listeners, tcpListener)

	controlTransform := func(data []byte, client, _ *net.UDPAddr) ([]byte, error) {
		return turnproxy.EncodeDatagram(key, turnproxy.Frame{
			Type:        turnproxy.ControlUDP,
			Source:      client,
			Destination: &net.UDPAddr{IP: publicIP, Port: controlPort},
			Payload:     data,
		})
	}
	controlMetrics := metrics.Register("turn-udp", "", cfg.TURN.UDPAddress, []string{cfg.TURN.ControlUpstream})
	controlListener, err := server.NewUDPListenerWithTransform(
		cfg.TURN.UDPAddress,
		cfg.TURN.ControlUpstream,
		cfg.TURN.MaxSessions,
		15*time.Minute,
		controlTransform,
		controlMetrics,
		logger,
	)
	if err != nil {
		closeListeners()
		return nil, fmt.Errorf("open TURN UDP proxy listener: %w", err)
	}
	listeners = append(listeners, controlListener)

	for port := cfg.TURN.RelayMinPort; port <= cfg.TURN.RelayMaxPort; port++ {
		upstreamPort := cfg.TURN.RelayUpstreamBasePort + port - cfg.TURN.RelayMinPort
		upstream := net.JoinHostPort(cfg.TURN.RelayUpstreamHost, strconv.Itoa(upstreamPort))
		address := net.JoinHostPort("", strconv.Itoa(port))
		listener, err := server.NewTURNRelayUDPListener(address, upstream, key, publicIP, port, logger)
		if err != nil {
			closeListeners()
			return nil, fmt.Errorf("open TURN relay UDP listener on %d: %w", port, err)
		}
		listeners = append(listeners, listener)
	}
	log.Printf("[TURN] L4 proxy listening on TCP/UDP %s and UDP relay ports %d-%d", cfg.TURN.TCPAddress, cfg.TURN.RelayMinPort, cfg.TURN.RelayMaxPort)
	return listeners, nil
}

func buildUDPUpstream(listener config.UDPConfig) (string, []string, error) {
	router := strings.ToLower(strings.TrimSpace(listener.Router))
	if router == "" {
		if len(listener.Routes) > 0 || listener.DefaultUpstream != "" {
			router = "hostname"
		} else {
			router = "static"
		}
	}
	switch router {
	case "static":
		return listener.Upstream, []string{listener.Upstream}, nil
	case "hostname":
		if len(listener.Routes) == 1 {
			return listener.Routes[0].Upstream, []string{listener.Routes[0].Upstream}, nil
		}
		if listener.DefaultUpstream != "" {
			return listener.DefaultUpstream, []string{listener.DefaultUpstream}, nil
		}
		return "", nil, errors.New("UDP hostname router requires one route or default_upstream")
	default:
		return "", nil, fmt.Errorf("router %q is not supported", listener.Router)
	}
}

func buildTCPRouter(listener config.TCPConfig) (stream.Router, []string, error) {
	switch strings.ToLower(strings.TrimSpace(listener.Router)) {
	case "static":
		return proxy.StaticTCPRouter{Upstream: listener.Upstream}, []string{listener.Upstream}, nil
	case "", "hostname":
		routes := make(map[string]string, len(listener.Routes))
		upstreams := make([]string, 0, len(listener.Routes)+1)
		seen := make(map[string]struct{}, len(listener.Routes)+1)
		for _, route := range listener.Routes {
			routes[route.Host] = route.Upstream
			if _, exists := seen[route.Upstream]; !exists {
				upstreams = append(upstreams, route.Upstream)
				seen[route.Upstream] = struct{}{}
			}
		}
		if listener.DefaultUpstream != "" {
			if _, exists := seen[listener.DefaultUpstream]; !exists {
				upstreams = append(upstreams, listener.DefaultUpstream)
			}
		}
		return hostroute.NewRouter(routes, listener.DefaultUpstream), upstreams, nil
	default:
		return nil, nil, fmt.Errorf("router %q is not supported", listener.Router)
	}
}
