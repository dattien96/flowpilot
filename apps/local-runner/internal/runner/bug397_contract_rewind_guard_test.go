package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// BUG-397 (live CP-64 run-5307): during the reproduce window the agent ran
// `git checkout -- .flowpilot/contracts/frozen_contracts.ndjson` (+restore/
// clean/reset) — the durable frozen-contract row was rewound out from under
// the runner and the next lock lookup hard-blocked "coder has no frozen
// contract". When a frozen contract is active, the shared approval bridge must
// deny git rewind/delete/overwrite commands that touch protected .flowpilot
// state or rewind the whole tree.
func seedFrozenContractRun(t *testing.T, svc *InteractiveService) (*interactiveRun, string) {
	t.Helper()
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	ws := t.TempDir()
	store, storeErr := changecontract.NewFrozenStore(ws)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	rec, frErr := changecontract.FreezeContract(ws, handle.RunID, "freeze", "coder",
		changecontract.PreflightContractDraft{
			FeatureKey: "feat", Intent: "i", DeclaredPaths: []string{"calc/calc.go"},
		}, "abc123", nil, "", 1, time.Now().UTC())
	if frErr != nil {
		t.Fatal(frErr)
	}
	if saveErr := store.SaveFrozen(rec); saveErr != nil {
		t.Fatal(saveErr)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[handle.RunID]
	rs.workspaceCwd = ws
	return rs, handle.RunID
}

func TestBug397_GitRewindOfFlowPilotStateDenied(t *testing.T) {
	svc, _ := newTestServer(t)
	rs, runID := seedFrozenContractRun(t, svc)

	denied := []string{
		"git checkout -- .flowpilot/contracts/frozen_contracts.ndjson",
		"git restore .flowpilot/contracts/frozen_contracts.ndjson",
		"git restore -- .flowpilot/contracts/",
		"git checkout HEAD -- .flowpilot",
		"git clean -fd .flowpilot/contracts",
		"git reset --hard",
		"git reset --hard HEAD~1",
		"git clean -fd",
		"git checkout -- .",
		"git checkout .",
		"git restore .",
		"rm -rf .flowpilot/contracts",
		"rm .flowpilot/guard/test_baseline.json",
		"echo '{}' > .flowpilot/contracts/frozen_contracts.ndjson",
		"git stash",
		"git rebase main",
		"git merge feature",
		"find .flowpilot -name '*.ndjson' -delete",
	}
	for _, cmd := range denied {
		decision, reason, handled := svc.decideFrozenContractStateGuard(rs, ApprovalDetails{Kind: "exec", Command: cmd})
		if !handled || decision != "deny" {
			t.Fatalf("run %s: %q must be denied, got handled=%v decision=%q reason=%q", runID, cmd, handled, decision, reason)
		}
	}
}

func TestBug397_SafeGitCommandsUnaffected(t *testing.T) {
	svc, _ := newTestServer(t)
	rs, _ := seedFrozenContractRun(t, svc)

	allowed := []string{
		"git status",
		"git diff -- .flowpilot/contracts/frozen_contracts.ndjson",
		"git log --oneline -5",
		"git show HEAD:.flowpilot/contracts/frozen_contracts.ndjson",
		"git add calc/calc.go",
		"git commit -m 'wip'",
		"git checkout -b new-branch",
		"git fetch origin",
		"cat .flowpilot/contracts/frozen_contracts.ndjson",
		"go test ./...",
		"rm -rf build/out",
		"git clean -fd calc/gen",
	}
	for _, cmd := range allowed {
		if _, _, handled := svc.decideFrozenContractStateGuard(rs, ApprovalDetails{Kind: "exec", Command: cmd}); handled {
			t.Fatalf("%q must fall through to the normal approval path", cmd)
		}
	}
}

func TestBug397_NoActiveContract_NoGuard(t *testing.T) {
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()
	// No frozen contract — rewinds fall through to the ordinary approval path.
	if _, _, handled := svc.decideFrozenContractStateGuard(rs, ApprovalDetails{Kind: "exec", Command: "git checkout -- .flowpilot/contracts/frozen_contracts.ndjson"}); handled {
		t.Fatal("guard must not fire without an active frozen contract")
	}
}
