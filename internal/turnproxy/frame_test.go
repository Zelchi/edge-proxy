package turnproxy

import (
	"bytes"
	"errors"
	"net"
	"testing"
)

func TestDatagramFrameRoundTripAndAuthentication(t *testing.T) {
	key := []byte("unit-test-turn-proxy-key-32-bytes")
	source := &net.UDPAddr{IP: net.ParseIP("198.51.100.9"), Port: 43210}
	destination := &net.UDPAddr{IP: net.ParseIP("163.176.149.9"), Port: 3478}
	payload := []byte("STUN payload")

	packet, err := EncodeDatagram(key, Frame{
		Type: ControlUDP, Source: source, Destination: destination, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := DecodeDatagram(key, packet)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Type != ControlUDP || !frame.Source.IP.Equal(source.IP) || frame.Source.Port != source.Port {
		t.Fatalf("unexpected source: %#v", frame.Source)
	}
	if !frame.Destination.IP.Equal(destination.IP) || frame.Destination.Port != destination.Port {
		t.Fatalf("unexpected destination: %#v", frame.Destination)
	}
	if !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("unexpected payload: %q", frame.Payload)
	}

	packet[len(packet)-1] ^= 1
	if _, err := DecodeDatagram(key, packet); !errors.Is(err, ErrFrameAuth) {
		t.Fatalf("tampered payload error = %v, want %v", err, ErrFrameAuth)
	}
}

func TestStreamHeaderRoundTrip(t *testing.T) {
	key := []byte("unit-test-turn-proxy-key-32-bytes")
	source := &net.TCPAddr{IP: net.ParseIP("203.0.113.17"), Port: 50822}
	destination := &net.TCPAddr{IP: net.ParseIP("163.176.149.9"), Port: 3478}

	header, err := EncodeStreamHeader(key, source, destination)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := DecodeStreamHeader(key, header)
	if err != nil {
		t.Fatal(err)
	}
	if !frame.Source.IP.Equal(source.IP) || frame.Source.Port != source.Port {
		t.Fatalf("unexpected source: %#v", frame.Source)
	}
	if !frame.Destination.IP.Equal(destination.IP) || frame.Destination.Port != destination.Port {
		t.Fatalf("unexpected destination: %#v", frame.Destination)
	}
	header[5] ^= 1
	if _, err := DecodeStreamHeader(key, header); !errors.Is(err, ErrFrameAuth) {
		t.Fatalf("tampered header error = %v, want %v", err, ErrFrameAuth)
	}
}

func TestFrameRejectsInvalidAddressesAndOversizedPayload(t *testing.T) {
	key := []byte("unit-test-turn-proxy-key-32-bytes")
	source := &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 1}
	destination := &net.UDPAddr{IP: net.ParseIP("163.176.149.9"), Port: 3478}
	if _, err := EncodeDatagram(key, Frame{Type: ControlUDP, Source: source, Destination: destination}); !errors.Is(err, ErrFrame) {
		t.Fatalf("IPv6 source error = %v, want %v", err, ErrFrame)
	}
	source = &net.UDPAddr{IP: net.ParseIP("198.51.100.9"), Port: 1}
	if _, err := EncodeDatagram(key, Frame{
		Type: ControlUDP, Source: source, Destination: destination, Payload: make([]byte, MaxPayload+1),
	}); !errors.Is(err, ErrFrame) {
		t.Fatalf("oversized payload error = %v, want %v", err, ErrFrame)
	}
}
