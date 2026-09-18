package lsp

import (
	"context"
	"strings"
	"testing"
	"time"
)

func lspDiag(line, char uint32, sev DiagnosticSeverity, msg string) Diagnostic {
	return Diagnostic{
		Range:    Range{Start: Position{Line: line, Character: char}, End: Position{Line: line, Character: char + 1}},
		Severity: sev,
		Message:  msg,
	}
}

func TestDiagnosticsCollectorStoresByURI(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: []Diagnostic{
		lspDiag(1, 0, DiagnosticSeverityError, "boom"),
	}})
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/b.py", Diagnostics: []Diagnostic{
		lspDiag(2, 0, DiagnosticSeverityWarning, "hmm"),
		lspDiag(3, 0, DiagnosticSeverityError, "bad"),
	}})

	if got := dc.GetDiagnostics("file:///tmp/a.go"); len(got) != 1 || got[0].Message != "boom" {
		t.Fatalf("a.go diagnostics = %+v", got)
	}
	if got := dc.GetDiagnostics("file:///tmp/b.py"); len(got) != 2 {
		t.Fatalf("b.py diagnostics = %+v", got)
	}
	if got := dc.GetDiagnostics("file:///tmp/missing.go"); len(got) != 0 {
		t.Fatalf("missing uri diagnostics = %+v", got)
	}

	// A republish replaces the previous set.
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: nil})
	if got := dc.GetDiagnostics("file:///tmp/a.go"); len(got) != 0 {
		t.Fatalf("a.go after republish = %+v", got)
	}
}

func TestDiagnosticsCollectorGetAllErrorsFiltersWarnings(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/b.py", Diagnostics: []Diagnostic{
		lspDiag(5, 0, DiagnosticSeverityError, "e2"),
	}})
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: []Diagnostic{
		lspDiag(9, 0, DiagnosticSeverityError, "e1"),
		lspDiag(1, 0, DiagnosticSeverityWarning, "w"),
		lspDiag(2, 0, DiagnosticSeverityInformation, "i"),
		lspDiag(3, 0, DiagnosticSeverityHint, "h"),
		lspDiag(4, 0, 0, "no-severity"),
	}})

	errs := dc.GetAllErrors()
	if len(errs) != 2 {
		t.Fatalf("errors = %+v", errs)
	}
	// Sorted by URI: a.go first.
	if errs[0].URI != "file:///tmp/a.go" || errs[0].Message != "e1" {
		t.Fatalf("first error = %+v", errs[0])
	}
	if errs[1].URI != "file:///tmp/b.py" || errs[1].Message != "e2" {
		t.Fatalf("second error = %+v", errs[1])
	}
}

func TestDiagnosticsCollectorHasErrorsReturnsFalseWhenClean(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	if dc.HasErrors() {
		t.Fatal("fresh collector must report no errors")
	}
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: []Diagnostic{
		lspDiag(1, 0, DiagnosticSeverityWarning, "just a warning"),
	}})
	if dc.HasErrors() {
		t.Fatal("warnings-only must not count as errors")
	}
	dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go", Diagnostics: []Diagnostic{
		lspDiag(1, 0, DiagnosticSeverityError, "real error"),
	}})
	if !dc.HasErrors() {
		t.Fatal("error must be reported")
	}
}

func TestDiagnosticsCollectorWaitForDiagnosticsTimesOut(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	if err := dc.WaitForDiagnostics(ctx, 150*time.Millisecond); err == nil {
		t.Fatal("expected timeout error when nothing arrives")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("wait took %v, expected fast timeout", elapsed)
	}
}

func TestDiagnosticsCollectorWaitForDiagnosticsSucceeds(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	go func() {
		time.Sleep(50 * time.Millisecond)
		// An empty publish still counts as an arrival (clean file).
		dc.Handle(PublishDiagnosticsParams{URI: "file:///tmp/a.go"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := dc.WaitForDiagnostics(ctx, 0); err != nil {
		t.Fatalf("WaitForDiagnostics: %v", err)
	}
}

func TestFormatDiagnosticsForAgentOutput(t *testing.T) {
	got := FormatDiagnosticsForAgent([]FileDiagnostic{
		{URI: "file:///tmp/a.go", Diagnostic: lspDiag(41, 9, DiagnosticSeverityError, "undefined: Foo")},
		{URI: "file:///tmp/b.py", Diagnostic: lspDiag(0, 0, DiagnosticSeverityError, "bad indent")},
	})
	want := "/tmp/a.go:42:10 error: undefined: Foo\n/tmp/b.py:1:1 error: bad indent"
	if got != want {
		t.Fatalf("formatted = %q, want %q", got, want)
	}
	if got := FormatDiagnosticsForAgent(nil); got != "" {
		t.Fatalf("empty input must format to empty string, got %q", got)
	}
}
