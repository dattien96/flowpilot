package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// CA-648: the coder's FrozenContractScopeDrift gate flagged tool/skill-pack
// owned scaffold surfaces (.claude/skills/gitnexus/*/SKILL.md, AGENTS.md,
// CLAUDE.md) as drift — the gitnexus skillpack re-installs them mid-flow,
// exactly like run-151954 flagged them at the freeze guard. CA-645 exempted
// them on the freeze side only; the gate needs the same exemption. CA-427's
// security hole (.flowpilot/**-wide exemption) stays closed: gate config
// rewrites must still drift.
//
// Additive file only — legacy suites untouched.

func TestGateDriftIgnoresSkillpackScaffoldMidFlow(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	// Skillpack scaffold lands during the writer's turn (run-151954 repro).
	for _, p := range []string{
		".claude/skills/gitnexus/gitnexus-cli/SKILL.md",
		".claude/skills/gitnexus/gitnexus-debugging/SKILL.md",
		".claude/skills/gitnexus/gitnexus-exploring/SKILL.md",
		".claude/skills/gitnexus/gitnexus-guide/SKILL.md",
		".claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md",
		".claude/skills/gitnexus/gitnexus-refactoring/SKILL.md",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("# skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{".claude/skills/gitnexus/gitnexus-cli/SKILL.md", "AGENTS.md"}}, 0) {
		t.Fatal("writing only tool-owned scaffold files must NOT block the gate")
	}
}

func TestGateDriftStillBlocksRealCodeAlongsideScaffold(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-2", parentID, dir, head)

	p4WriteFile(t, dir, ".claude/skills/gitnexus/gitnexus-cli/SKILL.md", "# skill\n")
	p4WriteFile(t, dir, "AGENTS.md", "# agents\n")
	p4WriteFile(t, dir, "src/surprise.go", "package x\n") // real out-of-scope code

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/surprise.go"}}, 0) {
		t.Fatal("a real out-of-scope code file must still block even with scaffold churn")
	}
}

// TestGateDriftStillDetectsRewrittenGateRulesFile guards CA-427 Finding 2:
// the .flowpilot/** tree must NOT be exempted wholesale — a writer rewriting
// its own gate rules must still drift.
func TestGateDriftStillDetectsRewrittenGateRulesFile(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-3", parentID, dir, head)

	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[{"id":"r-contract","enabled":false}]`)

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/settings/flow-rules.json"}}, 0) {
		t.Fatal("a writer rewriting its own gate rules file must still count as scope drift")
	}
}

func TestIsToolOwnedScaffoldPathMatrix(t *testing.T) {
	excluded := []string{
		".claude", ".claude/skills/gitnexus/gitnexus-cli/SKILL.md",
		".agents", ".agents/skills/x/SKILL.md",
		".grok", ".grok/skills/x/SKILL.md",
		"AGENTS.md", "CLAUDE.md", ".gitignore",
		".\\.claude\\skills\\gitnexus\\SKILL.md", // windows-slash normalized
	}
	for _, p := range excluded {
		if !changecontract.IsToolOwnedScaffoldPath(p) {
			t.Errorf("IsToolOwnedScaffoldPath(%q) = false, want true", p)
		}
	}
	kept := []string{
		"src/calc.go", "calc.go", "user.go", "planner_touched.go",
		".flowpilot", ".flowpilot/settings/flow-rules.json", ".flowpilot/ledger/chat_summary.ndjson",
		".gitnexus", ".gitnexus/meta.json",
		"docs/report.md", "change-audit/CA-123.md",
	}
	for _, p := range kept {
		if changecontract.IsToolOwnedScaffoldPath(p) {
			t.Errorf("IsToolOwnedScaffoldPath(%q) = true, want false", p)
		}
	}
}
