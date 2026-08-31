package runner

// Chat capture + timeline tests (Task-313 T-5/T-6): the persistEvent-adjacent
// capture hook (delta skip, mapping coverage, degraded flag) and the timeline
// endpoint (leg join, self-tag, pagination, flag-off typed 404). New file —
// additive only.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newCaptureTestService(t *testing.T) (*InteractiveService, *chatTranscriptWriter) {
	t.Helper()
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("ensureChatTranscriptWriter returned nil with flag on")
	}
	if svc.chatRuns == nil {
		t.Fatal("registry not initialized with flag on")
	}
	return svc, writer
}

func chatEvent(typ ProviderEventType, runID string) ProviderEvent {
	return ProviderEvent{Type: typ, WorkflowRunID: runID}
}

func TestRecordChatTranscriptSkipsDeltas(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.chatRuns.register("run-1", "cht_a")
	svc.recordChatTranscript(chatEvent(EventMessageDelta, "run-1"))
	svc.recordChatTranscript(chatEvent(EventMessageCompleted, "run-1"))
	got, err := writer.store.ReadChatRecords(context.Background(), "cht_a", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != EventTypeChatMessageCompleted {
		t.Fatalf("delta skip failed: %+v", got)
	}
}

func TestRecordChatTranscriptFlagOffNoop(t *testing.T) {
	// Flag removed: always ON — even with env 0 writer must exist and record.
	t.Setenv("FLOWPILOT_CHAT_SSOT", "0")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	if svc.ensureChatTranscriptWriter() == nil {
		t.Fatal("writer must be non-nil even with flag 0 (always ON)")
	}
	svc.chatRuns.register("run-1", "cht_a")
	svc.recordChatTranscript(chatEvent(EventTurnStarted, "run-1"))
}

func TestRecordChatTranscriptCapturesTurnToolFileApprovalQuestionUsage(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.chatRuns.register("run-1", "cht_a")
	svc.recordChatTranscript(ProviderEvent{Type: EventTurnStarted, WorkflowRunID: "run-1", Prompt: "hello ban la model gi"})
	svc.recordChatTranscript(ProviderEvent{Type: EventToolStarted, WorkflowRunID: "run-1", ToolName: "bash"})
	svc.recordChatTranscript(ProviderEvent{Type: EventToolCompleted, WorkflowRunID: "run-1", ToolName: "bash", Status: "completed"})
	svc.recordChatTranscript(ProviderEvent{Type: EventFileChanged, WorkflowRunID: "run-1", Path: "internal/foo.go", ChangeType: "edit"})
	svc.recordChatTranscript(ProviderEvent{Type: EventPermissionRequired, WorkflowRunID: "run-1", ApprovalID: "appr-1", Details: &ApprovalDetails{Kind: "exec", Command: "npm test"}})
	svc.recordChatTranscript(ProviderEvent{Type: EventUserQuestionRequired, WorkflowRunID: "run-1", QuestionID: "q-1", Prompt: "chon di"})
	svc.recordChatTranscript(ProviderEvent{Type: EventTokenUsageUpdated, WorkflowRunID: "run-1", TokenUsage: &TokenUsageSnapshot{Total: &TokenUsageBreakdown{TotalTokens: 9660}}})
	svc.recordChatTranscript(ProviderEvent{Type: EventTurnCompleted, WorkflowRunID: "run-1", FinalMessage: "toi la Muse Spark"})
	svc.recordChatTranscript(chatEvent(EventTurnFailed, "run-1")) // unknown-to-mapper types record nothing... (TurnFailed maps to nil)

	want := map[string]int{
		EventTypeChatTurnStarted:       1,
		EventTypeChatToolStarted:       1,
		EventTypeChatToolCompleted:     1,
		EventTypeChatFileChanged:       1,
		EventTypeChatApprovalRequested: 1,
		EventTypeChatQuestionAsked:     1,
		EventTypeChatTokenUsage:        1,
		EventTypeChatMessageCompleted:  1,
	}
	got, err := writer.store.ReadChatRecords(context.Background(), "cht_a", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, rec := range got {
		counts[rec.Type]++
		if rec.LegRunID != "run-1" || rec.ChatID != "cht_a" {
			t.Fatalf("record identity wrong: %+v", rec)
		}
	}
	for typ, n := range want {
		if counts[typ] != n {
			t.Fatalf("type %s captured %d times, want %d (all: %v)", typ, counts[typ], n, counts)
		}
	}
	// Approval payload carries kind + command.
	for _, rec := range got {
		if rec.Type == EventTypeChatApprovalRequested {
			var p map[string]any
			if err := json.Unmarshal(rec.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p["kind"] != "exec" || p["command"] != "npm test" {
				t.Fatalf("approval payload = %v", p)
			}
		}
	}
}

func TestRecordChatTranscriptDegradedFlagOnAppendFailure(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked") // a FILE where the store wants a dir
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", blocked) // MkdirAll(chats/cht_a) under a file → append error
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("writer nil")
	}
	svc.chatRuns.register("run-1", "cht_a")
	// Turn must proceed (no panic), flag flips on, second failure is quiet.
	svc.recordChatTranscript(chatEvent(EventTurnStarted, "run-1"))
	if !svc.chatRuns.isDegraded("run-1") {
		t.Fatal("run not marked degraded after append failure")
	}
	svc.recordChatTranscript(chatEvent(EventMessageCompleted, "run-1"))
	if !svc.chatRuns.isDegraded("run-1") {
		t.Fatal("degraded flag lost")
	}
}

func TestChatTimelineUnknownChat404(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	req := httptest.NewRequest(http.MethodGet, "/client/chats/cht_unknown/timeline", nil)
	req.SetPathValue("chatId", "cht_unknown")
	rec := httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "chat_not_found" && !strings.Contains(rec.Body.String(), "chat_not_found") {
		t.Fatalf("error code = %v body=%s", body["code"], rec.Body.String())
	}
}

func TestChatTimelineFlagOffDisabled404(t *testing.T) {
	// Flag removed: timeline must NOT return chat_ssot_disabled even with env 0.
	t.Setenv("FLOWPILOT_CHAT_SSOT", "0")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	req := httptest.NewRequest(http.MethodGet, "/client/chats/cht_x/timeline", nil)
	req.SetPathValue("chatId", "cht_x")
	rec := httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if strings.Contains(rec.Body.String(), "chat_ssot_disabled") {
		t.Fatalf("must not be chat_ssot_disabled when flag removed, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestChatTimelineJoinsLegsInOrderAndSelfTags(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	// Legacy untagged chat leg (chatId == runId).
	svc.runs["run-legacy"] = &interactiveRun{id: "run-legacy", runKind: "chat", providerKey: ProviderKeyCodex, status: RunStatusCompleted}
	// Tagged leg 1 of cht_b.
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_b", legSeq: 0, legState: LegStateClosed, legClosedReason: LegClosedReasonProviderSwitch, providerKey: ProviderKeyOpencode}
	// Tagged leg 2 of cht_b.
	svc.runs["run-2"] = &interactiveRun{id: "run-2", runKind: "chat", chatID: "cht_b", legSeq: 1, legState: LegStateActive, providerKey: ProviderKeyGrok, status: RunStatusIdle}
	// Workflow run must never join.
	svc.runs["run-wf"] = &interactiveRun{id: "run-wf", runKind: "workflow", chatID: "cht_b"}
	_ = writer

	req := httptest.NewRequest(http.MethodGet, "/client/chats/cht_b/timeline", nil)
	req.SetPathValue("chatId", "cht_b")
	rec := httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatTimelineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Legs) != 2 {
		t.Fatalf("legs = %d, want 2 (workflow must not join)", len(resp.Legs))
	}
	if resp.Legs[0].RunID != "run-1" || resp.Legs[1].RunID != "run-2" {
		t.Fatalf("legs not in legSeq order: %+v", resp.Legs)
	}
	if resp.Legs[0].LegClosedReason != LegClosedReasonProviderSwitch {
		t.Fatalf("leg0 reason = %q", resp.Legs[0].LegClosedReason)
	}

	// Legacy chat timeline: self-tag happens and unknown chat 404 stays.
	req = httptest.NewRequest(http.MethodGet, "/client/chats/run-legacy/timeline", nil)
	req.SetPathValue("chatId", "run-legacy")
	rec = httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy status = %d", rec.Code)
	}
	var legacy chatTimelineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Legs) != 1 || legacy.Legs[0].RunID != "run-legacy" {
		t.Fatalf("legacy legs = %+v", legacy.Legs)
	}
}

func TestChatTimelineTailPaginationTruncates(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_c", legSeq: 0, legState: LegStateActive}
	// Distinct texts: the read model collapses consecutive identical finals, so
	// a pagination test needs distinguishable records.
	for i := 0; i < 5; i++ {
		payload, _ := json.Marshal(map[string]any{"text": string(rune('a' + i))})
		if err := writer.append(context.Background(), ChatTranscriptRecord{ChatID: "cht_c", LegRunID: "run-1", Type: EventTypeChatMessageCompleted, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/client/chats/cht_c/timeline?afterSeq=2&limit=2", nil)
	req.SetPathValue("chatId", "cht_c")
	rec := httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp chatTimelineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Records) != 2 || resp.Records[0].ChatSeq != 3 || resp.Records[1].ChatSeq != 4 {
		t.Fatalf("page = %v", resp.Records)
	}
	if !resp.Truncated || resp.NextSeq != 4 {
		t.Fatalf("truncated=%v nextSeq=%d", resp.Truncated, resp.NextSeq)
	}
}

// TestCapturePersistsAcrossWriterRestart pins the writer's per-chat seq seed
// through the service accessor (restart continuation).
func TestCapturePersistsAcrossWriterRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", dir)
	first := &InteractiveService{runs: map[string]*interactiveRun{}}
	w1 := first.ensureChatTranscriptWriter()
	first.chatRuns.register("run-1", "cht_d")
	first.recordChatTranscript(chatEvent(EventTurnStarted, "run-1"))

	// A "restarted" service in the same process: fresh service, same dir.
	second := &InteractiveService{runs: map[string]*interactiveRun{}}
	w2 := second.ensureChatTranscriptWriter()
	second.chatRuns.register("run-2", "cht_d")
	second.recordChatTranscript(ProviderEvent{Type: EventMessageCompleted, WorkflowRunID: "run-2", Text: "after restart"})
	got, err := w2.store.ReadChatRecords(context.Background(), "cht_d", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].ChatSeq != 2 {
		t.Fatalf("restart continuation = %+v", got)
	}
	if got[1].LegRunID != "run-2" {
		t.Fatalf("second leg not attributed: %+v", got[1])
	}
	_ = w1
}
