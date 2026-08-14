package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// BUG-324: Grok mid-chat /model must call ACP session/set_model on the live
// session (history kept). Additive tests only — do not edit grok_adapter_test.go.

func TestGrokACPSessionSetModelParams(t *testing.T) {
	got := grokACPSessionSetModelParams("  sess-1  ", "  grok-4.6  ")
	if got["sessionId"] != "sess-1" || got["modelId"] != "grok-4.6" {
		t.Fatalf("params = %#v, want trimmed sessionId+modelId", got)
	}
	if _, ok := got["cwd"]; ok {
		t.Fatalf("set_model must not carry cwd (that belongs on session/new): %#v", got)
	}
}

func TestGrokACPSessionNewParamsStillOmitModel(t *testing.T) {
	got := grokACPSessionNewParams("/tmp/x", nil)
	if _, ok := got["model"]; ok {
		t.Fatalf("session/new must not carry model (CA-445): %#v", got)
	}
	if _, ok := got["modelId"]; ok {
		t.Fatalf("session/new must not carry modelId (CA-445): %#v", got)
	}
	if got["cwd"] != "/tmp/x" {
		t.Fatalf("cwd = %v, want /tmp/x", got["cwd"])
	}
}

func TestGrokSendTurn_ResumeSetModelThenPrompt(t *testing.T) {
	sessionID := "019ff9ec-9c4c-7432-b921-12a5650008a2"
	methods, setModel := sendGrokTurnRecording(t, TurnRequest{
		RunID:             "run-324-resume",
		Prompt:            "hi",
		ProviderSessionID: sessionID,
		ModelName:         "grok-4.6",
	}, nil)

	if want := []string{"session/load", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v (resume must not session/new)", methods, want)
	}
	if setModel["sessionId"] != sessionID || setModel["modelId"] != "grok-4.6" {
		t.Fatalf("set_model params = %#v, want sessionId=%s modelId=grok-4.6", setModel, sessionID)
	}
}

func TestGrokSendTurn_EmptyModelSkipsSetModel(t *testing.T) {
	methods, _ := sendGrokTurnRecording(t, TurnRequest{
		RunID:  "run-324-empty",
		Prompt: "hi",
	}, nil)
	if want := []string{"session/new", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v (empty ModelName must not call set_model)", methods, want)
	}
}

func TestGrokSendTurn_WhitespaceModelSkipsSetModel(t *testing.T) {
	methods, _ := sendGrokTurnRecording(t, TurnRequest{
		RunID:     "run-324-ws",
		Prompt:    "hi",
		ModelName: "   ",
	}, nil)
	if want := []string{"session/new", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v (whitespace ModelName must not call set_model)", methods, want)
	}
}

func TestGrokSendTurn_FreshSessionSetModelAfterNew(t *testing.T) {
	methods, setModel := sendGrokTurnRecording(t, TurnRequest{
		RunID:     "run-324-fresh",
		Prompt:    "hi",
		ModelName: "grok-4.6",
	}, nil)
	if want := []string{"session/new", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v", methods, want)
	}
	if setModel["modelId"] != "grok-4.6" {
		t.Fatalf("set_model params = %#v, want modelId=grok-4.6", setModel)
	}
}

func TestGrokSendTurn_SyntheticThreadIDSetModelUsesNewNotLoad(t *testing.T) {
	methods, _ := sendGrokTurnRecording(t, TurnRequest{
		RunID:             "run-324-synth",
		Prompt:            "hi",
		ProviderSessionID: "thread-99",
		ModelName:         "grok-4.6",
	}, nil)
	if want := []string{"session/new", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v (synthetic thread- id must not session/load)", methods, want)
	}
}

func TestGrokSendTurn_SessionNewOmitsModelWhenSetModelUsed(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	sessionID := "session-no-model-on-new"

	var mu sync.Mutex
	var newParams map[string]any
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "session/new":
			params, _ := m["params"].(map[string]any)
			mu.Lock()
			newParams = params
			mu.Unlock()
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/set_model":
			fg.reply(m["id"], map[string]any{"_meta": map[string]any{"model": map[string]any{"Ok": "grok-4.6"}}})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-324-new-shape", Prompt: "hi", ModelName: "grok-4.6"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := newParams["model"]; ok {
		t.Fatalf("session/new carried model: %#v", newParams)
	}
	if _, ok := newParams["modelId"]; ok {
		t.Fatalf("session/new carried modelId: %#v", newParams)
	}
}

func TestGrokSendTurn_SetModelMethodMissingStillPrompts(t *testing.T) {
	methods, _ := sendGrokTurnRecording(t, TurnRequest{
		RunID:     "run-324-missing",
		Prompt:    "hi",
		ModelName: "grok-4.6",
	}, func(fg *fakeGrok, m map[string]any) bool {
		fg.send(map[string]any{
			"jsonrpc": "2.0",
			"id":      m["id"],
			"error":   map[string]any{"code": -32601, "message": "Method not found"},
		})
		return true
	})
	if want := []string{"session/new", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v (missing set_model must still session/prompt)", methods, want)
	}
}

func TestGrokSendTurn_SetModelResultErrStillPrompts(t *testing.T) {
	methods, _ := sendGrokTurnRecording(t, TurnRequest{
		RunID:     "run-324-err",
		Prompt:    "hi",
		ModelName: "grok-4.6",
	}, func(fg *fakeGrok, m map[string]any) bool {
		fg.reply(m["id"], map[string]any{"_meta": map[string]any{"model": map[string]any{"Err": "unknown model"}}})
		return true
	})
	if want := []string{"session/new", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("methods = %v, want %v (set_model Err must still session/prompt)", methods, want)
	}
}

// TestChatModeModelChangeKeepsProviderSessionID locks the opposite of reverted
// CA-452: a mid-chat model change must not drop the provider resume handle.
// Drives runTurn directly (not HTTP startTurn) so Codex is not swapped onto a
// live resume adapter. Case 3: Grok/Codex get the real id; Claude keeps the
// synthetic pool key (BUG-295).
func TestChatModeModelChangeKeepsProviderSessionID(t *testing.T) {
	tests := []struct {
		name      string
		key       ProviderKey
		synthetic string
		planted   string
		wantID    string
	}{
		{
			name:      "grok-keeps-acp-id",
			key:       ProviderKeyGrok,
			synthetic: "thread-324-g",
			planted:   "019ff9ec-9c4c-7432-b921-12a5650008a2",
			wantID:    "019ff9ec-9c4c-7432-b921-12a5650008a2",
		},
		{
			name:      "codex-keeps-rollout-id",
			key:       ProviderKeyCodex,
			synthetic: "thread-324-x",
			planted:   "rollout-abc123",
			wantID:    "rollout-abc123",
		},
		{
			name:      "claude-keeps-synthetic-pool-key",
			key:       ProviderKeyClaude,
			synthetic: "thread-324-c",
			planted:   "real-claude-session",
			wantID:    "thread-324-c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewInteractiveService()
			capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
			rs := &interactiveRun{
				id:                    "run-324-" + string(tt.key),
				providerKey:           tt.key,
				providerSessionID:     tt.synthetic,
				realProviderSessionID: tt.planted,
				modelName:             "model-a",
				workspaceCwd:          t.TempDir(),
				runKind:               "chat",
				turnCount:             2,
			}
			nextModel := "model-b"
			svc.runTurn(context.Background(), rs, capture, TurnInput{
				StepID: "step-1",
				Prompt: "two",
				Model:  &nextModel,
			}, "", "turn-2", nil)

			select {
			case req := <-capture.ch:
				if req.ModelName != "model-b" {
					t.Fatalf("ModelName = %q, want model-b", req.ModelName)
				}
				if req.ProviderSessionID == "" {
					t.Fatalf("ProviderSessionID empty — mid-chat /model must not drop resume (CA-452 class)")
				}
				if req.ProviderSessionID != tt.wantID {
					t.Fatalf("ProviderSessionID = %q, want %q", req.ProviderSessionID, tt.wantID)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for captured turn request (%s)", tt.key)
			}
		})
	}
}

func sendGrokTurnRecording(t *testing.T, req TurnRequest, onSetModel func(fg *fakeGrok, m map[string]any) bool) (methods []string, setModelParams map[string]any) {
	t.Helper()
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	sessionID := "session-bug324"
	if id := req.ProviderSessionID; id != "" && !strings.HasPrefix(id, "thread-") {
		sessionID = id
	}

	var mu sync.Mutex
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "session/new", "session/load", "session/set_model", "session/prompt":
			mu.Lock()
			methods = append(methods, method)
			mu.Unlock()
		}
		switch method {
		case "session/new", "session/load":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/set_model":
			params, _ := m["params"].(map[string]any)
			mu.Lock()
			setModelParams = params
			mu.Unlock()
			if onSetModel != nil && onSetModel(fg, m) {
				return
			}
			fg.reply(m["id"], map[string]any{"_meta": map[string]any{"model": map[string]any{"Ok": req.ModelName}}})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	return methods, setModelParams
}

func stringSliceEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestTurnResumeProviderSessionID_GrokFallsBackToLastTurn(t *testing.T) {
	acp := "019ff9fb-224a-7d41-a326-7ab53c4a341f"
	rs := &interactiveRun{
		providerKey:           ProviderKeyGrok,
		providerSessionID:     "thread-92956",
		lastGrokTurnSessionID: acp,
	}
	if got := turnResumeProviderSessionID(rs); got != acp {
		t.Fatalf("got %q, want lastGrokTurnSessionID %q (run-92955 model-change respawn)", got, acp)
	}
}

func TestTurnResumeProviderSessionID_RealIdWinsOverLastTurn(t *testing.T) {
	rs := &interactiveRun{
		providerKey:           ProviderKeyGrok,
		providerSessionID:     "thread-92956",
		realProviderSessionID: "019ff9fc-1d81-77d1-b5da-eb2ecd299278",
		lastGrokTurnSessionID: "019ff9fb-224a-7d41-a326-7ab53c4a341f",
	}
	if got := turnResumeProviderSessionID(rs); got != rs.realProviderSessionID {
		t.Fatalf("got %q, want real id", got)
	}
}

func TestTurnResumeProviderSessionID_ClaudeKeepsSyntheticPoolKey(t *testing.T) {
	rs := &interactiveRun{
		providerKey:           ProviderKeyClaude,
		providerSessionID:     "thread-324-c",
		realProviderSessionID: "real-claude-session",
		lastGrokTurnSessionID: "019ff9fb-224a-7d41-a326-7ab53c4a341f",
	}
	if got := turnResumeProviderSessionID(rs); got != "thread-324-c" {
		t.Fatalf("got %q, want synthetic Claude pool key", got)
	}
}

func TestChatModeModelChange_GrokLoadsLastTurnWhenRealEmpty(t *testing.T) {
	acp := "019ff9fb-224a-7d41-a326-7ab53c4a341f"
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	rs := &interactiveRun{
		id:                    "run-92955",
		providerKey:           ProviderKeyGrok,
		providerSessionID:     "thread-92956",
		lastGrokTurnSessionID: acp,
		modelName:             "grok-4.5",
		workspaceCwd:          t.TempDir(),
		runKind:               "chat",
		turnCount:             2,
	}
	nextModel := "grok-4.6"
	svc.runTurn(context.Background(), rs, capture, TurnInput{
		StepID: "step-1",
		Prompt: "what did I ask",
		Model:  &nextModel,
	}, "", "turn-92981", nil)

	select {
	case req := <-capture.ch:
		if req.ProviderSessionID != acp {
			t.Fatalf("ProviderSessionID = %q, want %q so session/load keeps history", req.ProviderSessionID, acp)
		}
		if req.ModelName != "grok-4.6" {
			t.Fatalf("ModelName = %q, want grok-4.6", req.ModelName)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for captured turn request")
	}
}

func TestGrokSendTurn_RunScopedIndexLoadsAfterModelRespawn(t *testing.T) {
	idx := &grokRunSessionIndex{byRun: map[string]string{}}
	sessionID := "019ffa13-3c09-7df3-af6f-8e832bbfbc42"
	runID := "run-93161"

	methods1, _ := sendGrokTurnRecordingWithIndex(t, TurnRequest{
		RunID:             runID,
		Prompt:            "1+1",
		ProviderSessionID: "thread-93162",
		ModelName:         "grok-4.5",
	}, idx, sessionID)
	if want := []string{"session/new", "session/set_model", "session/prompt"}; !stringSliceEqual(methods1, want) {
		t.Fatalf("turn1 methods = %v, want %v", methods1, want)
	}
	if got := idx.lookup(runID); got != sessionID {
		t.Fatalf("index after turn1 = %q, want %q", got, sessionID)
	}

	// New adapter = grokProcessKey mismatch (model 4.6). Same run, still
	// synthetic thread-* on the request (dispatch envelope shape, run-93161).
	methods2, setModel := sendGrokTurnRecordingWithIndex(t, TurnRequest{
		RunID:             runID,
		Prompt:            "câu hỏi đầu tiên",
		ProviderSessionID: "thread-93162",
		ModelName:         "grok-4.6",
	}, idx, sessionID)
	if want := []string{"session/load", "session/set_model", "session/prompt"}; !stringSliceEqual(methods2, want) {
		t.Fatalf("turn2 methods = %v, want %v (respawn must not session/new)", methods2, want)
	}
	if setModel["sessionId"] != sessionID || setModel["modelId"] != "grok-4.6" {
		t.Fatalf("set_model params = %#v, want sessionId=%s modelId=grok-4.6", setModel, sessionID)
	}
}

func TestGrokSendTurn_RunScopedIndexDoesNotStealSiblingRun(t *testing.T) {
	idx := &grokRunSessionIndex{byRun: map[string]string{}}
	parentSID := "019ffa13-3c09-7df3-af6f-8e832bbfbc42"
	childSID := "019ffa99-0000-0000-0000-000000000001"

	_, _ = sendGrokTurnRecordingWithIndex(t, TurnRequest{
		RunID: "run-parent", Prompt: "hi", ProviderSessionID: "thread-p", ModelName: "grok-4.5",
	}, idx, parentSID)

	methods, _ := sendGrokTurnRecordingWithIndex(t, TurnRequest{
		RunID: "run-child", Prompt: "hi", ProviderSessionID: "thread-c", ModelName: "grok-4.6",
	}, idx, childSID)
	if want := []string{"session/new", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
		t.Fatalf("child methods = %v, want session/new (must not load parent's id)", methods)
	}
	if got := idx.lookup("run-parent"); got != parentSID {
		t.Fatalf("parent index clobbered: %q", got)
	}
	if got := idx.lookup("run-child"); got != childSID {
		t.Fatalf("child index = %q, want %q", got, childSID)
	}
}

func TestGrokEnsureResumeID_PrefersRequestThenRunMap(t *testing.T) {
	lookup := func(runID string) string {
		if runID == "run-1" {
			return "019ffa13-3c09-7df3-af6f-8e832bbfbc42"
		}
		return ""
	}
	if got := grokEnsureResumeID(TurnRequest{RunID: "run-1", ProviderSessionID: "thread-1"}, lookup); got != "019ffa13-3c09-7df3-af6f-8e832bbfbc42" {
		t.Fatalf("synthetic + map = %q", got)
	}
	if got := grokEnsureResumeID(TurnRequest{RunID: "run-1", ProviderSessionID: "019ffaaa-1111-1111-1111-111111111111"}, lookup); got != "019ffaaa-1111-1111-1111-111111111111" {
		t.Fatalf("real request id must win over map: %q", got)
	}
	if got := grokEnsureResumeID(TurnRequest{RunID: "run-other", ProviderSessionID: "thread-x"}, lookup); got != "thread-x" {
		t.Fatalf("unknown run must keep synthetic: %q", got)
	}
}

func TestChatModeReasoningChangeKeepsProviderSessionID(t *testing.T) {
	assertGrokTurnKeepsResumeID(t, TurnInput{
		StepID:          "step-1",
		Prompt:          "follow-up after /reasoning",
		ReasoningEffort: "high",
	})
}

func TestChatModeYoloChangeKeepsProviderSessionID(t *testing.T) {
	yolo := true
	assertGrokTurnKeepsResumeID(t, TurnInput{
		StepID:   "step-1",
		Prompt:   "follow-up after YOLO flip",
		YoloMode: &yolo,
	})
}

func TestRefreshResumeHandleGrokPromotesSyntheticProviderSessionID(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{
		id:                "run-93161",
		providerKey:       ProviderKeyGrok,
		providerSessionID: "thread-93162",
	}
	adapter := &keyedFakeAdapter{key: ProviderKeyGrok, lastGrokSessionID: "019ffa13-3c09-7df3-af6f-8e832bbfbc42"}
	_ = svc.refreshResumeHandleLocked(rs, adapter)
	if rs.providerSessionID != adapter.lastGrokSessionID {
		t.Fatalf("providerSessionID = %q, want promoted ACP id %q", rs.providerSessionID, adapter.lastGrokSessionID)
	}
	if rs.realProviderSessionID != adapter.lastGrokSessionID {
		t.Fatalf("realProviderSessionID = %q, want %q", rs.realProviderSessionID, adapter.lastGrokSessionID)
	}
}

func assertGrokTurnKeepsResumeID(t *testing.T, in TurnInput) {
	t.Helper()
	acp := "019ffa13-3c09-7df3-af6f-8e832bbfbc42"
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	rs := &interactiveRun{
		id:                    "run-93161",
		providerKey:           ProviderKeyGrok,
		providerSessionID:     "thread-93162",
		realProviderSessionID: acp,
		lastGrokTurnSessionID: acp,
		modelName:             "grok-4.5",
		reasoningEffort:       "low",
		workspaceCwd:          t.TempDir(),
		runKind:               "chat",
		turnCount:             2,
	}
	svc.runTurn(context.Background(), rs, capture, in, "", "turn-follow", nil)
	select {
	case req := <-capture.ch:
		if req.ProviderSessionID != acp {
			t.Fatalf("ProviderSessionID = %q, want %q so session/load keeps history", req.ProviderSessionID, acp)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for captured turn request")
	}
}

func sendGrokTurnRecordingWithIndex(t *testing.T, req TurnRequest, idx *grokRunSessionIndex, sessionID string) (methods []string, setModelParams map[string]any) {
	t.Helper()
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.runSessions = idx
	if sessionID == "" {
		sessionID = "session-bug324"
	}
	if id := req.ProviderSessionID; id != "" && !strings.HasPrefix(id, "thread-") {
		sessionID = id
	}

	var mu sync.Mutex
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "session/new", "session/load", "session/set_model", "session/prompt":
			mu.Lock()
			methods = append(methods, method)
			mu.Unlock()
		}
		switch method {
		case "session/new", "session/load":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/set_model":
			params, _ := m["params"].(map[string]any)
			mu.Lock()
			setModelParams = params
			mu.Unlock()
			fg.reply(m["id"], map[string]any{"_meta": map[string]any{"model": map[string]any{"Ok": req.ModelName}}})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	return methods, setModelParams
}

func TestGrokProcessKeyDiffersForModelEffortAndYolo(t *testing.T) {
	base := grokProcessKey("scope-a", "grok-4.5", "low", false)
	if grokProcessKey("scope-a", "grok-4.6", "low", false) == base {
		t.Fatal("model change must spawn a new grok process")
	}
	if grokProcessKey("scope-a", "grok-4.5", "medium", false) == base {
		t.Fatal("reasoning change must spawn a new grok process")
	}
	if grokProcessKey("scope-a", "grok-4.5", "low", true) == base {
		t.Fatal("YOLO change must spawn a new grok process")
	}
}

func TestGrokRunSessionIndex_RememberIgnoresSyntheticAndEmpty(t *testing.T) {
	var nilIdx *grokRunSessionIndex
	nilIdx.remember("run-1", "019ffa2a-13f8-7671-ac47-b8bbe89aa676")
	if got := nilIdx.lookup("run-1"); got != "" {
		t.Fatalf("nil index lookup = %q", got)
	}

	idx := &grokRunSessionIndex{}
	idx.remember("", "019ffa2a-13f8-7671-ac47-b8bbe89aa676")
	idx.remember("run-1", "thread-1")
	idx.remember("run-1", "  ")
	if got := idx.lookup("run-1"); got != "" {
		t.Fatalf("junk must not land in index, got %q", got)
	}
	idx.remember("run-1", "019ffa2a-13f8-7671-ac47-b8bbe89aa676")
	if got := idx.lookup("run-1"); got != "019ffa2a-13f8-7671-ac47-b8bbe89aa676" {
		t.Fatalf("lookup = %q", got)
	}
}

func TestGrokSendTurn_RunScopedIndexLoadsAfterEffortAndYoloRespawn(t *testing.T) {
	sessionID := "019ffa2a-13f8-7671-ac47-b8bbe89aa676"
	cases := []struct {
		name  string
		turn2 TurnRequest
	}{
		{
			name: "reasoning",
			turn2: TurnRequest{
				RunID:             "run-93638",
				Prompt:            "follow-up after /reasoning",
				ProviderSessionID: "thread-93640",
				ModelName:         "grok-4.6",
				ReasoningEffort:   "medium",
			},
		},
		{
			name: "yolo",
			turn2: TurnRequest{
				RunID:             "run-93638",
				Prompt:            "follow-up after YOLO flip",
				ProviderSessionID: "thread-93640",
				ModelName:         "grok-4.6",
				YoloMode:          true,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := &grokRunSessionIndex{byRun: map[string]string{}}
			_, _ = sendGrokTurnRecordingWithIndex(t, TurnRequest{
				RunID:             tc.turn2.RunID,
				Prompt:            "chào, 1 + 2 bằng mấy",
				ProviderSessionID: "thread-93640",
				ModelName:         "grok-4.5",
				ReasoningEffort:   "low",
			}, idx, sessionID)
			methods, setModel := sendGrokTurnRecordingWithIndex(t, tc.turn2, idx, sessionID)
			if want := []string{"session/load", "session/set_model", "session/prompt"}; !stringSliceEqual(methods, want) {
				t.Fatalf("methods = %v, want %v (respawn must not session/new)", methods, want)
			}
			if setModel["sessionId"] != sessionID {
				t.Fatalf("set_model sessionId = %#v, want %s", setModel, sessionID)
			}
		})
	}
}

func TestGrokSendTurn_AdoptedPromptSessionIdUpdatesRunIndex(t *testing.T) {
	idx := &grokRunSessionIndex{byRun: map[string]string{}}
	opened := "019ffa2a-opened-0000-0000-000000000001"
	adopted := "019ffa2a-adopted-0000-0000-000000000002"
	runID := "run-adopt"

	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.runSessions = idx
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": opened})
		case "session/set_model":
			fg.reply(m["id"], map[string]any{"_meta": map[string]any{"model": map[string]any{"Ok": "grok-4.5"}}})
		case "session/prompt":
			result := liveGrokPromptResult(opened)
			result["sessionId"] = adopted
			fg.reply(m["id"], result)
		}
	})
	if err := a.SendTurn(context.Background(), TurnRequest{
		RunID:     runID,
		Prompt:    "hi",
		ModelName: "grok-4.5",
	}, &fakeGrokBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if got := idx.lookup(runID); got != adopted {
		t.Fatalf("index = %q, want adopted prompt sessionId %q", got, adopted)
	}
}

func TestReconstructRunSeedsLastGrokTurnSessionID(t *testing.T) {
	svc := NewInteractiveService()
	acp := "019ffa2a-13f8-7671-ac47-b8bbe89aa676"
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:             "run-93638",
		ProjectID:         "proj",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: acp,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		ModelName:         "grok-4.6",
		StartedAt:         "2026-08-13T15:08:00Z",
		UpdatedAt:         "2026-08-13T15:10:00Z",
		LastPrompt:        "hi",
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.lastGrokTurnSessionID != acp {
		t.Fatalf("lastGrokTurnSessionID = %q, want persisted ACP id %q", rs.lastGrokTurnSessionID, acp)
	}
}

func TestTurnResumeProviderSessionID_CodexIgnoresLastGrok(t *testing.T) {
	rs := &interactiveRun{
		providerKey:           ProviderKeyCodex,
		providerSessionID:     "thread-1",
		realProviderSessionID: "rollout-abc",
		lastGrokTurnSessionID: "019ffa2a-13f8-7671-ac47-b8bbe89aa676",
	}
	if got := turnResumeProviderSessionID(rs); got != "rollout-abc" {
		t.Fatalf("Codex must use realProviderSessionID, got %q", got)
	}
}

func TestRefreshResumeHandleClaudeKeepsSyntheticPoolKey(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{
		id:                "run-claude",
		providerKey:       ProviderKeyClaude,
		providerSessionID: "thread-99",
	}
	adapter := &keyedFakeAdapter{key: ProviderKeyClaude, lastGrokSessionID: "019ffa2a-13f8-7671-ac47-b8bbe89aa676"}
	_ = svc.refreshResumeHandleLocked(rs, adapter)
	if rs.providerSessionID != "thread-99" {
		t.Fatalf("Claude providerSessionID = %q, want thread-99 (must not promote a Grok ACP id)", rs.providerSessionID)
	}
}

func TestMapGrokNotificationDropsReplayUpdates(t *testing.T) {
	replay := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"title":         "Read ledger",
				"isReplay":      true,
			},
		},
	}
	if events, ok := mapGrokNotification(replay); ok {
		t.Fatalf("replay tool_call must not map, got %+v", events)
	}

	replayMsg := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"isReplay":      true,
				"content":       map[string]any{"type": "text", "text": "Đây là phiên mới"},
			},
		},
	}
	if events, ok := mapGrokNotification(replayMsg); ok {
		t.Fatalf("replay message_chunk must not map, got %+v", events)
	}

	live := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": "Model đang dùng là Grok 4.6"},
			},
		},
	}
	events, ok := mapGrokNotification(live)
	if !ok || len(events) != 1 || events[0].Text != "Model đang dùng là Grok 4.6" {
		t.Fatalf("live chunk = %+v ok=%v", events, ok)
	}
}
