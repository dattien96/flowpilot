package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// appendPendingCanonicalRawLine appends a raw (possibly invalid) line
// directly to a pending-canonical store file, bypassing PendingCanonicalStore's
// own API — used only to simulate a corrupt/torn record for fail-closed tests.
func appendPendingCanonicalRawLine(t *testing.T, path, line string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

// relaxGateRulesForPendingCanonicalTests disables every doc-scope gate rule
// except r-contract (kept enabled so ContractDeclared/staging behavior is
// still exercised) — the same isolation pattern CA-427's
// TestFlowScopeDriftLegacyInferredContractBehavior test established, since
// none of r-ca/r-fk/r-bug/r-task/r-scope/r-tests/r-reg are what P-5's staging
// mechanism is about.
func relaxGateRulesForPendingCanonicalTests(t *testing.T, dir string) {
	t.Helper()
	// warn mode: focus these fixtures on contract/canonical head, not CP-53
	// gate_blind enforce (temp repos often have red/missing oracle baselines).
	p4WriteFile(t, dir, ".flowpilot/settings/gate-config.json", `{"gate_mode":"warn"}`)
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[
		{"id":"r-contract","scope":"step","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"warn","enabled":true},
		{"id":"r-ca","scope":"step","trigger":"code_changed","required_output":"change_audit_note","action":"warn","enabled":false},
		{"id":"r-fk","scope":"step","trigger":"commit_feature_key_missing","required_output":"verified_feature_key","action":"warn","enabled":false},
		{"id":"r-bug","scope":"step","trigger":"bug_fixed","required_output":"bugfix_doc","action":"warn","enabled":false},
		{"id":"r-task","scope":"step","trigger":"task_referenced","required_output":"task_doc","action":"warn","enabled":false},
		{"id":"r-scope","scope":"step","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn","enabled":false},
		{"id":"r-tests","scope":"step","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"warn","enabled":false},
		{"id":"r-reg","scope":"step","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"warn","enabled":false}
	]`)
}

// newPendingCanonicalCodingFixture builds a parent run whose Flow topology
// declares a single agent.delegate coding-writer node named "coder" — the
// production shape every real built-in flow uses today (unlike CP-55
// P-3/P-4's agent.code, which no built-in flow declares yet).
func newPendingCanonicalCodingFixture(t *testing.T, workspace string) (svc *InteractiveService, parentID string) {
	t.Helper()
	relaxGateRulesForPendingCanonicalTests(t, workspace)
	svc = newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}}
	prs.workspaceCwd = workspace
	prs.flowEngineDriven = true
	svc.mu.Unlock()
	return svc, parent.RunID
}

const pendingCanonicalDeclareMsg = "[Change Contract]\nfeature: calc-core\nintent: fix rounding\nfiles: src/calc.go\n\ndone"

// stageOneCodingPass drives a single Flow coder-child gate pass that declares
// feature "calc-core" and touches src/calc.go — the common setup every test
// below needs before asserting on the resulting pending/finalized state.
func stageOneCodingPass(t *testing.T, svc *InteractiveService, parentID, dir, head string) {
	t.Helper()
	rs := newP4ChildRun(svc, "child-"+head[:8], parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: pendingCanonicalDeclareMsg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked during test setup")
	}
}

// --- staging (coder gate pass) -----------------------------------------------

func TestCoderGateDoesNotSaveCanonicalHeadForFlow(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || found {
		t.Fatalf("Canonical Head must not be written immediately for a Flow coder gate pass, found=%v err=%v", found, err)
	}
}

func TestCoderGateStagesPendingCanonicalUpdate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := pstore.GetPending(parentID, "calc-core"); err != nil || !ok {
		t.Fatalf("expected a pending canonical update staged for (run, feature), ok=%v err=%v", ok, err)
	}
}

// --- finalize on genuine terminal acceptance ---------------------------------

func TestFlowDoneFinalizesCanonicalHead(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done", Summary: "complete"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || !found {
		t.Fatalf("expected Canonical Head finalized after done, found=%v err=%v", found, err)
	}
}

func TestFlowDoneFinalizesLatestAcceptedVersionOnly(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)

	rs1 := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	msg1 := "[Change Contract]\nfeature: calc-core\nintent: first pass intent\nfiles: src/calc.go\n\ndone"
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs1, "turn-1", finalizeInput{FinalMessage: msg1, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("first pass: gate unexpectedly blocked")
	}

	rs2 := newP4ChildRun(svc, "child-2", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc // updated\n")
	msg2 := "[Change Contract]\nfeature: calc-core\nintent: second pass intent, corrected\nfiles: src/calc.go\n\ndone"
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs2, "turn-2", finalizeInput{FinalMessage: msg2, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("second pass: gate unexpectedly blocked")
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	finalHead, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if finalHead.BehaviorStatement != "second pass intent, corrected" {
		t.Fatalf("BehaviorStatement = %q, want the LATEST staged pass's intent, not the first", finalHead.BehaviorStatement)
	}
}

func TestFlowDoneFinalizationFailureDoesNotPublishDone(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	// Corrupt the pending store so finalize fails to even open it.
	eventsPath := filepath.Join(dir, ".flowpilot", "canonical-pending", "pending_canonical_events.ndjson")
	appendPendingCanonicalRawLine(t, eventsPath, "{not valid json")

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err == nil {
		t.Fatal("expected applyFlowControl(done) to fail when canonical head finalization fails")
	}
	if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got == "done" {
		t.Fatalf("loop status = %q, must not be done when finalization failed", got)
	}
	if _, found, _ := changecontract.LoadHead(dir, "calc-core"); found {
		t.Fatal("Canonical Head must not exist when finalization failed")
	}
}

// TestFlowDoneFinalizationFailureEscalatesLoopForOperatorAction is the
// regression test for CP-55 P-5 review finding C-3: a finalize failure used
// to leave the loop silently "running" with the turn's one-decision slot
// already burned — a hub that retried flow_control(done) on the same turn
// was rejected as a duplicate, and no operator-actionable state was ever
// published, wedging the flow (the same recurring hang class as BUG-288 #9 /
// Task-242 T-9). A finalize failure must now escalate exactly like every
// other fail-closed gate site.
func TestFlowDoneFinalizationFailureEscalatesLoopForOperatorAction(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	eventsPath := filepath.Join(dir, ".flowpilot", "canonical-pending", "pending_canonical_events.ndjson")
	appendPendingCanonicalRawLine(t, eventsPath, "{not valid json")

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err == nil {
		t.Fatal("expected applyFlowControl(done) to fail when canonical head finalization fails")
	}

	got := svc.agentOrchestrator.loopStateFor(parentID)
	if got.Status != "blocked" || got.BlockReason != "escalate" {
		t.Fatalf("loop state = %+v, want Status=blocked BlockReason=escalate after a finalize failure", got)
	}
}

func TestFinalizeAcceptedCanonicalHeadsIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := pstore.Stage(changecontract.PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: changecontract.CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "fix"}}); err != nil {
		t.Fatal(err)
	}
	if err := finalizePendingCanonicalHeadsForRun(dir, "run-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := finalizePendingCanonicalHeadsForRun(dir, "run-1", nil); err != nil {
		t.Fatalf("a second finalize call must be idempotent, got error: %v", err)
	}
}

func TestRestartRetriesInterruptedCanonicalFinalization(t *testing.T) {
	dir := t.TempDir()
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := pstore.Stage(changecontract.PendingCanonicalRecord{RunID: "run-1", FeatureKey: "feat-a", Head: changecontract.CanonicalHead{FeatureKey: "feat-a", BehaviorStatement: "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := pstore.Stage(changecontract.PendingCanonicalRecord{RunID: "run-1", FeatureKey: "feat-b", Head: changecontract.CanonicalHead{FeatureKey: "feat-b", BehaviorStatement: "b"}}); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash: feat-a's real Head got written but the process died
	// before AppendStatus(finalized) — the store still shows it "pending".
	if err := changecontract.SaveHead(dir, changecontract.CanonicalHead{FeatureKey: "feat-a", BehaviorStatement: "a"}); err != nil {
		t.Fatal(err)
	}

	if err := finalizePendingCanonicalHeadsForRun(dir, "run-1", nil); err != nil {
		t.Fatalf("retry finalize after simulated crash must succeed: %v", err)
	}
	if _, found, _ := changecontract.LoadHead(dir, "feat-a"); !found {
		t.Fatal("feat-a Head must still exist after the retry")
	}
	if _, found, _ := changecontract.LoadHead(dir, "feat-b"); !found {
		t.Fatal("feat-b Head must be finalized on retry (never written before the simulated crash)")
	}
}

// TestFinalizePartialFailureCommitsNoHeadInTheBatch is the regression test
// for CP-55 P-5 review finding C-2: finalize used to write+mark each
// feature's Head one at a time, so if a later feature in the same "done"
// batch failed to write, an earlier feature's real Head was already
// permanently mutated even though the Flow's terminal acceptance as a whole
// was refused. Two-phase commit (stage every Head to a tmp file first, only
// then commit any of them) means a mid-batch failure must leave NO Head in
// the batch written and every record still pending, regardless of which
// feature failed or Go map iteration order.
func TestFinalizePartialFailureCommitsNoHeadInTheBatch(t *testing.T) {
	dir := t.TempDir()
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// FeatureKey names are deliberately chosen so "good" sorts BEFORE "bad"
	// (ListPendingForRunFresh returns records sorted by FeatureKey ascending)
	// — this is the exact ordering that would let a one-record-at-a-time
	// finalize write+mark "good" BEFORE ever reaching "bad", reproducing the
	// finding regardless of Go's randomized map iteration order.
	if err := pstore.Stage(changecontract.PendingCanonicalRecord{RunID: "run-1", FeatureKey: "aaa-good-feature", Head: changecontract.CanonicalHead{FeatureKey: "aaa-good-feature", BehaviorStatement: "ok"}}); err != nil {
		t.Fatal(err)
	}
	// "?" is an illegal NTFS filename character — this feature's Head file
	// write fails deterministically, simulating "one feature succeeds, the
	// next fails" without needing a real disk-full condition.
	if err := pstore.Stage(changecontract.PendingCanonicalRecord{RunID: "run-1", FeatureKey: "zzz-bad?feature", Head: changecontract.CanonicalHead{FeatureKey: "zzz-bad?feature", BehaviorStatement: "bad"}}); err != nil {
		t.Fatal(err)
	}

	if err := finalizePendingCanonicalHeadsForRun(dir, "run-1", nil); err == nil {
		t.Fatal("expected finalize to fail when one feature in the batch cannot be written")
	}

	if _, found, _ := changecontract.LoadHead(dir, "aaa-good-feature"); found {
		t.Fatal("two-phase commit: a sibling feature's Head must not be written when another feature in the same finalize batch failed")
	}
	fresh, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := fresh.GetPending("run-1", "aaa-good-feature"); !ok {
		t.Fatal("aaa-good-feature must remain pending (not marked finalized) since the batch as a whole did not commit")
	}
}

// TestDuplicateDoneIsRejectedWithoutRefinalizing exercises the terminal-guard
// duplicate-"done" path (renamed from the original TestStaleEpochCannot...:
// this drives two sequential applyFlowControl(done) calls, which is
// interactive_service.go's loop-status terminal guard, not gateEpoch at all
// — see TestStaleEpochGatePassDoesNotStagePendingCanonicalUpdate below for an
// actual epoch test; CP-55 P-5 review finding I-8).
func TestDuplicateDoneIsRejectedWithoutRefinalizing(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("first done: %v", err)
	}
	// A stale/duplicate "done" arriving after the loop is already terminal
	// must be rejected outright, not attempt to re-finalize.
	result, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"})
	if err != nil {
		t.Fatalf("stale done call itself should not error, got: %v", err)
	}
	if result.NextAction != "rejected_terminal" {
		t.Fatalf("NextAction = %q, want rejected_terminal", result.NextAction)
	}
}

// TestStaleEpochGatePassDoesNotStagePendingCanonicalUpdate is the genuine
// gateEpoch test CP-55 P-5 review finding I-8 asked for: a coder gate pass
// evaluated at an epoch that is no longer current (e.g. a Stop bumped
// gateEpoch concurrently, BUG-288 R17-P0) must not stage anything, since
// staging happens inside the same withGateEpochDurable-guarded commit as the
// rest of commitChangeContract.
func TestStaleEpochGatePassDoesNotStagePendingCanonicalUpdate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	rs := newP4ChildRun(svc, "child-stale-epoch", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")

	staleEpoch := rs.gateEpoch
	svc.mu.Lock()
	rs.gateEpoch++
	svc.mu.Unlock()

	svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: pendingCanonicalDeclareMsg, ChangedFiles: []string{"src/calc.go"}}, staleEpoch)

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); ok {
		t.Fatal("a gate pass evaluated at a stale epoch must not stage a pending canonical update")
	}
}

// --- abandon on non-"done" terminal outcomes ---------------------------------

func TestFlowFailureAbandonsPendingCanonicalUpdate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	pstoreBefore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstoreBefore.GetPending(parentID, "calc-core"); !ok {
		t.Fatal("expected a staged pending update before the flow fails")
	}

	svc.mu.Lock()
	rootRS := svc.runs[parentID]
	svc.emitLocked(rootRS, ProviderEvent{Type: EventTurnFailed, Error: "boom"})
	svc.mu.Unlock()

	waitLoop(t, "pending canonical update abandoned after failure", 3*time.Second, func() bool {
		fresh, err := changecontract.NewPendingCanonicalStore(dir)
		if err != nil {
			return false
		}
		_, ok, _ := fresh.GetPending(parentID, "calc-core")
		return !ok
	})
	if _, found, _ := changecontract.LoadHead(dir, "calc-core"); found {
		t.Fatal("a failed flow must never write the real Canonical Head")
	}
}

func TestFlowStopAbandonsPendingCanonicalUpdate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, apiErr := svc.stopAgentLoop(parentID); apiErr != nil {
		t.Fatalf("stopAgentLoop: %v", apiErr)
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); ok {
		t.Fatal("expected the pending canonical update to be abandoned after Stop")
	}
	if _, found, _ := changecontract.LoadHead(dir, "calc-core"); found {
		t.Fatal("a stopped flow must never write the real Canonical Head")
	}
}

func TestFlowCancelAbandonsPendingCanonicalUpdate(t *testing.T) {
	// This codebase has no separate Cancel code path — stopAgentLoop IS the
	// Cancel mechanism (same RunStatusCancelled stamping, same
	// AgentLoopState.Status="stopped" transition; confirmed by reading
	// interactive_handlers.go's stop endpoint and finding no distinct cancel
	// handler). This test exercises the identical call, kept as its own test
	// for direct traceability against the CP-55 P-5 spec's separate name.
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, apiErr := svc.stopAgentLoop(parentID); apiErr != nil {
		t.Fatalf("stopAgentLoop (Cancel): %v", apiErr)
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); ok {
		t.Fatal("expected the pending canonical update to be abandoned after Cancel")
	}
}

func TestFlowEscalationDoesNotFinalizeCanonicalHead(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "escalate", Summary: "need human"}); err != nil {
		t.Fatalf("applyFlowControl(escalate): %v", err)
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); !ok {
		t.Fatal("escalate must not finalize or abandon a pending canonical update — it stays active")
	}
	if _, found, _ := changecontract.LoadHead(dir, "calc-core"); found {
		t.Fatal("escalate must not write the real Canonical Head")
	}
}

// --- "continue" (validation retry / review retry) never finalizes -----------

func TestFlowContinueDoesNotFinalizeCanonicalHead(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue", Summary: "keep going"}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); !ok {
		t.Fatal("continue must not finalize or abandon a pending canonical update")
	}
	if _, found, _ := changecontract.LoadHead(dir, "calc-core"); found {
		t.Fatal("continue must not write the real Canonical Head")
	}
}

func TestValidationFailureDoesNotFinalizeCanonicalHead(t *testing.T) {
	// A validation-failure retry re-enters the loop via
	// applyFlowControl("continue") with failure feedback in this codebase —
	// not a distinct status value — so this exercises the same mechanism as
	// TestFlowContinueDoesNotFinalizeCanonicalHead, kept separate for direct
	// traceability against the spec's own test name.
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue", Summary: "tests failed, retry"}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); !ok {
		t.Fatal("a validation-failure retry must not finalize the pending canonical update")
	}
}

func TestReviewRetryDoesNotFinalizeCanonicalHead(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue", Summary: "reviewer requested changes"}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parentID, "calc-core"); !ok {
		t.Fatal("a review-retry continue must not finalize the pending canonical update")
	}
}

// --- spec drift ---------------------------------------------------------------

func TestSpecDriftDoesNotAutoFinalizeCanonicalHead(t *testing.T) {
	dir := t.TempDir()
	// A Head whose governing doc hash no longer matches the real (missing)
	// doc file — SpecDrifted(cwd, head) reads this as drifted.
	seed := changecontract.CanonicalHead{
		FeatureKey:         "calc-core",
		GoverningDocIDs:    []string{"nonexistent-doc-id"},
		GoverningDocHashes: map[string]string{"nonexistent-doc-id": "stale-hash-that-will-not-match"},
		Status:             changecontract.HeadStatusCurrent,
		SpecConfidence:     changecontract.SpecConfidenceSpecBacked,
	}
	if err := changecontract.SaveHead(dir, seed); err != nil {
		t.Fatal(err)
	}

	c := changecontract.Contract{FeatureKey: "calc-core", Intent: "fix", Confidence: changecontract.ConfidenceDeclared}
	if err := stagePendingCanonicalHead(dir, "run-1", "coder", c, false, nil); err != nil {
		t.Fatal(err)
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending("run-1", "calc-core"); ok {
		t.Fatal("a spec-drifted turn must not stage a pending canonical update")
	}
}

// --- explicit rebaseline/retire/merge remain immediate, unaffected ----------

func TestExplicitRebaselineBehaviorIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	seed := changecontract.CanonicalHead{FeatureKey: "calc-core", Status: changecontract.HeadStatusSpecLess, SpecConfidence: changecontract.SpecConfidenceSpecLess}
	if err := changecontract.SaveHead(dir, seed); err != nil {
		t.Fatal(err)
	}

	updated := changecontract.RebaselineWithSpec(dir, seed, []string{"SS-14-Code-Context-And-Regression-Safety"})
	if err := changecontract.SaveHead(dir, updated); err != nil {
		t.Fatal(err)
	}

	head, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found || head.SpecConfidence != changecontract.SpecConfidenceSpecBacked {
		t.Fatalf("rebaseline must still take effect immediately, got head=%+v found=%v err=%v", head, found, err)
	}
	// Stage rejects an empty RunID outright, so a GetPending("", ...) lookup
	// can never observe a staged record regardless of whether rebaseline
	// stages anything (CP-55 P-5 review finding I-6) — assert instead that
	// the pending-canonical store never got created at all.
	if _, err := os.Stat(filepath.Join(dir, ".flowpilot", "canonical-pending", "pending_canonical.ndjson")); !os.IsNotExist(err) {
		t.Fatalf("an explicit rebaseline must never touch the pending canonical store, stat err=%v", err)
	}
}

func TestExplicitRetireAndMergeBehaviorIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	seed := changecontract.CanonicalHead{FeatureKey: "old-feature", Status: changecontract.HeadStatusCurrent, SpecConfidence: changecontract.SpecConfidenceSpecBacked}
	if err := changecontract.SaveHead(dir, seed); err != nil {
		t.Fatal(err)
	}

	retired, _ := changecontract.RetireHead(seed, changecontract.RetireActionDeprecated, nil)
	if err := changecontract.SaveHead(dir, retired); err != nil {
		t.Fatal(err)
	}

	head, found, err := changecontract.LoadHead(dir, "old-feature")
	if err != nil || !found || head.Status != changecontract.HeadStatusDeprecated {
		t.Fatalf("retire must still take effect immediately, got head=%+v found=%v err=%v", head, found, err)
	}
	// Same reasoning as TestExplicitRebaselineBehaviorIsUnchanged (I-6): a
	// GetPending("", ...) lookup is vacuous since Stage rejects an empty
	// RunID unconditionally — assert the store file itself instead.
	if _, err := os.Stat(filepath.Join(dir, ".flowpilot", "canonical-pending", "pending_canonical.ndjson")); !os.IsNotExist(err) {
		t.Fatalf("an explicit retire must never touch the pending canonical store, stat err=%v", err)
	}
}

// --- Normal chat backward compatibility --------------------------------------

func TestNormalChatCanonicalPathRemainsBackwardCompatible(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	relaxGateRulesForPendingCanonicalTests(t, dir)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.workspaceCwd = dir
	// Deliberately NOT flowEngineDriven and NOT a child (parentRunID == "")
	// — a plain Normal chat root turn, no Flow topology at all.
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "root-turn", "", dir, head)
	rs.id = parent.RunID
	svc.mu.Lock()
	svc.runs[parent.RunID] = rs
	svc.runs[parent.RunID].workspaceCwd = dir
	svc.mu.Unlock()

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runFlowGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: pendingCanonicalDeclareMsg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("root gate unexpectedly blocked")
	}

	// Normal chat writes the real Canonical Head immediately — no pending
	// staging, no dependency on ever calling applyFlowControl("done").
	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || !found {
		t.Fatalf("Normal chat must still write Canonical Head immediately, found=%v err=%v", found, err)
	}
}

// TestFlowRootHubTurnWritesCanonicalHeadImmediately is the regression test
// for CP-55 P-5 review finding I-7: a Flow's own root/hub turn (parentRunID
// == "", but flowEngineDriven == true, unlike plain Normal chat) is safe by
// construction today — runFlowGateAtEpoch always passes an empty
// canonicalPendingRoute — but nothing pinned that, so a future refactor that
// threaded a route into the root path could silently regress it with no
// test failing.
func TestFlowRootHubTurnWritesCanonicalHeadImmediately(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	relaxGateRulesForPendingCanonicalTests(t, dir)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	rs := newP4ChildRun(svc, "root-turn", "", dir, head)
	rs.id = parent.RunID
	svc.mu.Lock()
	svc.runs[parent.RunID] = rs
	rs.workspaceCwd = dir
	rs.flowEngineDriven = true
	svc.mu.Unlock()

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runFlowGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: pendingCanonicalDeclareMsg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("root gate unexpectedly blocked")
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || !found {
		t.Fatalf("a Flow's own root/hub turn must write Canonical Head immediately, found=%v err=%v", found, err)
	}
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parent.RunID, "calc-core"); ok {
		t.Fatal("a Flow's own root/hub turn must never stage a pending canonical update")
	}
}

// TestAdHocCodingChildOutsideFlowEngineWritesImmediately is the companion
// regression test for CP-55 P-5 review finding I-7: the routing decision in
// gate_hook.go is `isCodingChild && s.isFlowEngineDriven(parentID)` — the
// false branch of isFlowEngineDriven (an ad hoc spawn_agent coding target
// outside any Flow) has no test pinning that it keeps today's immediate-write
// behavior.
func TestAdHocCodingChildOutsideFlowEngineWritesImmediately(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	relaxGateRulesForPendingCanonicalTests(t, dir)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}}
	prs.workspaceCwd = dir
	// Deliberately NOT flowEngineDriven — an ad hoc spawn_agent coding
	// target, not a Flow.
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-adhoc", parent.RunID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: pendingCanonicalDeclareMsg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked")
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || !found {
		t.Fatalf("an ad hoc coding child outside a Flow must write Canonical Head immediately, found=%v err=%v", found, err)
	}
	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pstore.GetPending(parent.RunID, "calc-core"); ok {
		t.Fatal("an ad hoc coding child outside a Flow must never stage a pending canonical update")
	}
}

// TestFlowFailureThenRedriveThenDoneFinalizesTheNewHead is the runner-level
// companion to TestPendingCanonicalStoreRestageAfterAbandonIsPendingAgain
// (CP-55 P-5 review finding C-1): this codebase explicitly supports a
// follow-up turn on an already-failed/stopped run (bug308_stopped_run_followup_allowed_test.go,
// bug302_chat_followup_after_flow_done_test.go) — a Flow that fails, gets
// redriven on the SAME run ID, stages a new pending update, and then reaches
// "done" must finalize the NEW value, not silently no-op because the old
// abandoned status is still on record for that key.
func TestFlowFailureThenRedriveThenDoneFinalizesTheNewHead(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newPendingCanonicalCodingFixture(t, dir)
	stageOneCodingPass(t, svc, parentID, dir, head)

	svc.mu.Lock()
	rootRS := svc.runs[parentID]
	svc.emitLocked(rootRS, ProviderEvent{Type: EventTurnFailed, Error: "boom"})
	svc.mu.Unlock()

	waitLoop(t, "pending canonical update abandoned after failure", 3*time.Second, func() bool {
		fresh, err := changecontract.NewPendingCanonicalStore(dir)
		if err != nil {
			return false
		}
		_, ok, _ := fresh.GetPending(parentID, "calc-core")
		return !ok
	})

	// Redrive: the operator continues on the same run ID, the coder passes
	// the gate again with a corrected intent.
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	rs2 := newP4ChildRun(svc, "child-redrive", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc // fixed\n")
	msg2 := "[Change Contract]\nfeature: calc-core\nintent: fixed after redrive\nfiles: src/calc.go\n\ndone"
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs2, "turn-2", finalizeInput{FinalMessage: msg2, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("redrive gate unexpectedly blocked")
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("applyFlowControl(done) after redrive: %v", err)
	}

	finalHead, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found {
		t.Fatalf("expected the redriven Flow's Head to finalize, found=%v err=%v", found, err)
	}
	if finalHead.BehaviorStatement != "fixed after redrive" {
		t.Fatalf("BehaviorStatement = %q, want the redrive's own intent — the stale abandoned status must not have silently no-opped finalize", finalHead.BehaviorStatement)
	}
}
