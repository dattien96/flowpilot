package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA799_ChatKindReconstructRestoresFlowSteps(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := "run-220036-" + string(pk)
			_, apiErr := svc.reconstructRun(ProviderSessionState{
				RunID:           runID,
				ProjectID:       "proj",
				ProviderKey:     pk,
				RunKind:         "chat",
				WorkingMode:     workingmode.Vibe,
				ChatFlowRef:     workingmode.PackPrefix + vibeSprintFlowID,
				Status:          RunStatusRunning,
				StartedAt:       "2026-09-09T00:00:00Z",
				UpdatedAt:       "2026-09-09T00:05:00Z",
				ActiveFlowNodes: nodes,
				ActiveFlowEdges: []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}},
			})
			if apiErr != nil {
				t.Fatalf("reconstructRun: %v", apiErr)
			}
			steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
			if err != nil {
				t.Fatalf("LoadRunSteps: %v", err)
			}
			if _, ok := stepByID(steps, "chat-"+runID); ok {
				t.Fatal("chat-kind vibe reconstruct must not replace flow steps with synthetic chat-*")
			}
			if _, ok := stepByID(steps, "tdd"); !ok {
				t.Fatalf("missing tdd step; steps=%#v", steps)
			}
			if _, ok := stepByID(steps, "coder"); !ok {
				t.Fatalf("missing coder step; steps=%#v", steps)
			}
		})
	}
}

func TestCA799_PlainChatEmptyNodesKeepsSyntheticStep(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := "run-plain-" + string(pk)
			_, apiErr := svc.reconstructRun(ProviderSessionState{
				RunID:       runID,
				ProjectID:   "proj",
				ProviderKey: pk,
				RunKind:     "chat",
				Status:      RunStatusCompleted,
				StartedAt:   "2026-09-09T00:00:00Z",
				UpdatedAt:   "2026-09-09T00:05:00Z",
			})
			if apiErr != nil {
				t.Fatalf("reconstructRun: %v", apiErr)
			}
			steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
			if err != nil {
				t.Fatalf("LoadRunSteps: %v", err)
			}
			if _, ok := stepByID(steps, "chat-"+runID); !ok {
				t.Fatalf("plain chat must keep synthetic chat-* step; steps=%#v", steps)
			}
			if _, ok := stepByID(steps, "tdd"); ok {
				t.Fatal("plain chat must not invent flow nodes")
			}
		})
	}
}

func TestCA799_SprintOverlayWritesChatFlowRef(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	svc.mu.Unlock()
	svc.startResolvedFlow(context.Background(), parent.RunID, workingmode.PackPrefix+vibeSprintFlowID, "Task-1")
	svc.mu.Lock()
	got := workingmode.BareFlowID(svc.runs[parent.RunID].chatFlowRef)
	svc.mu.Unlock()
	if got != vibeSprintFlowID {
		t.Fatalf("chatFlowRef=%q want vibe-sprint after overlay", got)
	}
}

func TestCA799_StaleCpIngestRefBecomesSprint(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_plan"},
		{ID: "tdd"},
		{ID: "coder"},
	}
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := "run-stale-ref-" + string(pk)
			rs, apiErr := svc.reconstructRun(ProviderSessionState{
				RunID:           runID,
				ProjectID:       "proj",
				ProviderKey:     pk,
				RunKind:         "chat",
				WorkingMode:     workingmode.Vibe,
				ChatFlowRef:     workingmode.PackPrefix + vibeCpIngestFlowID,
				Status:          RunStatusRunning,
				StartedAt:       "2026-09-09T00:00:00Z",
				UpdatedAt:       "2026-09-09T00:05:00Z",
				ActiveFlowNodes: nodes,
			})
			if apiErr != nil {
				t.Fatalf("reconstructRun: %v", apiErr)
			}
			if got := workingmode.BareFlowID(rs.chatFlowRef); got != vibeSprintFlowID {
				t.Fatalf("chatFlowRef=%q want vibe-sprint (stale ingest overlay)", rs.chatFlowRef)
			}
		})
	}
}

func TestCA799_InferPackFlowRefFromNodes(t *testing.T) {
	sprint := inferPackFlowRefFromNodes([]agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}, "fallback")
	if workingmode.BareFlowID(sprint) != vibeSprintFlowID {
		t.Fatalf("sprint=%q", sprint)
	}
	cp := inferPackFlowRefFromNodes([]agentpack.FlowNode{{ID: "cp_reader"}, {ID: vibeTaskSlicerNodeID}}, "fallback")
	if workingmode.BareFlowID(cp) != vibeCpIngestFlowID {
		t.Fatalf("cp=%q", cp)
	}
	ing := inferPackFlowRefFromNodes([]agentpack.FlowNode{{ID: "ingest_reader"}, {ID: vibeSSLockNodeID}}, "fallback")
	if workingmode.BareFlowID(ing) != vibeIngestFlowID {
		t.Fatalf("ingest=%q", ing)
	}
	deb := inferPackFlowRefFromNodes([]agentpack.FlowNode{{ID: "owner_1"}, {ID: "owner_2"}}, "fallback")
	if workingmode.BareFlowID(deb) != vibeOwnerDebateFlowID {
		t.Fatalf("debate=%q", deb)
	}
	if got := inferPackFlowRefFromNodes(nil, "keep-me"); got != "keep-me" {
		t.Fatalf("empty=%q", got)
	}
}
