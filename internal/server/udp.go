package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"edge-proxy/internal/proxy"
)

const (
	defaultUDPMaxSessions = 4096
	udpSessionIdleTimeout = 2 * time.Minute
	maxUDPDatagramSize    = 64 * 1024
)

type UDPDatagramTransform func(data []byte, client, local *net.UDPAddr) ([]byte, error)

var errMaxUDPSessions = errors.New("maximum UDP sessions reached")

type UDPListener struct {
	connection   *net.UDPConn
	upstream     string
	maxSessions  int
	idleTimeout  time.Duration
	transform    UDPDatagramTransform
	metrics      *proxy.StreamListenerMetrics
	logger       *slog.Logger
	mu           sync.Mutex
	sessions     map[string]*udpSession
	sessionWG    sync.WaitGroup
	activeClosed atomic.Bool
}

type udpSession struct {
	key       string
	client    *net.UDPAddr
	upstream  net.Conn
	metrics   *proxy.StreamConnectionMetrics
	lastSeen  atomic.Int64
	closeOnce sync.Once
}

func NewUDPListener(address, upstream string, maxSessions int, metrics *proxy.StreamListenerMetrics, logger *slog.Logger) (*UDPListener, error) {
	return NewUDPListenerWithTransform(address, upstream, maxSessions, udpSessionIdleTimeout, nil, metrics, logger)
}

func NewUDPListenerWithTransform(address, upstream string, maxSessions int, idleTimeout time.Duration, transform UDPDatagramTransform, metrics *proxy.StreamListenerMetrics, logger *slog.Logger) (*UDPListener, error) {
	listenAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	connection, err := net.ListenUDP("udp", listenAddress)
	if err != nil {
		return nil, err
	}
	if maxSessions <= 0 {
		maxSessions = defaultUDPMaxSessions
	}
	if idleTimeout <= 0 {
		idleTimeout = udpSessionIdleTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &UDPListener{
		connection:  connection,
		upstream:    upstream,
		maxSessions: maxSessions,
		idleTimeout: idleTimeout,
		transform:   transform,
		metrics:     metrics,
		logger:      logger,
		sessions:    make(map[string]*udpSession),
	}, nil
}

func (s *UDPListener) Serve(ctx context.Context) error {
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	defer s.sessionWG.Wait()

	buffer := make([]byte, maxUDPDatagramSize)
	for {
		count, clientAddress, err := s.connection.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if count == 0 {
			continue
		}
		session, err := s.session(ctx, clientAddress)
		if err != nil {
			if errors.Is(err, errMaxUDPSessions) && s.metrics != nil {
				s.metrics.Reject()
			}
			s.logger.Debug("UDP datagram rejected", "client", clientAddress.String(), "error", err)
			continue
		}
		session.touch(s.idleTimeout)
		datagram := buffer[:count]
		if s.transform != nil {
			localAddress, ok := s.connection.LocalAddr().(*net.UDPAddr)
			if !ok {
				s.logger.Error("UDP listener has an unexpected local address")
				continue
			}
			datagram, err = s.transform(datagram, clientAddress, localAddress)
			if err != nil {
				s.logger.Debug("UDP datagram transform rejected", "client", clientAddress.String(), "error", err)
				continue
			}
		}
		written, err := session.upstream.Write(datagram)
		if err != nil {
			if session.metrics != nil {
				session.metrics.Error()
			}
			s.removeSession(session)
			continue
		}
		if session.metrics != nil {
			session.metrics.AddIn(uint64(written))
		}
	}
}

func (s *UDPListener) Close() error {
	if s.activeClosed.Swap(true) {
		return nil
	}
	err := s.connection.Close()
	s.mu.Lock()
	sessions := make([]*udpSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	for _, session := range sessions {
		s.closeSession(session)
	}
	return err
}

func (s *UDPListener) session(ctx context.Context, client *net.UDPAddr) (*udpSession, error) {
	key := client.String()
	s.mu.Lock()
	if session, exists := s.sessions[key]; exists {
		s.mu.Unlock()
		return session, nil
	}
	if len(s.sessions) >= s.maxSessions {
		s.mu.Unlock()
		return nil, errMaxUDPSessions
	}
	s.mu.Unlock()

	dialer := &net.Dialer{Timeout: proxy.TCPDialTimeout, KeepAlive: proxy.TCPKeepAlive}
	upstream, err := dialer.DialContext(ctx, "udp", s.upstream)
	if err != nil {
		if s.metrics != nil {
			s.metrics.Error()
		}
		return nil, err
	}
	var connectionMetrics *proxy.StreamConnectionMetrics
	if s.metrics != nil {
		connectionMetrics = s.metrics.Begin(s.upstream)
	}
	session := &udpSession{key: key, client: client, upstream: upstream, metrics: connectionMetrics}
	session.touch(s.idleTimeout)

	s.mu.Lock()
	if s.activeClosed.Load() {
		s.mu.Unlock()
		_ = upstream.Close()
		if connectionMetrics != nil {
			connectionMetrics.Close()
		}
		return nil, net.ErrClosed
	}
	if existing, exists := s.sessions[key]; exists {
		s.mu.Unlock()
		_ = upstream.Close()
		if connectionMetrics != nil {
			connectionMetrics.Close()
		}
		return existing, nil
	}
	if len(s.sessions) >= s.maxSessions {
		s.mu.Unlock()
		_ = upstream.Close()
		if connectionMetrics != nil {
			connectionMetrics.Close()
		}
		return nil, errMaxUDPSessions
	}
	s.sessions[key] = session
	s.mu.Unlock()
	s.sessionWG.Add(1)
	go func() {
		defer s.sessionWG.Done()
		s.receive(session)
	}()
	return session, nil
}

func (s *UDPListener) receive(session *udpSession) {
	buffer := make([]byte, maxUDPDatagramSize)
	for {
		lastSeen := time.Unix(0, session.lastSeen.Load())
		deadline := lastSeen.Add(s.idleTimeout)
		if time.Now().After(deadline) {
			s.removeSession(session)
			return
		}
		if err := session.upstream.SetReadDeadline(deadline); err != nil {
			s.logger.Debug("set UDP session deadline", "error", err)
		}
		count, err := session.upstream.Read(buffer)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				s.logger.Debug("UDP upstream session ended", "upstream", s.upstream, "error", err)
			}
			s.removeSession(session)
			return
		}
		if _, err := s.connection.WriteToUDP(buffer[:count], session.client); err != nil {
			if session.metrics != nil {
				session.metrics.Error()
			}
			s.removeSession(session)
			return
		}
		if session.metrics != nil {
			session.metrics.AddOut(uint64(count))
		}
	}
}

func (s *UDPListener) removeSession(session *udpSession) {
	s.mu.Lock()
	if current, exists := s.sessions[session.key]; exists && current == session {
		delete(s.sessions, session.key)
	}
	s.mu.Unlock()
	s.closeSession(session)
}

func (s *UDPListener) closeSession(session *udpSession) {
	session.closeOnce.Do(func() {
		_ = session.upstream.Close()
		if session.metrics != nil {
			session.metrics.Close()
		}
	})
}

func (s *udpSession) touch(idleTimeout time.Duration) {
	now := time.Now()
	s.lastSeen.Store(now.UnixNano())
	_ = s.upstream.SetReadDeadline(now.Add(idleTimeout))
}
