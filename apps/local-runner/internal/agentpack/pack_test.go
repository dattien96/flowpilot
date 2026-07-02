package agentpack

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadBuiltinPack(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	if pack.Manifest.ID != "flowpilot-core-flow-pack" {
		t.Fatalf("manifest ID = %q, want flowpilot-core-flow-pack", pack.Manifest.ID)
	}
	if len(pack.Agents) < 4 {
		t.Fatalf("expected at least 4 built-in agents, got %d", len(pack.Agents))
	}
	if len(pack.Flows) != 2 {
		t.Fatalf("expected 2 built-in flows, got %d", len(pack.Flows))
	}
	names := SortedAgentNames(pack.Agents)
	for _, want := range []string{"coder", "reviewer", "synthesizer", "tester"} {
		found := false
		for _, got := range names {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing built-in agent %q in %v", want, names)
		}
	}
}

func TestLoadBuiltinReviewLoopFlow(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var review FlowDefinition
	found := false
	for _, flow := range pack.Flows {
		if flow.ID == "review-loop" {
			review = flow
			found = true
			break
		}
	}
	if !found {
		t.Fatal("review-loop flow missing")
	}
	if review.Mode != "chat" {
		t.Fatalf("Mode = %q, want chat", review.Mode)
	}
	if review.Builtin.Mirror.Source != "builtin" || !review.Builtin.Mirror.Required {
		t.Fatalf("mirror metadata not parsed: %+v", review.Builtin.Mirror)
	}
	if review.Builtin.ChatUI.Placement != "sub_mode_option" {
		t.Fatalf("chatUI placement = %q", review.Builtin.ChatUI.Placement)
	}
	if got := review.Nodes[0].Behavior; got != "agent.delegate" {
		t.Fatalf("coder behavior = %q, want agent.delegate", got)
	}
	if got := review.Nodes[3].Behavior; got != "hub.inline" {
		t.Fatalf("synthesis behavior = %q, want hub.inline", got)
	}
}

func TestLoadBuiltinRAGHarnessFlow(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var rag FlowDefinition
	found := false
	for _, flow := range pack.Flows {
		if flow.ID == "rag-harness" {
			rag = flow
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rag-harness flow missing")
	}
	if rag.Builtin.ChatBaseline != true {
		t.Fatal("rag-harness must be chatBaseline")
	}
	if len(rag.Contexts) == 0 || rag.Contexts["main_context"].Ref != "contexts/flow-context-package.yaml" {
		t.Fatalf("main_context ref not parsed: %+v", rag.Contexts)
	}
	if len(rag.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(rag.Nodes))
	}
	if !strings.EqualFold(rag.Nodes[0].Behavior, "context.produce") {
		t.Fatalf("first node behavior = %q, want context.produce", rag.Nodes[0].Behavior)
	}
}

// TestFlowStartWaitPromptIsDomainFree is the regression test for
// BUG-NOTE-CP42 #26: flow-start-wait.md (shown to the hub of ANY flow that
// auto-spawns an entry node, not just review-loop) used to hardcode
// "coder agent"/"review coordinator"/"review cycle" — misleading language for
// a non-review flow like rag-harness, whose hub is never actually reviewing
// anything.
func TestFlowStartWaitPromptIsDomainFree(t *testing.T) {
	prompt, ok, err := LoadBuiltinPrompt("prompts/flow-start-wait.md")
	if err != nil {
		t.Fatalf("LoadBuiltinPrompt: %v", err)
	}
	if !ok {
		t.Fatal("expected flow-start-wait prompt to be found")
	}
	for _, domainWord := range []string{"coder", "review coordinator", "review cycle"} {
		if strings.Contains(strings.ToLower(prompt.Contents), strings.ToLower(domainWord)) {
			t.Fatalf("flow-start-wait.md contains domain-specific wording %q, want flow-agnostic text: %q", domainWord, prompt.Contents)
		}
	}
}

func TestLoadBuiltinPrompt(t *testing.T) {
	prompt, ok, err := LoadBuiltinPrompt("prompts/auto-reinvoke.md")
	if err != nil {
		t.Fatalf("LoadBuiltinPrompt: %v", err)
	}
	if !ok {
		t.Fatal("expected auto-reinvoke prompt to be found")
	}
	if !strings.Contains(prompt.Contents, "Agent results ready") {
		t.Fatalf("unexpected prompt contents: %q", prompt.Contents)
	}
}

func TestLoadBuiltinCoderReentryPrompt(t *testing.T) {
	prompt, ok, err := LoadBuiltinPrompt("prompts/coder-reentry.md")
	if err != nil {
		t.Fatalf("LoadBuiltinPrompt: %v", err)
	}
	if !ok {
		t.Fatal("expected coder-reentry prompt to be found")
	}
	// BUG-NOTE-CP42 #26: this prompt used to hardcode review/coder-specific
	// language ("Review completed... resubmit the code"), even though it's
	// injected for any flow's continue back-edge reentry, not just review
	// loops — rag-harness's validate->implement continue would show the same
	// review-flavored text for a plain command-validation failure.
	if !strings.Contains(prompt.Contents, "Feedback received") || strings.Contains(prompt.Contents, "Review completed") {
		t.Fatalf("unexpected prompt contents: %q", prompt.Contents)
	}
}

func TestValidateManifestRejectsMissingReference(t *testing.T) {
	fsys := fstest.MapFS{
		"flow-pack/manifest.yaml": &fstest.MapFile{Data: []byte(`
id: test-pack
version: 1
schemaVersion: 1
agents:
  - agents/missing.md
flows: []
tools: []
contexts: []
prompts: []
`)},
	}
	manifest, err := LoadManifestFS(fsys, "flow-pack/manifest.yaml")
	if err != nil {
		t.Fatalf("LoadManifestFS: %v", err)
	}
	if err := ValidateManifestFS(fsys, "flow-pack", manifest); err == nil {
		t.Fatal("expected missing reference validation error")
	}
}

// buildTwoFlowPackFS returns a minimal two-flow pack fixture, with the given
// content substituted into both flow files' bodies (after the shared `id: `
// prefix each caller supplies), for the LoadPackFS-level regression tests
// below.
func buildTwoFlowPackFS(flowAContent, flowBContent string) fstest.MapFS {
	return fstest.MapFS{
		"pack/manifest.yaml": &fstest.MapFile{Data: []byte(`
id: test-pack
version: 1
schemaVersion: 1
agents: []
flows:
  - path: flows/a.yaml
  - path: flows/b.yaml
tools: []
contexts: []
prompts: []
`)},
		"pack/flows/a.yaml": &fstest.MapFile{Data: []byte(flowAContent)},
		"pack/flows/b.yaml": &fstest.MapFile{Data: []byte(flowBContent)},
	}
}

// TestLoadPackFSRejectsDuplicateFlowContentID is the regression test for
// BUG-NOTE-CP42 #5: ValidateManifestFS's seenFlowIDs was keyed by the
// manifest entry's file path, which is inherently unique per entry — it
// never actually checked the flow FILE's own content-level `id:` field. Two
// different files declaring the same `id: review-loop` used to pass
// validation, leaving the resolver/picker ambiguous about which one
// "review-loop" actually meant.
func TestLoadPackFSRejectsDuplicateFlowContentID(t *testing.T) {
	fsys := buildTwoFlowPackFS(
		"id: shared-id\nnodes:\n  - id: n1\n    behavior: agent.delegate\n    agent: agents/x.md\n",
		"id: shared-id\nnodes:\n  - id: n1\n    behavior: agent.delegate\n    agent: agents/x.md\n",
	)
	_, err := LoadPackFS(fsys, "pack")
	if err == nil {
		t.Fatal("expected an error for two flow files declaring the same content-level flow id")
	}
	if !strings.Contains(err.Error(), "shared-id") {
		t.Fatalf("error = %v, want it to mention the duplicated id", err)
	}
}

// TestContextArtifactParsesNestedRenderPromptTemplate is the regression test
// for BUG-NOTE-CP42 #6: the YAML shape is a nested `render.promptTemplate`
// field, not a scalar `render` string. The old code coerced the whole nested
// map via fmt.Sprint into RenderedTemplate, producing a Go stringified map
// ("map[promptTemplate:...]") instead of the actual template path.
func TestContextArtifactParsesNestedRenderPromptTemplate(t *testing.T) {
	ctx, err := contextArtifactFromMap(map[string]any{
		"id": "test_context",
		"render": map[string]any{
			"promptTemplate": "prompts/my-template.md",
		},
	})
	if err != nil {
		t.Fatalf("contextArtifactFromMap: %v", err)
	}
	if ctx.RenderedTemplate != "prompts/my-template.md" {
		t.Fatalf("RenderedTemplate = %q, want %q (not a stringified map)", ctx.RenderedTemplate, "prompts/my-template.md")
	}
}

// TestLoadPackFSRejectsManifestFlowMetadataDrift is the regression test for
// BUG-NOTE-CP42 #22: the manifest and each flow file both independently
// declare selectableIn/chatSubModes/cloneable/editable/chatBaseline, but the
// runtime picker/mirror path only ever reads the flow file's own
// builtin.* copy — nothing checked the two stayed in sync. A manifest-only
// edit to one of these values would previously load successfully and
// silently have no effect on actual behavior.
func TestLoadPackFSRejectsManifestFlowMetadataDrift(t *testing.T) {
	fsys := fstest.MapFS{
		"pack/manifest.yaml": &fstest.MapFile{Data: []byte(`
id: test-pack
version: 1
schemaVersion: 1
agents: []
flows:
  - path: flows/a.yaml
    editable: false
    cloneable: true
    selectableIn:
      - chat
    chatSubModes:
      - bug
tools: []
contexts: []
prompts: []
`)},
		"pack/flows/a.yaml": &fstest.MapFile{Data: []byte(`
id: flow-a
builtin:
  editable: false
  cloneable: true
  selectableIn:
    - flow
  chatSubModes:
    - bug
nodes:
  - id: n1
    behavior: agent.delegate
    agent: agents/x.md
`)},
	}
	_, err := LoadPackFS(fsys, "pack")
	if err == nil {
		t.Fatal("expected an error for manifest selectableIn disagreeing with the flow file's builtin.selectableIn")
	}
	if !strings.Contains(err.Error(), "selectableIn") {
		t.Fatalf("error = %v, want it to mention the disagreeing selectableIn field", err)
	}
}

// TestLoadPackFSRejectsPromptRefNotDeclaredInManifest is the regression test
// for the Task-173 T-8 half of BUG-NOTE-CP42 #6: a context's
// render.promptTemplate (or a node's promptTemplate) must resolve to an
// entry the manifest actually declares under `prompts:`, not just any file
// that happens to exist on disk.
func TestLoadPackFSRejectsPromptRefNotDeclaredInManifest(t *testing.T) {
	fsys := fstest.MapFS{
		"pack/manifest.yaml": &fstest.MapFile{Data: []byte(`
id: test-pack
version: 1
schemaVersion: 1
agents: []
flows:
  - path: flows/a.yaml
tools: []
contexts:
  - contexts/c.yaml
prompts: []
`)},
		"pack/flows/a.yaml": &fstest.MapFile{Data: []byte("id: flow-a\nnodes:\n  - id: n1\n    behavior: agent.delegate\n    agent: agents/x.md\n")},
		"pack/contexts/c.yaml": &fstest.MapFile{Data: []byte(`
id: c1
render:
  promptTemplate: prompts/undeclared.md
`)},
	}
	_, err := LoadPackFS(fsys, "pack")
	if err == nil {
		t.Fatal("expected an error for a promptTemplate ref not declared in manifest.prompts")
	}
	if !strings.Contains(err.Error(), "undeclared.md") {
		t.Fatalf("error = %v, want it to mention the undeclared prompt ref", err)
	}
}

// TestValidateFlowDefinitionRejectsDuplicateContinueBackEdge is the
// regression test for BUG-NOTE-CP42 #25: resolveContinueBackEdgeTarget can
// only ever pick the first declared match when more than one (kind=back,
// when=continue) edge exists — nothing in the live path disambiguates by
// which node emitted the signal. A pack declaring two such edges used to
// pass validation and silently route "continue" to whichever was declared
// first.
func TestValidateFlowDefinitionRejectsDuplicateContinueBackEdge(t *testing.T) {
	def := FlowDefinition{
		ID: "dup-back-edge",
		Nodes: []FlowNode{
			{ID: "a", Behavior: "agent.delegate", Agent: "agents/a.md"},
			{ID: "b", Behavior: "agent.delegate", Agent: "agents/b.md"},
			{ID: "hub", Behavior: "hub.inline"},
		},
		Edges: []FlowEdge{
			{From: "hub", To: "a", When: "continue", Kind: "back"},
			{From: "hub", To: "b", When: "continue", Kind: "back"},
		},
	}
	if err := ValidateFlowDefinition(def); err == nil {
		t.Fatal("expected an error for duplicate (kind=back, when=continue) edges")
	}
}

// TestValidateFlowDefinitionRejectsMissingDependsOnReference is the
// regression test for BUG-NOTE-CP42 #33: dependsOn references another
// node's id within the same flow, but this was never actually checked — a
// typo'd dependsOn entry used to silently pass load/mirror instead of
// failing fast.
func TestValidateFlowDefinitionRejectsMissingDependsOnReference(t *testing.T) {
	def := FlowDefinition{
		ID: "bad-dependson",
		Nodes: []FlowNode{
			{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md", DependsOn: []string{"cotnext"}}, // typo
		},
	}
	if err := ValidateFlowDefinition(def); err == nil {
		t.Fatal("expected an error for a dependsOn entry referencing a missing node id")
	}
}

func TestLoadAgentSpecParsesMarkdownFrontmatter(t *testing.T) {
	spec, err := LoadAgentSpecFS(embeddedPackFS, "flow-pack/agents/coder.md")
	if err != nil {
		t.Fatalf("LoadAgentSpecFS: %v", err)
	}
	if spec.Name != "coder" {
		t.Fatalf("agent name = %q, want coder", spec.Name)
	}
	if len(spec.Tools) < 3 {
		t.Fatalf("expected parsed tools, got %v", spec.Tools)
	}
	if !strings.Contains(spec.SystemPrompt, "implementation agent") {
		t.Fatalf("unexpected system prompt: %q", spec.SystemPrompt)
	}
}
