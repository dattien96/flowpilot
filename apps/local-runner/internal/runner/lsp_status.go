package runner

import (
	"net/http"
	"strings"

	"flowpilot-runner/internal/lsp"
)

// handleLSPStatus serves GET /client/lsp-status?path=<workspaceDir>: whether
// the workspace gets live LSP diagnostics (platform detected, server binary
// present) plus the install hint when missing. Pure query — never spawns.
// Consumed by the TUI right sidebar and the desktop client.
func (s *InteractiveService) handleLSPStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "missing_path", "query param 'path' (workspace directory) is required"))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, lsp.DefaultSet().Status(path))
}
