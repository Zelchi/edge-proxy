package turnproxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"

	"edge-proxy/internal/stream"
)

type TCPRouter struct {
	Upstream    string
	PublicIP    net.IP
	PublicPort  int
	ProxySecret []byte
}

func (r TCPRouter) Route(_ context.Context, connection net.Conn) (stream.RouteResult, error) {
	source, ok := connection.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return stream.RouteResult{}, fmt.Errorf("TURN TCP client address is not TCP")
	}
	destination := &net.TCPAddr{IP: r.PublicIP, Port: r.PublicPort}
	header, err := EncodeStreamHeader(r.ProxySecret, source, destination)
	if err != nil {
		return stream.RouteResult{}, err
	}
	return stream.RouteResult{
		Upstream: r.Upstream,
		Reader:   io.MultiReader(bytes.NewReader(header), connection),
		Host:     "turn",
	}, nil
}
