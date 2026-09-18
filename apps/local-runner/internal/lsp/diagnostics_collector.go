package lsp

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileDiagnostic pairs one Diagnostic with the document it belongs to.
type FileDiagnostic struct {
	URI string
	Diagnostic
}

// DiagnosticsCollector stores the latest publishDiagnostics per URI. It is
// registered as the Client diagnostics callback (or driven directly in
// tests via Handle).
type DiagnosticsCollector struct {
	mu         sync.Mutex
	byURI      map[string][]Diagnostic
	generation uint64 // bumped on every publish, even empty ones
}

// NewDiagnosticsCollector creates a collector and, unless client is nil,
// registers it as the client's diagnostics handler.
func NewDiagnosticsCollector(client *Client) *DiagnosticsCollector {
	dc := &DiagnosticsCollector{byURI: make(map[string][]Diagnostic)}
	if client != nil {
		client.SetDiagnosticsHandler(dc.Handle)
	}
	return dc
}

// Handle stores one publishDiagnostics notification.
func (dc *DiagnosticsCollector) Handle(p PublishDiagnosticsParams) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	stored := append([]Diagnostic(nil), p.Diagnostics...)
	dc.byURI[p.URI] = stored
	dc.generation++
}

// GetDiagnostics returns a copy of the latest diagnostics for uri.
func (dc *DiagnosticsCollector) GetDiagnostics(uri string) []Diagnostic {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return append([]Diagnostic(nil), dc.byURI[uri]...)
}

// GetAllErrors returns every severity-Error diagnostic across documents,
// sorted by URI, line and character for deterministic output.
func (dc *DiagnosticsCollector) GetAllErrors() []FileDiagnostic {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	var out []FileDiagnostic
	for uri, diags := range dc.byURI {
		for _, d := range diags {
			if d.Severity == DiagnosticSeverityError {
				out = append(out, FileDiagnostic{URI: uri, Diagnostic: d})
			}
		}
	}
	sortFileDiagnostics(out)
	return out
}

// sortFileDiagnostics orders errors deterministically.
func sortFileDiagnostics(out []FileDiagnostic) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		a, b := out[i].Range.Start, out[j].Range.Start
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})
}

// HasErrors reports whether any stored diagnostic is an error.
func (dc *DiagnosticsCollector) HasErrors() bool {
	return len(dc.GetAllErrors()) > 0
}

// Generation returns how many publish notifications arrived so far.
func (dc *DiagnosticsCollector) Generation() uint64 {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.generation
}

// WaitForDiagnostics blocks until a new publish notification arrives after
// the call (or the timeout elapses). Servers push an empty diagnostic list
// for clean files, so waiting also terminates on "no errors".
func (dc *DiagnosticsCollector) WaitForDiagnostics(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	start := dc.Generation()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("lsp: diagnostics wait timed out: %w", ctx.Err())
		case <-ticker.C:
			if dc.Generation() != start {
				return nil
			}
		}
	}
}

// FormatDiagnosticsForAgent renders errors one per line as
// "path:line:col severity: message" with 1-based positions, ready to paste
// into an agent reprompt.
func FormatDiagnosticsForAgent(diagnostics []FileDiagnostic) string {
	var sb strings.Builder
	for i, fd := range diagnostics {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s:%d:%d %s: %s",
			uriFilePath(fd.URI),
			fd.Range.Start.Line+1,
			fd.Range.Start.Character+1,
			fd.Severity.String(),
			fd.Message,
		)
	}
	return sb.String()
}

// uriFilePath converts a file:// URI back to a filesystem path for display.
func uriFilePath(uri string) string {
	s := strings.TrimPrefix(uri, "file://")
	if u, err := url.Parse("file://" + s); err == nil && u.Path != "" {
		s = u.Path
	} else if decoded, err := url.PathUnescape(s); err == nil {
		s = decoded
	}
	// Windows URIs look like /D:/x after parsing; drop the leading slash.
	if len(s) > 2 && s[0] == '/' && s[2] == ':' {
		s = s[1:]
	}
	return s
}
