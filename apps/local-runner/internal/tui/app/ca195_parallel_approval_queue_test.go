package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// G1 (CA-195, BUG-157/158): a turn can fan out several approval/question cards
// in parallel. The TUI used to keep a single pointer, silently dropping every
// card but the last — the dropped card then blocked the run with no UI to
// resolve it. These tests lock in the queued (head + slice) behavior.

func TestParallelApprovals_QueuedNoOrphan(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-1", WorkflowRunID: "run-1",
	}})
	am := m2.(*AppModel)
	m2, _ = am.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-2", WorkflowRunID: "run-1",
	}})
	am = m2.(*AppModel)

	if len(am.approvals) != 2 {
		t.Fatalf("queue len=%d, want 2", len(am.approvals))
	}
	if am.approval == nil || am.approval.ID != "ap-1" {
		t.Fatalf("head=%+v, want ap-1", am.approval)
	}
	if am.approvals[1].ID != "ap-2" {
		t.Fatalf("second card=%+v, want ap-2", am.approvals[1])
	}
	if !am.sendBlocked() {
		t.Fatal("pending parallel approvals must block a new chat turn")
	}
	if n := strings.Count(am.View(), "[APPROVAL]"); n != 2 {
		t.Fatalf("[APPROVAL] lines=%d, want 2 (both cards must surface)\n%s", n, am.View())
	}
}

func TestParallelApprovals_DedupKeepsOneCard(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-1", WorkflowRunID: "run-1",
	}})
	am := m2.(*AppModel)
	m2, _ = am.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-1", WorkflowRunID: "run-1",
	}})
	am = m2.(*AppModel)
	if len(am.approvals) != 1 {
		t.Fatalf("queue len=%d, want 1 (same ID deduped)", len(am.approvals))
	}
	if n := strings.Count(am.View(), "[APPROVAL]"); n != 1 {
		t.Fatalf("[APPROVAL] lines=%d, want 1", n)
	}
}

func TestParallelApprovals_ResolveHeadAdvancesQueue(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-1", WorkflowRunID: "run-1",
	}})
	am := m2.(*AppModel)
	m2, _ = am.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-2", WorkflowRunID: "run-1",
	}})
	am = m2.(*AppModel)

	m2, _ = am.Update(ApprovalResolvedMsg{ID: "ap-1", Decision: "approve"})
	am = m2.(*AppModel)

	if len(am.approvals) != 1 {
		t.Fatalf("queue len=%d, want 1", len(am.approvals))
	}
	if am.approval == nil || am.approval.ID != "ap-2" {
		t.Fatalf("head=%+v, want ap-2 to advance", am.approval)
	}
	if am.connStatus != ConnWaiting {
		t.Fatalf("status=%v, want ConnWaiting while ap-2 remains", am.connStatus)
	}
	if !am.sendBlocked() {
		t.Fatal("composer must stay blocked while ap-2 is still pending")
	}
	if !strings.Contains(am.View(), "1 more pending") {
		t.Fatalf("missing queue message:\n%s", am.View())
	}
}

func TestParallelApprovals_ResolveAllSendsBatch(t *testing.T) {
	var mu sync.Mutex
	var decisions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/client/approvals/") && strings.HasSuffix(r.URL.Path, "/decision") {
			var body struct {
				Decision string `json:"decision"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			decisions = append(decisions, body.Decision)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.yolo = false
	m.pushApproval(ApprovalState{ID: "ap-1", RunID: "run-1"})
	m.pushApproval(ApprovalState{ID: "ap-2", RunID: "run-1"})

	resModel, cmd := m.resolveAllApprovals("approve")
	if cmd == nil {
		t.Fatal("expected batch cmd for all queued approvals")
	}
	// tea.Batch returns a BatchMsg ([]Cmd); the runtime runs each sub-cmd.
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("batch cmd returned %T, want tea.BatchMsg", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("batch len=%d, want 2", len(batch))
	}
	for _, c := range batch {
		_ = c()
	}
	mu.Lock()
	got := append([]string(nil), decisions...)
	mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("POSTs=%d, want 2", len(got))
	}
	for _, d := range got {
		if d != "approve" {
			t.Fatalf("decision=%q, want approve", d)
		}
	}

	am := resModel.(*AppModel)
	m2, _ := am.Update(ApprovalResolvedMsg{ID: "ap-1", Decision: "approve"})
	m2, _ = m2.Update(ApprovalResolvedMsg{ID: "ap-2", Decision: "approve"})
	am = m2.(*AppModel)
	if len(am.approvals) != 0 || am.approval != nil {
		t.Fatalf("queue not empty after resolve-all: approvals=%+v head=%+v", am.approvals, am.approval)
	}
	// sendBlocked stays true while the run is ConnRunning (a turn is in
	// progress); with no pending approvals and an idle run it must unblock.
	am.connStatus = ConnIdle
	if am.sendBlocked() {
		t.Fatal("composer must unblock after all approvals resolved")
	}
}

func TestApproveAll_NoPendingShowsMessage(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	_, cmd := m.resolveAllApprovals("approve")
	if cmd != nil {
		t.Fatal("no cmd when no approvals pending")
	}
	if !strings.Contains(m.View(), "No pending approval.") {
		t.Fatalf("missing no-pending message:\n%s", m.View())
	}
}

func TestParallelApprovals_SingleCardPreservesOldUx(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.pushApproval(ApprovalState{ID: "ap-1", RunID: "run-1"})
	m2, _ := m.Update(ApprovalResolvedMsg{ID: "ap-1", Decision: "approve"})
	am := m2.(*AppModel)
	if am.approval != nil {
		t.Fatal("single approval must clear after resolve")
	}
	if len(am.approvals) != 0 {
		t.Fatalf("queue len=%d, want 0", len(am.approvals))
	}
	if am.connStatus != ConnRunning {
		t.Fatalf("status=%v, want ConnRunning", am.connStatus)
	}
	if !strings.Contains(am.View(), "Approved.") {
		t.Fatalf("missing Approved. message:\n%s", am.View())
	}
	if strings.Contains(am.View(), "more pending") {
		t.Fatal("single card must not mention a queue")
	}
}

func TestParallelQuestions_QueueAdvances(t *testing.T) {
	m := New(config.ChatConfig{Provider: "claude", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "user_question_required", QuestionID: "q-1", Prompt: "Pick one", WorkflowRunID: "run-1",
	}})
	am := m2.(*AppModel)
	m2, _ = am.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "user_question_required", QuestionID: "q-2", Prompt: "Pick another", WorkflowRunID: "run-1",
	}})
	am = m2.(*AppModel)

	if len(am.questions) != 2 {
		t.Fatalf("queue len=%d, want 2", len(am.questions))
	}
	if am.question == nil || am.question.ID != "q-1" {
		t.Fatalf("head=%+v, want q-1", am.question)
	}

	m2, _ = am.Update(QuestionResolvedMsg{ID: "q-1", Choice: "A"})
	am = m2.(*AppModel)
	if len(am.questions) != 1 {
		t.Fatalf("queue len=%d, want 1", len(am.questions))
	}
	if am.question == nil || am.question.ID != "q-2" {
		t.Fatalf("head=%+v, want q-2 to advance", am.question)
	}
	if am.connStatus != ConnWaiting {
		t.Fatalf("status=%v, want ConnWaiting while q-2 remains", am.connStatus)
	}
	if !strings.Contains(am.View(), "1 question(s) still pending.") {
		t.Fatalf("missing queue message:\n%s", am.View())
	}
}

func TestParallelApprovals_BarShowsCountAndBulkChips(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.pushApproval(ApprovalState{ID: "ap-1", RunID: "run-1"})
	m.pushApproval(ApprovalState{ID: "ap-2", RunID: "run-1"})

	view := m.View()
	if !strings.Contains(view, "approval 1/2") {
		t.Fatalf("missing head count in bar:\n%s", view)
	}
	if !strings.Contains(view, "Approve all") || !strings.Contains(view, "Deny all") {
		t.Fatalf("missing bulk chips in bar:\n%s", view)
	}
	if _, _, ok := findClickTarget(m, "approve-all"); !ok {
		t.Fatal("expected clickable Approve all")
	}
	if _, _, ok := findClickTarget(m, "deny-all"); !ok {
		t.Fatal("expected clickable Deny all")
	}
}

func TestParallelQuestions_BarShowsCount(t *testing.T) {
	m := New(config.ChatConfig{Provider: "claude", Yolo: false}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.pushQuestion(QuestionState{ID: "q-1", Prompt: "Pick one", Options: []map[string]string{{"label": "A"}}})
	m.pushQuestion(QuestionState{ID: "q-2", Prompt: "Pick another", Options: []map[string]string{{"label": "B"}}})

	view := m.View()
	if !strings.Contains(view, "(1/2)") {
		t.Fatalf("missing head count in question bar:\n%s", view)
	}
}
