package client

import (
	"context"

	neturl "net/url"
)

// LSPStatus mirrors the runner's GET /client/lsp-status response: whether a
// workspace gets live LSP diagnostics, and what to install when not.
type LSPStatus struct {
	Platform    string `json:"platform"`
	Binary      string `json:"binary"`
	Installed   bool   `json:"installed"`
	InstallHint string `json:"installHint,omitempty"`
	Warn        bool   `json:"warn"`
	Notice      string `json:"notice,omitempty"`
}

// GetLSPStatus fetches GET /client/lsp-status?path=<workspaceDir>.
func (c *Client) GetLSPStatus(ctx context.Context, workspacePath string) (LSPStatus, error) {
	var out LSPStatus
	err := c.getJSON(ctx, "/client/lsp-status?path="+neturl.QueryEscape(workspacePath), &out)
	return out, err
}
