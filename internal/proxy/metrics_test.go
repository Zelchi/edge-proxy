package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithMetricsCountsTransferredBytesAndErrors(t *testing.T) {
	metrics := &Metrics{}
	handler := WithMetrics(metrics, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("error"))
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader("input")))
	snapshot := metrics.Snapshot()
	if snapshot.Requests != 1 || snapshot.Errors != 1 || snapshot.InBytes != 5 || snapshot.OutBytes != 5 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestMetricsResponseWriterCountsHijackedConnectionBytes(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	metrics := &Metrics{}
	writer := &metricsResponseWriter{
		ResponseWriter: &hijackerResponseWriter{conn: server},
		inBytes:        &metrics.inBytes,
		outBytes:       &metrics.outBytes,
	}
	connection, _, err := http.NewResponseController(&responseRecorder{ResponseWriter: writer}).Hijack()
	if err != nil {
		t.Fatal(err)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := client.Write([]byte("from-client"))
		writeDone <- err
	}()
	buffer := make([]byte, len("from-client"))
	if _, err := io.ReadFull(connection, buffer); err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	writeDone = make(chan error, 1)
	go func() {
		_, err := connection.Write([]byte("from-backend"))
		writeDone <- err
	}()
	buffer = make([]byte, len("from-backend"))
	if _, err := io.ReadFull(client, buffer); err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.InBytes != uint64(len("from-client")) || snapshot.OutBytes != uint64(len("from-backend")) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

type hijackerResponseWriter struct {
	header http.Header
	conn   net.Conn
}

func (w *hijackerResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *hijackerResponseWriter) Write(data []byte) (int, error) { return len(data), nil }

func (w *hijackerResponseWriter) WriteHeader(int) {}

func (w *hijackerResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}
