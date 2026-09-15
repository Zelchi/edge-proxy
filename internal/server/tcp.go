package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"edge-proxy/internal/proxy"
	"edge-proxy/internal/stream"
)

const defaultTCPMaxConnections = 1024

type TCPListener struct {
	listener       net.Listener
	router         stream.Router
	maxConnections int
	metrics        *proxy.StreamListenerMetrics
	logger         *slog.Logger
	connections    chan struct{}
	handles        sync.WaitGroup
	clientsMu      sync.Mutex
	clients        map[net.Conn]struct{}
	closeOnce      sync.Once
	closeErr       error
}

func NewTCPListener(address string, router stream.Router, maxConnections int, metrics *proxy.StreamListenerMetrics, logger *slog.Logger) (*TCPListener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	if maxConnections <= 0 {
		maxConnections = defaultTCPMaxConnections
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TCPListener{
		listener:       listener,
		router:         router,
		maxConnections: maxConnections,
		metrics:        metrics,
		logger:         logger,
		connections:    make(chan struct{}, maxConnections),
		clients:        make(map[net.Conn]struct{}, maxConnections),
	}, nil
}

func (s *TCPListener) Serve(ctx context.Context) error {
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	defer s.handles.Wait()

	for {
		connection, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		select {
		case s.connections <- struct{}{}:
			s.registerClient(connection)
			s.handles.Add(1)
			go func() {
				defer s.handles.Done()
				s.handle(ctx, connection)
			}()
		default:
			if s.metrics != nil {
				s.metrics.Reject()
			}
			_ = connection.Close()
		}
	}
}

func (s *TCPListener) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.listener.Close()
		s.clientsMu.Lock()
		clients := make([]net.Conn, 0, len(s.clients))
		for client := range s.clients {
			clients = append(clients, client)
		}
		s.clientsMu.Unlock()
		for _, client := range clients {
			_ = client.Close()
		}
	})
	return s.closeErr
}

func (s *TCPListener) handle(ctx context.Context, client net.Conn) {
	defer func() {
		s.unregisterClient(client)
		<-s.connections
		_ = client.Close()
	}()
	if ctx.Err() != nil {
		return
	}

	result, err := s.router.Route(ctx, client)
	if err != nil {
		if s.metrics != nil {
			s.metrics.Reject()
		}
		s.logger.Debug("TCP connection rejected", "error", err)
		return
	}
	s.logger.Debug("TCP route selected", "host", result.Host, "upstream", result.Upstream)

	connectionMetrics := (*proxy.StreamConnectionMetrics)(nil)
	if s.metrics != nil {
		connectionMetrics = s.metrics.Begin(result.Upstream)
		defer connectionMetrics.Close()
	}
	if err := client.SetReadDeadline(time.Time{}); err != nil {
		s.logger.Debug("clear TCP handshake deadline", "error", err)
	}
	if err := proxy.RelayTCP(ctx, client, result, connectionMetrics); err != nil {
		if connectionMetrics != nil {
			connectionMetrics.Error()
		}
		s.logger.Debug("TCP relay ended", "upstream", result.Upstream, "host", result.Host, "error", err)
	}
}

func (s *TCPListener) registerClient(client net.Conn) {
	s.clientsMu.Lock()
	s.clients[client] = struct{}{}
	s.clientsMu.Unlock()
}

func (s *TCPListener) unregisterClient(client net.Conn) {
	s.clientsMu.Lock()
	delete(s.clients, client)
	s.clientsMu.Unlock()
}
