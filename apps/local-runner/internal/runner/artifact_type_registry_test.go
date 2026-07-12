package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestResolveEnabledContextSourceIDsArtifactBindingOverridesStepAndFlow
// verifies CP-45/SD-23 D-6's new highest precedence tier: a bound
// context_artifact OUTPUT instance's config_json.sources (the context node
// outputs a context_artifact — SD-23 D-5) wins even when the node also
// carries a step-level ContextSources (Task-196's existing tier).
func TestResolveEnabledContextSourceIDsArtifactBindingOverridesStepAndFlow(t *testing.T) {
	def := agentpack.FlowDefinition{ID: "test-flow"}
	node := agentpack.FlowNode{
		ID:             "context",
		ContextSources: []string{"feature.history"},
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{
				Direction:      "output",
				ArtifactTypeID: ArtifactTypeContext,
				ConfigJSON:     map[string]any{"sources": []any{"mcp.driver"}},
			},
		},
	}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "mcp.driver" {
		t.Fatalf("got %v, want artifact-binding [mcp.driver] to win over step-level", got)
	}
}

// TestResolveEnabledContextSourceIDsFallsThroughWhenNoArtifactBinding
// verifies a node with no artifact binding at all resolves exactly as it did
// before CP-45 (SD-23 D-6 soft migration): no regression to the CP-44
// fallback chain this function already implemented.
func TestResolveEnabledContextSourceIDsFallsThroughWhenNoArtifactBinding(t *testing.T) {
	def := agentpack.FlowDefinition{ID: "test-flow"}
	node := agentpack.FlowNode{ID: "context", ContextSources: []string{"source.excerpt"}}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "source.excerpt" {
		t.Fatalf("got %v, want step-level [source.excerpt] unchanged", got)
	}
}

// TestResolveArtifactBoundContextSourcesIgnoresInputAndOtherTypeBindings
// verifies the D-6 lookup only considers OUTPUT bindings of type
// context_artifact.v1 — an input binding (a consumer's own binding to that
// same instance, e.g. on a downstream coder/reviewer node) or a
// differently-typed binding (e.g. file_artifact.v1) must never be mistaken
// for a context source list on the producing node.
func TestResolveArtifactBoundContextSourcesIgnoresInputAndOtherTypeBindings(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "input", ArtifactTypeID: ArtifactTypeContext, ConfigJSON: map[string]any{"sources": []any{"chat.summary"}}},
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, ConfigJSON: map[string]any{"paths": []any{"README.md"}}},
		},
	}
	if _, ok := resolveArtifactBoundContextSources(node); ok {
		t.Fatal("expected ok=false: no output context_artifact binding present")
	}
}

// TestResolveArtifactBoundMCPDriverRefReadsConfiguredFileID is the regression
// test for Task-204 Q-2: a context_artifact.v1 OUTPUT binding's
// config_json.mcpDriverFileId is what threads into FlowContextHints.MCPDriverRef.
func TestResolveArtifactBoundMCPDriverRefReadsConfiguredFileID(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{
				Direction:      "output",
				ArtifactTypeID: ArtifactTypeContext,
				ConfigJSON:     map[string]any{"sources": []any{"mcp.driver"}, "mcpDriverFileId": "  drive-file-abc  "},
			},
		},
	}
	ref, ok := resolveArtifactBoundMCPDriverRef(node)
	if !ok {
		t.Fatal("expected ok=true: mcpDriverFileId is configured")
	}
	if ref != "drive-file-abc" {
		t.Fatalf("ref = %q, want trimmed %q", ref, "drive-file-abc")
	}
}

// TestResolveArtifactBoundMCPDriverRefEmptyWhenUnconfigured verifies the
// no-op case: no bound instance, or one with no mcpDriverFileId set, is a
// normal "nothing configured" — not an error.
func TestResolveArtifactBoundMCPDriverRefEmptyWhenUnconfigured(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeContext, ConfigJSON: map[string]any{"sources": []any{"feature.history"}}},
		},
	}
	if _, ok := resolveArtifactBoundMCPDriverRef(node); ok {
		t.Fatal("expected ok=false: no mcpDriverFileId configured")
	}
	if _, ok := resolveArtifactBoundMCPDriverRef(agentpack.FlowNode{}); ok {
		t.Fatal("expected ok=false for a node with no artifact bindings at all")
	}
}

func TestArtifactTypeRegistryRegisterRejectsDuplicate(t *testing.T) {
	r := NewArtifactTypeRegistry()
	if err := r.Register(&fileArtifactResolver{}); err != nil {
		t.Fatalf("unexpected error on first register: %v", err)
	}
	if err := r.Register(&fileArtifactResolver{}); err == nil {
		t.Fatal("expected error registering a duplicate artifact type id")
	}
}

func TestArtifactTypeRegistryResolveUnknownFails(t *testing.T) {
	r := NewArtifactTypeRegistry()
	if _, err := r.Resolve("totally.unknown.v1"); err == nil {
		t.Fatal("expected error resolving an unregistered artifact type id")
	}
}

func TestDefaultArtifactTypeRegistryHasFileArtifact(t *testing.T) {
	if _, err := DefaultArtifactTypeRegistry().Resolve(ArtifactTypeFile); err != nil {
		t.Fatalf("expected file_artifact.v1 registered by default: %v", err)
	}
}

// TestFileArtifactResolverReadsWorkspaceSafePaths verifies Task-202's DOD-3:
// the resolver reads bounded content for an in-workspace path and returns a
// SourceRef.
func TestFileArtifactResolverReadsWorkspaceSafePaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hello from file artifact"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	binding := agentpack.FlowArtifactBinding{
		ArtifactInstanceID: "file-1",
		ConfigJSON:         map[string]any{"paths": []any{"notes.md"}},
	}
	result, err := (fileArtifactResolver{}).Resolve(dir, binding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Body, "hello from file artifact") {
		t.Fatalf("body %q missing file content", result.Body)
	}
	if result.SourceRef == "" {
		t.Fatal("expected non-empty SourceRef")
	}
}

// TestFileArtifactResolverRejectsOutsideWorkspacePath verifies Task-202's
// DOD-4: an outside-workspace path is omitted with a clear reason (reusing
// readSourceExcerpts' guard), never silently read.
func TestFileArtifactResolverRejectsOutsideWorkspacePath(t *testing.T) {
	dir := t.TempDir()
	binding := agentpack.FlowArtifactBinding{
		ArtifactInstanceID: "file-1",
		ConfigJSON:         map[string]any{"paths": []any{"../outside.md"}},
	}
	result, err := (fileArtifactResolver{}).Resolve(dir, binding)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Body != "" {
		t.Fatalf("expected empty body for outside-workspace path, got %q", result.Body)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "outside_workspace") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an outside_workspace warning, got %v", result.Warnings)
	}
}

// TestResolveInputArtifactPromptSkipsContextArtifactBindings verifies
// context_artifact bindings are never routed through ArtifactTypeRegistry —
// they stay on the dedicated context.produce/render pipeline (SD-23 D-5).
func TestResolveInputArtifactPromptSkipsContextArtifactBindings(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "input", ArtifactTypeID: ArtifactTypeContext, ConfigJSON: map[string]any{"sources": []any{"chat.summary"}}},
		},
	}
	got := resolveInputArtifactPrompt(t.TempDir(), node)
	if got != "" {
		t.Fatalf("expected no prompt injection for a context_artifact binding, got %q", got)
	}
}

// TestResolveInputArtifactPromptMentionsFilePathsOnly verifies BUG-276: INPUT
// file_artifact injects path mentions only — not full file body.
func TestResolveInputArtifactPromptMentionsFilePathsOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("spec body unique-xyz"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "input", ArtifactTypeID: ArtifactTypeFile, ConfigJSON: map[string]any{"paths": []any{"spec.md"}}},
		},
	}
	got := resolveInputArtifactPrompt(dir, node)
	if !strings.Contains(got, "spec.md") {
		t.Fatalf("expected path mention, got %q", got)
	}
	if strings.Contains(got, "unique-xyz") || strings.Contains(got, "spec body") {
		t.Fatalf("BUG-276: must not paste file body into prompt, got %q", got)
	}
}

func TestRequiredFileArtifactOutputPathsExtractsRequiredOutputBindings(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"docs/coder-summary.md", "docs/coder-summary.md"}}},
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: false, ConfigJSON: map[string]any{"paths": []any{"optional.md"}}},
			{Direction: "input", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"should-not-include.md"}}},
			{Direction: "output", ArtifactTypeID: ArtifactTypeContext, Required: true, ConfigJSON: map[string]any{"sources": []any{"chat.summary"}}},
		},
	}
	got := requiredFileArtifactOutputPaths(node)
	if len(got) != 1 || got[0] != "docs/coder-summary.md" {
		t.Fatalf("got %v, want [docs/coder-summary.md]", got)
	}
}

func TestRequiredFileArtifactOutputPromptListsPaths(t *testing.T) {
	// Task-225: paths-only — write contract without global What/Why/Baseline.
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"docs/coder-summary.md"}}},
		},
	}
	got := appendRequiredOutputArtifactPrompt("base", node)
	if !strings.Contains(got, "Required file outputs") || !strings.Contains(got, "docs/coder-summary.md") {
		t.Fatalf("expected write-contract section, got %q", got)
	}
	for _, ban := range []string{"## What", "## Why", "## Baseline", "Past decisions already closed"} {
		if strings.Contains(got, ban) {
			t.Fatalf("paths-only OUTPUT must not include global template %q, got %q", ban, got)
		}
	}
}

func TestFileArtifactStructureParsePathsOnly(t *testing.T) {
	if secs := fileArtifactStructureFromConfig(map[string]any{"paths": []any{"a.md"}}); len(secs) != 0 {
		t.Fatalf("paths-only should have no structure, got %v", secs)
	}
}

func TestFileArtifactStructureParseSections(t *testing.T) {
	cfg := map[string]any{
		"paths": []any{"docs/coder-summary.md"},
		"structure": map[string]any{
			"kind":     "markdown_sections",
			"sections": []any{"What", "Why", "Baseline"},
		},
	}
	secs := fileArtifactStructureFromConfig(cfg)
	if len(secs) != 3 || secs[0] != "What" || secs[1] != "Why" || secs[2] != "Baseline" {
		t.Fatalf("got %v", secs)
	}
	// Unknown kind ignored.
	if got := fileArtifactStructureFromConfig(map[string]any{
		"structure": map[string]any{"kind": "other", "sections": []any{"What"}},
	}); len(got) != 0 {
		t.Fatalf("unknown kind should be empty, got %v", got)
	}
}

func TestAppendRequiredOutputArtifactPromptWithStructureIncludesSections(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{
				"paths": []any{"docs/coder-summary.md"},
				"structure": map[string]any{
					"kind":     "markdown_sections",
					"sections": []any{"What", "Why", "Baseline"},
				},
			}},
		},
	}
	got := appendRequiredOutputArtifactPrompt("base", node)
	for _, want := range []string{"docs/coder-summary.md", "## What", "## Why", "## Baseline", "Past decisions already closed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("structured OUTPUT should include %q, got %q", want, got)
		}
	}
}

func TestRequiredStructuredFileArtifactOutputsMergesSamePath(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{
				"paths":     []any{"docs/out.md"},
				"structure": map[string]any{"kind": "markdown_sections", "sections": []any{"What", "Why"}},
			}},
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{
				"paths":     []any{"docs/out.md"},
				"structure": map[string]any{"kind": "markdown_sections", "sections": []any{"Why", "Baseline"}},
			}},
		},
	}
	got := requiredStructuredFileArtifactOutputs(node)
	if len(got) != 1 || got[0].Path != "docs/out.md" {
		t.Fatalf("got %+v", got)
	}
	// Merge should keep What, Why, Baseline (first-seen order).
	want := []string{"What", "Why", "Baseline"}
	if len(got[0].Sections) != 3 {
		t.Fatalf("sections = %v, want %v", got[0].Sections, want)
	}
	for i, s := range want {
		if !strings.EqualFold(got[0].Sections[i], s) {
			t.Fatalf("sections = %v, want %v", got[0].Sections, want)
		}
	}
	// Prompt must still include all merged sections.
	prompt := appendRequiredOutputArtifactPrompt("base", node)
	for _, sec := range want {
		if !strings.Contains(prompt, "## "+sec) {
			t.Fatalf("prompt missing merged section %q: %s", sec, prompt)
		}
	}
}

func TestFileArtifactStructureIgnoresFormatField(t *testing.T) {
	// Stale format key must not invent sections; only structure does.
	cfg := map[string]any{
		"paths":  []any{"a.md"},
		"format": "coder_decision_memo",
	}
	if secs := fileArtifactStructureFromConfig(cfg); len(secs) != 0 {
		t.Fatalf("format field must be ignored, got %v", secs)
	}
}

func TestBuildFlowReviewHandoffOmitsBodyWhenFileInputBound(t *testing.T) {
	node := agentpack.FlowNode{
		ID: "reviewer",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "input", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"docs/coder-summary.md"}}},
		},
	}
	body := "UNIQUE_CODER_FINAL_MESSAGE_XYZ that must not appear when file input is bound"
	got := buildFlowReviewHandoffPrompt("coder", body, node)
	if strings.Contains(got, "UNIQUE_CODER_FINAL_MESSAGE_XYZ") {
		t.Fatalf("Task-224: must omit coder final body when file INPUT bound, got %q", got)
	}
	if !strings.Contains(got, "[flow-engine] Review this result from node") {
		t.Fatalf("expected review handoff header, got %q", got)
	}
	// Without file INPUT, body is included (truncated).
	plain := agentpack.FlowNode{ID: "reviewer"}
	got2 := buildFlowReviewHandoffPrompt("coder", body, plain)
	if !strings.Contains(got2, "UNIQUE_CODER_FINAL_MESSAGE_XYZ") {
		t.Fatalf("without file INPUT, coder final body should be present, got %q", got2)
	}
}

func TestComposeFlowNodeAgentPromptFileInputIsPathOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deliverable.md"), []byte("from coder unique-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Reviewer node: INPUT only — BUG-276 path mention, no body dump.
	reviewNode := agentpack.FlowNode{
		ID: "reviewer",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "input", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"deliverable.md"}}},
		},
	}
	got := composeFlowNodeAgentPrompt(dir, "review base", reviewNode)
	if !strings.Contains(got, "read with tools") || !strings.Contains(got, "deliverable.md") {
		t.Fatalf("review prompt should mention path and tools instruction, got %q", got)
	}
	if strings.Contains(got, "unique-body") || strings.Contains(got, "from coder") {
		t.Fatalf("BUG-276: review prompt must not paste file body, got %q", got)
	}
	if strings.Contains(got, "Required file outputs") {
		t.Fatalf("review INPUT-only node should not get write contract, got %q", got)
	}
	// Coder node: OUTPUT write contract (file may not exist yet).
	coderNode := agentpack.FlowNode{
		ID: "coder",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"docs/coder-summary.md"}}},
		},
	}
	gotCoder := composeFlowNodeAgentPrompt(dir, "code base", coderNode)
	if !strings.Contains(gotCoder, "Required file outputs") || !strings.Contains(gotCoder, "docs/coder-summary.md") {
		t.Fatalf("coder prompt should list required outputs, got %q", gotCoder)
	}
}
