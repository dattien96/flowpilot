package runner

// Task-421 (windowed timeline store): backward-paging coverage for the chat
// and run timeline endpoints. The desktop keeps only a bounded tail window in
// memory; `beforeSeq` pages older records on demand. Records must come back
// ascending and `truncated` must mean "older records still exist".
// New file — additive only.

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func appendChatRecs(t *testing.T, writer *chatTranscriptWriter, chatID, legRunID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		payload, _ := json.Marshal(map[string]any{"text": chatID + "-" + legRunID + "-" + string(rune('a'+i))})
		if err := writer.append(context.Background(), ChatTranscriptRecord{
			ChatID:   chatID,
			LegRunID: legRunID,
			Type:     EventTypeChatMessageCompleted,
			Payload:  payload,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func getChatTimeline(t *testing.T, svc *InteractiveService, path, chatID string) chatTimelineResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("chatId", chatID)
	rec := httptest.NewRecorder()
	svc.handleChatTimeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatTimelineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestChatTimelineBeforeSeqReturnsTailBoundedAscendingPage(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_pg", legSeq: 0, legState: LegStateActive}
	appendChatRecs(t, writer, "cht_pg", "run-1", 10)

	resp := getChatTimeline(t, svc, "/client/chats/cht_pg/timeline?beforeSeq=8&limit=3", "cht_pg")
	if len(resp.Records) != 3 {
		t.Fatalf("backward page len = %d, want 3", len(resp.Records))
	}
	for i, want := range []int64{5, 6, 7} {
		if resp.Records[i].ChatSeq != want {
			t.Fatalf("record[%d].seq = %d, want %d (page: %+v)", i, resp.Records[i].ChatSeq, want, resp.Records)
		}
	}
	if !resp.Truncated {
		t.Fatal("full backward page must report truncated (older records exist)")
	}
}

func TestChatTimelineBeforeSeqMinusOneReturnsLatestPage(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_lat", legSeq: 0, legState: LegStateActive}
	appendChatRecs(t, writer, "cht_lat", "run-1", 10)

	resp := getChatTimeline(t, svc, "/client/chats/cht_lat/timeline?beforeSeq=-1&limit=4", "cht_lat")
	if len(resp.Records) != 4 {
		t.Fatalf("latest page len = %d, want 4", len(resp.Records))
	}
	if resp.Records[0].ChatSeq != 7 || resp.Records[3].ChatSeq != 10 {
		t.Fatalf("latest page seqs = %d..%d, want 7..10", resp.Records[0].ChatSeq, resp.Records[3].ChatSeq)
	}
	if !resp.Truncated {
		t.Fatal("latest page of a longer transcript must report truncated")
	}
}

func TestChatTimelineBeforeSeqExhaustedReportsNotTruncated(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_end", legSeq: 0, legState: LegStateActive}
	appendChatRecs(t, writer, "cht_end", "run-1", 10)

	// beforeSeq=2 leaves exactly one older record (seq 1) — under the limit,
	// so the client learns the page is final.
	resp := getChatTimeline(t, svc, "/client/chats/cht_end/timeline?beforeSeq=2&limit=5", "cht_end")
	if len(resp.Records) != 1 || resp.Records[0].ChatSeq != 1 {
		t.Fatalf("exhausted page = %+v, want just seq 1", resp.Records)
	}
	if resp.Truncated {
		t.Fatal("short backward page must report truncated=false — nothing older")
	}

	// beforeSeq at/below the first record: empty page, not truncated.
	resp = getChatTimeline(t, svc, "/client/chats/cht_end/timeline?beforeSeq=1&limit=5", "cht_end")
	if len(resp.Records) != 0 || resp.Truncated {
		t.Fatalf("page before first record = %+v truncated=%v, want empty/false", resp.Records, resp.Truncated)
	}
}

func TestChatTimelineBeforeSeqExactMultipleBoundary(t *testing.T) {
	// Edge: when the remaining older records exactly equal the limit the
	// truncated flag is a "probably more" heuristic — the NEXT page must come
	// back empty so the client can converge.
	svc, writer := newCaptureTestService(t)
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_exact", legSeq: 0, legState: LegStateActive}
	appendChatRecs(t, writer, "cht_exact", "run-1", 6)

	resp := getChatTimeline(t, svc, "/client/chats/cht_exact/timeline?beforeSeq=4&limit=3", "cht_exact")
	if len(resp.Records) != 3 || resp.Records[0].ChatSeq != 1 || resp.Records[2].ChatSeq != 3 {
		t.Fatalf("boundary page = %+v, want seqs 1..3", resp.Records)
	}
	// len == limit → truncated=true (heuristic); following page is empty.
	resp = getChatTimeline(t, svc, "/client/chats/cht_exact/timeline?beforeSeq=1&limit=3", "cht_exact")
	if len(resp.Records) != 0 || resp.Truncated {
		t.Fatalf("convergence page = %+v truncated=%v, want empty/false", resp.Records, resp.Truncated)
	}
}

func TestChatTimelineForwardPagingUnchangedWhenBeforeSeqAbsent(t *testing.T) {
	// Regression guard: omitting beforeSeq must keep the legacy afterSeq
	// forward-read semantics byte-for-byte.
	svc, writer := newCaptureTestService(t)
	svc.runs["run-1"] = &interactiveRun{id: "run-1", runKind: "chat", chatID: "cht_fwd", legSeq: 0, legState: LegStateActive}
	appendChatRecs(t, writer, "cht_fwd", "run-1", 5)

	resp := getChatTimeline(t, svc, "/client/chats/cht_fwd/timeline?afterSeq=2&limit=2", "cht_fwd")
	if len(resp.Records) != 2 || resp.Records[0].ChatSeq != 3 || resp.Records[1].ChatSeq != 4 {
		t.Fatalf("forward page = %+v, want seqs 3,4", resp.Records)
	}
}

func TestRunTimelineBeforeSeqBackwardPage(t *testing.T) {
	// Workflow runs persist transcript records under the run id — the same
	// backward paging contract must hold on /client/workflow-runs/{id}/timeline.
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("writer nil")
	}
	svc.runs["run-wf"] = &interactiveRun{id: "run-wf", runKind: "workflow", providerKey: ProviderKeyCodex, status: RunStatusCompleted}
	appendChatRecs(t, writer, "run-wf", "run-wf", 8)

	req := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/run-wf/timeline?beforeSeq=6&limit=2", nil)
	req.SetPathValue("runId", "run-wf")
	rec := httptest.NewRecorder()
	svc.handleGetRunTimeline(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatTimelineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Records) != 2 || resp.Records[0].ChatSeq != 4 || resp.Records[1].ChatSeq != 5 {
		t.Fatalf("run backward page = %+v, want seqs 4,5", resp.Records)
	}
	if !resp.Truncated {
		t.Fatal("full backward page must report truncated")
	}
}

func TestLocalChatTranscriptReadBeforeTailBoundedAscending(t *testing.T) {
	store := newLocalFileChatTranscriptStore(t.TempDir())
	ctx := context.Background()
	recs := []ChatTranscriptRecord{}
	for i := int64(1); i <= 7; i++ {
		recs = append(recs, chatRec("cht_rb", i, EventTypeChatMessageCompleted))
	}
	if err := store.AppendChatRecords(ctx, recs); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadChatRecordsBefore(ctx, "cht_rb", 6, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ChatSeq != 3 || got[1].ChatSeq != 4 || got[2].ChatSeq != 5 {
		t.Fatalf("ReadChatRecordsBefore = %+v, want seqs 3,4,5 ascending", got)
	}
	// beforeSeq beyond the end → full tail page.
	got, err = store.ReadChatRecordsBefore(ctx, "cht_rb", math.MaxInt64, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ChatSeq != 6 || got[1].ChatSeq != 7 {
		t.Fatalf("latest page = %+v, want seqs 6,7", got)
	}
	// limit 0 → all older records.
	got, err = store.ReadChatRecordsBefore(ctx, "cht_rb", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ChatSeq != 1 || got[1].ChatSeq != 2 {
		t.Fatalf("unbounded before page = %+v, want seqs 1,2", got)
	}
}

func TestReadChatRecordsBeforeFallbackForForwardOnlyStore(t *testing.T) {
	// Stores without ChatTranscriptBackwardReader fall back to a forward read
	// + in-memory tail slice — same observable contract.
	store := &forwardOnlyTranscriptStore{recs: []ChatTranscriptRecord{
		chatRec("cht_fb", 1, EventTypeChatMessageCompleted),
		chatRec("cht_fb", 2, EventTypeChatMessageCompleted),
		chatRec("cht_fb", 3, EventTypeChatMessageCompleted),
		chatRec("cht_fb", 4, EventTypeChatMessageCompleted),
	}}
	got, err := readChatRecordsBefore(context.Background(), store, "cht_fb", 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ChatSeq != 2 || got[1].ChatSeq != 3 {
		t.Fatalf("fallback page = %+v, want seqs 2,3", got)
	}
}

// forwardOnlyTranscriptStore implements only the base ChatTranscriptStore —
// exercises the readChatRecordsBefore in-memory fallback path.
type forwardOnlyTranscriptStore struct {
	recs []ChatTranscriptRecord
}

func (f *forwardOnlyTranscriptStore) AppendChatRecords(_ context.Context, recs []ChatTranscriptRecord) error {
	f.recs = append(f.recs, recs...)
	return nil
}

func (f *forwardOnlyTranscriptStore) ReadChatRecords(_ context.Context, _ string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error) {
	out := []ChatTranscriptRecord{}
	for _, rec := range f.recs {
		if rec.ChatSeq <= afterSeq {
			continue
		}
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, rec)
	}
	return out, nil
}

func (f *forwardOnlyTranscriptStore) LatestChatSeq(_ context.Context, _ string) (int64, error) {
	var latest int64
	for _, rec := range f.recs {
		if rec.ChatSeq > latest {
			latest = rec.ChatSeq
		}
	}
	return latest, nil
}
