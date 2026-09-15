package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"edge-proxy/internal/stream"
)

const (
	TCPDialTimeout = 5 * time.Second
	TCPKeepAlive   = 30 * time.Second
)

// StaticTCPRouter routes every connection to the same host:port endpoint.
type StaticTCPRouter struct {
	Upstream string
}

func (r StaticTCPRouter) Route(_ context.Context, conn net.Conn) (stream.RouteResult, error) {
	return stream.RouteResult{Upstream: r.Upstream, Reader: conn}, nil
}

// RelayTCP connects a client to the selected upstream and copies bytes in both
// directions. It preserves half-closes where the underlying connection allows
// them, which is important for protocols that use EOF as a message boundary.
func RelayTCP(ctx context.Context, client net.Conn, result stream.RouteResult, metrics *StreamConnectionMetrics) error {
	dialer := &net.Dialer{Timeout: TCPDialTimeout, KeepAlive: TCPKeepAlive}
	upstream, err := dialer.DialContext(ctx, "tcp", result.Upstream)
	if err != nil {
		return err
	}
	defer upstream.Close()

	reader := result.Reader
	if reader == nil {
		reader = client
	}

	copyErrors := make(chan error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		copyErrors <- copyTCPDirection(upstream, reader, metrics, true, func() { closeWrite(upstream) })
	}()
	go func() {
		defer waitGroup.Done()
		copyErrors <- copyTCPDirection(client, upstream, metrics, false, func() { closeWrite(client) })
	}()

	done := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = client.Close()
			_ = upstream.Close()
		case <-done:
		}
	}()
	waitGroup.Wait()
	close(done)
	<-watcherDone

	var firstErr error
	for range 2 {
		if copyErr := <-copyErrors; copyErr != nil && !errors.Is(copyErr, net.ErrClosed) && !errors.Is(copyErr, io.EOF) && firstErr == nil {
			firstErr = copyErr
		}
	}
	return firstErr
}

func copyTCPDirection(dst io.Writer, src io.Reader, metrics *StreamConnectionMetrics, incoming bool, halfClose func()) error {
	writer := io.Writer(dst)
	if metrics != nil {
		writer = countingWriter{Writer: dst, connection: metrics, incoming: incoming}
	}
	_, err := io.Copy(writer, src)
	halfClose()
	return err
}

type countingWriter struct {
	io.Writer
	connection *StreamConnectionMetrics
	incoming   bool
}

func (w countingWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	if n > 0 {
		if w.connection != nil {
			if w.incoming {
				w.connection.AddIn(uint64(n))
			} else {
				w.connection.AddOut(uint64(n))
			}
		}
	}
	return n, err
}

type closeWriter interface {
	CloseWrite() error
}

func closeWrite(conn net.Conn) {
	if writer, ok := conn.(closeWriter); ok {
		_ = writer.CloseWrite()
		return
	}
	_ = conn.Close()
}
