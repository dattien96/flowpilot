package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// cmdFetchLSPStatus queries the runner for this workspace's LSP server
// presence (best-effort sidebar hint; failures stay silent so the sidebar
// simply shows nothing).
func (m *AppModel) cmdFetchLSPStatus(workspacePath string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		st, err := cl.GetLSPStatus(ctx, workspacePath)
		if err != nil {
			return nil
		}
		return LSPStatusMsg{Path: workspacePath, Status: &st}
	}
}

// lspSidebarLines renders the compact LSP warning for the right sidebar:
// at most 2 width-fitted lines, nil unless a missing server was reported.
// Callers insert the result where the steps budget still adapts (before the
// steps section), so the sidebar never overflows the viewport.
func (m *AppModel) lspSidebarLines(w int) []string {
	st := m.lspStatus
	if st == nil || !st.Warn || strings.TrimSpace(st.Binary) == "" {
		return nil
	}
	glyph := "⚠"
	if m.asciiMode {
		glyph = "!"
	}
	out := []string{styleTool.Render(truncateVisual(glyph+" lsp: "+strings.TrimSpace(st.Binary)+" missing", w))}
	if hint := strings.TrimSpace(st.InstallHint); hint != "" {
		out = append(out, styleSystem.Render(truncateVisual("→ "+hint, w)))
	}
	return out
}
