package lsp

// DiagnosticsProvider is the collector surface the context source reads.
// *DiagnosticsCollector and *ServerSet both satisfy it.
type DiagnosticsProvider interface {
	GetAllErrors() []FileDiagnostic
}

// DiagnosticsSectionTitle heads the rendered context section.
const DiagnosticsSectionTitle = "## Current Compiler Diagnostics (LSP)"

// noDiagnosticsMessage is rendered when no compiler errors are known.
const noDiagnosticsMessage = "No compiler errors detected."

// Source exposes the latest LSP errors as a renderable context section.
// It deliberately stays free of runner imports (the runner's CP-44 adapter
// wraps Source) so the lsp package never depends on runner.
type Source struct {
	// Provider supplies the errors; nil reads as clean.
	Provider DiagnosticsProvider
	// PriorityValue orders the section in the context package.
	PriorityValue int
}

// ID is the CP-44 slot key.
func (s *Source) ID() string { return "lsp.diagnostics" }

// Priority returns the packing order.
func (s *Source) Priority() int {
	if s == nil {
		return 0
	}
	return s.PriorityValue
}

// Render returns the section title and body: the error list, or the clean
// message when no errors are known (including when no server runs).
func (s *Source) Render() (title, body string) {
	var errs []FileDiagnostic
	if s != nil && s.Provider != nil {
		errs = s.Provider.GetAllErrors()
	}
	return RenderDiagnosticsSection(errs)
}

// RenderDiagnosticsSection renders errs as a context section body.
func RenderDiagnosticsSection(errs []FileDiagnostic) (title, body string) {
	if len(errs) == 0 {
		return DiagnosticsSectionTitle, noDiagnosticsMessage
	}
	return DiagnosticsSectionTitle, FormatDiagnosticsForAgent(errs)
}

// GetAllErrors aggregates severity-Error diagnostics across every owned
// server, so one context section covers multi-language workspaces.
func (s *ServerSet) GetAllErrors() []FileDiagnostic {
	s.mu.Lock()
	collectors := make([]*DiagnosticsCollector, 0, len(s.servers))
	for _, srv := range s.servers {
		if srv != nil && srv.Diags != nil {
			collectors = append(collectors, srv.Diags)
		}
	}
	s.mu.Unlock()
	var out []FileDiagnostic
	for _, dc := range collectors {
		out = append(out, dc.GetAllErrors()...)
	}
	sortFileDiagnostics(out)
	return out
}
