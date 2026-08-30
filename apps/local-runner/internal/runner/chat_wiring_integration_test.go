package runner

// Integration wiring tests (Task-313 DOD-1): chatId mint/adopt through the
// REAL createRun path, and the one-shot legacy backfill. New file — additive.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestChatIDMintedOnFirstNormalChat(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	pattern := regexp.MustCompile(`^cht_[0-9a-f]{12}$`)
	if !pattern.MatchString(handle.ChatID) {
		t.Fatalf("handle.ChatID = %q, want cht_<12-hex>", handle.ChatID)
	}
	if handle.LegSeq != 0 {
		t.Fatalf("first leg legSeq = %d, want 0", handle.LegSeq)
	}
	// Persisted row carries the chat identity too (ChatSessionReader surface).
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	svc.mu.Unlock()
	if rs == nil || rs.chatID != handle.ChatID || rs.legState != LegStateActive {
		t.Fatalf("run not stamped: %+v", rs)
	}

	// Workflow runs stay untagged (chat-kind gate absolute — CS-12 substrate).
	wf, err := svc.createRun(StartRunInput{ProjectID: "proj", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4", StepID: "fix"})
	if err != nil {
		t.Fatalf("workflow createRun: %v", err)
	}
	if wf.ChatID != "" || wf.LegSeq != 0 {
		t.Fatalf("workflow run tagged: chatID=%q legSeq=%d", wf.ChatID, wf.LegSeq)
	}
}

func TestChatIDAdoptedFromSwitchFromRunID(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc, _ := newTestServer(t)
	first, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("first createRun: %v", err)
	}
	// Same chat, explicit next leg (Task-314 phase-A computed seq). Provider
	// choice is irrelevant to mint/adopt — codex is the only fake-available
	// runtime in the test registry.
	second, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, ChatID: first.ChatID, LegSeq: 1, SwitchFromRunID: first.RunID})
	if err != nil {
		t.Fatalf("second createRun: %v", err)
	}
	if second.ChatID != first.ChatID || second.LegSeq != 1 {
		t.Fatalf("adopt = (chatID:%q legSeq:%d), want (%q 1)", second.ChatID, second.LegSeq, first.ChatID)
	}
	// Same chat, implicit seq → resident max + 1.
	third, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, ChatID: first.ChatID})
	if err != nil {
		t.Fatalf("third createRun: %v", err)
	}
	if third.ChatID != first.ChatID || third.LegSeq != 2 {
		t.Fatalf("implicit seq = (chatID:%q legSeq:%d), want (%q 2)", third.ChatID, third.LegSeq, first.ChatID)
	}
	// SwitchFromRunID adoption without explicit chat/seq — switches always
	// originate from the CURRENT active leg, so adopt from the latest.
	fourth, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, SwitchFromRunID: third.RunID})
	if err != nil {
		t.Fatalf("fourth createRun: %v", err)
	}
	if fourth.ChatID != first.ChatID || fourth.LegSeq != 3 {
		t.Fatalf("switchFromRunID adopt = (chatID:%q legSeq:%d)", fourth.ChatID, fourth.LegSeq)
	}
	// Timeline joins all legs of the chat, ordered.
	req := httptest.NewRequest(http.MethodGet, "/client/chats/"+first.ChatID+"/timeline", nil)
	req.SetPathValue("chatId", first.ChatID)
	rec := httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatTimelineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Legs) != 4 {
		t.Fatalf("legs = %d, want 4", len(resp.Legs))
	}
	for i, leg := range resp.Legs {
		if leg.LegSeq != i {
			t.Fatalf("legs not ordered by legSeq: [%d]=%d", i, leg.LegSeq)
		}
	}
}

func TestLegacyChatBackfillRawOneShot(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc, _ := newTestServer(t)
	// A pre-flag chat run: created with flag semantics but events attached
	// without capture (simulate by an unregistered runID → capture skipped).
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	// Strip the SSOT identity: this run now looks like a pre-CP-59 legacy chat.
	legacyID := rs.id
	rs.chatID = ""
	rs.legSeq = 0
	rs.legState = ""
	// Attach a two-turn transcript directly (as reconstructRun would after a
	// restart — capture never saw these events).
	rs.events = []ProviderEvent{
		{Type: EventTurnStarted, WorkflowRunID: legacyID, Prompt: "hello ban la model gi"},
		{Type: EventTurnCompleted, WorkflowRunID: legacyID, FinalMessage: "toi la Muse Spark"},
		{Type: EventTurnStarted, WorkflowRunID: legacyID, Prompt: "con ban la model gi"},
		{Type: EventTurnCompleted, WorkflowRunID: legacyID, FinalMessage: "deepseek-v4-flash"},
	}
	svc.mu.Unlock()

	readTimeline := func() chatTimelineResponse {
		req := httptest.NewRequest(http.MethodGet, "/client/chats/"+legacyID+"/timeline", nil)
		req.SetPathValue("chatId", legacyID)
		rec := httptest.NewRecorder()
		svc.handleChatTimeline(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("timeline status = %d body=%s", rec.Code, rec.Body.String())
		}
		var resp chatTimelineResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}

	first := readTimeline()
	// 2 turns × (prompt + final) + marker.
	if len(first.Records) != 5 {
		t.Fatalf("backfill records = %d, want 5", len(first.Records))
	}
	var sawBackfillTurn, sawMarker bool
	for _, rec := range first.Records {
		if rec.Type == EventTypeChatBackfillMarker {
			sawMarker = true
			continue
		}
		if rec.Type == EventTypeChatTurnStarted || rec.Type == EventTypeChatMessageCompleted {
			var p map[string]any
			if err := json.Unmarshal(rec.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p["backfill"] == true {
				sawBackfillTurn = true
			}
		}
	}
	if !sawBackfillTurn || !sawMarker {
		t.Fatalf("backfill records missing: sawBackfillTurn=%v sawMarker=%v", sawBackfillTurn, sawMarker)
	}

	// Second read: one-shot — no duplicate records.
	second := readTimeline()
	if len(second.Records) != len(first.Records) {
		t.Fatalf("backfill not one-shot: first=%d second=%d", len(first.Records), len(second.Records))
	}
}
