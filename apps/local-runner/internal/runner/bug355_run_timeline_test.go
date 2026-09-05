package runner

// BUG-355 F2 tests (new file — additive only): workflow runs carry no chat
// id, but their transcript must persist under the run id and the run-scoped
// timeline endpoint must serve it so TUI /open can restore messages.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRunTimelinePersistsWorkflowRunWithoutChat pins the F2 capture fix: a
// run with no chat registration must still persist records, keyed by run id.
func TestRunTimelinePersistsWorkflowRunWithoutChat(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	// No chatRuns.register — workflow runs never get a chat id.
	svc.recordChatTranscript(ProviderEvent{Type: EventTurnStarted, WorkflowRunID: "run-wf", Prompt: "do the thing"})
	svc.recordChatTranscript(ProviderEvent{Type: EventMessageCompleted, WorkflowRunID: "run-wf", Text: "did the thing"})
	got, err := writer.store.ReadChatRecords(context.Background(), "run-wf", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("records = %d, want 2 persisted under the run id", len(got))
	}
	for _, rec := range got {
		if rec.ChatID != "run-wf" || rec.LegRunID != "run-wf" {
			t.Fatalf("record identity wrong: %+v", rec)
		}
	}
}

// TestRunTimelineCaptureStampsEventSeq pins the BUG-355 F2 overlap join key:
// turn/message records carry the source event seq so the TUI can skip turns
// the open replay already rendered.
func TestRunTimelineCaptureStampsEventSeq(t *testing.T) {
	svc, writer := newCaptureTestService(t)
	svc.recordChatTranscript(ProviderEvent{Type: EventTurnStarted, WorkflowRunID: "run-wf", Prompt: "hi", Seq: 7})
	svc.recordChatTranscript(ProviderEvent{Type: EventMessageCompleted, WorkflowRunID: "run-wf", Text: "yo", Seq: 9})
	got, err := writer.store.ReadChatRecords(context.Background(), "run-wf", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("records = %d, want 2", len(got))
	}
	for i, want := range []int64{7, 9} {
		var p map[string]any
		if err := json.Unmarshal(got[i].Payload, &p); err != nil {
			t.Fatal(err)
		}
		if seq, ok := p["eseq"].(float64); !ok || int64(seq) != want {
			t.Fatalf("record %d eseq = %v, want %d", i, p["eseq"], want)
		}
	}
}

// TestRunTimelineEndpointServesRunRecords drives the F2 read path end to end:
// resident workflow run + persisted records → 200 with one leg + records.
func TestRunTimelineEndpointServesRunRecords(t *testing.T) {
	svc, _ := newCaptureTestService(t)
	svc.runs["run-wf"] = &interactiveRun{id: "run-wf", runKind: "workflow", providerKey: ProviderKeyOpencode, status: RunStatusCompleted}
	svc.recordChatTranscript(ProviderEvent{Type: EventTurnStarted, WorkflowRunID: "run-wf", Prompt: "do the thing"})
	svc.recordChatTranscript(ProviderEvent{Type: EventMessageCompleted, WorkflowRunID: "run-wf", Text: "did the thing"})

	req := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/run-wf/timeline", nil)
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
	if len(resp.Legs) != 1 || resp.Legs[0].RunID != "run-wf" {
		t.Fatalf("legs = %+v, want exactly the run", resp.Legs)
	}
	if len(resp.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(resp.Records))
	}
}

// TestRunTimelineUnknownRun404 pins the empty case: no resident run and no
// persisted session → typed 404, not an empty 200.
func TestRunTimelineUnknownRun404(t *testing.T) {
	svc, _ := newCaptureTestService(t)
	req := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/run-missing/timeline", nil)
	req.SetPathValue("runId", "run-missing")
	rec := httptest.NewRecorder()
	svc.handleGetRunTimeline(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
