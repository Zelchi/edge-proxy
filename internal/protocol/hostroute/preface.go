// Package hostroute contains an optional bounded router for protocols that
// put a hostname in a length-prefixed connection preamble. The transport and
// relay remain protocol-agnostic; this package only selects an upstream.
package hostroute

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"edge-proxy/internal/stream"
)

const (
	MaxPacketLength = 8192
	MaxHostBytes    = 255
	MaxVarIntBytes  = 5
	PrefaceTimeout  = 5 * time.Second
)

var (
	ErrMalformedPreface = errors.New("malformed host preface")
	ErrUnknownHost      = errors.New("unknown host")
)

// Preface is the minimum information extracted by the optional hostname
// router. The remaining connection bytes are never interpreted here.
type Preface struct {
	ProtocolVersion int32
	Host            string
	Port            uint16
	State           int32
}

type Router struct {
	routes          map[string]string
	defaultUpstream string
}

func NewRouter(routes map[string]string, defaultUpstream string) *Router {
	normalized := make(map[string]string, len(routes))
	for host, upstream := range routes {
		normalized[normalizeHost(host)] = upstream
	}
	return &Router{routes: normalized, defaultUpstream: defaultUpstream}
}

func (r *Router) Route(ctx context.Context, conn net.Conn) (stream.RouteResult, error) {
	deadline := time.Now().Add(PrefaceTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return stream.RouteResult{}, fmt.Errorf("set host preface deadline: %w", err)
	}

	reader := bufio.NewReaderSize(conn, MaxPacketLength+MaxVarIntBytes)
	packet, err := peekPreface(reader)
	if err != nil {
		return stream.RouteResult{}, err
	}
	preface, err := ParsePreface(packet)
	if err != nil {
		return stream.RouteResult{}, err
	}
	host := normalizeHost(preface.Host)
	upstream := r.routes[host]
	if upstream == "" {
		upstream = r.defaultUpstream
	}
	if upstream == "" {
		return stream.RouteResult{}, fmt.Errorf("%w: %s", ErrUnknownHost, host)
	}
	return stream.RouteResult{Upstream: upstream, Reader: reader, Host: host}, nil
}

// ParsePreface validates a bounded length-prefixed preamble and extracts its
// hostname. It intentionally rejects trailing bytes so the original preamble
// can be forwarded without ambiguity.
func ParsePreface(packet []byte) (Preface, error) {
	if len(packet) == 0 || len(packet) > MaxPacketLength+MaxVarIntBytes {
		return Preface{}, ErrMalformedPreface
	}
	packetLength, packetStart, err := readVarInt(packet, 0)
	if err != nil || packetLength <= 0 || packetLength > MaxPacketLength || packetStart+int(packetLength) != len(packet) {
		return Preface{}, ErrMalformedPreface
	}

	position := packetStart
	packetID, next, err := readVarInt(packet, position)
	if err != nil || packetID != 0 {
		return Preface{}, ErrMalformedPreface
	}
	position = next
	protocolVersion, next, err := readVarInt(packet, position)
	if err != nil || protocolVersion < 0 {
		return Preface{}, ErrMalformedPreface
	}
	position = next
	hostLength, next, err := readVarInt(packet, position)
	if err != nil || hostLength < 0 || hostLength > MaxHostBytes {
		return Preface{}, ErrMalformedPreface
	}
	position = next
	if position+int(hostLength) > len(packet) {
		return Preface{}, ErrMalformedPreface
	}
	hostBytes := packet[position : position+int(hostLength)]
	if len(hostBytes) == 0 {
		return Preface{}, ErrMalformedPreface
	}
	if !utf8.Valid(hostBytes) {
		return Preface{}, ErrMalformedPreface
	}
	host := string(hostBytes)
	position += int(hostLength)
	if position+2 > len(packet) {
		return Preface{}, ErrMalformedPreface
	}
	port := uint16(packet[position])<<8 | uint16(packet[position+1])
	position += 2
	state, position, err := readVarInt(packet, position)
	if err != nil || (state != 1 && state != 2) || position != len(packet) {
		return Preface{}, ErrMalformedPreface
	}
	return Preface{ProtocolVersion: protocolVersion, Host: host, Port: port, State: state}, nil
}

func peekPreface(reader *bufio.Reader) ([]byte, error) {
	packetLength, prefixLength, err := peekVarInt(reader)
	if err != nil {
		return nil, err
	}
	if packetLength <= 0 || packetLength > MaxPacketLength {
		return nil, ErrMalformedPreface
	}
	packet, err := reader.Peek(prefixLength + int(packetLength))
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) {
			return nil, fmt.Errorf("%w: preface exceeds buffer", ErrMalformedPreface)
		}
		return nil, fmt.Errorf("read host preface: %w", err)
	}
	return packet, nil
}

func peekVarInt(reader *bufio.Reader) (int32, int, error) {
	var value int32
	for index := 0; index < MaxVarIntBytes; index++ {
		data, err := reader.Peek(index + 1)
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				return 0, 0, ErrMalformedPreface
			}
			return 0, 0, fmt.Errorf("read preface length: %w", err)
		}
		value |= int32(data[index]&0x7f) << (7 * index)
		if data[index]&0x80 == 0 {
			return value, index + 1, nil
		}
	}
	return 0, 0, ErrMalformedPreface
}

func readVarInt(data []byte, offset int) (int32, int, error) {
	var value int32
	for index := 0; index < MaxVarIntBytes; index++ {
		position := offset + index
		if position >= len(data) {
			return 0, 0, io.ErrUnexpectedEOF
		}
		value |= int32(data[position]&0x7f) << (7 * index)
		if data[position]&0x80 == 0 {
			return value, position + 1, nil
		}
	}
	return 0, 0, ErrMalformedPreface
}

func normalizeHost(address string) string {
	if separator := strings.IndexByte(address, 0); separator >= 0 {
		address = address[:separator]
	}
	address = strings.TrimSpace(strings.ToLower(strings.TrimRight(address, ".")))
	if host, port, err := net.SplitHostPort(address); err == nil && port != "" {
		return strings.TrimRight(strings.ToLower(host), ".")
	}
	if strings.Count(address, ":") == 1 {
		separator := strings.LastIndexByte(address, ':')
		if separator > 0 && separator < len(address)-1 {
			if _, err := strconv.Atoi(address[separator+1:]); err == nil {
				return strings.TrimRight(address[:separator], ".")
			}
		}
	}
	return address
}
