package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

func TestGetLSPStatusParsesResponse(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/lsp-status" {
			http.NotFound(w, r)
			return
		}
		gotPath = r.URL.Query().Get("path")
		json.NewEncoder(w).Encode(map[string]any{
			"platform":    "golang",
			"binary":      "gopls",
			"installed":   false,
			"installHint": "go install golang.org/x/tools/gopls@latest",
			"warn":        true,
			"notice":      "LSP: gopls not installed (go install golang.org/x/tools/gopls@latest)",
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := c.GetLSPStatus(ctx, `D:\proj\go-app`)
	if err != nil {
		t.Fatalf("GetLSPStatus: %v", err)
	}
	if gotPath != `D:\proj\go-app` {
		t.Fatalf("path query = %q", gotPath)
	}
	if st.Platform != "golang" || st.Binary != "gopls" {
		t.Fatalf("status = %+v", st)
	}
	if st.Installed || !st.Warn {
		t.Fatalf("status = %+v, want missing with warning", st)
	}
	if !strings.Contains(st.InstallHint, "gopls") {
		t.Fatalf("hint = %q", st.InstallHint)
	}
}

func TestGetLSPStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"missing_path"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.GetLSPStatus(ctx, ""); err == nil {
		t.Fatal("expected error on 400")
	}
}
