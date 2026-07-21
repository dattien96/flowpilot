package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Expanded BUG-299 residual coverage (run-35329 investigate checklist):
//
//  1. Chat YOLO off → still gates (permission_required card)
//  2. Workflow/coding child + git commit → still deny under forced YOLO
//  3. Rehydrate + follow-up → yolo=true for Claude AND Codex AND Grok
//  4. Stopped loop follow-up still forces yolo=true
//  5. Session NDJSON Yolo round-trip (durable persist)
//  6. Policy denylist still evaluated before human gate when YOLO off
//  7. Blocked child ordinary shell under forced YOLO auto-approves (flow product)
//     but coding-commit denylist still wins first (not "auto-approve sai")
//
// additive-tests-only: this file only adds tests.
// cross-provider-parity: Case 1 (shared force) + Case 2 wiring exercised by
// parameterizing Claude/Codex/Grok on the same follow-up path.

// keyedCaptureAdapter records TurnRequest.YoloMode and advertises a real provider key.
type keyedCaptureAdapter struct {
	key  ProviderKey
	ch   chan TurnRequest
	once sync.Once
}

func (a *keyedCaptureAdapter) Key() ProviderKey { return a.key }
func (a *keyedCaptureAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, ApprovalEvents: true}
}
func (a *keyedCaptureAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	select {
	case a.ch <- req:
	default:
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
	return nil
}

func registerKeyedCapture(reg *ProviderRegistry, key ProviderKey, ch chan TurnRequest) {
	reg.register(ProviderRegistration{
		Key:          key,
		DisplayName:  string(key),
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, ApprovalEvents: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &keyedCaptureAdapter{key: key, ch: ch}
		},
	})
}

// --- 3) Claude + Codex + Grok follow-up after done forces yolo=true ---------

func TestFlowFollowUpAfterDoneForcesYoloTrue_AllProviders(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			ch := make(chan TurnRequest, 1)
			reg := newProviderRegistry()
			registerKeyedCapture(reg, pk, ch)
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{
				ProjectID: "p", ChatMode: "normal_chat", ProviderKey: pk, YoloMode: false,
			})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			stepID := "chat-" + parent.RunID
			svc.mu.Lock()
			rs := svc.runs[parent.RunID]
			rs.flowEngineDriven = true
			rs.autoOrchestrate = true
			rs.yolo = false // rehydrate residual
			rs.workflowID = "wf-review"
			svc.mu.Unlock()
			svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
				Status: "done", Round: 1, Cap: 3, Mode: "explicit",
			})

			if _, apiErr := svc.startTurn(parent.RunID, TurnInput{
				StepID: stepID, Prompt: "follow-up after done",
			}, "", ""); apiErr != nil {
				t.Fatalf("startTurn: %s", apiErr.msg)
			}
			select {
			case req := <-ch:
				if !req.YoloMode {
					t.Fatalf("%s: TurnRequest.YoloMode=false, want true", pk)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s: timed out waiting for turn", pk)
			}
			waitLoop(t, "follow-up completes", 2*time.Second, func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				return !svc.runs[parent.RunID].turnInFlight
			})
		})
	}
}

// --- 4) Stopped loop follow-up also forces yolo=true ------------------------

func TestFlowFollowUpAfterStoppedForcesYoloTrue(t *testing.T) {
	ch := make(chan TurnRequest, 1)
	reg := newProviderRegistry()
	registerKeyedCapture(reg, ProviderKeyCodex, ch)
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, YoloMode: false,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.yolo = false
	rs.runKind = "workflow"
	rs.workflowID = "wf-1"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "stopped", Round: 0, Cap: 3, Mode: "explicit",
	})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{
		StepID: stepID, Prompt: "continue after stop",
	}, "", ""); apiErr != nil {
		t.Fatalf("startTurn after stopped: %s (code=%s)", apiErr.msg, apiErr.code)
	}
	select {
	case req := <-ch:
		if !req.YoloMode {
			t.Fatal("stopped-flow follow-up YoloMode=false, want true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for turn")
	}
}

// --- 1) Chat YOLO off still gates (permission_required) ---------------------

func TestChatYoloOffStillEmitsPermissionRequired(t *testing.T) {
	// Use a fresh service (not newTestServer) so approvalTTL is not 50ms.
	svc := NewInteractiveService()
	svc.policy = DefaultApprovalPolicyEngine()
	svc.approvalTTL = 5 * time.Second

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, YoloMode: false,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	svc.mu.Unlock()
	if rs.yolo {
		t.Fatal("chat create with YoloMode=false left rs.yolo=true")
	}

	bridge := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-gate", yolo: false}

	type result struct {
		decision string
		err      error
	}
	details := ApprovalDetails{
		Kind:    "exec",
		Command: "echo hello-gate",
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
	done := make(chan result, 1)
	go func() {
		d, e := bridge.RequestApproval(details)
		done <- result{d, e}
	}()

	var approvalID string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		if rs.pendingApprovalID != "" {
			approvalID = rs.pendingApprovalID
			return true
		}
		return false
	}, "pending approval card")

	// Card must already be on the stream before we resolve it.
	svc.mu.Lock()
	evs := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()
	if !hasPermissionRequired(evs) {
		t.Fatal("chat yolo=false must emit permission_required (approval card)")
	}

	if apiErr := svc.SubmitApprovalDecision(approvalID, "approve"); apiErr != nil {
		t.Fatalf("SubmitApprovalDecision: %s", apiErr.msg)
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("RequestApproval: %v", r.err)
		}
		if r.decision != "approve" {
			t.Fatalf("decision=%q, want approve (after human card)", r.decision)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for RequestApproval to return after approve")
	}
}

// Chat YOLO off + policy denylist: still deny without card (denylist before human).
func TestChatYoloOffPolicyDenylistStillDenies(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.policy = NewApprovalPolicyEngine(nil, []string{"rm -rf"})

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, YoloMode: false,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	svc.mu.Unlock()
	bridge := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-deny", yolo: false}

	decision, aerr := bridge.RequestApproval(ApprovalDetails{Kind: "exec", Command: "rm -rf /tmp/x"})
	if aerr != nil {
		t.Fatalf("RequestApproval: %v", aerr)
	}
	if decision != "deny" {
		t.Fatalf("decision=%q, want deny (policy denylist under yolo=false)", decision)
	}
	svc.mu.Lock()
	evs := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()
	if hasPermissionRequired(evs) {
		t.Fatal("denylist must not emit permission_required card")
	}
}

// --- 2) Coding commit still deny under forced flow YOLO ---------------------

func TestFlowCodingCommitStillDeniedUnderForcedYolo(t *testing.T) {
	// Force path: parent/child would have yolo forced true; denylist must still win.
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", WorkflowID: "", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	child, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, YoloMode: true,
	})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := []agentpack.FlowNode{{ID: "coder", Run: "agent.delegate", Agent: "coder"}}
	// Prefer reviewLoopTestNodes when available for realistic labels.
	if n := reviewLoopTestNodes(); len(n) > 0 {
		nodes = n
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].yolo = true // forced
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.runs[child.RunID].yolo = true
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: rs, yolo: true, turnID: "t-commit"}
	decision, aerr := bridge.RequestApproval(ApprovalDetails{Kind: "exec", Command: "git commit -m forced-yolo"})
	if aerr != nil {
		t.Fatalf("RequestApproval: %v", aerr)
	}
	if decision != "deny" {
		t.Fatalf("decision=%q, want deny (coding commit under forced YOLO)", decision)
	}

	// Ordinary shell under same forced YOLO must still auto-approve (product).
	decision2, aerr := bridge.RequestApproval(ApprovalDetails{Kind: "exec", Command: "ls -la"})
	if aerr != nil {
		t.Fatalf("RequestApproval ls: %v", aerr)
	}
	if decision2 != "approve" {
		t.Fatalf("ordinary shell decision=%q, want approve under forced YOLO", decision2)
	}
}

// --- 7) Blocked / stop child: commit denylist still wins (no wrong auto-approve)

func TestBlockedCodingChildStillDeniesCommitUnderForcedYolo(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	child, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, YoloMode: true,
	})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].yolo = true
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Mode: "explicit"})
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.runs[child.RunID].yolo = true
	svc.runs[child.RunID].intentBlockedKind = "resume"
	svc.runs[child.RunID].intentBlockedReason = "provider_unavailable"
	svc.runs[child.RunID].agentStatus = "blocked"
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: rs, yolo: true, turnID: "t-blocked-commit"}
	decision, aerr := bridge.RequestApproval(ApprovalDetails{Kind: "exec", Command: "git commit -m blocked"})
	if aerr != nil {
		t.Fatalf("RequestApproval: %v", aerr)
	}
	if decision != "deny" {
		t.Fatalf("blocked coding child commit decision=%q, want deny (must not auto-approve)", decision)
	}
}

// --- 5) Session NDJSON Yolo round-trip --------------------------------------

func TestLocalFileSessionStoreYoloRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	// Chat yolo on
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-chat-on", ProjectID: "p", ProviderKey: ProviderKeyClaude,
		Status: RunStatusIdle, RunKind: "chat", Yolo: true,
		StartedAt: "2026-07-22T00:00:00Z", UpdatedAt: "2026-07-22T00:00:01Z",
	}); err != nil {
		t.Fatalf("upsert chat on: %v", err)
	}
	// Chat yolo off (must persist false — cannot rely on omitempty alone after force path)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-chat-off", ProjectID: "p", ProviderKey: ProviderKeyCodex,
		Status: RunStatusIdle, RunKind: "chat", Yolo: false,
		StartedAt: "2026-07-22T00:00:00Z", UpdatedAt: "2026-07-22T00:00:02Z",
	}); err != nil {
		t.Fatalf("upsert chat off: %v", err)
	}
	// Workflow yolo true
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-wf", ProjectID: "p", ProviderKey: ProviderKeyGrok,
		Status: RunStatusIdle, RunKind: "workflow", WorkflowID: "wf-1", Yolo: true,
		StartedAt: "2026-07-22T00:00:00Z", UpdatedAt: "2026-07-22T00:00:03Z",
	}); err != nil {
		t.Fatalf("upsert wf: %v", err)
	}

	// Reload from disk (simulates process restart).
	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	on, ok, err := reloaded.GetProviderSession(context.Background(), "run-chat-on")
	if err != nil || !ok {
		t.Fatalf("get chat on: ok=%v err=%v", ok, err)
	}
	if !on.Yolo {
		t.Fatal("reloaded chat-on Yolo=false, want true")
	}
	off, ok, err := reloaded.GetProviderSession(context.Background(), "run-chat-off")
	if err != nil || !ok {
		t.Fatalf("get chat off: ok=%v err=%v", ok, err)
	}
	// Note: json omitempty drops false; document current contract — false may
	// come back as zero. reconstruct still forces only for flow; pure chat off
	// stays false either way.
	if off.Yolo {
		t.Fatal("reloaded chat-off Yolo=true, want false")
	}
	wf, ok, err := reloaded.GetProviderSession(context.Background(), "run-wf")
	if err != nil || !ok {
		t.Fatalf("get wf: ok=%v err=%v", ok, err)
	}
	if !wf.Yolo {
		t.Fatal("reloaded workflow Yolo=false, want true")
	}

	// sessionRecordFrom / sessionStateFromRecord direct unit check for true.
	rec := sessionRecordFrom(ProviderSessionState{RunID: "r", ProjectID: "p", Yolo: true})
	if !rec.Yolo {
		t.Fatal("sessionRecordFrom dropped Yolo=true")
	}
	back := sessionStateFromRecord(rec)
	if !back.Yolo {
		t.Fatal("sessionStateFromRecord dropped Yolo=true")
	}
}

// Reconstruct after disk reload still forces workflow YOLO even if durable false.
func TestReconstructAfterDiskReloadForcesWorkflowYolo(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// Legacy-ish row: workflow with Yolo left false on disk.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-legacy-wf", ProjectID: "p", ProviderKey: ProviderKeyCodex,
		Status: RunStatusIdle, RunKind: "workflow", WorkflowID: "wf-1", Yolo: false,
		StartedAt: "2026-07-22T00:00:00Z", UpdatedAt: "2026-07-22T00:00:01Z",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	st, ok, err := reloaded.GetProviderSession(context.Background(), "run-legacy-wf")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), reloaded)
	rs, apiErr := svc.reconstructRun(st)
	if apiErr != nil {
		t.Fatalf("reconstruct: %s", apiErr.msg)
	}
	if !rs.yolo {
		t.Fatal("reconstruct after disk reload left workflow yolo=false, want forced true")
	}
}

// sessionStateOf must include Yolo so the next persist is not a silent drop.
func TestSessionStateOfIncludesYolo(t *testing.T) {
	rs := &interactiveRun{id: "r1", projectID: "p", yolo: true, runKind: "chat"}
	snap := sessionStateOf(rs)
	if !snap.Yolo {
		t.Fatal("sessionStateOf dropped rs.yolo=true")
	}
	rs.yolo = false
	snap2 := sessionStateOf(rs)
	if snap2.Yolo {
		t.Fatal("sessionStateOf should reflect rs.yolo=false")
	}
}

// createRun workflow stamps Yolo=true into the initial persist snapshot path.
func TestCreateRunWorkflowPersistsYoloTrue(t *testing.T) {
	store := newFakeWorkflowStore()
	catalog := baseTestCatalog()
	catalog.workflows["proj-1"][0].YoloMode = false // pre-CA-378
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, store)
	handle, apiErr := svc.createRun(StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1"})
	if apiErr != nil {
		t.Fatalf("createRun: %s", apiErr.msg)
	}
	st, ok, err := store.GetProviderSession(context.Background(), handle.RunID)
	if err != nil || !ok {
		t.Fatalf("GetProviderSession: ok=%v err=%v", ok, err)
	}
	if !st.Yolo {
		t.Fatal("initial workflow session Yolo=false, want true after force")
	}
	svc.mu.Lock()
	live := svc.runs[handle.RunID].yolo
	svc.mu.Unlock()
	if !live {
		t.Fatal("live rs.yolo=false after workflow createRun force")
	}
}

// resolveEffectiveYolo unit table (defense for accidental chat force).
func TestResolveEffectiveYoloKeepsChatToggle(t *testing.T) {
	if got := resolveEffectiveYolo(false, "chat", "", false); got {
		t.Fatal("chat off forced true")
	}
	if got := resolveEffectiveYolo(true, "chat", "", false); !got {
		t.Fatal("chat on became false")
	}
	if got := resolveEffectiveYolo(false, "workflow", "", false); !got {
		t.Fatal("workflow off not forced")
	}
	if got := resolveEffectiveYolo(false, "chat", "wf", false); !got {
		t.Fatal("chat+workflowID not forced")
	}
	if got := resolveEffectiveYolo(false, "chat", "", true); !got {
		t.Fatal("flowEngineDriven not forced")
	}
}
