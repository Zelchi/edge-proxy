package stream

import (
	"context"
	"io"
	"net"
)

// RouteResult describes the only information the generic TCP relay needs after
// a protocol-specific router has inspected a connection.
type RouteResult struct {
	Upstream string
	Reader   io.Reader
	Host     string
}

// Router selects an upstream for an accepted TCP connection. Reader can wrap
// the connection when protocol parsing has buffered bytes that must be sent to
// the upstream unchanged.
type Router interface {
	Route(context.Context, net.Conn) (RouteResult, error)
}
