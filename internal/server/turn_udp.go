package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"edge-proxy/internal/turnproxy"
)

type TURNRelayUDPListener struct {
	connection *net.UDPConn
	upstream   *net.UDPAddr
	key        []byte
	publicIP   net.IP
	publicPort int
	logger     *slog.Logger
	closeOnce  sync.Once
	closeErr   error
}

func NewTURNRelayUDPListener(address, upstream string, key []byte, publicIP net.IP, publicPort int, logger *slog.Logger) (*TURNRelayUDPListener, error) {
	if len(key) < 32 {
		return nil, turnproxy.ErrKey
	}
	if !turnproxy.IsPublicIPv4(publicIP) || publicPort < 1 || publicPort > 65535 {
		return nil, fmt.Errorf("invalid TURN relay public endpoint")
	}
	listenAddress, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, err
	}
	connection, err := net.ListenUDP("udp4", listenAddress)
	if err != nil {
		return nil, err
	}
	upstreamAddress, err := net.ResolveUDPAddr("udp4", upstream)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TURNRelayUDPListener{
		connection: connection,
		upstream:   upstreamAddress,
		key:        append([]byte(nil), key...),
		publicIP:   append(net.IP(nil), publicIP.To4()...),
		publicPort: publicPort,
		logger:     logger,
	}, nil
}

func (s *TURNRelayUDPListener) Serve(ctx context.Context) error {
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-closed:
		}
	}()
	defer close(closed)

	buffer := make([]byte, 65507)
	for {
		count, remote, err := s.connection.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		packet := buffer[:count]
		if turnproxy.HasMagic(packet) {
			s.handleCommand(packet, remote)
			continue
		}
		if len(packet) > turnproxy.MaxPayload {
			continue
		}
		if !turnproxy.IsPublicIPv4(remote.IP) || remote.Port < 1 {
			continue
		}
		frame, err := turnproxy.EncodeDatagram(s.key, turnproxy.Frame{
			Type:        turnproxy.PeerData,
			Source:      remote,
			Destination: &net.UDPAddr{IP: s.publicIP, Port: s.publicPort},
			Payload:     packet,
		})
		if err != nil {
			s.logger.Debug("TURN peer datagram rejected", "error", err)
			continue
		}
		if _, err := s.connection.WriteToUDP(frame, s.upstream); err != nil {
			s.logger.Debug("TURN peer datagram forwarding failed", "error", err)
		}
	}
}

func (s *TURNRelayUDPListener) handleCommand(packet []byte, remote *net.UDPAddr) {
	frame, err := turnproxy.DecodeDatagram(s.key, packet)
	if err != nil {
		return
	}
	if frame.Type != turnproxy.PeerSend ||
		!frame.Source.IP.Equal(s.publicIP) || frame.Source.Port != s.publicPort ||
		!turnproxy.IsPublicIPv4(frame.Destination.IP) || frame.Destination.Port == 0 {
		return
	}
	if _, err := s.connection.WriteToUDP(frame.Payload, frame.Destination); err != nil {
		s.logger.Debug("TURN relay send failed", "source", remote.String(), "error", err)
	}
}

func (s *TURNRelayUDPListener) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.connection.Close()
	})
	return s.closeErr
}
