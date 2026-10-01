package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

// CA-1086: live run-2280 — an armed vibe-tasks chat run ("xin chào", zero
// file delta, no code asked) produced violations=1 rules=[r-requirement]
// and a user-only block even though the run gates warn (Task-455
// chat-surface). The run parked waiting_user → the user's Start-flow
// forward bounced 409 flow_awaiting_user.
//
// Two defects, both reproduced here:
//
//  1. injectVibeSSDrift treats rs.sourceDocID as the locked-SS fallback —
//     but CP-sourced vibe runs pin a CP-*.md (and task/bugfix chats pin
//     Task/BUG docs). Any AC-* token in that doc counts "uncovered" on a
//     turn that wrote no test files, because testSource is empty.
//  2. classifyVibeGateWithDrift routes requirement-class violations to the
//     user-block even when Enforce downgraded the action to "warn" — only
//     owner_debate honored the Task-352 downgrade.

// A CP pinned as sourceDocID is not a locked SS — the signature-drift
// check must not treat its AC-* lines as coverage targets.
func TestCA1086_InjectVibeSSDrift_SkipsCPSourceDoc(t *testing.T) {
	cwd := t.TempDir()
	cp := filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo", "CP-90-live.md")
	if err := os.MkdirAll(filepath.Dir(cp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cp, []byte("Document ID: CP-90\n- AC-1 something\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{
		workingMode:  workingmode.Vibe,
		workspaceCwd: cwd,
		sourceDocID:  "requirements/07-Coding-Plan/todo/CP-90-live.md",
	}
	tr := flowgate.TurnResult{Tests: flowgate.TestOutcome{Ran: true}}
	svc.injectVibeSSDrift(rs, &tr)
	if tr.RequirementDrift {
		t.Fatalf("CP source doc must not seed the SS signature check: %q", tr.RequirementDriftDetail)
	}
}

// A locked SS exists but the turn wrote no test sources — there is nothing
// to compare; every AC would falsely read "uncovered".
func TestCA1086_InjectVibeSSDrift_SkipsZeroTestWrites(t *testing.T) {
	cwd := t.TempDir()
	ss := filepath.Join(cwd, "SS-18.md")
	if err := os.WriteFile(ss, []byte("- AC-9 missing mapping\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{workingMode: workingmode.Vibe, vibeLockedSS: ss, workspaceCwd: cwd}
	tr := flowgate.TurnResult{Tests: flowgate.TestOutcome{Ran: true}}
	svc.injectVibeSSDrift(rs, &tr)
	if tr.RequirementDrift {
		t.Fatalf("zero test writes cannot produce signature drift: %q", tr.RequirementDriftDetail)
	}
}

// The sourceDocID fallback survives for real SS docs — a chat armed
// directly with an SS-*.md still gets the drift check.
func TestCA1086_InjectVibeSSDrift_KeepsSpecSourceDoc(t *testing.T) {
	cwd := t.TempDir()
	ssDir := filepath.Join(cwd, "requirements", "05-System-Specs")
	if err := os.MkdirAll(ssDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "requirements/05-System-Specs/SS-9-locked.md"
	if err := os.WriteFile(filepath.Join(cwd, rel), []byte("- AC-3 needs a test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testPath := filepath.Join(cwd, "foo_test.go")
	if err := os.WriteFile(testPath, []byte("func TestFoo(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{workingMode: workingmode.Vibe, workspaceCwd: cwd, sourceDocID: rel}
	tr := flowgate.TurnResult{
		Tests:        flowgate.TestOutcome{Ran: true},
		WrittenPaths: []string{testPath},
	}
	svc.injectVibeSSDrift(rs, &tr)
	if !tr.RequirementDrift {
		t.Fatal("SS source doc must still feed the signature-drift check")
	}
}

// Task-455 + Task-352: on a warn-gated (chat-surface) run EVERY
// violation-routed escalation passes through — requirement-class included.
// The warn event still surfaces to the user (BR-4 routing preserved); it
// just cannot park the run into flow_awaiting_user.
func TestCA1086_WarnModeRequirementViolationPassesThrough(t *testing.T) {
	got := classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{
		Action: "warn",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: flowgate.RequirementRuleID, Action: "block"},
		}},
	}, 0)
	if got != vibeGatePassthrough {
		t.Fatalf("warn-mode requirement violation must pass through, got %d", got)
	}
	// Enforce-mode runs keep the user-only block.
	got = classifyVibeGateWithDrift(workingmode.Vibe, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: flowgate.RequirementRuleID, Action: "block"},
		}},
	}, 0)
	if got != vibeGateRequirement {
		t.Fatalf("enforce-mode requirement violation must still block to the user, got %d", got)
	}
}
