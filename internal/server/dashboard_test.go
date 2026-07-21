package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edge-proxy/internal/proxy"
)

func TestWithDashboardServesConfiguredHost(t *testing.T) {
	handler := WithDashboard("info.example", &proxy.Metrics{}, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "https://info.example/", nil)
	request.Host = "info.example:443"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Requisições") {
		t.Fatalf("dashboard response = %d %q", response.Code, response.Body.String())
	}
}

func TestWithDashboardExposesReadOnlyMetrics(t *testing.T) {
	handler := WithDashboard("info.example", &proxy.Metrics{}, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPost, "https://info.example/api/metrics", nil)
	request.Host = "info.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
