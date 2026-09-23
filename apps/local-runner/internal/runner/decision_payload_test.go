package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// CP-84 / Task-430 — DecisionPayload contract tests (additive; no existing
// test is touched). Covers: schema/version, bounds+redaction, durable
// round-trip of ids+revisions, multiple pending decisions, resolution state,
// and the missing-SS-quickView non-actionable marker.
// ============================================================================

func mkDecisionRun(svc *InteractiveService, id, projectID string) *interactiveRun {
	rs := &interactiveRun{
		id:          id,
		projectID:   projectID,
		providerKey: ProviderKeyClaude,
		status:      RunStatusRunning,
		createdAt:   time.Now().UTC().Format(time.RFC3339Nano),
		updatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	svc.mu.Lock()
	svc.runs[id] = rs
	svc.mu.Unlock()
	return rs
}

// mkDecisionRunLocked is mkDecisionRun for callers already holding svc.mu.
func mkDecisionRunLocked(svc *InteractiveService, id, projectID string) *interactiveRun {
	rs := &interactiveRun{
		id:          id,
		projectID:   projectID,
		providerKey: ProviderKeyClaude,
		status:      RunStatusRunning,
		createdAt:   time.Now().UTC().Format(time.RFC3339Nano),
		updatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	svc.runs[id] = rs
	return rs
}

func pendingApprovalRec(id, runID string) *approvalRecord {
	return &approvalRecord{
		id:        id,
		runID:     runID,
		status:    "pending",
		resolve:   make(chan string, 1),
		revision:  1,
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		details: ApprovalDetails{
			Command: "npm test",
			Kind:    "exec",
			Reason:  "needs approval",
			Decisions: []ApprovalDecisionOption{
				{Value: "approve", Label: "Approve"},
				{Value: "deny", Label: "Deny"},
			},
		},
	}
}

// TestDecisionPayload_SchemaVersion asserts every projected payload stamps the
// current contract version (T-1).
func TestDecisionPayload_SchemaVersion(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-a", "proj-1")
	rec := pendingApprovalRec("appr-430-a", rs.id)
	svc.mu.Lock()
	svc.approvals[rec.id] = rec
	svc.mu.Unlock()

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	if got[0].Version != DecisionPayloadVersion {
		t.Fatalf("version = %d, want %d", got[0].Version, DecisionPayloadVersion)
	}
	if got[0].Kind != DecisionKindApproval {
		t.Fatalf("kind = %q, want %q", got[0].Kind, DecisionKindApproval)
	}
	if got[0].ID != "approval:appr-430-a" {
		t.Fatalf("id = %q", got[0].ID)
	}
	if !got[0].Actionable || got[0].Status != "pending" {
		t.Fatalf("status=%q actionable=%v", got[0].Status, got[0].Actionable)
	}
}

// TestDecisionPayload_BoundsAndRedaction proves prompt/command fields are
// truncated and credential-shaped fragments masked (T-3).
func TestDecisionPayload_BoundsAndRedaction(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-b", "proj-1")
	rec := pendingApprovalRec("appr-430-b", rs.id)
	rec.details.Command = "curl -H token=ghp_supersecretvalue " + strings.Repeat("x", 4000)
	rec.details.Reason = strings.Repeat("y", 4000)
	svc.mu.Lock()
	svc.approvals[rec.id] = rec
	svc.mu.Unlock()

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(got))
	}
	d := got[0]
	if strings.Contains(d.Approval.Command, "ghp_supersecretvalue") {
		t.Fatal("credential-shaped fragment was not redacted from command")
	}
	if len(d.Approval.Command) > decisionFieldMaxLen*4+8 {
		t.Fatalf("command not bounded: len=%d", len(d.Approval.Command))
	}
	if len(d.Prompt) > decisionPromptMaxLen+8 {
		t.Fatalf("prompt not bounded: len=%d", len(d.Prompt))
	}
}

// TestDecisionPayloads_MultiplePending asserts several pending records on one
// run each project their own payload with stable distinct ids.
func TestDecisionPayloads_MultiplePending(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-c", "proj-1")
	svc.mu.Lock()
	svc.approvals["appr-1"] = pendingApprovalRec("appr-1", rs.id)
	svc.approvals["appr-2"] = pendingApprovalRec("appr-2", rs.id)
	svc.questions["q-1"] = &questionRecord{
		id: "q-1", runID: rs.id, status: "pending",
		prompt: "Pick an option", resolve: make(chan questionResolveResult, 1),
		revision: 1, createdAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	svc.mu.Unlock()

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, d := range got {
		seen[d.ID] = true
	}
	for _, id := range []string{"approval:appr-1", "approval:appr-2", "question:q-1"} {
		if !seen[id] {
			t.Fatalf("missing decision id %q in %v", id, seen)
		}
	}
}

// TestDecisionPayloads_ResolutionState asserts a resolved/expired record drops
// out of the pending projection (the frame IS the actionable surface).
func TestDecisionPayloads_ResolutionState(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-d", "proj-1")
	rec := pendingApprovalRec("appr-430-d", rs.id)
	svc.mu.Lock()
	svc.approvals[rec.id] = rec
	svc.mu.Unlock()

	if apiErr := svc.SubmitApprovalDecision(rec.id, "approve"); apiErr != nil {
		t.Fatalf("submit: %v", apiErr)
	}
	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()
	for _, d := range got {
		if d.ID == "approval:appr-430-d" && d.Actionable {
			t.Fatalf("resolved approval still actionable: %+v", d)
		}
	}
}

// TestDecisionPayloads_RecoverSameIDsAndRevisionsAfterRestart proves the
// durable shard round-trips revision/createdAt so a rehydrated pending record
// projects the SAME decision id+revision (Task-430 durable-replay row).
func TestDecisionPayloads_RecoverSameIDsAndRevisionsAfterRestart(t *testing.T) {
	dir := t.TempDir()
	svc := NewInteractiveService()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc.workflowStore = store

	rs := mkDecisionRun(svc, "run-430-e", "proj-1")
	rec := pendingApprovalRec("appr-430-e", rs.id)
	rec.createdAt = "2026-01-02T03:04:05.000000006Z"
	rec.revision = 4
	svc.mu.Lock()
	svc.approvals[rec.id] = rec
	svc.mu.Unlock()
	if err := svc.persistApproval(approvalStateFromRecord(rs, rec, rec.expiresAt)); err != nil {
		t.Fatalf("persistApproval: %v", err)
	}

	svc.mu.Lock()
	before := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	// Simulate restart: fresh service on the same store dir rehydrates the
	// pending record for the run.
	svc2 := NewInteractiveService()
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("store2: %v", err)
	}
	svc2.workflowStore = store2
	rs2 := mkDecisionRun(svc2, rs.id, "proj-1")
	rs2.status = RunStatusWaitingApproval
	svc2.mu.Lock()
	svc2.rehydratePendingGatesLocked(rs2.id)
	got := svc2.decisionPayloadsForRunLocked(rs2)
	svc2.mu.Unlock()

	var a, b *DecisionPayload
	for i := range before {
		if before[i].ID == "approval:appr-430-e" {
			a = &before[i]
		}
	}
	for i := range got {
		if got[i].ID == "approval:appr-430-e" {
			b = &got[i]
		}
	}
	if a == nil || b == nil {
		t.Fatalf("decision id missing across restart: before=%v after=%v", a != nil, b != nil)
	}
	if a.Revision != b.Revision {
		t.Fatalf("revision changed across restart: %q -> %q", a.Revision, b.Revision)
	}
	if b.CreatedAt != "2026-01-02T03:04:05.000000006Z" {
		t.Fatalf("createdAt did not round-trip: %q", b.CreatedAt)
	}
}

// TestDecisionPayloads_MissingSSQuickViewIsNonActionable: a reconstructed run
// parked at waiting_user_confirm whose gate is gone (RAM-only record) yields a
// non-actionable ss_lock marker, not a live card.
func TestDecisionPayloads_MissingSSQuickViewIsNonActionable(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-f", "proj-1")
	rs.status = RunStatus(ssLockWaiting)

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	var found *DecisionPayload
	for i := range got {
		if got[i].Kind == DecisionKindSSLock {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatal("expected an ss_lock marker for waiting_user_confirm run without a gate")
	}
	if found.Actionable {
		t.Fatal("marker must be non-actionable (Open-only) when the gate/quickView is missing")
	}
	if found.SSLock == nil || found.SSLock.QuickView != "" {
		t.Fatalf("expected empty quickView, got %+v", found.SSLock)
	}
}

// TestDecisionPayload_LiveSSLockWithQuickView covers the live gate path:
// waiting gate with draft content → actionable decision with bounded preview.
func TestDecisionPayload_LiveSSLockWithQuickView(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-430-g", "proj-1")
	rs.status = RunStatus(ssLockWaiting)
	g := &ssLockGate{
		runID:          rs.id,
		featureBase:    "Auth",
		workspaceRoot:  t.TempDir(),
		status:         ssLockWaiting,
		draftSSPath:    "/tmp/SS-Auth.md",
		draftSSContent: "# SS\n" + strings.Repeat("line\n", 200),
	}
	svc.registerSSLockGate(g)
	defer func() {
		ssLockGatesMu.Lock()
		delete(ssLockGates, ssLockKey{svc: svc, runID: rs.id})
		ssLockGatesMu.Unlock()
	}()

	svc.mu.Lock()
	got := svc.decisionPayloadsForRunLocked(rs)
	svc.mu.Unlock()

	var found *DecisionPayload
	for i := range got {
		if got[i].Kind == DecisionKindSSLock {
			found = &got[i]
		}
	}
	if found == nil || !found.Actionable {
		t.Fatalf("expected actionable ss_lock decision, got %+v", found)
	}
	if len(found.SSLock.QuickView) > decisionQuickViewMaxLen+8 {
		t.Fatalf("quickView not bounded: %d", len(found.SSLock.QuickView))
	}
}

// ============================================================================
// CP-84 / Task-429 — mux stream tests (spec §7 names; additive-only).
// ============================================================================

// TestRunUpdates_InitialSnapshotIsAtomicWithSubscription: a mutation racing
// subscribe lands either inside the snapshot or as a drained upsert — never
// lost between registration and projection (T-4).
func TestRunUpdates_InitialSnapshotIsAtomicWithSubscription(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-a", "proj-1")
	svc.mu.Lock()
	svc.approvals["appr-429-a"] = pendingApprovalRec("appr-429-a", rs.id)
	svc.mu.Unlock()

	subID, _, snapshot := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	var found *RunRealtimeProjection
	for i := range snapshot {
		if snapshot[i].RunID == rs.id {
			found = &snapshot[i]
		}
	}
	if found == nil {
		t.Fatalf("snapshot missing lane %q", rs.id)
	}
	if len(found.Decisions) != 1 || found.Decisions[0].ID != "approval:appr-429-a" {
		t.Fatalf("snapshot decisions = %+v", found.Decisions)
	}
	if found.Status != RunStatusRunning || found.ProjectID != "proj-1" {
		t.Fatalf("projection = %+v", found)
	}

	// Post-subscribe mutation must surface via drain.
	svc.mu.Lock()
	svc.approvals["appr-429-a2"] = pendingApprovalRec("appr-429-a2", rs.id)
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()
	frames := svc.drainRunUpdates(subID)
	if len(frames) != 1 || frames[0].Kind != RunRealtimeUpsert || frames[0].Run == nil {
		t.Fatalf("expected one upsert, got %+v", frames)
	}
	if len(frames[0].Run.Decisions) != 2 {
		t.Fatalf("upsert decisions = %+v", frames[0].Run.Decisions)
	}
}

// TestRunUpdates_ExcludesDelegatedChildRuns: child-agent runs (parentRunID
// set) are never in the lane set — not in the snapshot, not via drain (T-3).
func TestRunUpdates_ExcludesDelegatedChildRuns(t *testing.T) {
	svc := NewInteractiveService()
	parent := mkDecisionRun(svc, "run-429-parent", "proj-1")
	child := mkDecisionRun(svc, "run-429-child", "proj-1")
	child.parentRunID = parent.id

	subID, _, snapshot := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)
	for _, p := range snapshot {
		if p.RunID == child.id {
			t.Fatalf("child run %q leaked into snapshot", child.id)
		}
	}

	svc.mu.Lock()
	svc.approvals["appr-child"] = pendingApprovalRec("appr-child", child.id)
	svc.markRunRealtimeDirtyLocked(child.id)
	svc.markRunRealtimeDirtyLocked(parent.id)
	svc.mu.Unlock()

	for _, f := range svc.drainRunUpdates(subID) {
		if f.RunID == child.id {
			t.Fatalf("child run produced mux frame: %+v", f)
		}
	}
}

// TestRunUpdates_DirtyOverflowClosesRetryableAndBoundsMemory: a subscriber
// whose dirty set exceeds the cap is closed with a retryable resync frame and
// removed from the registry — memory stays bounded (T-5).
func TestRunUpdates_DirtyOverflowClosesRetryableAndBoundsMemory(t *testing.T) {
	svc := NewInteractiveService()
	subID, _, _ := svc.subscribeRunUpdates()

	svc.mu.Lock()
	for i := 0; i < runUpdateDirtyCap+10; i++ {
		svc.markRunRealtimeDirtyLocked(fmt.Sprintf("run-x%d", i))
	}
	svc.mu.Unlock()

	frames := svc.drainRunUpdates(subID)
	if len(frames) != 1 || frames[0].Kind != RunRealtimeResync || !frames[0].Retryable {
		t.Fatalf("expected single retryable resync, got %+v", frames)
	}
	svc.mu.Lock()
	_, stillRegistered := svc.runUpdateSubs[subID]
	svc.mu.Unlock()
	if stillRegistered {
		t.Fatal("overflowed subscriber must be removed from the registry")
	}
}

// TestRunUpdates_CoalescesSlowSubscriberWithoutLosingWaitingState: repeated
// dirty marks collapse into ONE drain carrying the LATEST waiting projection.
func TestRunUpdates_CoalescesSlowSubscriberWithoutLosingWaitingState(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-co", "proj-1")
	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	svc.mu.Lock()
	for i := 0; i < 500; i++ {
		svc.markRunRealtimeDirtyLocked(rs.id)
	}
	rs.status = RunStatusWaitingApproval
	svc.approvals["appr-429-co"] = pendingApprovalRec("appr-429-co", rs.id)
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()

	frames := svc.drainRunUpdates(subID)
	var upsert *RunRealtimeProjection
	for i := range frames {
		if frames[i].Kind == RunRealtimeUpsert && frames[i].Run != nil && frames[i].RunID == rs.id {
			upsert = frames[i].Run
		}
	}
	if upsert == nil {
		t.Fatalf("coalesced drain missing upsert for %q: %+v", rs.id, frames)
	}
	if upsert.Status != RunStatusWaitingApproval || len(upsert.Decisions) != 1 {
		t.Fatalf("drain lost latest waiting state: %+v", upsert)
	}
}

// TestRunUpdates_DoesNotEmitMessageDeltaOrToolNoise: transcript noise (event
// seq advance without status/summary/decision change) produces NO frame (T-2).
func TestRunUpdates_DoesNotEmitMessageDeltaOrToolNoise(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-quiet", "proj-1")
	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	// Simulate delta noise: seq/updatedAt advance, nothing meaningful moves.
	svc.mu.Lock()
	for i := 0; i < 50; i++ {
		rs.seq++
		rs.updatedAt = fmt.Sprintf("t%d", i)
		svc.markRunRealtimeDirtyLocked(rs.id)
	}
	svc.mu.Unlock()

	for _, f := range svc.drainRunUpdates(subID) {
		if f.RunID == rs.id {
			t.Fatalf("noise produced a mux frame for %q: %+v", rs.id, f)
		}
	}
}

// TestRunUpdates_TurnCompletedBeforeGatePassIsNotTerminal: a flow-driven run
// whose TurnCompleted is deferred behind the post-turn gate must NOT drain a
// remove frame — terminal classification reads rs.status, not the event type.
func TestRunUpdates_TurnCompletedBeforeGatePassIsNotTerminal(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-gate", "proj-1")
	rs.flowEngineDriven = true
	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventFileChanged, Path: "src/main.go"})
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
	svc.mu.Unlock()

	for _, f := range svc.drainRunUpdates(subID) {
		if f.Kind == RunRealtimeRemove && f.RunID == rs.id {
			t.Fatalf("deferred-completion run tombstoned while gate-armed: %+v", f)
		}
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if runStatusTerminal(rs.status) {
		t.Fatalf("run marked terminal before gate pass: status=%q", rs.status)
	}
}

// TestRunUpdates_TerminalRemoveEmitsOnce: a terminal lane re-marked dirty by
// trailing settle events must produce exactly ONE remove frame — live testing
// (CP-84 L-7) showed duplicate removes for every completed run.
func TestRunUpdates_TerminalRemoveEmitsOnce(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-term", "proj-1")
	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	svc.mu.Lock()
	rs.status = RunStatusCompleted
	for i := 0; i < 3; i++ {
		svc.markRunRealtimeDirtyLocked(rs.id) // settle emits several trailing marks
	}
	svc.mu.Unlock()

	removes := 0
	for _, f := range svc.drainRunUpdates(subID) {
		if f.Kind == RunRealtimeRemove && f.RunID == rs.id {
			removes++
		}
	}
	if removes != 1 {
		t.Fatalf("expected exactly 1 remove for terminal lane, got %d", removes)
	}

	// A second dirty mark on the still-terminal lane emits nothing.
	svc.mu.Lock()
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()
	for _, f := range svc.drainRunUpdates(subID) {
		if f.RunID == rs.id {
			t.Fatalf("repeat dirty mark on terminal lane produced %+v", f)
		}
	}
}

// TestRunUpdates_ProviderSwitchLegAppearsWithoutReattach: a provider switch
// creates a new leg (new runId, same chatId) that appears on the SAME
// subscription — no attach/detach race (T-7).
func TestRunUpdates_ProviderSwitchLegAppearsWithoutReattach(t *testing.T) {
	svc := NewInteractiveService()
	leg1 := mkDecisionRun(svc, "run-429-leg1", "proj-1")
	leg1.chatID = "chat-429"
	subID, _, snapshot := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)
	for _, p := range snapshot {
		if p.RunID == leg1.id && p.ChatID != "chat-429" {
			t.Fatalf("leg1 missing chatId grouping: %+v", p)
		}
	}

	// Switch: leg1 ends, leg2 (same chat) starts under a different provider.
	svc.mu.Lock()
	leg1.status = RunStatusCompleted
	svc.markRunRealtimeDirtyLocked(leg1.id)
	leg2 := mkDecisionRunLocked(svc, "run-429-leg2", "proj-1")
	leg2.chatID = "chat-429"
	leg2.providerKey = ProviderKeyCodex
	svc.markRunRealtimeDirtyLocked(leg2.id)
	svc.mu.Unlock()

	frames := svc.drainRunUpdates(subID)
	var sawRemove, sawLeg2 bool
	for _, f := range frames {
		if f.Kind == RunRealtimeRemove && f.RunID == leg1.id {
			sawRemove = true
		}
		if f.Kind == RunRealtimeUpsert && f.RunID == leg2.id && f.Run != nil && f.Run.ChatID == "chat-429" {
			sawLeg2 = true
		}
	}
	if !sawRemove || !sawLeg2 {
		t.Fatalf("leg switch incomplete: remove=%v upsert-leg2=%v frames=%+v", sawRemove, sawLeg2, frames)
	}
}

// TestRunUpdates_ClaudeCodexGrokSameProjection: the lane projection shape is
// provider-agnostic — same pending record, same frame (parity, §5 contract).
func TestRunUpdates_ClaudeCodexGrokSameProjection(t *testing.T) {
	svc := NewInteractiveService()
	for i, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		rs := mkDecisionRun(svc, fmt.Sprintf("run-429-p%d", i), "proj-1")
		rs.providerKey = pk
		svc.mu.Lock()
		svc.approvals["appr-429-"+string(pk)] = pendingApprovalRec("appr-429-"+string(pk), rs.id)
		svc.mu.Unlock()
	}

	_, _, snapshot := svc.subscribeRunUpdates()
	kinds := map[string][]DecisionKind{}
	for _, p := range snapshot {
		for _, d := range p.Decisions {
			kinds[string(p.Status)] = append(kinds[string(p.Status)], d.Kind)
		}
		if len(p.Decisions) != 1 || p.Decisions[0].Kind != DecisionKindApproval {
			t.Fatalf("provider %s projection malformed: %+v", p.RunID, p.Decisions)
		}
	}
	if len(snapshot) != 3 {
		t.Fatalf("expected 3 provider lanes, got %d", len(snapshot))
	}
}

// TestRunUpdates_UnsubscribeOnCancelAndWriteFailure: the registry shrinks on
// unsubscribe and drains of unknown subs are nil (T-4 cleanup proof).
func TestRunUpdates_UnsubscribeOnCancelAndWriteFailure(t *testing.T) {
	svc := NewInteractiveService()
	subID, _, _ := svc.subscribeRunUpdates()
	svc.unsubscribeRunUpdates(subID)
	if got := svc.drainRunUpdates(subID); got != nil {
		t.Fatalf("drain on removed subscriber = %+v", got)
	}
	svc.mu.Lock()
	n := len(svc.runUpdateSubs)
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("subscriber registry not cleaned: %d left", n)
	}
}

// TestRunUpdates_PerRunEventStreamUnchanged: the per-run SSE subscribe path
// is untouched — same snapshot+live channel contract as before CP-84.
func TestRunUpdates_PerRunEventStreamUnchanged(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-per", "proj-1")
	rs.subs = map[int64]chan ProviderEvent{}
	subID, ch, snap, found := svc.subscribe(rs.id, 0)
	if !found {
		t.Fatal("per-run subscribe lost the run")
	}
	defer svc.unsubscribe(rs.id, subID)
	_ = snap
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnStarted})
	svc.mu.Unlock()
	select {
	case ev := <-ch:
		if ev.Type != EventTurnStarted {
			t.Fatalf("per-run stream got %q", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("per-run stream did not deliver a live event")
	}
}

// TestRunUpdates_NoCrossRunContamination: a decision on run A must never
// appear inside run B's projection.
func TestRunUpdates_NoCrossRunContamination(t *testing.T) {
	svc := NewInteractiveService()
	rsA := mkDecisionRun(svc, "run-429-xa", "proj-1")
	rsB := mkDecisionRun(svc, "run-429-xb", "proj-1")
	svc.mu.Lock()
	svc.approvals["appr-XA"] = pendingApprovalRec("appr-XA", rsA.id)
	svc.mu.Unlock()

	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)
	svc.mu.Lock()
	svc.markRunRealtimeDirtyLocked(rsA.id)
	svc.markRunRealtimeDirtyLocked(rsB.id)
	svc.mu.Unlock()

	for _, f := range svc.drainRunUpdates(subID) {
		if f.Kind != RunRealtimeUpsert || f.Run == nil || f.RunID != rsB.id {
			continue
		}
		for _, d := range f.Run.Decisions {
			if d.RunID != rsB.id {
				t.Fatalf("run %q frame contains foreign decision %q", rsB.id, d.ID)
			}
		}
	}
}

// TestRunUpdates_TerminalRunEmitsRemove: resolving the last decision leaves
// the lane, and a terminal transition drains a remove tombstone.
func TestRunUpdates_TerminalRunEmitsRemove(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-429-term", "proj-1")
	svc.mu.Lock()
	svc.approvals["appr-429-term"] = pendingApprovalRec("appr-429-term", rs.id)
	svc.mu.Unlock()

	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	if apiErr := svc.SubmitApprovalDecision("appr-429-term", "approve"); apiErr != nil {
		t.Fatalf("submit: %v", apiErr)
	}
	svc.mu.Lock()
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()

	frames := svc.drainRunUpdates(subID)
	if len(frames) == 0 {
		t.Fatal("expected frame after resolve")
	}
	last := frames[len(frames)-1]
	if last.Kind != RunRealtimeUpsert || last.Run == nil || len(last.Run.Decisions) != 0 {
		t.Fatalf("resolved run still lists decisions: %+v", last)
	}

	svc.mu.Lock()
	rs.status = RunStatusCompleted
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()
	var sawRemove bool
	for _, f := range svc.drainRunUpdates(subID) {
		if f.Kind == RunRealtimeRemove && f.RunID == rs.id {
			sawRemove = true
		}
	}
	if !sawRemove {
		t.Fatal("terminal transition did not emit remove tombstone")
	}
}

// TestDecisionPayload_JSONRoundTrip ensures the wire shape serializes
// cleanly (desktop parses by kind).
func TestDecisionPayload_JSONRoundTrip(t *testing.T) {
	d := DecisionPayload{
		Version:    DecisionPayloadVersion,
		ID:         "approval:x",
		RunID:      "run-x",
		Kind:       DecisionKindApproval,
		Revision:   "3",
		Status:     "pending",
		Actionable: true,
		Approval:   &DecisionApprovalPayload{Command: "ls"},
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var back DecisionPayload
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != d.ID || back.Kind != d.Kind || back.Revision != "3" || back.Approval == nil || back.Approval.Command != "ls" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}
