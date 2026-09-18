package runner

import (
	"context"

	"flowpilot-runner/internal/lsp"
)

// ContextSourceLSPDiagnostics is the CP-44 slot key for live compiler
// diagnostics (CP-63 P-6 / Task-359). It is opt-in: flows enable it
// explicitly, so default prompt output never changes.
const ContextSourceLSPDiagnostics ContextSourceID = "lsp.diagnostics"

// lspDiagnosticsSource adapts the lsp package's diagnostics Source to the
// CP-44 ContextSource interface. The adapter lives here (not in package
// lsp) because ContextSource/FlowContextSection are runner types and lsp
// must never import runner (import cycle).
type lspDiagnosticsSource struct{ priority int }

func (s *lspDiagnosticsSource) ID() string          { return string(ContextSourceLSPDiagnostics) }
func (s *lspDiagnosticsSource) Priority() int       { return s.priority }
func (s *lspDiagnosticsSource) Deterministic() bool { return true }

func (s *lspDiagnosticsSource) Fetch(_ context.Context, _ FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	src := &lsp.Source{Provider: lsp.DefaultSet(), PriorityValue: s.priority}
	title, body := src.Render()
	section.SourceRef = "lsp://diagnostics"
	section.Body = title + "\n" + body
	return section, nil
}
