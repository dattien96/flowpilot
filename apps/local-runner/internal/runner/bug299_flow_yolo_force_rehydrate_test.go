package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-299 residual (run-35329 / CA-378 follow-up):
//
// Flow/Workflow mode must always run YOLO=true. After a Flow reaches done/cancel,
// a follow-up prompt (and any rehydrate/restart) used to land with rs.yolo=false
// because:
//  1. sessionStateOf never persisted yolo
//  2. reconstructRun left the zero-value false
//  3. desktop workflow launches omit yoloMode on every turn
//
// Product: force true for workflow / flow-engine / workflowID paths. Chat mode
// still respects the UI toggle.
//
// cross-provider-parity: resolveEffectiveYolo / shouldForceFlowYolo / runTurn
// force path take no providerKey and do not branch on provider — Case 1
// (provider-agnostic). Representative Codex capture is sufficient; ForceShellBridge
// denylist remains a separate shared posture (tested elsewhere).
//
// additive-tests-only: this file only adds new tests.

func TestShouldForceFlowYoloMatrix(t *testing.T) {
	cases := []struct {
		name             string
		runKind          string
		workflowID       string
		flowEngineDriven bool
		want             bool
	}{
		{name: "plain-chat", runKind: "chat", want: false},
		{name: "workflow-kind", runKind: "workflow", want: true},
		{name: "empty-kind-legacy-hub", runKind: "", want: true},
		{name: "chat-with-workflow-id", runKind: "chat", workflowID: "wf-1", want: true},
		{name: "chat-flow-engine", runKind: "chat", flowEngineDriven: true, want: true},
		{name: "workflow-plus-flow-engine", runKind: "workflow", flowEngineDriven: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldForceFlowYolo(tc.runKind, tc.workflowID, tc.flowEngineDriven); got != tc.want {
				t.Fatalf("shouldForceFlowYolo(%q,%q,%v)=%v, want %v",
					tc.runKind, tc.workflowID, tc.flowEngineDriven, got, tc.want)
			}
		})
	}
}

func TestCreateRunForcesYoloTrueForWorkflowEvenWhenCatalogFalse(t *testing.T) {
	catalog := baseTestCatalog()
	// Pre-CA-378 row: workflow yolo_mode still false.
	catalog.workflows["proj-1"][0].YoloMode = false

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1"}); !got {
		t.Fatal("workflow run yolo = false, want true (BUG-299 residual force, independent of catalog)")
	}
}

func TestCreateRunChatStillRespectsYoloOffToggle(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.workflows["proj-1"][0].YoloMode = true // must not leak into pure chat

	if got := resolvedRunYolo(t, catalog, StartRunInput{
		ProjectID: "proj-1", ChatMode: "normal_chat", YoloMode: false,
	}); got {
		t.Fatal("normal chat yolo = true, want false when request leaves it off")
	}
	if got := resolvedRunYolo(t, catalog, StartRunInput{
		ProjectID: "proj-1", ChatMode: "normal_chat", YoloMode: true,
	}); !got {
		t.Fatal("normal chat yolo = false, want true when request enables it")
	}
}

// Follow-up on a done flow must keep YOLO=true even when the turn body omits
// yoloMode (desktop workflow path) and rs.yolo was stale false (rehydrate residual).
func TestFlowFollowUpAfterDoneForcesYoloTrueOnTurn(t *testing.T) {
	gotYolo := make(chan bool, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case gotYolo <- req.YoloMode:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, YoloMode: false,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID

	// Simulate rehydrate residual + sealed flow: product still requires YOLO on.
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.yolo = false // the bug: lost on rehydrate
	rs.workflowID = "wf-review"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})

	// Desktop workflow follow-up omits YoloMode.
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "follow-up after done"}, "", ""); apiErr != nil {
		t.Fatalf("follow-up startTurn: %s", apiErr.msg)
	}
	select {
	case yolo := <-gotYolo:
		if !yolo {
			t.Fatal("follow-up TurnRequest.YoloMode = false, want true for flow-engine run")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider turn")
	}
	waitLoop(t, "follow-up completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})
	svc.mu.Lock()
	sticky := svc.runs[parent.RunID].yolo
	svc.mu.Unlock()
	if !sticky {
		t.Fatal("rs.yolo stayed false after forced follow-up; want sticky true")
	}
}

// reconstructRun must not leave Flow/Workflow YOLO=false when the durable session
// row omits yolo (legacy) or still stores false.
func TestReconstructRunForcesYoloTrueForWorkflowAndFlowEngine(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	// Legacy workflow row: no Yolo field → zero false → must force true.
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:      "run-legacy-wf",
		ProjectID:  "p",
		WorkflowID: "wf-1",
		RunKind:    "workflow",
		// Yolo intentionally zero / missing
	})
	if apiErr != nil {
		t.Fatalf("reconstruct workflow: %s", apiErr.msg)
	}
	if !rs.yolo {
		t.Fatal("reconstructed workflow yolo = false, want true")
	}

	// Chat + flow topology restored → flowEngineDriven + force.
	rs2, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-legacy-flow",
		ProjectID:       "p",
		RunKind:         "chat",
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "coder"}},
		Yolo:            false,
	})
	if apiErr != nil {
		t.Fatalf("reconstruct flow chat: %s", apiErr.msg)
	}
	if !rs2.flowEngineDriven {
		t.Fatal("expected flowEngineDriven from ActiveFlowNodes")
	}
	if !rs2.yolo {
		t.Fatal("reconstructed flow-engine chat yolo = false, want true")
	}

	// Pure chat keeps stored false (UI toggle off across restart when field present).
	rs3, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:     "run-chat-off",
		ProjectID: "p",
		RunKind:   "chat",
		Yolo:      false,
	})
	if apiErr != nil {
		t.Fatalf("reconstruct chat: %s", apiErr.msg)
	}
	if rs3.yolo {
		t.Fatal("reconstructed pure chat yolo = true, want false (toggle off)")
	}

	// Pure chat with durable true restores true.
	rs4, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:     "run-chat-on",
		ProjectID: "p",
		RunKind:   "chat",
		Yolo:      true,
	})
	if apiErr != nil {
		t.Fatalf("reconstruct chat on: %s", apiErr.msg)
	}
	if !rs4.yolo {
		t.Fatal("reconstructed pure chat yolo = false, want true from durable field")
	}
}
