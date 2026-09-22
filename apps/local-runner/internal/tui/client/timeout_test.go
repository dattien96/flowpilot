package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStartRun_HonorsContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := New(srv.URL).StartRun(ctx, StartRunInput{ProjectID: "p1", ChatMode: "normal_chat"})
	if err == nil {
		t.Fatal("expected timeout/cancel error from hung StartRun")
	}
}

// A scaffold dispatch is synchronous server-side — the AI turn plus the
// compiler gate legitimately hold response headers for minutes, far past the
// normal transport's ResponseHeaderTimeout. The dispatch must ride a transport
// without that guard (its own 60min ctx is the real bound), while ordinary RPCs
// keep the hung-runner protection.
func TestDispatchScaffold_NotCutByResponseHeaderTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/scaffold") {
			time.Sleep(250 * time.Millisecond) // server is slow but alive
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"skipped","message":"scaffold: skipped"}`))
			return
		}
		time.Sleep(500 * time.Millisecond) // every other endpoint delays past the cutoff
	}))
	defer srv.Close()

	c := New(srv.URL)
	// Shrink the normal client's header guard so the test doesn't wait 60s.
	c.http.Transport.(*http.Transport).ResponseHeaderTimeout = 50 * time.Millisecond

	// Normal RPCs must still be cut by the header timeout (guard intact).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Health(ctx); err == nil {
		t.Fatal("expected Health to hit the shrunk ResponseHeaderTimeout")
	}

	// The scaffold dispatch must NOT be cut — it legitimately outlives the
	// header timeout (real flow: AI turn + compiler gate = minutes).
	res, err := c.DispatchScaffold(context.Background(), "p1", t.TempDir(), "react-native", "devin", "devin/swe-2-max")
	if err != nil {
		t.Fatalf("DispatchScaffold err = %v, want nil — scaffold must outlive ResponseHeaderTimeout", err)
	}
	if res == nil || res.Status != "skipped" {
		t.Fatalf("DispatchScaffold result = %+v, want skipped echo", res)
	}
}
