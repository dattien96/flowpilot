package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CP-89 desktop-wiring review fixes (Task-454 follow-up):
//  - the forward turn may carry the user's FINAL flowRef — it overrides /
//    completes the provisional create-time pin and is fenced identically;
//  - an unpinned, never-launched chat may late-attach a flow via an explicit
//    forwardFlow+flowRef turn (desktop "start flow" on a normal chat);
//  - the forward transcript is CHAT-scoped (durable store across legs), not
//    leg-scoped — reattach/provider-switch legs keep the discussion context.

// A pending run pinned to a provisional/bogus ref can be redirected by the
// forward turn's explicit flowRef — the fences validate the NEW ref and the
// commit stamps it onto chatFlowRef.
func TestCP89Fix_ForwardTurnFlowRefOverridesPin(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "definitely-not-a-real-flow", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{
		StepID: "chat", ForwardFlow: true,
		FlowRef: "task-harness", Prompt: "start the flow",
	}, "", ""); e != nil {
		t.Fatalf("forward with explicit flowRef must pass, got %v", e)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	arm, ref := rs.flowArm, rs.chatFlowRef
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("arm = %q, want started", arm)
	}
	if !strings.HasSuffix(ref, "task-harness") {
		t.Fatalf("committed pin = %q, want the turn-level task-harness ref", ref)
	}
}

// A normal chat (immediate arm, no pin, never flow-driven) can attach a flow
// at forward time — the explicit turn-level ref IS the pin being committed.
func TestCP89Fix_LateAttachForwardOnUnpinnedChat(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", Prompt: "let us discuss the CP first"}, "", ""); e != nil {
		t.Fatalf("chat turn: %v", e)
	}
	waitTurnIdle(t, svc, h.RunID)
	if _, e := svc.startTurn(h.RunID, TurnInput{
		StepID: "chat", ForwardFlow: true,
		FlowRef: "task-harness", Prompt: "ok start it",
	}, "", ""); e != nil {
		t.Fatalf("late-attach forward must pass, got %v", e)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	arm, driven, ref := rs.flowArm, rs.flowEngineDriven, rs.chatFlowRef
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("arm = %q, want started", arm)
	}
	if !driven || !strings.HasSuffix(ref, "task-harness") {
		t.Fatalf("late attach must stamp ref+engineDriven, ref=%q driven=%v", ref, driven)
	}
	// Second forward must NOT relaunch — the latch is consumed.
	_, e := svc.startTurn(h.RunID, TurnInput{
		StepID: "chat", ForwardFlow: true, FlowRef: "task-harness",
	}, "", "")
	if e == nil || e.code != "flow_already_started" {
		t.Fatalf("second forward must 422 flow_already_started, got %v", e)
	}
}

// A forwardFlow on an unpinned immediate run WITHOUT a turn-level flowRef is
// still the old no-pin rejection — late attach is explicit, never inferred.
func TestCP89Fix_LateAttachStillRequiresExplicitFlowRef(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusUnprocessableEntity || e.code != "forward_requires_flow_pin" {
		t.Fatalf("forward without any ref must 422 forward_requires_flow_pin, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmImmediate {
		t.Fatalf("rejected late attach must leave arm untouched, got %q", arm)
	}
}

// A pinned immediate run must not be re-armed through the forward seam even
// when the turn carries a flowRef — it already owns a launch path.
func TestCP89Fix_ForwardOnPinnedImmediateStaysRejected(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	_, e := svc.startTurn(h.RunID, TurnInput{
		StepID: "chat", ForwardFlow: true, FlowRef: "task-harness",
	}, "", "")
	if e == nil || e.code != "flow_already_started" {
		t.Fatalf("forward on pinned immediate must 422 flow_already_started, got %v", e)
	}
}

// The forward transcript read is chat-scoped: seed the durable chat store
// with a discussion that happened on a DIFFERENT leg (pre-switch/reattach
// context the fresh leg's rs.events never saw). The forward entry prompt
// must carry it — observed through the forward_prompt_packed diag entry,
// whose turns_included counts ONLY transcript turns the packer kept.
func TestCP89Fix_ForwardReadsChatScopedTranscript(t *testing.T) {
	storeDir := t.TempDir()
	diagDir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", storeDir)
	t.Setenv("FLOWPILOT_FLOW_DIAG_DIR", diagDir)
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	if rs.chatID == "" {
		rs.chatID = "cht_legshare"
	}
	chatID := rs.chatID
	svc.mu.Unlock()
	// Records as the PREVIOUS leg wrote them (different runId, same chatId).
	w := svc.ensureChatTranscriptWriter()
	if w == nil {
		t.Fatal("transcript writer unavailable")
	}
	if err := w.append(context.Background(),
		ChatTranscriptRecord{ChatID: chatID, LegRunID: "run-old-leg", Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"pre-switch discussion marker"}`)},
		ChatTranscriptRecord{ChatID: chatID, LegRunID: "run-old-leg", Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"old leg reply"}`)},
	); err != nil {
		t.Fatalf("seed transcript: %v", err)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{
		StepID: "chat", ForwardFlow: true, Prompt: "start the flow",
	}, "", ""); e != nil {
		t.Fatalf("forward: %v", e)
	}
	// The diag line is written synchronously inside startTurn.
	raw, readErr := os.ReadFile(filepath.Join(diagDir, h.RunID+".ndjson"))
	if readErr != nil {
		t.Fatalf("read diag: %v", readErr)
	}
	var sawPacked bool
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "forward_prompt_packed") {
			continue
		}
		sawPacked = true
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil {
			if got, ok := entry["turns_included"].(float64); !ok || int(got) != 1 {
				t.Fatalf("turns_included = %v, want 1 (chat-scoped turn survived)", entry["turns_included"])
			}
		}
	}
	if !sawPacked {
		t.Fatal("forward_prompt_packed diag entry missing")
	}
}

// forwardChatTranscriptTurns: system prompts and the trailing unanswered
// (in-flight / the forward's own) turn are excluded; settled cross-leg turns
// survive.
func TestCP89Fix_ForwardTranscriptFiltersUnsettledTail(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := task451Service(t)
	w := svc.ensureChatTranscriptWriter()
	if w == nil {
		t.Fatal("transcript writer unavailable")
	}
	chatID := "cht_filter"
	if err := w.append(context.Background(),
		ChatTranscriptRecord{ChatID: chatID, Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"[flow-engine] gate reprompt"}`)},
		ChatTranscriptRecord{ChatID: chatID, Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"system reply"}`)},
		ChatTranscriptRecord{ChatID: chatID, Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"real user question"}`)},
		ChatTranscriptRecord{ChatID: chatID, Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"real answer"}`)},
		ChatTranscriptRecord{ChatID: chatID, Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"unanswered tail"}`)},
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	turns := svc.forwardChatTranscriptTurns(context.Background(), chatID)
	if len(turns) != 1 {
		t.Fatalf("want exactly 1 settled turn, got %+v", turns)
	}
	if turns[0].User != "real user question" || turns[0].Assistant != "real answer" {
		t.Fatalf("wrong turn kept: %+v", turns[0])
	}
}
