package runner

// CP-84 84-g gap-fill (KR-005): the run-updates projector runs while s.mu is
// held, so it must be a pure read — deterministic output and no mutation of
// the source records. A projector that mutates or reads outside state would
// corrupt revisions or deadlock; this test is the regression guard.

import (
	"reflect"
	"testing"
)

func TestDecisionPayloads_ProjectorPerformsNoIOWhileLocked(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-purity", "proj-1")
	svc.mu.Lock()
	svc.approvals["appr-purity"] = pendingApprovalRec("appr-purity", rs.id)
	svc.questions["q-purity"] = pendingQuestionRec("q-purity", rs.id)
	svc.mu.Unlock()

	// Snapshot the source records' mutable fields.
	svc.mu.Lock()
	apprRev := svc.approvals["appr-purity"].revision
	apprStatus := svc.approvals["appr-purity"].status
	qRev := svc.questions["q-purity"].revision
	qStatus := svc.questions["q-purity"].status

	first := svc.decisionPayloadsForRunLocked(rs)
	second := svc.decisionPayloadsForRunLocked(rs)
	lane1 := svc.projectRealtimeRunLocked(rs)
	lane2 := svc.projectRealtimeRunLocked(rs)

	// Records must be byte-identical after projection — a pure read.
	if svc.approvals["appr-purity"].revision != apprRev ||
		svc.approvals["appr-purity"].status != apprStatus ||
		svc.questions["q-purity"].revision != qRev ||
		svc.questions["q-purity"].status != qStatus {
		t.Fatal("projector mutated source records while locked")
	}
	svc.mu.Unlock()

	if !reflect.DeepEqual(first, second) {
		t.Fatal("decisionPayloadsForRunLocked must be deterministic under lock")
	}
	if !reflect.DeepEqual(lane1, lane2) {
		t.Fatal("projectRealtimeRunLocked must be deterministic under lock")
	}
	if len(first) != 2 {
		t.Fatalf("expected approval+question payloads, got %d", len(first))
	}
	// Fingerprint of two identical projections must match — the coalescer
	// depends on fingerprint stability.
	if decisionFingerprint(first) != decisionFingerprint(second) {
		t.Fatal("fingerprint drifted between identical projections")
	}
}
