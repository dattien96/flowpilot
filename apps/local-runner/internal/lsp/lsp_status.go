package lsp

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ServerStatus is the pure-query answer to "does this workspace get LSP?".
// It never spawns anything, so UIs can poll it freely. JSON tags serve the
// runner's GET /client/lsp-status endpoint consumed by the TUI sidebar and
// the desktop client.
type ServerStatus struct {
	// Platform is the DetectPlatform token, "" when the workspace is empty.
	Platform string `json:"platform"`
	// Binary is the server executable for the platform, "" when unregistered.
	Binary string `json:"binary"`
	// Installed reports whether Binary resolves via PATH right now.
	Installed bool `json:"installed"`
	// InstallHint is the one-liner to install Binary (registry data).
	InstallHint string `json:"installHint,omitempty"`
	// Warn is true exactly when the workspace has a registered platform
	// whose server is missing: the single condition UIs render.
	Warn bool `json:"warn"`
	// Notice is the compact one-liner for logs and simple consumers.
	Notice string `json:"notice,omitempty"`
}

// Status answers "does this workspace get LSP?" without side effects: no
// spawn, no warnings, safe to call on every UI refresh.
func (s *ServerSet) Status(workspaceRoot string) ServerStatus {
	st := ServerStatus{}
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return st
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	st.Platform = DetectPlatform(root)
	cfg, ok := s.registry.Lookup(st.Platform)
	if !ok {
		return st
	}
	st.Binary = cfg.Binary
	st.InstallHint = cfg.InstallHint
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		st.Warn = true
		st.Notice = "LSP: " + cfg.Binary + " not installed (" + cfg.InstallHint + ")"
		return st
	}
	st.Installed = true
	return st
}
