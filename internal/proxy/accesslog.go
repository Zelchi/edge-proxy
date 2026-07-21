package proxy

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

var requestSequence atomic.Uint64

func WithAccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strconv.FormatUint(requestSequence.Add(1), 36)
		w.Header().Set("X-Request-ID", requestID)
		recorder := &responseRecorder{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		logger.Info("request completed",
			"request_id", requestID,
			"method", r.Method,
			"host", r.Host,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration", time.Since(started),
			"upstream", UpstreamFromRequest(r),
		)
	})
}
