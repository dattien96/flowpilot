package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestListWorkspaceFiles_ParsesAndPassesQuery(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]string{"apps/foo.go", "apps/bar.ts"})
	}))
	defer srv.Close()
	paths, err := client.New(srv.URL).ListWorkspaceFiles(context.Background(), "/work/project", "Chat")
	if err != nil {
		t.Fatalf("ListWorkspaceFiles: %v", err)
	}
	if gotPath != "/client/workspace-files" {
		t.Fatalf("path=%q", gotPath)
	}
	if !strings.Contains(gotQuery, "cwd=") || !strings.Contains(gotQuery, "q=Chat") {
		t.Fatalf("query=%q", gotQuery)
	}
	if len(paths) != 2 || paths[0] != "apps/foo.go" {
		t.Fatalf("paths=%v", paths)
	}
}
