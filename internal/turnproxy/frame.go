package turnproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"net"
)

const (
	HeaderSize = 35
	MACSize    = 16
	MaxPayload = 65507 - HeaderSize
)

const (
	ControlUDP byte = 1
	PeerData   byte = 2
	PeerSend   byte = 3
	TCPStream  byte = 4
)

var frameMagic = [4]byte{'L', 'T', 'P', '1'}

var (
	ErrKey       = errors.New("TURN proxy key must contain at least 32 bytes")
	ErrFrame     = errors.New("invalid TURN proxy frame")
	ErrFrameAuth = errors.New("invalid TURN proxy frame authentication")
)

var blockedIPv4Ranges = mustParsePrefixes([]string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
	"192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
})

type Frame struct {
	Type        byte
	Source      *net.UDPAddr
	Destination *net.UDPAddr
	Payload     []byte
}

func EncodeDatagram(key []byte, frame Frame) ([]byte, error) {
	if len(key) < 32 {
		return nil, ErrKey
	}
	if frame.Type < ControlUDP || frame.Type > PeerSend {
		return nil, ErrFrame
	}
	if len(frame.Payload) > MaxPayload {
		return nil, ErrFrame
	}
	header, err := encodeHeader(key, frame.Type, frame.Source, frame.Destination, uint16(len(frame.Payload)), frame.Payload)
	if err != nil {
		return nil, err
	}
	return append(header, frame.Payload...), nil
}

func EncodeStreamHeader(key []byte, source, destination *net.TCPAddr) ([]byte, error) {
	if len(key) < 32 {
		return nil, ErrKey
	}
	sourceUDP := &net.UDPAddr{IP: source.IP, Port: source.Port}
	destinationUDP := &net.UDPAddr{IP: destination.IP, Port: destination.Port}
	return encodeHeader(key, TCPStream, sourceUDP, destinationUDP, ^uint16(0), nil)
}

func DecodeDatagram(key, packet []byte) (Frame, error) {
	if len(key) < 32 {
		return Frame{}, ErrKey
	}
	if len(packet) < HeaderSize || !hasMagic(packet) {
		return Frame{}, ErrFrame
	}
	kind := packet[4]
	if kind < ControlUDP || kind > PeerSend {
		return Frame{}, ErrFrame
	}
	payloadLength := int(binary.BigEndian.Uint16(packet[17:19]))
	if payloadLength > MaxPayload || len(packet) != HeaderSize+payloadLength {
		return Frame{}, ErrFrame
	}
	if !verifyMAC(key, packet[:19], packet[HeaderSize:], packet[19:HeaderSize]) {
		return Frame{}, ErrFrameAuth
	}
	return Frame{
		Type:        kind,
		Source:      decodeAddress(packet[5:11]),
		Destination: decodeAddress(packet[11:17]),
		Payload:     append([]byte(nil), packet[HeaderSize:]...),
	}, nil
}

func HasMagic(packet []byte) bool {
	return hasMagic(packet)
}

func IsPublicIPv4(address net.IP) bool {
	ip := address.To4()
	if ip == nil {
		return false
	}
	for _, network := range blockedIPv4Ranges {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func DecodeStreamHeader(key, header []byte) (Frame, error) {
	if len(key) < 32 {
		return Frame{}, ErrKey
	}
	if len(header) != HeaderSize || !hasMagic(header) || header[4] != TCPStream ||
		binary.BigEndian.Uint16(header[17:19]) != ^uint16(0) {
		return Frame{}, ErrFrame
	}
	if !verifyMAC(key, header[:19], nil, header[19:HeaderSize]) {
		return Frame{}, ErrFrameAuth
	}
	return Frame{
		Type:        TCPStream,
		Source:      decodeAddress(header[5:11]),
		Destination: decodeAddress(header[11:17]),
	}, nil
}

func encodeHeader(key []byte, kind byte, source, destination *net.UDPAddr, length uint16, payload []byte) ([]byte, error) {
	sourceIP, err := ipv4(source)
	if err != nil {
		return nil, err
	}
	destinationIP, err := ipv4(destination)
	if err != nil {
		return nil, err
	}
	if source.Port < 1 || source.Port > 65535 || destination.Port < 1 || destination.Port > 65535 {
		return nil, ErrFrame
	}
	header := make([]byte, HeaderSize)
	copy(header[:4], frameMagic[:])
	header[4] = kind
	copy(header[5:9], sourceIP)
	binary.BigEndian.PutUint16(header[9:11], uint16(source.Port))
	copy(header[11:15], destinationIP)
	binary.BigEndian.PutUint16(header[15:17], uint16(destination.Port))
	binary.BigEndian.PutUint16(header[17:19], length)
	mac := calculateMAC(key, header[:19], payload)
	copy(header[19:], mac[:MACSize])
	return header, nil
}

func decodeAddress(data []byte) *net.UDPAddr {
	return &net.UDPAddr{
		IP:   net.IPv4(data[0], data[1], data[2], data[3]),
		Port: int(binary.BigEndian.Uint16(data[4:6])),
	}
}

func ipv4(address *net.UDPAddr) (net.IP, error) {
	if address == nil || address.IP == nil {
		return nil, ErrFrame
	}
	ip := address.IP.To4()
	if ip == nil {
		return nil, ErrFrame
	}
	return ip, nil
}

func hasMagic(packet []byte) bool {
	return len(packet) >= len(frameMagic) && string(packet[:4]) == string(frameMagic[:])
}

func calculateMAC(key, header, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(header)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func verifyMAC(key, header, payload, supplied []byte) bool {
	expected := calculateMAC(key, header, payload)
	return len(supplied) == MACSize && subtle.ConstantTimeCompare(expected[:MACSize], supplied) == 1
}

func mustParsePrefixes(prefixes []string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(prefixes))
	for _, prefix := range prefixes {
		_, network, err := net.ParseCIDR(prefix)
		if err != nil {
			panic(err)
		}
		networks = append(networks, network)
	}
	return networks
}
