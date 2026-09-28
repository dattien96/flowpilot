package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// CP-89 review pass 8 (R8-*): post-R7 regression audit. Three findings, all
// reproduced red before the fix.
//
// ---- R8-1: a switched leg must not re-launch a flow that already ran -------
//
// CP-89 made every provider switch carry FlowRefFallback=src.chatFlowRef so a
// PENDING latch keeps its forward target on the new leg (L-14). But the ref
// also rides when the flow already launched on the source leg (immediate
// first-turn or a committed forward — both leave flowEngineDriven set). The
// new leg restarts at turnCount=0, so when the seed turn fails or was never
// sent, the next plain follow-up goes through handleStartTurn →
// resolveWorkflowFlowRef, which resolves a root run's chatFlowRef as a
// first-turn mount — re-running the whole flow on the switched leg. The ref
// must only ride while the launch has NOT happened yet.
func TestR8_SwitchedLegDoesNotRelaunchLaunchedFlow(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// The source leg already launched its mounted flow: the first turn ran,
	// the engine is driving, children exist on THIS leg's run id.
	svc.mu.Lock()
	src := svc.runs[handle.RunID]
	src.flowArm = FlowArmImmediate
	src.chatFlowRef = "task-harness"
	src.flowEngineDriven = true
	src.turnCount = 3
	svc.mu.Unlock()

	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != 200 {
		t.Fatalf("switch status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatSwitchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	newLeg := svc.runs[resp.Handle.RunID]
	if newLeg == nil {
		svc.mu.Unlock()
		t.Fatalf("new leg %q missing", resp.Handle.RunID)
	}
	carried := newLeg.chatFlowRef
	svc.mu.Unlock()
	if carried != "" {
		t.Fatalf("launched flow's chatFlowRef rode the switch leg: %q — the first follow-up resolves it as a mount and re-runs the whole flow (turnCount restarts at 0)", carried)
	}
	// And the resolver must never hand the leg a flowRef on its first turn.
	if ref, ok := svc.resolveWorkflowFlowRef(context.Background(), resp.Handle.RunID); ok {
		t.Fatalf("switched leg resolved a first-turn flowRef %q — a failed/empty seed turn would let a follow-up re-launch the flow", ref)
	}
}

// Control: a PENDING latch still needs the ref — it is the forward target.
// The flow never launched (flowEngineDriven stays false), so it rides.
func TestR8_SwitchedLegKeepsPendingForwardTarget(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	src := svc.runs[handle.RunID]
	src.flowArm = FlowArmPending
	src.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	src.sourceDocID = "requirements/07-Coding-Plan/CP-89.md"
	src.workingMode = "vibe"
	svc.mu.Unlock()

	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude})
	if rec.Code != 200 {
		t.Fatalf("switch status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatSwitchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	newLeg := svc.runs[resp.Handle.RunID]
	carried := newLeg.chatFlowRef
	arm := newLeg.flowArm
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("pending latch must ride the leg: arm=%q", arm)
	}
	if carried != workingmode.PackPrefix+vibeCpIngestFlowID {
		t.Fatalf("pending forward target must ride the leg: chatFlowRef=%q", carried)
	}
}

// ---- R8-2: heal must not wipe a run whose flow swapped topology mid-run ----
//
// Vibe sprint/debate replace ActiveFlowNodes mid-run (startResolvedFlowFromNode)
// and park the old topology on the parent row. In the swap window the durable
// parent row carries the NEW topology while its just-minted child is still
// idle and the engaged children carry PARKED node ids — no current-topology
// engaged child exists, so flowEntryChildExists reports "never launched" and
// the heal drops the latch to pending; the pending normalization below then
// wipes the lock/task plan/sprint index/parked topology. The parked nodes and
// task plan are themselves post-launch progress markers — the flow engine only
// writes them once nodes ran — so their presence must veto the heal.
func TestR8_TopologySwapEngagedChildKeepsStartedAndProgress(t *testing.T) {
	store := newFakeWorkflowStore()
	svc := task451ServiceWithStore(t, store)
	// The pre-swap topology's children ran (engaged), but their labels are
	// parked node ids — invisible to a current-topology-only evidence scan.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-c-old", ProjectID: "proj", ParentRunID: "run-v1",
		Label: "debate-node", Status: RunStatusCompleted, TurnCount: 2,
	}); err != nil {
		t.Fatal(err)
	}
	// The new topology's entry child was minted and killed before engaging
	// (the R7-2 window, now under a swapped topology).
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-c-new", ProjectID: "proj", ParentRunID: "run-v1",
		Label: "sprint-node", Status: RunStatusIdle, AgentStatus: "spawned",
	}); err != nil {
		t.Fatal(err)
	}
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-v1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusRunning,
		ProviderAccountID: "default",
		ChatFlowRef:     workingmode.PackPrefix + vibeSprintFlowID,
		FlowArm:         "started", TurnCount: 4,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "sprint-node"}},
		// Post-launch progress: the debate topology is parked and the sprint
		// plan exists — the flow demonstrably ran.
		VibeParkedNodes: []agentpack.FlowNode{{ID: "debate-node"}},
		VibeTaskPlan:    []string{"task-one"},
		VibeSprintIndex: 1,
		VibeLockedCP:    "CP-89",
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmStarted {
		t.Fatalf("a topology-swapped run with engaged parked-topology children must stay started — healing to %q also wipes all vibe progress", rs.flowArm)
	}
	if rs.vibeLockedCP != "CP-89" || len(rs.vibeTaskPlan) != 1 || rs.vibeSprintIndex != 1 || len(rs.vibeParkedNodes) != 1 {
		t.Fatalf("vibe progress wiped by a wrong heal: lockedCP=%q plan=%v sprintIndex=%d parked=%v",
			rs.vibeLockedCP, rs.vibeTaskPlan, rs.vibeSprintIndex, rs.vibeParkedNodes)
	}
}

// ---- R8-3: a foreign manifest must never downgrade a committed latch -------
//
// R7-1 only merges local state for byte-extended files (localAhead) or this
// machine's own snapshot (selfRerestore). A restored copy of ANOTHER
// machine's chat hits neither: re-restoring the foreign machine's stale
// pending manifest over a local row that already forwarded (started —
// a commit that writes zero provider bytes, so the file stays identical)
// persists pending over started. The latch is monotonic: once this machine
// durably committed started, no manifest may downgrade it.
func TestR8_ForeignRerestoreKeepsCommittedFlowLatch(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-arm", []byte("line1\n"))

	// Snapshot while pending, as the foreign machine would have synced it.
	row, found, err := store.GetProviderSession(context.Background(), "run-arm")
	if err != nil || !found {
		t.Fatalf("GetProviderSession found=%v err=%v", found, err)
	}
	row.FlowArm = "pending"
	row.ChatFlowRef = "flowpilot-core-flow-pack/vibe-cp-ingest"
	row.SourceDocID = "requirements/07-Coding-Plan/CP-89.md"
	row.WorkingMode = "vibe"
	row.ChangeType = "feature"
	row.ChatSubMode = "vibe"
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession pin: %v", err)
	}
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-arm", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Retag the remote record as FOREIGN machine A's snapshot of run-A — the
	// drive blobs keep their paths, only the claimed source identity changes.
	for id, file := range api.files {
		switch file.Name {
		case "sessions.ndjson":
			var lines []string
			for _, ln := range strings.Split(strings.TrimSpace(string(file.Content)), "\n") {
				var rec chatSessionDriveIndexRecord
				if err := json.Unmarshal([]byte(ln), &rec); err != nil {
					continue
				}
				rec.SourceMachineID = "machine-A"
				rec.SourceRunID = "run-A"
				raw, _ := json.Marshal(rec)
				lines = append(lines, string(raw))
			}
			file.Content = []byte(strings.Join(lines, "\n") + "\n")
			api.files[id] = file
		case "manifest.json":
			var manifest ChatSessionSyncManifest
			if err := json.Unmarshal(file.Content, &manifest); err != nil {
				t.Fatalf("manifest unmarshal: %v", err)
			}
			manifest.SourceMachineID = "machine-A"
			manifest.SourceRunID = "run-A"
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatalf("manifest marshal: %v", err)
			}
			file.Content = raw
			api.files[id] = file
		}
	}

	// The local row is this machine's restored copy of A's chat — and it
	// forwarded AFTER the restore: latch started, corrected pin adopted,
	// turnCount bumped. Zero provider bytes were appended (synthetic turn),
	// so the rollout file stays byte-identical to the manifest's.
	row, found, err = store.GetProviderSession(context.Background(), "run-arm")
	if err != nil || !found {
		t.Fatalf("GetProviderSession found=%v err=%v", found, err)
	}
	row.SourceMachineID = "machine-A"
	row.SourceRunID = "run-A"
	row.FlowArm = "started"
	row.SourceDocID = "requirements/07-Coding-Plan/CP-90.md"
	row.TurnCount++
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession newer local: %v", err)
	}

	// Re-restoring A's stale pending manifest must not downgrade the
	// locally-committed latch — the next forward would double-launch a flow
	// that already has children on this machine.
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: "machine-A",
		SourceRunID:     "run-A",
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	persisted, found, err := store.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(%q) found=%v err=%v", restored.RunID, found, err)
	}
	if persisted.FlowArm != "started" {
		t.Fatalf("foreign re-restore regressed the committed latch: got %q, want started", persisted.FlowArm)
	}
	if persisted.SourceDocID != "requirements/07-Coding-Plan/CP-90.md" {
		t.Fatalf("forward-adopted source pin regressed to stale manifest value: got %q", persisted.SourceDocID)
	}
	if persisted.WorkingMode != "vibe" {
		t.Fatalf("working mode regressed to stale manifest value: got %q", persisted.WorkingMode)
	}
}
