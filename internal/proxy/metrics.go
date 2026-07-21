package proxy

import (
	"io"
	"net/http"
	"sync/atomic"
)

type Metrics struct {
	requests atomic.Uint64
	errors   atomic.Uint64
	inBytes  atomic.Uint64
	outBytes atomic.Uint64
}

type MetricsSnapshot struct {
	Requests uint64 `json:"requests"`
	Errors   uint64 `json:"errors"`
	InBytes  uint64 `json:"in_bytes"`
	OutBytes uint64 `json:"out_bytes"`
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{m.requests.Load(), m.errors.Load(), m.inBytes.Load(), m.outBytes.Load()}
}

func WithMetrics(metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = &countingReadCloser{ReadCloser: r.Body, count: &metrics.inBytes}
		}
		recorder := &metricsResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		metrics.requests.Add(1)
		metrics.outBytes.Add(recorder.bytes)
		if recorder.status >= http.StatusInternalServerError {
			metrics.errors.Add(1)
		}
	})
}

type countingReadCloser struct {
	io.ReadCloser
	count *atomic.Uint64
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.count.Add(uint64(n))
	}
	return n, err
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  uint64
}

func (w *metricsResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *metricsResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *metricsResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.bytes += uint64(n)
	}
	return n, err
}
