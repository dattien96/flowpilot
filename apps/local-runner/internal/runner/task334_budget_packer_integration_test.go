package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"flowpilot-runner/internal/promptpacker"
)

// task334ComposedPrompt builds a composed-style Flow coding prompt — trusted
// flow-context envelope + Context header + Canonical head + change.contract +
// a source excerpt (excerptLines padded filler lines) + the trailing user
// instruction — the shape runTurn assembles at the Task-334 seam.
func task334ComposedPrompt(excerptLines int) string {
	var sb strings.Builder
	sb.WriteString("[FlowPilot flow context package]\n")
	sb.WriteString("<!-- flowpilot-fcp:run-334:mac -->\n\n")
	sb.WriteString("## Context\n- feature: calc-core\n\n")
	sb.WriteString("## Canonical \"calc-core\" [approved] sig=abc123\nKeep arithmetic in the core package.\n\n")
	sb.WriteString("### change.contract\n\nDeclared paths: internal/calc\n\n")
	sb.WriteString("### Source: internal/calc/calc.go\n\n```go\n")
	for i := 0; i < excerptLines; i++ {
		sb.WriteString("func paddedExcerpt334() int { return 1 }\n")
	}
	sb.WriteString("```\n\n---\n\n[Context: use sections above as feature truth. Stay in scope.]\n\nImplement the add function per the contract.")
	return sb.String()
}

// MVP posture: the packer is always ON and the env flag is ignored —
// but a prompt that fits the budget passes through byte-identical (no
// gratuitous reformatting), so the pre-Task-334 contract still holds for
// every under-budget turn.
func TestTask334_BudgetPackerDisabled_PromptByteIdentical(t *testing.T) {
	t.Setenv(budgetPackerEnvFlag, "0") // env no longer gates — must be ignored
	prompt := task334ComposedPrompt(20)
	s := &InteractiveService{}
	got := s.applyBudgetPackerIfEnabled(nil, prompt, "turn-1")
	if !bytes.Equal([]byte(got), []byte(prompt)) {
		t.Fatalf("under-budget prompt must pass through byte-identical (got %d bytes, want %d)", len(got), len(prompt))
	}
}

// Task-334 DOD: with the flag ON, oversized context gets pruned and the
// prompt_context_audit record is written next to the composed-prompt log.
func TestTask334_BudgetPackerEnabled_PrunesOversizedContextAndWritesAudit(t *testing.T) {
	t.Setenv(budgetPackerEnvFlag, "1")
	if !budgetPackerEnabled() {
		t.Fatalf("budget packer must be enabled when FLOWPILOT_ENABLE_BUDGET_PACKER=1")
	}
	workspace := t.TempDir()
	s := &InteractiveService{runner: &Runner{workspace: workspace}}

	// 9000 filler lines ~ 370KB ~ 92k tokens: far over the 8000-token default
	// budget, forcing the raw excerpt to be pruned (truncated to its cap).
	prompt := task334ComposedPrompt(9000)
	packed := s.applyBudgetPackerIfEnabled(&interactiveRun{id: "run-334", projectID: "proj-334"}, prompt, "turn-1")

	if len(packed) >= len(prompt) {
		t.Fatalf("flag ON must prune oversized context: packed=%d bytes >= original=%d", len(packed), len(prompt))
	}
	if len(packed) > 8000*4 {
		t.Fatalf("packed prompt must stay within the 8000-token budget (~32KB), got %d bytes", len(packed))
	}
	if !strings.Contains(packed, "Implement the add function per the contract.") {
		t.Fatalf("current task instruction must be retained")
	}
	if !strings.Contains(packed, "flowpilot-fcp:run-334") {
		t.Fatalf("trusted flow-context marker must survive packing")
	}
	if !strings.Contains(packed, "Keep arithmetic in the core package.") {
		t.Fatalf("mandatory canonical head must be retained")
	}

	auditPath := filepath.Join(workspace, ".flowpilot", "runs", "proj-334", "run-334", "prompt-context-audit-turn-1.jsonl")
	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("prompt_context_audit record must be written: %v", err)
	}
	if !strings.Contains(string(raw), "exceeded_section_budget") {
		t.Fatalf("audit record must name the drop reason, got: %s", raw)
	}
	if !strings.Contains(string(raw), `"dropped_tokens":`) || !strings.Contains(string(raw), `"section_usage":`) {
		t.Fatalf("audit record must include dropped tokens and section usage, got: %s", raw)
	}
}

// Task-334 integration helper coverage: the Context Resolver maps composed
// blocks to the right PromptSection kinds, and applyBudgetPackerIfEnabled's packing prunes
// only the raw excerpt when over budget while keeping the current task.
func TestTask334_PackTurnPrompt_SplitsSectionsAndPrunesUnderBudget(t *testing.T) {
	prompt := task334ComposedPrompt(10)
	sections := splitPromptIntoSections(prompt)
	gotKinds := make([]promptpacker.SectionKind, 0, len(sections))
	for _, sec := range sections {
		gotKinds = append(gotKinds, sec.Kind)
	}
	wantKinds := []promptpacker.SectionKind{
		promptpacker.SectionMandatoryDoc, // trusted flow-context envelope
		promptpacker.SectionMandatoryDoc, // ## Context header
		promptpacker.SectionMandatoryDoc, // ## Canonical head
		promptpacker.SectionMandatoryDoc, // ### change.contract
		promptpacker.SectionRawExcerpt,   // ### Source: fenced excerpt
		promptpacker.SectionCurrentTask,  // trailing user instruction
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("section kinds = %v, want %v", gotKinds, wantKinds)
	}
	for _, sec := range sections {
		if sec.Kind == promptpacker.SectionRawExcerpt && sec.Title != "internal/calc/calc.go" {
			t.Fatalf("raw excerpt title must carry the source path, got %q", sec.Title)
		}
	}

	// Small budget: the excerpt exceeds its per-kind cap and is truncated, the
	// mandatory context and current task stay whole.
	budget := promptpacker.SectionBudget{TotalMaxTokens: 300, MaxExcerptTokens: 60}
	packed, report, err := promptpacker.PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if report.SelectedTokens > budget.TotalMaxTokens {
		t.Fatalf("SelectedTokens = %d, must be <= %d", report.SelectedTokens, budget.TotalMaxTokens)
	}
	if report.SectionUsage["raw_excerpt"] > budget.MaxExcerptTokens {
		t.Fatalf("SectionUsage[raw_excerpt] = %d, must be <= %d", report.SectionUsage["raw_excerpt"], budget.MaxExcerptTokens)
	}
	if !strings.Contains(packed, "Implement the add function per the contract.") {
		t.Fatalf("current task must survive packing")
	}
	if len(packed) >= len(prompt) {
		t.Fatalf("packed prompt must be smaller than the original (pruned): %d vs %d", len(packed), len(prompt))
	}
	if len(report.DroppedItems) == 0 || !strings.Contains(report.DroppedItems[0], "exceeded_section_budget") {
		t.Fatalf("audit must record the excerpt pruning, got %v", report.DroppedItems)
	}
}

// The helper sectioner keeps fenced code atomic: "## "-style lines inside a
// fenced body must not be split into separate blocks, while the "---"
// separator between context and the user instruction is a real boundary.
func TestTask334_SplitMarkdownBlocks_KeepsFencedBodyAtomic(t *testing.T) {
	prompt := "Intro paragraph\n\n```markdown\n## Inside Fence\n### Also Inside\n```\n\n## Real Heading\n\nbody\n"
	blocks := splitMarkdownBlocks(prompt)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2: %q", len(blocks), blocks)
	}
	// Prose before the fence stays with the fenced body (blank lines do not
	// split blocks — only headings and rules do), but the fence content must
	// not become a block boundary of its own.
	if !strings.HasPrefix(blocks[0], "Intro paragraph") ||
		!strings.Contains(blocks[0], "```markdown") ||
		!strings.Contains(blocks[0], "## Inside Fence") ||
		!strings.Contains(blocks[0], "### Also Inside") {
		t.Fatalf("fenced body must stay atomic inside its block, got %q", blocks[0])
	}
	if !strings.HasPrefix(blocks[1], "## Real Heading") {
		t.Fatalf("heading after the fence must start a new block, got %q", blocks[1])
	}

	// In a composed Flow prompt the "---" seam separates the excerpt from the
	// trailing instruction, and the excerpt's own fence stays intact.
	composed := task334ComposedPrompt(2)
	cblocks := splitMarkdownBlocks(composed)
	var sourceBlock, tailBlock string
	for _, block := range cblocks {
		if strings.HasPrefix(strings.TrimSpace(block), "### Source: ") {
			sourceBlock = block
		}
		if strings.Contains(block, "Implement the add function per the contract.") {
			tailBlock = block
		}
	}
	if sourceBlock == "" || tailBlock == "" || sourceBlock == tailBlock {
		t.Fatalf("source excerpt and current task must be separate blocks")
	}
	if !strings.HasSuffix(strings.TrimSpace(sourceBlock), "```") {
		t.Fatalf("source excerpt fence must stay intact, got %q", sourceBlock)
	}
	if !strings.HasPrefix(strings.TrimSpace(tailBlock), "---") {
		t.Fatalf("current task block must start at the --- seam, got %q", tailBlock)
	}
}
