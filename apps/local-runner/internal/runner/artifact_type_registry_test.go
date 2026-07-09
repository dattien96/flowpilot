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
// context_artifact input instance's config_json.sources wins even when the
// node also carries a step-level ContextSources and the flow declares a
// contexts.<name>.sources binding (Task-196/Task-194's existing tiers).
func TestResolveEnabledContextSourceIDsArtifactBindingOverridesStepAndFlow(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Contexts: map[string]agentpack.FlowContextBinding{
			"main_context": {Ref: "contexts/flow-context-package.yaml", Sources: []string{"chat.summary"}},
		},
	}
	node := agentpack.FlowNode{
		ID:             "context",
		Outputs:        map[string]string{"main_context": "flow_context_package.v1"},
		ContextSources: []string{"feature.history"},
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{
				Direction:      "input",
				ArtifactTypeID: ArtifactTypeContext,
				ConfigJSON:     map[string]any{"sources": []any{"mcp.driver"}},
			},
		},
	}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "mcp.driver" {
		t.Fatalf("got %v, want artifact-binding [mcp.driver] to win over step- and flow-level", got)
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

// TestResolveArtifactBoundContextSourcesIgnoresOutputAndOtherTypeBindings
// verifies the D-6 lookup only considers input bindings of type
// context_artifact.v1 — an output binding or a differently-typed input
// binding (e.g. file_artifact.v1) must never be mistaken for a context
// source list.
func TestResolveArtifactBoundContextSourcesIgnoresOutputAndOtherTypeBindings(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeContext, ConfigJSON: map[string]any{"sources": []any{"chat.summary"}}},
			{Direction: "input", ArtifactTypeID: ArtifactTypeFile, ConfigJSON: map[string]any{"paths": []any{"README.md"}}},
		},
	}
	if _, ok := resolveArtifactBoundContextSources(node); ok {
		t.Fatal("expected ok=false: no input context_artifact binding present")
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

// TestResolveInputArtifactPromptInjectsFileArtifactContent verifies Task-202's
// end-to-end claim: a file_artifact input binding's content reaches the
// prompt string appended to a consumer node.
func TestResolveInputArtifactPromptInjectsFileArtifactContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte("spec body"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "input", ArtifactTypeID: ArtifactTypeFile, ConfigJSON: map[string]any{"paths": []any{"spec.md"}}},
		},
	}
	got := resolveInputArtifactPrompt(dir, node)
	if !strings.Contains(got, "spec body") {
		t.Fatalf("expected injected prompt to contain file content, got %q", got)
	}
}
