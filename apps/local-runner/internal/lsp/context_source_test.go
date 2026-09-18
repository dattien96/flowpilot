package lsp

import (
	"strings"
	"testing"
)

func TestLSPDiagnosticsSourceRendersErrors(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: []Diagnostic{
		lspDiag(41, 9, DiagnosticSeverityError, "undefined: Foo"),
	}})
	src := &Source{Provider: dc, PriorityValue: 5}
	title, body := src.Render()
	if title != DiagnosticsSectionTitle {
		t.Fatalf("title = %q", title)
	}
	if !strings.Contains(body, "undefined: Foo") || !strings.Contains(body, "error:") {
		t.Fatalf("body = %q", body)
	}
}

func TestLSPDiagnosticsSourceRendersCleanMessage(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	src := &Source{Provider: dc}
	if _, body := src.Render(); body != "No compiler errors detected." {
		t.Fatalf("clean body = %q", body)
	}
	// Warnings alone still read as clean.
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: []Diagnostic{
		lspDiag(1, 0, DiagnosticSeverityWarning, "unused"),
	}})
	if _, body := src.Render(); body != "No compiler errors detected." {
		t.Fatalf("warnings-only body = %q", body)
	}
}

func TestLSPDiagnosticsSourceSlotKey(t *testing.T) {
	src := &Source{PriorityValue: 5}
	if got := src.ID(); got != "lsp.diagnostics" {
		t.Fatalf("ID = %q, want lsp.diagnostics", got)
	}
	if got := src.Priority(); got != 5 {
		t.Fatalf("Priority = %d, want 5", got)
	}
}

func TestLSPDiagnosticsSourceSkipsWhenNoServer(t *testing.T) {
	// Nil provider (never configured).
	var nilSrc *Source
	if title, body := nilSrc.Render(); title != DiagnosticsSectionTitle || body != "No compiler errors detected." {
		t.Fatalf("nil source = %q / %q", title, body)
	}
	src := &Source{}
	if _, body := src.Render(); body != "No compiler errors detected." {
		t.Fatalf("empty source = %q", body)
	}
	// Empty server set (no servers started).
	set := NewServerSet(nil)
	empty := &Source{Provider: set}
	if _, body := empty.Render(); body != "No compiler errors detected." {
		t.Fatalf("empty set source = %q", body)
	}
	if errs := set.GetAllErrors(); len(errs) != 0 {
		t.Fatalf("empty set errors = %+v", errs)
	}
}

func TestServerSetAllErrorsAggregates(t *testing.T) {
	set := NewServerSet(nil)
	dcGo := NewDiagnosticsCollector(nil)
	dcGo.Handle(PublishDiagnosticsParams{URI: "file:///w/b.go", Diagnostics: []Diagnostic{
		lspDiag(2, 0, DiagnosticSeverityError, "go boom"),
	}})
	dcPy := NewDiagnosticsCollector(nil)
	dcPy.Handle(PublishDiagnosticsParams{URI: "file:///w/a.py", Diagnostics: []Diagnostic{
		lspDiag(0, 0, DiagnosticSeverityError, "py boom"),
	}})
	set.servers[serverKey{root: "/w", platform: "golang"}] = &Server{Diags: dcGo}
	set.servers[serverKey{root: "/w", platform: "python"}] = &Server{Diags: dcPy}

	errs := set.GetAllErrors()
	if len(errs) != 2 {
		t.Fatalf("errors = %+v", errs)
	}
	// Sorted across servers by URI.
	if errs[0].URI != "file:///w/a.py" || errs[1].URI != "file:///w/b.go" {
		t.Fatalf("errors = %+v", errs)
	}
}
