package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithHealth(t *testing.T) {
	nextCalled := false
	handler := WithHealth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
	if nextCalled {
		t.Fatal("health request reached the application handler")
	}
}
