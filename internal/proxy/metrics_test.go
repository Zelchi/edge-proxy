package proxy

import (
	"io"
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
