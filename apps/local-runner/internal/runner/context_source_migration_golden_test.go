package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/changeledger"
)

// TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor is the
// behavior-preserving regression guard for Task-192 (CP-44 P-2). Each golden
// value below was captured verbatim from the pre-refactor BuildFlowContextPackage
// (the direct 3-step hardcoded retrieval) and must still match byte-for-byte
// once the builder is refactored to run through ContextSourceRegistry.Collect.
// If this test ever needs to change, that is a sign the refactor altered
// observable behavior — which Task-192/CP-44 P-2 explicitly forbids.
func TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor(t *testing.T) {
	t.Run("verified_feature_history_only", func(t *testing.T) {
		workspace, _ := fcpFixture(t)
		pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}

		wantHistory := "## Prior work on \"agent-flow-engine\" (oldest → newest — build on the NEWEST, do not undo it)\n" +
			"- [abc1 2026-06] add FlowNode/FlowEdge types Task-089\n" +
			"- [abc2 2026-06] add applyFlowControl state machine Task-090   ← current truth"
		if pkg.HistoryBlock != wantHistory {
			t.Errorf("HistoryBlock mismatch:\ngot:  %q\nwant: %q", pkg.HistoryBlock, wantHistory)
		}
		if pkg.DiscussionBlock != "" {
			t.Errorf("DiscussionBlock = %q, want empty (no chat summary in this fixture)", pkg.DiscussionBlock)
		}
		if pkg.FeatureKey != "agent-flow-engine" || pkg.FeatureConfidence != ConfidenceVerified {
			t.Errorf("feature resolution changed: key=%q confidence=%q", pkg.FeatureKey, pkg.FeatureConfidence)
		}
		if len(pkg.Warnings) != 0 {
			t.Errorf("Warnings = %v, want none for a verified feature with history present", pkg.Warnings)
		}

		wantRender := "## Flow Context Package\n\n" +
			"- **Package ID**: " + pkg.PackageID + "\n" +
			"- **Feature**: agent-flow-engine (confidence: verified)\n" +
			"- **Source doc**: Task-168\n" +
			"- **No vector retrieval used**\n\n" +
			"### Change History\n\n" +
			wantHistory + "\n"
		if got := RenderFlowContextPackage(pkg); got != wantRender {
			t.Errorf("render mismatch:\ngot:  %q\nwant: %q", got, wantRender)
		}
	})

	t.Run("verified_feature_with_chat_summary", func(t *testing.T) {
		workspace, _ := fcpFixture(t)
		dotFP := filepath.Join(workspace, ".flowpilot")
		summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP)
		if err != nil {
			t.Fatal(err)
		}
		if err := summaryLedger.Append([]changeledger.ChatSummaryEntry{
			{FeatureKey: "agent-flow-engine", Summary: "discussed bounded cap semantics"},
		}); err != nil {
			t.Fatal(err)
		}

		pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}

		wantDiscussion := "## Prior discussion on \"agent-flow-engine\" (oldest → newest — keep the newest summary in mind)\n" +
			"- [] discussed bounded cap semantics   ← current discussion"
		if pkg.DiscussionBlock != wantDiscussion {
			t.Errorf("DiscussionBlock mismatch:\ngot:  %q\nwant: %q", pkg.DiscussionBlock, wantDiscussion)
		}

		rendered := RenderFlowContextPackage(pkg)
		if !strings.Contains(rendered, "### Prior Discussion\n\n"+wantDiscussion+"\n") {
			t.Errorf("render missing expected Prior Discussion block, got:\n%s", rendered)
		}
		// Discussion must render after History, matching the original packing order.
		if idx1, idx2 := strings.Index(rendered, "### Change History"), strings.Index(rendered, "### Prior Discussion"); idx1 == -1 || idx2 == -1 || idx1 > idx2 {
			t.Errorf("expected Change History before Prior Discussion, got:\n%s", rendered)
		}
	})

	t.Run("source_excerpts_cap_and_order", func(t *testing.T) {
		workspace, _ := fcpFixture(t)
		bigContent := strings.Repeat("x", perFileExcerptBytes+100)
		if err := os.WriteFile(filepath.Join(workspace, "big.go"), []byte(bigContent), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "small.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		hints := fcpHints(workspace)
		hints.ExplicitSourcePaths = []string{"big.go", "small.go"}
		pkg, err := BuildFlowContextPackage(workspace, hints)
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}

		if len(pkg.SourceExcerpts) != 2 {
			t.Fatalf("SourceExcerpts count = %d, want 2", len(pkg.SourceExcerpts))
		}
		if pkg.SourceExcerpts[0].Path != "big.go" || !pkg.SourceExcerpts[0].BytesCap {
			t.Errorf("excerpt[0] = %#v, want big.go with BytesCap=true", pkg.SourceExcerpts[0])
		}
		if len(pkg.SourceExcerpts[0].Excerpt) != perFileExcerptBytes {
			t.Errorf("excerpt[0] length = %d, want exactly %d (per-file cap)", len(pkg.SourceExcerpts[0].Excerpt), perFileExcerptBytes)
		}
		if pkg.SourceExcerpts[1].Path != "small.go" || pkg.SourceExcerpts[1].Excerpt != "package main\n" || pkg.SourceExcerpts[1].BytesCap {
			t.Errorf("excerpt[1] = %#v, want small.go uncapped", pkg.SourceExcerpts[1])
		}

		rendered := RenderFlowContextPackage(pkg)
		wantBigHeader := "### Source: big.go"
		wantSmallHeader := "### Source: small.go"
		idxBig, idxSmall := strings.Index(rendered, wantBigHeader), strings.Index(rendered, wantSmallHeader)
		if idxBig == -1 || idxSmall == -1 || idxBig > idxSmall {
			t.Errorf("expected big.go excerpt rendered before small.go (declared order preserved), got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "_(excerpt truncated)_") {
			t.Error("expected truncation marker for big.go excerpt")
		}
	})

	t.Run("outside_workspace_path_omitted", func(t *testing.T) {
		workspace, _ := fcpFixture(t)
		hints := fcpHints(workspace)
		hints.ExplicitSourcePaths = []string{"../outside.txt"}
		pkg, err := BuildFlowContextPackage(workspace, hints)
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}
		wantOmitted := []string{"../outside.txt: outside_workspace"}
		if len(pkg.Omitted) != len(wantOmitted) || pkg.Omitted[0] != wantOmitted[0] {
			t.Errorf("Omitted = %v, want %v", pkg.Omitted, wantOmitted)
		}
	})

	t.Run("low_confidence_no_history_injected", func(t *testing.T) {
		workspace, _ := fcpFixture(t)
		hints := fcpHints(workspace)
		hints.UserPrompt = "zzzunknownfeaturexxx"
		pkg, err := BuildFlowContextPackage(workspace, hints)
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}
		if pkg.HistoryBlock != "" || pkg.DiscussionBlock != "" {
			t.Errorf("expected no history/discussion for unresolved feature, got history=%q discussion=%q", pkg.HistoryBlock, pkg.DiscussionBlock)
		}
		wantWarnings := []string{"feature key unresolved for prompt"}
		if len(pkg.Warnings) != len(wantWarnings) || pkg.Warnings[0] != wantWarnings[0] {
			t.Errorf("Warnings = %v, want %v", pkg.Warnings, wantWarnings)
		}
	})

	t.Run("package_id_stable_for_same_inputs", func(t *testing.T) {
		workspace, _ := fcpFixture(t)
		hints := fcpHints(workspace)
		pkg1, err := BuildFlowContextPackage(workspace, hints)
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}
		pkg2, err := BuildFlowContextPackage(workspace, hints)
		if err != nil {
			t.Fatalf("BuildFlowContextPackage: %v", err)
		}
		if pkg1.PackageID != pkg2.PackageID {
			t.Errorf("PackageID not stable: %q vs %q", pkg1.PackageID, pkg2.PackageID)
		}
		if pkg1.PackageID != "fcp-796a6b29" {
			t.Errorf("PackageID = %q, want fcp-796a6b29 (fixed hash of run-168|step-plan-1|agent-flow-engine)", pkg1.PackageID)
		}
	})
}
