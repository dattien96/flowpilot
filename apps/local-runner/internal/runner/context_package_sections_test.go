package runner

import (
	"context"
	"strings"
	"testing"
)

// TestRegisterCustomSourceAppearsInPackageAndRender is the core CP-44 proof:
// a brand-new context source, registered without touching FlowContextPackage
// or RenderFlowContextPackage, shows up in both the package and its rendered
// prompt (CP-44 DOD-3 / Task-193 acceptance).
func TestRegisterCustomSourceAppearsInPackageAndRender(t *testing.T) {
	r := NewDefaultContextSourceRegistry() // fresh registry: 3 built-ins, no global state touched
	custom := &fakeContextSource{
		id: "custom.thing", priority: 9, deterministic: true,
		section: FlowContextSection{
			SourceType: "custom.thing",
			Priority:   9,
			SourceRef:  "custom-ref-123",
			Body:       "custom body content unique-marker-xyz",
		},
	}
	if err := r.Register(custom); err != nil {
		t.Fatalf("register custom source: %v", err)
	}

	enabled := append(append([]string{}, defaultContextSourceIDs...), "custom.thing")
	sections, warnings := r.Collect(context.Background(), enabled, FlowContextHints{
		FeatureConfidence: ConfidenceUnresolved,
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	pkg := FlowContextPackage{PackageID: "fcp-test"}
	pkg.Sections = sections
	projectContextSections(&pkg, sections)

	var found bool
	for _, s := range pkg.Sections {
		if s.SourceType == "custom.thing" && s.SourceRef == "custom-ref-123" {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom section missing from pkg.Sections: %#v", pkg.Sections)
	}

	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "### custom.thing") {
		t.Errorf("render missing generic section heading, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "custom body content unique-marker-xyz") {
		t.Errorf("render missing custom section body, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "_Source: custom-ref-123_") {
		t.Errorf("render missing custom section source ref, got:\n%s", rendered)
	}
}

// TestPackageProjectionMatchesSections verifies the legacy fields
// (HistoryBlock/DiscussionBlock/SourceExcerpts) are a faithful projection of
// the corresponding Sections entries — not an independent code path.
func TestPackageProjectionMatchesSections(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	var historySection, excerptSection *FlowContextSection
	for i := range pkg.Sections {
		switch ContextSourceID(pkg.Sections[i].SourceType) {
		case ContextSourceFeatureHistory:
			historySection = &pkg.Sections[i]
		case ContextSourceSourceExcerpt:
			excerptSection = &pkg.Sections[i]
		}
	}
	if historySection == nil {
		t.Fatal("expected a feature.history section")
	}
	if historySection.Body != pkg.HistoryBlock {
		t.Errorf("HistoryBlock projection mismatch: section.Body=%q pkg.HistoryBlock=%q", historySection.Body, pkg.HistoryBlock)
	}
	if excerptSection == nil {
		t.Fatal("expected a source.excerpt section")
	}
	if len(excerptSection.Excerpts) != len(pkg.SourceExcerpts) {
		t.Errorf("SourceExcerpts projection mismatch: section has %d, pkg has %d", len(excerptSection.Excerpts), len(pkg.SourceExcerpts))
	}
}

// TestRenderFlowContextPackageStillHasNoVectorLine guards the CP-41 audit
// line survives the Sections-based render path.
func TestRenderFlowContextPackageStillHasNoVectorLine(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "No vector retrieval used") {
		t.Error("rendered package must still contain 'No vector retrieval used'")
	}
}

// TestPackageIDUnaffectedBySections verifies PackageID is computed only from
// (run, step, feature) and does not change when Sections content changes —
// audit (Task-171) and retry (Task-170) traceability depend on this identity
// staying stable regardless of what sources are enabled.
func TestPackageIDUnaffectedBySections(t *testing.T) {
	workspace, _ := fcpFixture(t)
	hints := fcpHints(workspace)
	pkg1, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	// Same run/step/feature but with extra explicit source paths, so Sections
	// content differs (more excerpts) — PackageID must still match.
	hints2 := hints
	hints2.ExplicitSourcePaths = []string{"nonexistent-file.go"}
	pkg2, err := BuildFlowContextPackage(workspace, hints2)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	if pkg1.PackageID != pkg2.PackageID {
		t.Errorf("PackageID changed when Sections content changed: %q vs %q", pkg1.PackageID, pkg2.PackageID)
	}
	if len(pkg1.Sections) == 0 || len(pkg2.Sections) == 0 {
		t.Fatal("expected both packages to have sections")
	}
}

// TestAuditDraftStillReadsFeatureKeyAndSourceDocFromPackage verifies
// BuildAuditDraft (Task-171) — a consumer that predates Sections — still
// reads FeatureKey/SourceDocIDs correctly from a Sections-carrying package.
func TestAuditDraftStillReadsFeatureKeyAndSourceDocFromPackage(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if len(pkg.Sections) == 0 {
		t.Fatal("expected pkg.Sections to be populated")
	}

	draft := BuildAuditDraft(AuditDraftInput{
		WorkflowRunID:   pkg.WorkflowRunID,
		ContextPackage:  pkg,
		ValidationState: FlowValidationRetryState{Status: "passed"},
		ChangeType:      "feature",
		WhatChanged:     "did the thing",
		Workspace:       workspace,
	})
	if draft.FeatureKey != pkg.FeatureKey {
		t.Errorf("draft.FeatureKey = %q, want %q", draft.FeatureKey, pkg.FeatureKey)
	}
	if draft.SourceDocID != pkg.SourceDocIDs[0] {
		t.Errorf("draft.SourceDocID = %q, want %q", draft.SourceDocID, pkg.SourceDocIDs[0])
	}
	if draft.OriginalPackageID != pkg.PackageID {
		t.Errorf("draft.OriginalPackageID = %q, want %q", draft.OriginalPackageID, pkg.PackageID)
	}
}
