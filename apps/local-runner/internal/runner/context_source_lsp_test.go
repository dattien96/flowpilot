package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLSPDiagnosticsSourceRegisteredInCP44Registry(t *testing.T) {
	reg := NewDefaultContextSourceRegistry()
	src, err := reg.Resolve("lsp.diagnostics")
	if err != nil {
		t.Fatalf("Resolve lsp.diagnostics: %v", err)
	}
	if src.ID() != "lsp.diagnostics" {
		t.Fatalf("ID = %q", src.ID())
	}
	if !src.Deterministic() {
		t.Fatal("lsp.diagnostics must be deterministic (CP-41 no-vector invariant)")
	}
	// Opt-in only: never part of the default enabled set.
	for _, id := range defaultContextSourceIDs {
		if id == "lsp.diagnostics" {
			t.Fatal("lsp.diagnostics must NOT be in defaultContextSourceIDs")
		}
	}
	// Fetch degrades to the clean message with no server running.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	section, err := src.Fetch(ctx, FlowContextHints{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if section.SourceType != "lsp.diagnostics" {
		t.Fatalf("SourceType = %q", section.SourceType)
	}
	if section.SourceRef == "" {
		t.Fatal("SourceRef is required (CP-41 T-3)")
	}
	if !strings.Contains(section.Body, "Current Compiler Diagnostics") ||
		!strings.Contains(section.Body, "No compiler errors detected.") {
		t.Fatalf("body = %q", section.Body)
	}
}
