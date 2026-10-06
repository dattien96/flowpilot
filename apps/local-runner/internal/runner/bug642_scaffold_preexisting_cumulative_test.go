package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// BUG-642 (live run-306526): the scaffold "modified a file that existed at
// contract freeze" check derives its write set from the CUMULATIVE git diff
// vs the contract's BaseSHA. A declared/locked file legitimately changed by
// a sanctioned remediation COMMITTED before the current scaffold turn still
// diffs vs base forever, so r-scaffold-red re-fired on every later scaffold
// turn — operator discharges only cleared the park, never the diff, so the
// owner-debate mount cap eventually wedged the flow (cap gate re-fire ~5x).
//
// Correct behaviour: the check attributes writes to THIS turn — a path
// identical to turnStartGitHead (committed or clean) was not modified by
// this scaffold turn and must not count as scaffold-touched.
func bug642ScaffoldFixture(t *testing.T, dir string) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	prs.workspaceCwd = dir
	svc.mu.Unlock()
	return svc, parent.RunID
}

func bug642Head(t *testing.T, dir string) string {
	t.Helper()
	head, err := captureGitHead(dir)
	if err != nil || head == "" {
		t.Fatalf("captureGitHead: %q, %v", head, err)
	}
	return head
}

// bug642Violations runs the child artifact gate and returns the blocked
// verdict plus every emitted flow-gate violation message.
func bug642Violations(t *testing.T, svc *InteractiveService, rs *interactiveRun) (bool, []string) {
	t.Helper()
	ch := make(chan ProviderEvent, 32)
	rs.subs[7] = ch
	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"existing.go"},
	}, 0)
	var msgs []string
	for {
		select {
		case ev := <-ch:
			if ev.Type == EventFlowGateViolation {
				msgs = append(msgs, ev.Error)
			}
		default:
			return blocked, msgs
		}
	}
}

func bug642FreezeLocked(t *testing.T, dir, parentID, baseHead string) {
	t.Helper()
	freezeP4Contract(t, dir, parentID, "tdd", baseHead, []string{"existing.go"})
	if _, err := changecontract.LockReproduceTestPaths(dir, parentID, "tdd", []string{"existing.go"}, time.Now().UTC()); err != nil {
		t.Fatalf("lock: %v", err)
	}
}

func bug642Flagged(msgs []string) bool {
	for _, m := range msgs {
		if strings.Contains(m, "existed at contract freeze") && strings.Contains(m, "existing.go") {
			return true
		}
	}
	return false
}

func TestScaffoldGateIgnoresRemediationCommittedBeforeTurn(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	// existing.go exists before the freeze → declared pre-existing → locked
	// for the scaffold writer.
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 1 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "pre-existing impl")
	baseHead := bug642Head(t, dir)

	svc, parentID := bug642ScaffoldFixture(t, dir)
	bug642FreezeLocked(t, dir, parentID, baseHead)

	// Sanctioned remediation lands as a COMMIT before the scaffold turn
	// starts (the live run-306526 remediation shape: coder committed the
	// CMake split, then the next scaffold turn re-flagged it).
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 42 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "sanctioned remediation")
	turnStartHead := bug642Head(t, dir)

	rs := newReproduceChildRun(svc, "child-tdd", parentID, dir, turnStartHead, "tdd")
	if _, msgs := bug642Violations(t, svc, rs); bug642Flagged(msgs) {
		t.Fatalf("pre-turn committed remediation must not count as scaffold-touched: %v", msgs)
	}
}

// Guard the other direction: a scaffold turn that rewrites the locked file
// THIS turn (uncommitted) must still trip the check.
func TestScaffoldGateStillFlagsLockedFileWrittenThisTurn(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 1 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "pre-existing impl")
	baseHead := bug642Head(t, dir)

	svc, parentID := bug642ScaffoldFixture(t, dir)
	bug642FreezeLocked(t, dir, parentID, baseHead)

	rs := newReproduceChildRun(svc, "child-tdd", parentID, dir, baseHead, "tdd")
	// The turn itself rewrites the locked file — dirtier than at turn start.
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 9 }\n")

	if _, msgs := bug642Violations(t, svc, rs); !bug642Flagged(msgs) {
		t.Fatalf("a this-turn write to a freeze-locked file must still flag: %v", msgs)
	}
}

// Commit evasion guard: a scaffold that commits its locked-file write
// mid-turn must still block. The commit-reserved guard (Task-242 D-7)
// fires before the scaffold check emits, so the assertion is on the
// blocked verdict — a commit can never smuggle a locked-path write past
// the gate.
func TestScaffoldGateFlagsMidTurnCommitOnLockedFile(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 1 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "pre-existing impl")
	baseHead := bug642Head(t, dir)

	svc, parentID := bug642ScaffoldFixture(t, dir)
	bug642FreezeLocked(t, dir, parentID, baseHead)

	rs := newReproduceChildRun(svc, "child-tdd", parentID, dir, baseHead, "tdd")
	// The turn rewrites AND commits — HEAD moved, diff vs base still shows it.
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 9 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "scaffold smuggle")

	blocked, _ := bug642Violations(t, svc, rs)
	if !blocked {
		t.Fatal("a mid-turn committed write to a locked file must still block")
	}
}

// Fingerprint branch (reviewer M-1): a file dirty at turn START — snapshot
// holds its content fingerprint — is untouched by this turn when the bytes
// still match, even though the cumulative diff vs contract base shows it.
func TestScaffoldGateIgnoresFileDirtyAtTurnStart(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 1 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "pre-existing impl")
	baseHead := bug642Head(t, dir)

	svc, parentID := bug642ScaffoldFixture(t, dir)
	bug642FreezeLocked(t, dir, parentID, baseHead)

	// Pre-turn uncommitted remediation — dirtier than base but identical to
	// the fingerprint captured when this scaffold turn began.
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 42 }\n")
	rs := newReproduceChildRun(svc, "child-tdd", parentID, dir, baseHead, "tdd")
	rs.turnStartWorktree = map[string]string{
		"existing.go": worktreeFileFingerprint(dir, "existing.go"),
	}

	if _, msgs := bug642Violations(t, svc, rs); bug642Flagged(msgs) {
		t.Fatalf("file identical to its turn-start fingerprint must not count as scaffold-touched: %v", msgs)
	}
}

// Counter-arm: a fingerprint mismatch proves the content moved on this
// turn's watch — still flagged even when the git fallback would be clean.
func TestScaffoldGateFlagsTurnStartFingerprintMismatch(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 1 }\n")
	runGitForTest(t, dir, "add", "-A")
	runGitForTest(t, dir, "commit", "-m", "pre-existing impl")
	baseHead := bug642Head(t, dir)

	svc, parentID := bug642ScaffoldFixture(t, dir)
	bug642FreezeLocked(t, dir, parentID, baseHead)

	rs := newReproduceChildRun(svc, "child-tdd", parentID, dir, baseHead, "tdd")
	rs.turnStartWorktree = map[string]string{
		"existing.go": worktreeFileFingerprint(dir, "existing.go"), // clean bytes at turn start
	}
	// Then the turn rewrote it.
	p4WriteFile(t, dir, "existing.go", "package app\n\nfunc Live() int { return 9 }\n")

	if _, msgs := bug642Violations(t, svc, rs); !bug642Flagged(msgs) {
		t.Fatalf("fingerprint mismatch on a locked file must flag: %v", msgs)
	}
}
