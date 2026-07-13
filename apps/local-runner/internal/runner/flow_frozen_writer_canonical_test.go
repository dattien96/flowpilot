package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// commitAllForTest stages and commits every current change in dir, returning
// the new HEAD SHA — used when a test needs a file (e.g. a custom
// flow-rules.json) to be part of the committed baseline a frozen contract's
// BaseSHA is captured against, rather than looking like an untracked,
// out-of-scope write relative to freeze time.
func commitAllForTest(t *testing.T, dir, message string) string {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "-A")
	run("commit", "-m", message)
	head, err := captureGitHead(dir)
	if err != nil || head == "" {
		t.Fatalf("captureGitHead after commit: %q, %v", head, err)
	}
	return head
}

// TestFrozenAgentCodeWriterStagesPendingCanonicalUpdate is the regression
// test for the CP-55 P-8 research finding: before this fix, a frozen
// agent.code writer's gate pass reached the artifact-output-only early
// return with hasPreparedContract still false (that flag was only ever set
// inside the isCodingChild block, which agent.code — isDelegate is always
// false for it — could never enter), so commitChangeContract was never
// called for it at all. Not "falls back to immediate write" but "does
// nothing to Canonical Head whatsoever": no immediate write, no P-5 staged
// write. Every one of CP-55 P-5's own ~20 tests exercised only an
// agent.delegate coding child (see flow_pending_canonical_test.go's own
// newPendingCanonicalCodingFixture comment), so this gap was untested until
// P-8's own research surfaced it.
func TestFrozenAgentCodeWriterStagesPendingCanonicalUpdate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked an in-scope frozen agent.code writer")
	}

	// The real Canonical Head must NOT exist yet — the effect must be
	// staged, not written immediately, exactly like an agent.delegate coder.
	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || found {
		t.Fatalf("Canonical Head must not be written immediately for a frozen agent.code writer, found=%v err=%v", found, err)
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	pending, ok, err := pstore.GetPending(parentID, "calc-core")
	if err != nil || !ok {
		t.Fatalf("expected a pending canonical update staged for the frozen agent.code writer, ok=%v err=%v", ok, err)
	}
	if pending.Head.BehaviorStatement != "fix rounding" {
		t.Fatalf("staged Head.BehaviorStatement = %q, want the frozen contract's own Intent (%q)", pending.Head.BehaviorStatement, "fix rounding")
	}
}

// TestFrozenAgentCodeWriterFinalizesOnTerminalDone completes the round trip:
// once the Flow reaches genuine terminal "done", the pending update staged
// by a frozen agent.code writer finalizes into the real Canonical Head,
// exactly once, using the frozen contract's own declared intent — proving
// the fix connects all the way through to CP-55 P-5's own finalize path.
func TestFrozenAgentCodeWriterFinalizesOnTerminalDone(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked")
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	head2, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found {
		t.Fatalf("expected the frozen agent.code writer's staged Head to finalize on terminal done, found=%v err=%v", found, err)
	}
	if head2.BehaviorStatement != "fix rounding" {
		t.Fatalf("BehaviorStatement = %q, want fix rounding", head2.BehaviorStatement)
	}
}

// TestFrozenAgentCodeWriterSecondGatePassDoesNotSelfTriggerDrift is the
// regression test for a genuine, production-breaking bug found while
// implementing CP-55 P-8: a frozen writer's FIRST gate pass stages a pending
// Canonical Head update by writing to
// .flowpilot/canonical-pending/pending_canonical.ndjson (CP-55 P-5). Without
// exempting PendingCanonicalStore's own two files from the scope-drift diff
// (the same exemption FrozenStore's own files already had, per CA-427
// Finding 2), that write showed up as "drift" on the SAME writer's very next
// gate pass — meaning ANY validation-retry or review-loop retry for a
// migrated Flow would self-block on its second pass, every single time. This
// went undetected until P-8 because no CP-55 P-4/P-5 test ever drove more
// than one gate pass against the same frozen contract.
func TestFrozenAgentCodeWriterSecondGatePassDoesNotSelfTriggerDrift(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})

	rs1 := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs1, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("first gate pass unexpectedly blocked")
	}

	// The second pass writes only src/calc.go again (in scope) — its own
	// prior staging write to .flowpilot/canonical-pending/ must not count
	// against it.
	rs2 := newP4ChildRun(svc, "child-2", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc // v2\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs2, "turn-2", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("second gate pass must not self-block on its own prior pending-canonical staging write")
	}
}

// TestFinalizeRejectsForgedPendingCanonicalRecord is the CP-55 P-8
// Claude-agent review Critical Finding 1 regression test, driven through the
// real gate/finalize path (not just the changecontract unit-level tests in
// pending_head_test.go): exempting PendingCanonicalStore's bookkeeping files
// from gate scope-drift comparison (this same phase's own fix, above) means
// their CONTENTS were never otherwise checked — a coder's ordinary
// file-write tool could append a raw NDJSON line to
// .flowpilot/canonical-pending/pending_canonical.ndjson for an entirely
// unrelated feature_key, and finalizePendingCanonicalHeadsForRun would have
// trusted it byte-for-byte at genuine Flow "done", writing an
// attacker-controlled Canonical Head for a feature the frozen contract never
// declared anything about. This proves the HMAC-signature fix
// (PendingCanonicalRecord.Signature / NewPendingCanonicalStoreWithSecret)
// closes it: the legitimately staged feature still finalizes; the forged one
// does not.
func TestFinalizeRejectsForgedPendingCanonicalRecord(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked the legitimate frozen writer")
	}

	// Simulate the exploit: the coder's own file-write tool appends a raw,
	// unsigned NDJSON line for a feature ("auth") the frozen contract never
	// declared anything about — bypassing Stage entirely.
	forged := changecontract.PendingCanonicalRecord{
		RunID:      parentID,
		FeatureKey: "auth",
		Head:       changecontract.CanonicalHead{FeatureKey: "auth", BehaviorStatement: "attacker-controlled"},
	}
	appendForgedPendingCanonicalRecord(t, dir, forged)

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || !found {
		t.Fatalf("the legitimately staged feature must still finalize, found=%v err=%v", found, err)
	}
	if _, found, err := changecontract.LoadHead(dir, "auth"); err != nil || found {
		t.Fatalf("the forged feature's Canonical Head must NOT have been written, found=%v err=%v", found, err)
	}
}

// appendForgedPendingCanonicalRecord appends rec directly to the
// pending-canonical records file, bypassing PendingCanonicalStore.Stage
// entirely — simulating a coder's own file-write tool forging an entry
// rather than a legitimate gate-pass stage.
func appendForgedPendingCanonicalRecord(t *testing.T, workspace string, rec changecontract.PendingCanonicalRecord) {
	t.Helper()
	rec.StagedAt = time.Now().UTC()
	rec.Seq = 1
	path := filepath.Join(workspace, changecontract.PendingCanonicalStoreBookkeepingPaths()[0])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

// TestFrozenAgentCodeWriterStagesCanonicalUpdateEvenWithArtifactRulesDisabled
// is the CP-55 P-8 Claude-agent review Important Finding 4 regression test:
// `only` (the flowgate rule subset this gate pass evaluates) is populated for
// a frozen agent.code writer ONLY by the unconditional artifact-rule scan —
// the doc-scope/test-rule block is deliberately never entered for it (scope
// enforcement for a frozen writer is the hardcoded frozen-contract check, not
// the configurable rules engine). Before this fix, `if len(only) == 0 {
// return false }` had no `!isCodingChild` guard (unlike the two similar
// no-op checks right below it), so a workspace that disables all three
// artifact rules would discard this node's prepared Canonical Head effect
// silently — recreating the exact "this node's Canonical Head effect does
// nothing" bug this phase's own core fix exists to close, just conditionally
// on rule configuration.
func TestFrozenAgentCodeWriterStagesCanonicalUpdateEvenWithArtifactRulesDisabled(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	settings := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	disabled := flowgate.DefaultRules()
	for i := range disabled {
		if flowgate.IsArtifactRule(disabled[i].ID) {
			disabled[i].Enabled = false
		}
	}
	rulesJSON, err := json.Marshal(disabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settings, "flow-rules.json"), rulesJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	// Commit flow-rules.json into the SAME baseline the frozen contract will
	// use: an untracked file appearing only after freeze would itself look
	// like an out-of-scope write to the frozen-contract drift check below
	// (this exact file is the CA-427-cited example of a security-sensitive
	// path scope enforcement must not exempt) — this test is about the
	// isCodingChild/`only` gating, not scope drift, so the rules file must
	// already be part of history before BaseSHA is captured.
	head = commitAllForTest(t, dir, "add flow-rules.json")

	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked an in-scope frozen agent.code writer")
	}

	pstore, err := changecontract.NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := pstore.GetPending(parentID, "calc-core"); err != nil || !ok {
		t.Fatalf("expected the Canonical Head update to still be staged with every artifact rule disabled, ok=%v err=%v", ok, err)
	}
}
