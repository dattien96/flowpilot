package runner

// BUG-627 (live run-174243): the contract-write guards bind a STEP — the
// scaffold/coder child's label — while the hub run carries neither a
// parentRunID nor a step label, so its writes slid through every guard.
// A post-debate reprompt turned the hub into the "materializing node" and it
// edited VaultContainer.h — a path the tdd contract had frozen read-only —
// which the planner-purity fingerprint then flagged, fueling the
// restore↔remediate unwindable loop.
//
// BUG-626 (same run): the remediation reprompt carried the debate verdict
// verbatim — including a demand to edit a contract-frozen path — with no
// contract-scope context, so the demand looked executable. The reprompt must
// name the gated step's read-only surface and forbid edits to it; the hub
// fallback prompt must route artifact work to the owning leg, never to the
// hub itself.

import (
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// newBug627HubFixture builds a hub run (no parentRunID) whose flow froze a
// scaffold contract over [existing.go, new_stub.go], with existing.go
// additionally read-only (the freeze-time pre-existing split).
func newBug627HubFixture(t *testing.T) (svc *InteractiveService, hub *interactiveRun, dir string) {
	t.Helper()
	var head string
	dir, head = newContractFreezeTestRepo(t)
	svc = newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	freezeP4Contract(t, dir, parent.RunID, "tdd", head, []string{"existing.go", "new_stub.go"})
	freezeP4Contract(t, dir, parent.RunID, "coder", head, []string{"impl.go"})
	if _, err := changecontract.LockReproduceTestPaths(dir, parent.RunID, "tdd", []string{"existing.go"}, time.Now().UTC()); err != nil {
		t.Fatalf("lock pre-existing path on scaffold record: %v", err)
	}
	return svc, svc.runs[parent.RunID], dir
}

// A hub write aimed at any path an active contract owns — read-only or plain
// declared — must be silent-denied at the bridge: the hub is the orchestrator,
// never a materializing leg.
func TestBUG627_HubWriteToContractOwnedPathDenied(t *testing.T) {
	svc, hub, _ := newBug627HubFixture(t)

	// Read-only frozen path (the live VaultContainer.h shape).
	if d, r, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Write"}); !handled || d != "deny" {
		t.Fatalf("hub write to read-only contract path must be denied, got handled=%v d=%q r=%q", handled, d, r)
	}
	// Declared-but-not-read-only: still owned by the tdd leg — the hub must
	// not create the scaffold's stub for it.
	if d, _, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "file", Command: "new_stub.go", Reason: "Write"}); !handled || d != "deny" {
		t.Fatal("hub write to a contract-declared path must be denied — the owning leg creates it")
	}
	// Paths owned by a DIFFERENT step's contract bind the hub all the same.
	if d, _, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "file", Command: "impl.go", Reason: "Write"}); !handled || d != "deny" {
		t.Fatal("hub write to the coder contract's declared path must be denied")
	}
	// A path no contract owns stays writable for the hub (docs, notes).
	if _, _, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "file", Command: "docs/notes.md", Reason: "Write"}); handled {
		t.Fatal("hub write to a non-contract path must not be handled by the contract guard")
	}
	// Reads stay allowed — the hub must read the files it orchestrates.
	if _, _, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Read"}); handled {
		t.Fatal("hub read of a locked file must stay allowed")
	}
	// Shell bypass on an owned path is denied too.
	if _, _, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "exec", Command: "cat > existing.go <<EOF\npackage app\nEOF", Reason: "Bash"}); !handled {
		t.Fatal("an exec mutation naming an owned file must be denied")
	}
	// Read-only shell work falls through.
	if _, _, handled := svc.decideHubContractScopeLock(hub, ApprovalDetails{Kind: "exec", Command: "cat existing.go", Reason: "Bash"}); handled {
		t.Fatal("a read-only exec must not be handled")
	}
}

// The hub guard is hub-scoped: child runs carry parentRunID and are already
// bound by the step-scoped guards; a run with no contracts is untouched.
func TestBUG627_HubGuardScopeBoundaries(t *testing.T) {
	svc, hub, dir := newBug627HubFixture(t)
	svc.mu.Lock()
	hubID := hub.id
	svc.mu.Unlock()

	child := newReproduceChildRun(svc, "child-tdd", hubID, dir, "head", "tdd")
	if _, _, handled := svc.decideHubContractScopeLock(child, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Write"}); handled {
		t.Fatal("child runs must NOT be handled by the hub guard — the step-scoped guards own them")
	}

	// A hub-shaped run in a workspace with no frozen contracts: guard inert.
	emptyDir := t.TempDir()
	lonely, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[lonely.RunID].workspaceCwd = emptyDir
	lonelyRS := svc.runs[lonely.RunID]
	svc.mu.Unlock()
	if _, _, handled := svc.decideHubContractScopeLock(lonelyRS, ApprovalDetails{Kind: "file", Command: "existing.go", Reason: "Write"}); handled {
		t.Fatal("no frozen contracts for the run — the guard must not fire")
	}
}

// The gated child's remediation reprompt must carry its contract's read-only
// surface and forbid edits to it — a verdict demanding a frozen-path change
// is routed back as "belongs to the owning step", not attempted.
func TestBUG626_ResumeRepromptCarriesContractScope(t *testing.T) {
	svc, hub, dir := newBug627HubFixture(t)
	svc.mu.Lock()
	hubID := hub.id
	svc.mu.Unlock()

	prompt := svc.vibeDebateResumeRepromptPrompt(hubID, "tdd", dir, []VerdictRow{
		{ACID: "AC-1", Verdict: "add the missing guard implementation to existing.go"},
	})
	if !strings.Contains(prompt, "existing.go") {
		t.Fatalf("reprompt must name the gated step's read-only paths, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "read-only") {
		t.Fatalf("reprompt must state the paths are read-only under the contract, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "owning step") {
		t.Fatalf("reprompt must route frozen-path demands to the owning step, got:\n%s", prompt)
	}

	// A gated step with no contract keeps the legacy prompt shape — the
	// scope clause is contract-derived, not boilerplate.
	plain := svc.vibeDebateResumeRepromptPrompt(hubID, "synthesis", dir, []VerdictRow{{ACID: "AC-1", Verdict: "rework"}})
	if strings.Contains(plain, "read-only") {
		t.Fatalf("no contract for the step — no read-only clause expected, got:\n%s", plain)
	}
}

// The hub fallback resume prompt (no gated child recorded) must pin the
// orchestrator role: never write contract-owned artifact files itself —
// enumerate them when contracts exist so the constraint is concrete.
func TestBUG626_HubResumePromptRoutesArtifactsToLegs(t *testing.T) {
	svc, hub, dir := newBug627HubFixture(t)
	svc.mu.Lock()
	hubID := hub.id
	svc.mu.Unlock()

	prompt := svc.vibeDebateResumePrompt(hubID, dir)
	if !strings.Contains(prompt, "orchestrator") {
		t.Fatalf("hub resume prompt must pin the orchestrator role, got:\n%s", prompt)
	}
	for _, p := range []string{"existing.go", "new_stub.go", "impl.go"} {
		if !strings.Contains(prompt, p) {
			t.Fatalf("hub resume prompt must enumerate contract-owned path %q, got:\n%s", p, prompt)
		}
	}
}
