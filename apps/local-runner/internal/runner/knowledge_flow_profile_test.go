package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/promptpacker"
)

// loadHarnessDef loads one embedded-pack flow by id (Task-376 T-3 pattern:
// real YAML, real registry, real builder — no hand-made definitions).
func loadHarnessDef(t *testing.T, flowID string) agentpack.FlowDefinition {
	t.Helper()
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID == flowID {
			return def
		}
	}
	t.Fatalf("flow %q not in builtin pack", flowID)
	return agentpack.FlowDefinition{}
}

func profileSources(t *testing.T, def agentpack.FlowDefinition, profile string) []string {
	t.Helper()
	p, ok := def.ContextProfiles[profile]
	if !ok {
		t.Fatalf("flow %q has no profile %q", def.ID, profile)
	}
	return p.CandidateSources
}

func hasSource(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// TestKnowledgeFlowProfilesDeclareSource pins the P-4 wiring: scout and
// plan_writer opt into knowledge.flow (right after conventions); reviewer
// and coder stay untouched — macro knowledge plans, raw code writes.
func TestKnowledgeFlowProfilesDeclareSource(t *testing.T) {
	for _, flowID := range []string{"task-harness", "bug-plan-harness"} {
		def := loadHarnessDef(t, flowID)
		for _, profile := range []string{"scout", "plan_writer"} {
			got := profileSources(t, def, profile)
			if !hasSource(got, "knowledge.flow") {
				t.Errorf("%s/%s missing knowledge.flow: %v", flowID, profile, got)
			}
			if len(got) < 2 || got[0] != "conventions" || got[1] != "knowledge.flow" {
				t.Errorf("%s/%s order = %v, want [conventions knowledge.flow ...]", flowID, profile, got)
			}
		}
		for _, profile := range []string{"reviewer", "coder"} {
			if got := profileSources(t, def, profile); hasSource(got, "knowledge.flow") {
				t.Errorf("%s/%s must not opt into knowledge.flow: %v", flowID, profile, got)
			}
		}
	}
}

// TestWiredHarnessProfilesValidate pins AC-1: the pack loads and the new
// profile source id resolves — sequencing guaranteed (P-4 after P-2).
func TestWiredHarnessProfilesValidate(t *testing.T) {
	for _, flowID := range []string{"task-harness", "bug-plan-harness"} {
		if err := ValidateFlowContextSources(loadHarnessDef(t, flowID)); err != nil {
			t.Errorf("%s: %v", flowID, err)
		}
	}
}

func buildProfilePackage(t *testing.T, ws string, sources []string) FlowContextPackage {
	t.Helper()
	hints := FlowContextHints{ChangedPaths: []string{"shop/cart/service.go"}}
	pkg, err := BuildFlowContextPackageWithSources(context.Background(), ws, hints, sources)
	if err != nil {
		t.Fatalf("BuildFlowContextPackageWithSources: %v", err)
	}
	return pkg
}

func findSection(pkg FlowContextPackage, sourceType string) *FlowContextSection {
	for i, s := range pkg.Sections {
		if s.SourceType == sourceType {
			return &pkg.Sections[i]
		}
	}
	return nil
}

// TestFlowPlanWriterReceivesExecutionFlowContext pins AC-2/DOD-4: the
// plan_writer profile renders the distilled Checkout section (right flow,
// ~500 tokens, no unrelated flow) instead of 40–60k tokens of raw code.
func TestFlowPlanWriterReceivesExecutionFlowContext(t *testing.T) {
	ws := t.TempDir()
	seedKnowledgeFixture(t, ws)
	def := loadHarnessDef(t, "task-harness")
	pkg := buildProfilePackage(t, ws, profileSources(t, def, "plan_writer"))
	sec := findSection(pkg, "knowledge.flow")
	if sec == nil {
		t.Fatalf("plan_writer package has no knowledge.flow section (sources=%v)",
			profileSources(t, def, "plan_writer"))
	}
	if !strings.Contains(sec.Body, "## Flow: Checkout_Pipeline") {
		t.Errorf("plan_writer missing the checkout flow section:\n%s", sec.Body)
	}
	if strings.Contains(sec.Body, "## Flow: Login_Flow") {
		t.Errorf("plan_writer leaks the unrelated login flow:\n%s", sec.Body)
	}
	if got := promptpacker.EstimateTokens(sec.Body); got > knowledgeFlowSectionCap+100 {
		t.Errorf("plan_writer knowledge body = %d tokens, want ~=500", got)
	}
}

// TestCoderNodeDoesNotReceiveKnowledgeFlow pins AC-3 / the token-economy
// guard: coder plans from real code, never from distilled macro context.
func TestCoderNodeDoesNotReceiveKnowledgeFlow(t *testing.T) {
	ws := t.TempDir()
	seedKnowledgeFixture(t, ws)
	def := loadHarnessDef(t, "task-harness")
	pkg := buildProfilePackage(t, ws, profileSources(t, def, "coder"))
	if sec := findSection(pkg, "knowledge.flow"); sec != nil && strings.TrimSpace(sec.Body) != "" {
		t.Errorf("coder package must not carry knowledge.flow body, got:\n%s", sec.Body)
	}
}

// TestNonOptedInFlowOmitsKnowledgeFlow pins AC-4: flows that never opted in
// render exactly as before (no knowledge.flow section anywhere).
func TestNonOptedInFlowOmitsKnowledgeFlow(t *testing.T) {
	ws := t.TempDir()
	seedKnowledgeFixture(t, ws)
	def := loadHarnessDef(t, "rag-harness")
	for name, profile := range def.ContextProfiles {
		if hasSource(profile.CandidateSources, "knowledge.flow") {
			t.Errorf("rag-harness profile %q unexpectedly opts into knowledge.flow", name)
		}
	}
	var sources []string
	if p, ok := def.ContextProfiles["scout"]; ok {
		sources = p.CandidateSources
	}
	pkg := buildProfilePackage(t, ws, sources)
	if sec := findSection(pkg, "knowledge.flow"); sec != nil && strings.TrimSpace(sec.Body) != "" {
		t.Errorf("non-opted-in flow renders knowledge.flow body:\n%s", sec.Body)
	}
}
