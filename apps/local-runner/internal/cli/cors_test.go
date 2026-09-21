package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithCORSAllowsDesktopClientHeader(t *testing.T) {
	handlerCalled := false
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/client/workflow-runs/run-1/turns", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-client,idempotency-key")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if handlerCalled {
		t.Fatal("preflight must not reach the wrapped handler")
	}
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status=%d want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5174" {
		t.Fatalf("Access-Control-Allow-Origin=%q", got)
	}
	allowed := strings.ToLower(resp.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowed, "x-client") {
		t.Fatalf("Access-Control-Allow-Headers=%q does not allow X-Client", allowed)
	}
}
