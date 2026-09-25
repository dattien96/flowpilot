package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-449 (CP-87 P-5/P-7): the quota gate — manual parks on a durable
// quota_route_required card, auto commits only exact candidates, rotation
// mints claims/new legs, answers are durable and restart-safe.

// task449WriteSettings installs machine-global quota settings for the test.
func task449WriteSettings(t *testing.T, s QuotaRoutingSettings) {
	t.Helper()
	if s.Mode == "" {
		s.Mode = QuotaRotationManual
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	path := filepath.Join(t.TempDir(), "quota-routing.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_FILE", path)
}

// task449ChatRun registers a plain chat/vibe leg on codex pinned to acct.
func task449ChatRun(s *InteractiveService, runID, acctID, mode string) *interactiveRun {
	rs := &interactiveRun{
		id:                runID,
		providerKey:       ProviderKeyCodex,
		modelName:         "gpt-5.4",
		providerAccountID: acctID,
		chatID:            "chat-1",
		status:            RunStatusIdle,
		runKind:           "chat",
		workingMode:       mode,
		legState:          LegStateActive,
		legSeq:            1,
	}
	s.mu.Lock()
	s.runs[runID] = rs
	s.mu.Unlock()
	return rs
}

// task449FlowChild registers a flow child run under a parent carrying the
// node in activeFlowNodes (the run→node mapping the executor uses).
func task449FlowChild(t *testing.T, s *InteractiveService, acctID string) (*interactiveRun, *interactiveRun) {
	t.Helper()
	parent := &interactiveRun{
		id: "run-0", providerKey: ProviderKeyCodex, modelName: "gpt-5.4",
		providerAccountID: "cx-0", chatID: "chat-1", status: RunStatusRunning,
		runKind: "flow", legState: LegStateActive, flowEngineDriven: true,
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "coder", WorkloadClass: agentpack.WorkloadCoding},
		},
	}
	child := &interactiveRun{
		id: "run-1", providerKey: ProviderKeyCodex, modelName: "gpt-5.4",
		providerAccountID: acctID, chatID: "chat-1", parentRunID: "run-0",
		label: "coder", stepID: "coder", agentName: "coder",
		status: RunStatusFailed, runKind: "flow", legState: LegStateActive,
	}
	s.mu.Lock()
	s.runs["run-0"] = parent
	s.runs["run-1"] = child
	s.mu.Unlock()
	return parent, child
}

func task449Questions(s *InteractiveService, runID string) []*questionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*questionRecord
	for _, rec := range s.questions {
		if rec != nil && rec.runID == runID && rec.status == "pending" {
			out = append(out, rec)
		}
	}
	return out
}

func task449Events(s *InteractiveService, runID string, t ProviderEventType) []ProviderEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return nil
	}
	var out []ProviderEvent
	for _, ev := range rs.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

// --- gate outcomes ----------------------------------------------------------

func TestTask449_ManualFlowGates(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	_, child := task449FlowChild(t, s, "cx-0")

	if err := s.enterQuotaGate(context.Background(), child.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	qs := task449Questions(s, child.id)
	if len(qs) != 1 {
		t.Fatalf("manual flow must park on exactly one card, got %d", len(qs))
	}
	if questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatalf("card kind = %q", questionRecordKind(qs[0]))
	}
	s.mu.Lock()
	st := s.runs[child.id].status
	s.mu.Unlock()
	if st != RunStatusWaitingQuestion {
		t.Fatalf("run must park waiting on the card, status=%s", st)
	}
	// The card mirrors to the flow root so the decision is answerable there.
	if evs := task449Events(s, "run-0", EventUserQuestionRequired); len(evs) == 0 {
		t.Fatal("quota card must mirror to the flow root timeline")
	}
}

func TestTask449_ManualVibeGates(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "vibe")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	if qs := task449Questions(s, rs.id); len(qs) != 1 {
		t.Fatalf("vibe's auto lifecycle must not bypass the quota card, got %d cards", len(qs))
	}
}

func TestTask449_AutoFlowRotatesExactCandidate(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	_, child := task449FlowChild(t, s, "cx-0")

	if err := s.enterQuotaGate(context.Background(), child.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	s.mu.Lock()
	rs := s.runs[child.id]
	pinned, acct := rs.accountPinned, rs.providerAccountID
	s.mu.Unlock()
	if !pinned || acct != "cx-1" {
		t.Fatalf("auto rotation must claim+pin the exact candidate, pinned=%v acct=%q", pinned, acct)
	}
	if qs := task449Questions(s, child.id); len(qs) != 0 {
		t.Fatal("auto mode with an exact candidate commits without a card")
	}
	if evs := task449Events(s, child.id, EventQuotaRouteCommitted); len(evs) != 1 {
		t.Fatal("auto rotation publishes the quota_route_committed notice")
	}
}

func TestTask449_AutoVibeRotatesExactCandidate(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "vibe")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	s.mu.Lock()
	pinned, acct := s.runs[rs.id].accountPinned, s.runs[rs.id].providerAccountID
	s.mu.Unlock()
	if !pinned || acct != "cx-1" {
		t.Fatalf("vibe auto rotation must claim+pin, pinned=%v acct=%q", pinned, acct)
	}
}

func TestTask449_AutoUnknownQuotaGates(t *testing.T) {
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto})
	// No telemetry for cx-1 → unknown headroom → never auto-eligible.
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0}, time.Now().UTC().Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatalf("enterQuotaGate: %v", err)
	}
	if qs := task449Questions(s, rs.id); len(qs) != 1 {
		t.Fatal("auto mode with only uncertain candidates must park at the gate")
	}
	s.mu.Lock()
	pinned := s.runs[rs.id].accountPinned
	s.mu.Unlock()
	if pinned {
		t.Fatal("uncertain quota must never auto-pin")
	}
}

// --- legs / answers ---------------------------------------------------------

func TestTask449_RotationCreatesNewLeg(t *testing.T) {
	now := time.Now().UTC()
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	// Hub/chat demands are classless — an empty-class binding is the
	// provider's configured default for unrouted-class work.
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto, ModelBindings: []ModelClassBinding{
		{ProviderKey: ProviderKeyClaude, WorkloadClass: "", Model: "claude-sonnet"},
	}})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cl-0": 95}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	demand, err := s.ResolveExecutionDemand(context.Background(), rs.id, nil)
	if err != nil {
		t.Fatal(err)
	}
	demand.ObservedLimit = ProviderLimitQuotaExhausted
	res, err := s.ResolveQuotaPreflight(context.Background(), demand)
	if err != nil {
		t.Fatal(err)
	}
	// Force the cross-provider pick as the selected route (same-provider has
	// no healthy alternate in this fixture).
	if res.Outcome != QuotaRotate && res.Outcome != QuotaGate {
		t.Fatalf("unexpected outcome %s", res.Outcome)
	}
	sel := candByProvider(res.Candidates.Eligible, ProviderKeyClaude)
	if sel == nil {
		t.Fatalf("claude candidate must be eligible, got %+v", res.Candidates)
	}
	res.Outcome = QuotaRotate
	res.Selected = sel
	if err := s.CommitQuotaResolution(context.Background(), res); err != nil {
		t.Fatalf("commit rotation: %v", err)
	}
	s.mu.Lock()
	old := s.runs["run-1"]
	var newLeg *interactiveRun
	for _, r := range s.runs {
		if r.chatID == rs.chatID && r.id != "run-1" {
			newLeg = r
		}
	}
	oldClosed := old != nil && old.legState == LegStateClosed
	var newProvider ProviderKey
	var newAcct string
	var newSeq int
	if newLeg != nil {
		newProvider, newAcct, newSeq = newLeg.providerKey, newLeg.providerAccountID, newLeg.legSeq
	}
	s.mu.Unlock()
	if newLeg == nil {
		t.Fatal("cross-provider rotation must mint a new leg")
	}
	if !oldClosed {
		t.Fatal("the exhausted leg must be closed, not migrated")
	}
	if newProvider != ProviderKeyClaude || newAcct != "cl-0" {
		t.Fatalf("new leg binds claude/cl-0, got %s/%s", newProvider, newAcct)
	}
	if newSeq != 2 {
		t.Fatalf("new leg carries legSeq 2, got %d", newSeq)
	}
	if evs := task449Events(s, "run-1", EventQuotaRouteCommitted); len(evs) != 1 {
		t.Fatal("rotation publishes the committed notice on the source leg")
	}
}

func TestTask449_NoSelectionNoProviderCall(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	pending := s.runs[rs.id].pendingQuestionID
	status := s.runs[rs.id].status
	pinned := s.runs[rs.id].accountPinned
	s.mu.Unlock()
	if pending == "" || status != RunStatusWaitingQuestion {
		t.Fatalf("unanswered card must hold the run parked (pending=%q status=%s)", pending, status)
	}
	if pinned {
		t.Fatal("no selection = no claim = no provider binding change")
	}
}

func TestTask449_StopEndsStructured(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatal(err)
	}
	qs := task449Questions(s, rs.id)
	if len(qs) != 1 {
		t.Fatalf("expected one pending card, got %d", len(qs))
	}
	if aerr := s.AnswerQuestion(qs[0].id, []string{"stop"}); aerr != nil {
		t.Fatalf("answer: %v", aerr)
	}
	if evs := task449Events(s, rs.id, EventQuotaRouteStopped); len(evs) != 1 {
		t.Fatal("stop answer must emit the structured quota_route_stopped event")
	}
}

// --- durability -------------------------------------------------------------

func TestTask449_RestartRestoresPendingGate(t *testing.T) {
	// A rehydrated card loses its in-memory kind; the persisted prompt prefix
	// must still route answers to the quota handler.
	rec := &questionRecord{
		id: "q-1", runID: "run-1",
		prompt: quotaRouteQuestionKind + ": binding codex/cx-0 unusable (quota_exhausted)",
		status: "pending", rehydrated: true,
	}
	if questionRecordKind(rec) != quotaRouteQuestionKind {
		t.Fatal("rehydrated quota card must recover its kind from the durable prompt")
	}
	rec2 := &questionRecord{
		id: "q-2", runID: "run-1",
		prompt: usageBudgetQuestionKind + ": node burned tokens",
		status: "pending", rehydrated: true,
	}
	if questionRecordKind(rec2) != usageBudgetQuestionKind {
		t.Fatal("kind recovery must not misroute usage-budget cards")
	}
}

func TestTask449_RestartDoesNotReplayCommittedRotation(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationAuto})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")

	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	pinned := s.runs[rs.id].providerAccountID
	s.mu.Unlock()
	if pinned != "cx-1" {
		t.Fatalf("first gate pass must rotate to cx-1, got %q", pinned)
	}
	if n := len(task449Events(s, rs.id, EventQuotaRouteCommitted)); n != 1 {
		t.Fatalf("exactly one committed rotation, got %d", n)
	}
	// Replay the trigger: cx-1 is now the pinned binding and cx-0 is exhausted
	// — the durable claim ledger prevents any second auto-rotation firing.
	if err := s.enterQuotaGate(context.Background(), rs.id, string(ProviderLimitQuotaExhausted)); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	pinned = s.runs[rs.id].providerAccountID
	s.mu.Unlock()
	if pinned != "cx-1" {
		t.Fatalf("replay must not rebind, got %q", pinned)
	}
	if n := len(task449Events(s, rs.id, EventQuotaRouteCommitted)); n != 1 {
		t.Fatalf("replayed gate must not emit a second committed rotation, got %d", n)
	}
}

func hasReasonForTest(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

// --- escalation seams --------------------------------------------------------

func TestTask449_PressureReset_HeadroomFail_EscalatesToRouting(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs := task449ChatRun(s, "run-1", "cx-0", "dev")
	s.mu.Lock()
	rs.contextResetPending = true
	s.mu.Unlock()
	// Pinned account cannot afford the re-seed → the reset escalates into
	// quota routing instead of minting a doomed leg.
	s.contextResetHeadroomOK = func(*interactiveRun) bool { return false }

	if got := s.consumePendingContextReset(context.Background(), rs.id); got != "" {
		t.Fatalf("headroom-failed reset must not mint a leg, got %q", got)
	}
	if qs := task449Questions(s, rs.id); len(qs) != 1 || questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatal("headroom failure must escalate into the quota routing gate")
	}
}

func TestTask449_PostCompactionLegOffersRotate(t *testing.T) {
	// With usageRouter wired (Task-449), the usage_budget_exceeded card on a
	// profiled node offers `rotate` alongside extend/stop — the rotation path
	// exists after a leg reset exactly as before one.
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	parent, child := task449FlowChild(t, s, "cx-0")
	s.mu.Lock()
	parent.chatFlowRef = "task-harness"
	parent.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", ContextProfile: "coder", WorkloadClass: agentpack.WorkloadCoding},
	}
	child.events = append(child.events, ProviderEvent{
		Type:              EventTokenUsageUpdated,
		ProviderSessionID: "ps-1",
		TokenUsage:        &TokenUsageSnapshot{Total: &TokenUsageBreakdown{TotalTokens: 600001}},
	})
	child.lastPrompt = "x"
	s.mu.Unlock()

	s.checkUsageBudgetPostTurn(child, "turn-1")
	qs := task449Questions(s, child.id)
	if len(qs) != 1 {
		t.Fatalf("expected one usage-budget card, got %d", len(qs))
	}
	var rotate, extend bool
	for _, o := range qs[0].options {
		rotate = rotate || o.Label == "rotate"
		extend = extend || o.Label == "extend"
	}
	if !rotate || !extend {
		t.Fatalf("budget card must offer extend+rotate+stop, got %+v", qs[0].options)
	}
}

func TestTask449_BudgetTrigger_ExtendNeverAuto(t *testing.T) {
	opts := []QuestionOption{
		{Label: "extend"}, {Label: "rotate"}, {Label: "stop"},
	}
	auto := usageBudgetAutoSelectable(opts)
	for _, o := range auto {
		if o.Label == "extend" {
			t.Fatal("extend mutates the user's declared budget — never auto-selectable")
		}
	}
	if len(auto) != 2 {
		t.Fatalf("rotate+stop remain auto-selectable, got %v", auto)
	}
}
