package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

// BUG-615 (live run-150388 / chat cht_ce24f4dd4cbc): an engine-internal turn —
// a hub reinvoke, gate reprompt, or owner-debate/synthesis turn whose prompt
// was display-redacted by liveTurnStartedDisplayPrompt — still recorded its
// message_completed/tool_* prose into the chat transcript and rendered it as
// main-chat bubbles ("subagent chat in the main UI"). The fix marks the whole
// turn Internal at emitLocked (turn_scoped), skips prose records at capture
// (chatRecordsFromProviderEvent), and suppresses pre-fix records at read
// (suppressInternalTurnRecords) — the durable log keeps everything.

func mkTS(leg, prompt string, internal bool) ChatTranscriptRecord {
	p, _ := json.Marshal(map[string]any{"prompt": prompt, "internal": internal})
	return ChatTranscriptRecord{ChatID: "c", LegRunID: leg, Type: EventTypeChatTurnStarted, Payload: p}
}

// hubAll treats every leg as a flow hub — the shape these window tests target.
var hubAll = func(string) bool { return true }

// noHubs treats every leg as a child/plain leg — the empty-prompt legacy
// heuristic must never fire there.
var noHubs = func(string) bool { return false }

func mkRec(leg, typ string, fields map[string]any) ChatTranscriptRecord {
	p, _ := json.Marshal(fields)
	return ChatTranscriptRecord{ChatID: "c", LegRunID: leg, Type: typ, Payload: p}
}

// Internal window: prose/tool records after an empty-prompt turn_started are
// suppressed until the next turn boundary; a user prompt closes the window.
func TestSuppressInternalTurnRecords_InternalWindow(t *testing.T) {
	recs := []ChatTranscriptRecord{
		mkTS("leg-1", "run this CP", false), // user prompt — window closed
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "user-facing reply"}),
		mkTS("leg-1", "", false), // engine turn (redacted prompt)
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "hub synthesis narration"}),
		mkRec("leg-1", EventTypeChatToolStarted, map[string]any{"tool": "submit_review_outcome"}),
		mkRec("leg-1", EventTypeChatToolCompleted, map[string]any{"tool": "submit_review_outcome", "status": "success"}),
		mkRec("leg-1", EventTypeChatFileChanged, map[string]any{"path": "x.go"}), // file rows stay
		mkTS("leg-1", "", false), // another engine turn
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "owner debate prose"}),
		mkTS("leg-1", "thanks", false), // user prompt closes the window
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "welcome"}),
	}
	out := suppressInternalTurnRecords(recs, hubAll)
	var texts []string
	var kinds []string
	for _, r := range out {
		kinds = append(kinds, r.Type)
		if r.Type == EventTypeChatMessageCompleted {
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(r.Payload, &p)
			texts = append(texts, p.Text)
		}
	}
	joined := strings.Join(texts, "|")
	if strings.Contains(joined, "hub synthesis narration") || strings.Contains(joined, "owner debate prose") {
		t.Fatalf("internal prose leaked through suppression: %v", texts)
	}
	if !strings.Contains(joined, "user-facing reply") || !strings.Contains(joined, "welcome") {
		t.Fatalf("user-facing prose dropped by suppression: %v", texts)
	}
	toolRecs, fileRecs := 0, 0
	for _, k := range kinds {
		if k == EventTypeChatToolStarted || k == EventTypeChatToolCompleted {
			toolRecs++
		}
		if k == EventTypeChatFileChanged {
			fileRecs++
		}
	}
	if toolRecs != 0 {
		t.Fatalf("internal tool records kept: %d", toolRecs)
	}
	if fileRecs != 1 {
		t.Fatalf("file_changed record dropped: got %d want 1 (artifacts are not prose)", fileRecs)
	}
}

// Explicit internal:true on the turn_started payload (new records) opens the
// same window even when a prompt text is present.
func TestSuppressInternalTurnRecords_ExplicitFlag(t *testing.T) {
	recs := []ChatTranscriptRecord{
		mkTS("leg-1", "Synthesize the owner debate now", true),
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "narration"}),
	}
	out := suppressInternalTurnRecords(recs, hubAll)
	for _, r := range out {
		if r.Type == EventTypeChatMessageCompleted {
			t.Fatalf("internal:true turn prose leaked")
		}
	}
}

// Records before a leg's first turn boundary fail open — a mid-turn page must
// not hide possibly-user-facing prose it cannot classify.
func TestSuppressInternalTurnRecords_FailsOpenBeforeBoundary(t *testing.T) {
	recs := []ChatTranscriptRecord{
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "page-start prose"}),
	}
	out := suppressInternalTurnRecords(recs, hubAll)
	if len(out) != 1 {
		t.Fatalf("pre-boundary record dropped: got %d want 1", len(out))
	}
}

// A turn_started whose payload omits the prompt key (non-internal redacted
// turn: handoff seed, gate reprompt) must NOT open the window — its reply is
// user-visible output. New records use this shape so seed turns keep their
// answers after reopen.
func TestSuppressInternalTurnRecords_AbsentPromptFailsOpen(t *testing.T) {
	p, _ := json.Marshal(map[string]any{"eseq": 3, "internal": false}) // no "prompt" key
	recs := []ChatTranscriptRecord{
		{ChatID: "c", LegRunID: "leg-1", Type: EventTypeChatTurnStarted, Payload: p},
		mkRec("leg-1", EventTypeChatMessageCompleted, map[string]any{"text": "continuation answer"}),
	}
	out := suppressInternalTurnRecords(recs, hubAll)
	found := false
	for _, r := range out {
		if r.Type == EventTypeChatMessageCompleted {
			found = true
		}
	}
	if !found {
		t.Fatalf("seed-turn reply suppressed by absent-prompt boundary — must fail open")
	}
}

// A legacy empty-prompt boundary on a NON-hub leg (child leg spawn envelope,
// plain-chat handoff seed) must not open the window — the leg's replies are
// real work. Hub gating is what keeps "subagent responses show in their own
// chat" true for pre-fix records too.
func TestSuppressInternalTurnRecords_ChildLegLegacyRecordsKept(t *testing.T) {
	recs := []ChatTranscriptRecord{
		mkTS("child-1", "", false), // legacy sub-agent envelope boundary
		mkRec("child-1", EventTypeChatMessageCompleted, map[string]any{"text": "child work reply"}),
	}
	out := suppressInternalTurnRecords(recs, noHubs)
	if len(out) != 2 {
		t.Fatalf("child-leg legacy records suppressed — must keep, got %d", len(out))
	}
	// Same records on a hub leg still suppress (the flood being fixed).
	out = suppressInternalTurnRecords(recs, func(string) bool { return true })
	if len(out) != 1 {
		t.Fatalf("hub-leg legacy internal prose kept — got %d want 1 (boundary only)", len(out))
	}
}

// Capture side: Internal events write no prose/tool transcript records, while
// the turn_started boundary record is kept (it is the read-side marker).
func TestChatRecordsFromProviderEvent_InternalSkipsProse(t *testing.T) {
	internal := ProviderEvent{Type: EventMessageCompleted, Text: "narration", Internal: true}
	if recs := chatRecordsFromProviderEvent("c", internal); len(recs) != 0 {
		t.Fatalf("internal message_completed recorded: %d recs", len(recs))
	}
	for _, ev := range []ProviderEvent{
		{Type: EventTurnCompleted, FinalMessage: "narration", Internal: true},
		{Type: EventToolStarted, ToolName: "submit_review_outcome", Internal: true},
		{Type: EventToolCompleted, ToolName: "submit_review_outcome", Internal: true},
	} {
		if recs := chatRecordsFromProviderEvent("c", ev); len(recs) != 0 {
			t.Fatalf("internal %s recorded: %d recs", ev.Type, len(recs))
		}
	}
	// The boundary record survives — its (redacted) prompt is the read-side
	// window marker, and it now also carries the explicit internal flag.
	ts := chatRecordsFromProviderEvent("c", ProviderEvent{Type: EventTurnStarted, Internal: true})
	if len(ts) != 1 || ts[0].Type != EventTypeChatTurnStarted {
		t.Fatalf("internal turn_started not kept as boundary: %v", ts)
	}
	var p struct {
		Internal bool    `json:"internal"`
		Prompt   *string `json:"prompt"`
	}
	if err := json.Unmarshal(ts[0].Payload, &p); err != nil || !p.Internal {
		t.Fatalf("internal flag missing on boundary record: %s", ts[0].Payload)
	}
	if p.Prompt == nil || *p.Prompt != "" {
		t.Fatalf("internal boundary must carry empty prompt marker: %s", ts[0].Payload)
	}
	// A non-internal redacted turn (handoff seed / gate reprompt) omits the
	// prompt key so the read-side window fails open on its reply.
	seed := chatRecordsFromProviderEvent("c", ProviderEvent{Type: EventTurnStarted})
	var sp struct {
		Internal bool    `json:"internal"`
		Prompt   *string `json:"prompt"`
	}
	if err := json.Unmarshal(seed[0].Payload, &sp); err != nil {
		t.Fatalf("seed boundary unmarshal: %v", err)
	}
	if sp.Prompt != nil {
		t.Fatalf("non-internal redacted boundary must omit prompt key: %s", seed[0].Payload)
	}
	// Non-prose internals still record — approvals/questions must surface.
	ap := chatRecordsFromProviderEvent("c", ProviderEvent{Type: EventPermissionRequired, ApprovalID: "a1", Internal: true})
	if len(ap) != 1 {
		t.Fatalf("internal approval dropped: %v", ap)
	}
}

// Non-internal events keep recording exactly as before (no over-redaction).
func TestChatRecordsFromProviderEvent_UserFacingUntouched(t *testing.T) {
	if recs := chatRecordsFromProviderEvent("c", ProviderEvent{Type: EventMessageCompleted, Text: "hi"}); len(recs) != 1 {
		t.Fatalf("user message_completed dropped")
	}
	if recs := chatRecordsFromProviderEvent("c", ProviderEvent{Type: EventToolStarted, ToolName: "Bash"}); len(recs) != 1 {
		t.Fatalf("user tool_started dropped")
	}
}

// Resume reconstruction: a system-prompt turn_started opens an internal window
// so the turn's reconstructed prose carries Internal — matching the live
// emitLocked stamping — while a user turn keeps its prose unmarked.
func TestUserFacingTranscriptEvents_StampsInternalWindow(t *testing.T) {
	hist := []ProviderEvent{
		{Type: EventTurnStarted, Prompt: "real user question"},
		{Type: EventMessageCompleted, Text: "answer one"},
		{Type: EventTurnStarted, Prompt: "[flow-engine] Agent results ready. Synthesize the join note above. You must call submit_review_outcome."},
		{Type: EventMessageCompleted, Text: "engine narration"},
		{Type: EventToolCompleted, ToolName: "submit_review_outcome"},
		{Type: EventTurnStarted, Prompt: "next user question"},
		{Type: EventMessageCompleted, Text: "answer two"},
	}
	out := userFacingTranscriptEvents(hist, true)
	var msgs []ProviderEvent
	for _, e := range out {
		if e.Type == EventMessageCompleted {
			msgs = append(msgs, e)
		}
	}
	if len(msgs) != 3 {
		t.Fatalf("message count = %d, want 3 (internal prose marked, not dropped)", len(msgs))
	}
	if msgs[0].Internal || msgs[2].Internal {
		t.Fatalf("user-facing prose stamped internal: %+v", msgs)
	}
	if !msgs[1].Internal {
		t.Fatalf("engine narration not stamped internal: %+v", msgs[1])
	}
}

// Regression guard: a flow-engine prompt landing on a CHILD leg is that leg's
// real work — the "[FlowPilot sub-agent — …]" spawn envelope and
// "[flow-engine] Review this result" reviewer instructions are instructions to
// the child, and the child's reply is user-facing work output in its own chat.
// Internal classification is hub-only (BUG-615 follow-up: CP-02 child legs
// rendered nothing after the prompt-only classifier shipped).
func TestUserFacingTranscriptEvents_ChildLegTurnsStayVisible(t *testing.T) {
	hist := []ProviderEvent{
		{Type: EventTurnStarted, Prompt: "[FlowPilot sub-agent — agent: coder | role: coder]\n\nimplement it"},
		{Type: EventMessageCompleted, Text: "child work reply"},
		{Type: EventTurnStarted, Prompt: "[flow-engine] Review this result from node \"coder\""},
		{Type: EventMessageCompleted, Text: "review verdict reply"},
	}
	out := userFacingTranscriptEvents(hist, false)
	for _, e := range out {
		if e.Type == EventMessageCompleted && e.Internal {
			t.Fatalf("child-leg reply stamped internal — child chats would go blank: %+v", e)
		}
	}
}

// turnIsInternal: hub-only classification — dispatch-marked or flow-engine
// prompt on the flow hub is internal; the same prompt on a child leg is the
// leg's own work.
func TestTurnIsInternal_HubOnly(t *testing.T) {
	hub := &interactiveRun{parentRunID: "", flowEngineDriven: true}
	child := &interactiveRun{parentRunID: "hub", flowEngineDriven: false}
	plain := &interactiveRun{}
	if !turnIsInternal(TurnInput{Prompt: "[flow-engine] Agent results ready"}, hub) {
		t.Fatal("flow-engine prompt on hub must be internal")
	}
	if turnIsInternal(TurnInput{Prompt: "[flow-engine] Agent results ready"}, child) {
		t.Fatal("flow-engine prompt on a child leg must stay visible")
	}
	if turnIsInternal(TurnInput{Prompt: "[FlowPilot sub-agent — agent: coder]"}, child) {
		t.Fatal("sub-agent envelope on the child leg must stay visible")
	}
	if !turnIsInternal(TurnInput{Internal: true}, hub) {
		t.Fatal("dispatch-marked internal must win on hub")
	}
	if turnIsInternal(TurnInput{Internal: true}, plain) {
		t.Fatal("internal flag on a plain chat is meaningless — dispatch guards it")
	}
	if turnIsInternal(TurnInput{Prompt: "user ask"}, hub) {
		t.Fatal("user prompt on hub stays visible")
	}
}
