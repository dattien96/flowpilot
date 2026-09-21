package runner

// E2E tests for CP-70 (Devin provider): drive InteractiveService end-to-end
// through the provider registry → live devinAdapter → fake `devin acp` process
// over pipes. Covers the run-level seams the adapter unit tests can't reach:
// registry routing, session-id promotion onto the run (durable resume handle),
// turn-log devin_session entries, and model-switch session/load.

import (
	"strings"
	"testing"
	"time"
)

// devinE2EService builds an InteractiveService whose "devin" registration is
// backed by a real devinAdapter talking to the fake ACP process.
func devinE2EService(t *testing.T, d *devinDispatcher) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyDevin,
		DisplayName:  "Devin",
		Status:       ProviderStatusAvailable,
		Capabilities: (&devinAdapter{}).Capabilities(),
		newAdapter:   func() ProviderRuntimeAdapter { return newDevinAdapter(d, t.TempDir()) },
	})
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

// devinServePromptTurn replies to session/new, replays a streamed answer, and
// records which methods ran. Returns the observed session/new count pointer.
func devinServePromptTurn(fg *fakeDevin, sessionID, answer string, sawNew, sawLoad *int) {
	fg.serve(func(fg *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			if sawNew != nil {
				*sawNew++
			}
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/load":
			if sawLoad != nil {
				*sawLoad++
			}
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/set_config_option":
			fg.reply(m["id"], map[string]any{})
		case "session/prompt":
			go func() {
				fg.notify("session/update", map[string]any{"sessionId": sessionID, "update": map[string]any{
					"sessionUpdate": "agent_message_chunk",
					"content":       map[string]any{"type": "text", "text": answer},
				}})
				fg.notify("session/update", map[string]any{"sessionId": sessionID, "update": map[string]any{
					"sessionUpdate": "usage_update",
					"usage":         map[string]any{"totalTokens": float64(42), "inputTokens": float64(30), "outputTokens": float64(12)},
				}})
				fg.reply(m["id"], map[string]any{"stopReason": "end_turn"})
			}()
		}
	})
}

func devinRunEvents(t *testing.T, svc *InteractiveService, runID string) []ProviderEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[runID]
		var events []ProviderEvent
		var inFlight bool
		if rs != nil {
			events = append(events, rs.events...)
			inFlight = rs.turnInFlight
		}
		svc.mu.Unlock()
		done := false
		for _, ev := range events {
			if ev.Type == EventTurnCompleted || ev.Type == EventTurnFailed {
				done = true
			}
		}
		if done && !inFlight {
			return events
		}
		time.Sleep(5 * time.Millisecond)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs := svc.runs[runID]; rs != nil {
		return append([]ProviderEvent{}, rs.events...)
	}
	return nil
}

// TestDevinE2EChatTurnPromotesSlugSession drives a full chat turn through the
// service: registry resolves the devin adapter, session/new returns a slug id,
// the turn completes, and the run's durable resume handle is promoted to the
// real slug (LastDevinSessionID → realProviderSessionID).
func TestDevinE2EChatTurnPromotesSlugSession(t *testing.T) {
	d, fg := startFakeDevin(t, nil)
	const slug = "working-pentagon"
	devinServePromptTurn(fg, slug, "hello from devin", nil, nil)
	svc := devinE2EService(t, d)

	ph, apiErr := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyDevin, Model: "devin/swe-2-high"})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	if _, apiErr := svc.startTurn(ph.RunID, TurnInput{StepID: "step-chat", Prompt: "hi devin"}, "chat", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	events := devinRunEvents(t, svc, ph.RunID)

	var sawCompleted, sawText bool
	for _, ev := range events {
		if ev.Type == EventTurnCompleted {
			sawCompleted = true
			if strings.Contains(ev.FinalMessage, "hello from devin") {
				sawText = true
			}
		}
		if ev.Type == EventMessageDelta && strings.Contains(ev.Text, "hello from devin") {
			sawText = true
		}
	}
	if !sawCompleted || !sawText {
		t.Fatalf("expected streamed text + turn_completed, got %+v", events)
	}

	svc.mu.Lock()
	rs := svc.runs[ph.RunID]
	realID := ""
	lastTurn := ""
	if rs != nil {
		realID = rs.realProviderSessionID
		lastTurn = rs.lastDevinTurnSessionID
	}
	svc.mu.Unlock()
	if realID != slug {
		t.Fatalf("realProviderSessionID must be promoted to the devin slug, got %q", realID)
	}
	if lastTurn != slug {
		t.Fatalf("lastDevinTurnSessionID must record the slug, got %q", lastTurn)
	}
}

// TestDevinE2EMidChatModelSwitchLoadsSession verifies the BUG-329 parity path:
// a second turn with a model override must session/load the existing slug —
// never session/new a fresh session (that would drop history).
func TestDevinE2EMidChatModelSwitchLoadsSession(t *testing.T) {
	d, fg := startFakeDevin(t, nil)
	const slug = "sore-router"
	var sawNew, sawLoad int
	devinServePromptTurn(fg, slug, "ok", &sawNew, &sawLoad)
	svc := devinE2EService(t, d)

	ph, apiErr := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyDevin, Model: "devin/swe-2-high"})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	if _, apiErr := svc.startTurn(ph.RunID, TurnInput{StepID: "step-chat", Prompt: "first"}, "chat", ""); apiErr != nil {
		t.Fatalf("turn 1: %v", apiErr)
	}
	devinRunEvents(t, svc, ph.RunID)
	if sawNew != 1 {
		t.Fatalf("first turn must session/new once, got %d", sawNew)
	}

	model := "devin/claude-opus-5-xhigh"
	if _, apiErr := svc.startTurn(ph.RunID, TurnInput{StepID: "step-chat", Prompt: "second", Model: &model}, "chat", ""); apiErr != nil {
		t.Fatalf("turn 2: %v", apiErr)
	}
	devinRunEvents(t, svc, ph.RunID)
	if sawNew != 1 {
		t.Fatalf("model switch must reuse session/load, not session/new (sawNew=%d)", sawNew)
	}
	if sawLoad != 1 {
		t.Fatalf("model switch must session/load the slug once, got %d", sawLoad)
	}
}

// TestDevinE2EPermissionRequestFlowsToBridge sends an inbound
// session/request_permission mid-turn and asserts the run surfaces a pending
// approval through the service-level bridge (approval_required event).
func TestDevinE2EPermissionRequestFlowsToBridge(t *testing.T) {
	d, fg := startFakeDevin(t, nil)
	const slug = "dark-zinnia"
	promptStarted := make(chan string, 1)
	fg.serve(func(fg *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": slug})
		case "session/set_config_option":
			fg.reply(m["id"], map[string]any{})
		case "session/prompt":
			go func() {
				promptStarted <- slug
				fg.send(map[string]any{
					"jsonrpc": "2.0", "id": "perm-1", "method": "session/request_permission",
					"params": map[string]any{
						"sessionId": slug,
						"toolCall": map[string]any{"toolCallId": "tc-1", "title": "Run command", "kind": "execute",
							"rawInput": map[string]any{"command": "weird-unrecognized-tool-xyz --frobnicate"}},
						"options":   []any{map[string]any{"optionId": "allow", "name": "Allow"}, map[string]any{"optionId": "reject", "name": "Reject"}},
					},
				})
			}()
		}
	})
	svc := devinE2EService(t, d)

	ph, apiErr := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyDevin, Model: "devin/swe-2-high"})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	if _, apiErr := svc.startTurn(ph.RunID, TurnInput{StepID: "step-chat", Prompt: "write a file"}, "chat", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	select {
	case <-promptStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("session/prompt never reached the fake")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[ph.RunID]
		pending := ""
		if rs != nil {
			pending = rs.pendingApprovalID
		}
		svc.mu.Unlock()
		if pending != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("session/request_permission never surfaced a pending approval")
}
