package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWithRecovery_PanicReturns500NotProcessDeath verifies a handler panic on
// the request goroutine is converted into a 500 JSON error instead of
// propagating out of the mux (which would kill the whole runner process and
// turn every later client call into "connection refused").
func TestWithRecovery_PanicReturns500NotProcessDeath(t *testing.T) {
	panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom: test panic in child resume")
	})

	srv := httptest.NewServer(withRecovery(panicky))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/resume")
	if err != nil {
		t.Fatalf("runner process must survive handler panic, got transport error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
}

// TestWithRecovery_PassesThroughHealthyRequests verifies the middleware does
// not interfere with normal handlers: status, body, and CORS headers survive.
func TestWithRecovery_PassesThroughHealthyRequests(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	srv := httptest.NewServer(withCORS(withRecovery(next)))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3002")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Test") != "ok" {
		t.Fatalf("X-Test header lost through middleware")
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:3002" {
		t.Fatalf("CORS header lost: %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "healthy" {
		t.Fatalf("body=%q want healthy", string(body))
	}
}