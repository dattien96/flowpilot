package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/promptpacker"
	"flowpilot-runner/internal/workingmode"
)

// Task-341 (CP-62 P-5): per-node context profiles resolve the candidate
// source set + token budget; the catalog tier keeps one-line metadata for
// pruned sections.

func profileDefs() agentpack.FlowDefinition {
	return agentpack.FlowDefinition{
		ID: "task-harness",
		ContextProfiles: map[string]agentpack.ContextProfile{
			"scout":    {Name: "scout", CandidateSources: []string{"canonical.head", "feature.history"}, MaxTokens: 6000},
			"reviewer": {Name: "reviewer", CandidateSources: []string{"change.contract", "source.excerpt"}, MaxTokens: 12000},
		},
		Nodes: []agentpack.FlowNode{
			{ID: "preflight_contract_plan", ContextProfile: "scout"},
			{ID: "reviewer", ContextProfile: "reviewer"},
			{ID: "implement"},
		},
	}
}

// Scenario: Resolver chọn đúng tập nguồn dữ liệu ứng viên theo profile của node
func TestContextProfile_ResolverSelectsCandidateSet(t *testing.T) {
	def := profileDefs()
	got := resolveEnabledContextSourceIDs(def, def.Nodes[1]) // reviewer
	want := []string{"change.contract", "source.excerpt"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// Scenario: Node không khai báo context_profile -> fallback về main_context nguyên bản
func TestContextProfile_MissingProfile_FallbackMainContext(t *testing.T) {
	def := profileDefs()
	if got := resolveEnabledContextSourceIDs(def, def.Nodes[2]); got != nil {
		t.Fatalf("no profile + no node sources = default (nil), got %v", got)
	}
	node := agentpack.FlowNode{ID: "x", ContextSources: []string{"chat.summary"}}
	if got := resolveEnabledContextSourceIDs(def, node); len(got) != 1 || got[0] != "chat.summary" {
		t.Fatalf("node sources fallback broken: %v", got)
	}
}

// Scenario: Artifact binding vẫn là tier cao nhất (profile không phá CP-45)
func TestContextProfile_ArtifactBindingWins(t *testing.T) {
	def := profileDefs()
	node := agentpack.FlowNode{
		ID:             "reviewer",
		ContextProfile: "reviewer",
		ArtifactBindings: []agentpack.FlowArtifactBinding{{
			Direction:      "output",
			ArtifactTypeID: ArtifactTypeContext,
			ConfigJSON:     map[string]any{"sources": []any{"chat.summary"}},
		}},
	}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "chat.summary" {
		t.Fatalf("artifact-bound sources must win over profile: %v", got)
	}
}

// Error: profile ref không tồn tại / profile khai báo source lạ -> fail flow load
func TestContextProfile_UnknownProfileFailsFlowLoad(t *testing.T) {
	def := profileDefs()
	def.Nodes = append(def.Nodes, agentpack.FlowNode{ID: "bad", ContextProfile: "ghost"})
	if err := ValidateFlowContextSources(def); err == nil || !strings.Contains(err.Error(), "unknown context profile") {
		t.Fatalf("err=%v want unknown context profile", err)
	}
	def2 := profileDefs()
	def2.ContextProfiles["bad-profile"] = agentpack.ContextProfile{Name: "bad-profile", CandidateSources: []string{"ghost.source"}}
	if err := ValidateFlowContextSources(def2); err == nil || !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("err=%v want unknown source", err)
	}
}

// Scenario: Hạn mức token của profile được Budget Packer thực thi nghiêm ngặt
// (packing must stay flag-gated; the profile only refines the budget).
func TestContextProfile_TokenCapEnforcedOnProfiles(t *testing.T) {
	budget := defaultBudgetPackerBudget()
	profileBudget := 100
	if profileBudget < budget.TotalMaxTokens {
		budget.TotalMaxTokens = profileBudget
	}
	sections := []promptpacker.PromptSection{
		// Mandatory kinds are retained whole (CP-23 R-1) — keep them small so
		// the prunable sections drive the budget check.
		{Kind: promptpacker.SectionCurrentTask, Title: "task", Content: "do the thing", Priority: 2},
		{Kind: promptpacker.SectionRawExcerpt, Title: "big excerpt", Content: strings.Repeat("x", 4000), Priority: 5},
		{Kind: promptpacker.SectionMemorySummary, Title: "memory", Content: strings.Repeat("m", 4000), Priority: 4},
	}
	packed, report, err := promptpacker.PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if report.SelectedTokens > profileBudget {
		t.Fatalf("selected %d > profile budget %d", report.SelectedTokens, profileBudget)
	}
	if len(report.DroppedItems) == 0 {
		t.Fatalf("expected pruned sections under the tiny profile budget")
	}
	_ = packed
}

// Scenario: Catalog tier hiển thị dòng mô tả 1 dòng trong pack
func TestCatalogTier_OneLineSummaryInPack(t *testing.T) {
	catalog := buildCatalogSummary([]string{"exceeded_section_budget: big excerpt", "exceeded_section_budget: memory"})
	if !strings.Contains(catalog, catalogSummaryHeading) {
		t.Fatalf("catalog missing heading: %q", catalog)
	}
	if !strings.Contains(catalog, "- exceeded_section_budget: big excerpt\n") {
		t.Fatalf("catalog missing dropped item line: %q", catalog)
	}
	if strings.Count(catalog, "\n") != 2 { // heading + 2 items, trailing trimmed
		t.Fatalf("catalog must be one line per item: %q", catalog)
	}
	if buildCatalogSummary(nil) != "" {
		t.Fatalf("empty drops -> empty catalog")
	}
}

// Scenario: profile budget resolve từ run hiện hành (parent node + pack)
func TestContextProfile_ProfileBudgetForRun(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	parentRs := svc.runs[parent.RunID]
	parentRs.activeFlowNodes = []agentpack.FlowNode{{ID: "reviewer", ContextProfile: "reviewer"}}
	parentRs.chatFlowRef = workingmode.PackPrefix + "task-harness"
	svc.mu.Unlock()

	if got := svc.flowNodeProfileBudgetFor(&interactiveRun{id: "run-c", parentRunID: parent.RunID, stepID: "reviewer"}); got != 12000 {
		t.Fatalf("got %d want 12000 (task-harness reviewer profile)", got)
	}
	// No parent → 0.
	if got := svc.flowNodeProfileBudgetFor(&interactiveRun{id: "run-root"}); got != 0 {
		t.Fatalf("root run budget = %d want 0", got)
	}
}
