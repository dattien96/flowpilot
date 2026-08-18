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

// G2 (BUG-246): the TUI printed only "[APPROVAL] {id}" and always submitted the
// default approve/deny with remember=false. The runner already ships typed
// details (command/cwd/reason/kind/decisions) and a replay Decision; these tests
// lock in the desktop-parity surface: kind + command on the line, decision
// buttons from the runner, and the "don't ask again" path for exec commands.

func TestApprovalDetails_PopulatedFromEvent(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-1", WorkflowRunID: "run-1",
		Details: &client.ApprovalDetails{
			Command: "git commit -am demo",
			Cwd:     "/work",
			Reason:  "commit the generated change",
			Kind:    "exec",
			Decisions: []client.ApprovalDecisionOption{
				{Value: "approve", Label: "Approve"},
				{Value: "approve_for_session", Label: "Approve for session"},
				{Value: "deny", Label: "Deny"},
			},
		},
	}})
	am := m2.(*AppModel)

	if am.approval == nil || am.approval.ID != "ap-1" {
		t.Fatalf("head=%+v", am.approval)
	}
	a := am.approval
	if a.Kind != "exec" || a.Command != "git commit -am demo" || a.Cwd != "/work" || a.Reason != "commit the generated change" {
		t.Fatalf("typed details not populated: %+v", a)
	}
	if len(a.Decisions) != 3 {
		t.Fatalf("decisions=%+v", a.Decisions)
	}
	view := am.View()
	if !strings.Contains(view, "git commit -am demo") {
		t.Fatalf("transcript must surface the command:\n%s", view)
	}
	if !strings.Contains(view, "Approve for session") {
		t.Fatalf("bar must offer the runner decision:\n%s", view)
	}
	if !strings.Contains(view, "Approve forever") {
		t.Fatalf("exec non-compound approval must offer don't-ask-again:\n%s", view)
	}
}

func TestApprovalReplayDecision_ReadOnlyNoCard(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "permission_required", ApprovalID: "ap-replay", WorkflowRunID: "run-1",
		Decision: "approve",
	}})
	am := m2.(*AppModel)
	if am.approval != nil || len(am.approvals) != 0 {
		t.Fatalf("replayed resolved approval must not mount an interactive card: %+v", am.approval)
	}
	if am.connStatus == ConnWaiting {
		t.Fatal("replayed resolved approval must not park the composer waiting")
	}
	if !strings.Contains(am.View(), "already resolved: approve") {
		t.Fatalf("missing resolved copy:\n%s", am.View())
	}
}

func TestApprovalRememberable_OnlyExecNonCompound(t *testing.T) {
	base := ApprovalState{Kind: "exec", Command: "git status", Decisions: []client.ApprovalDecisionOption{{Value: "approve", Label: "Approve"}, {Value: "deny", Label: "Deny"}}}

	if !approvalRememberable(&base) {
		t.Fatal("simple exec command must be rememberable")
	}
	compound := base
	compound.Command = "git commit && git push"
	if approvalRememberable(&compound) {
		t.Fatal("compound command must never be rememberable (BUG-246)")
	}
	fileKind := base
	fileKind.Kind = "file"
	if approvalRememberable(&fileKind) {
		t.Fatal("non-exec kind must not be rememberable")
	}
	noApprove := base
	noApprove.Decisions = []client.ApprovalDecisionOption{{Value: "deny", Label: "Deny"}}
	if approvalRememberable(&noApprove) {
		t.Fatal("card without an approve decision must not offer don't-ask-again")
	}
	if approvalRememberable(nil) {
		t.Fatal("nil card must not be rememberable")
	}
}

func TestApproveForever_SendsRememberTrue(t *testing.T) {
	var mu sync.Mutex
	var sent map[string]bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/client/approvals/") && strings.HasSuffix(r.URL.Path, "/decision") {
			var body struct {
				Decision string `json:"decision"`
				Remember bool   `json:"remember"`
				Forever  bool   `json:"forever"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			sent = map[string]bool{"remember": body.Remember, "forever": body.Forever}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.pushApproval(ApprovalState{ID: "ap-1", RunID: "run-1", Kind: "exec", Command: "npm test"})
	_, cmd := m.submitPendingApprovalRemember("approve", true)
	if cmd == nil {
		t.Fatal("expected remember cmd")
	}
	_ = cmd()
	mu.Lock()
	defer mu.Unlock()
	if sent == nil || !sent["remember"] || !sent["forever"] {
		t.Fatalf("remember/forever not sent: %+v", sent)
	}
}

func TestDenyForever_NeverPersists(t *testing.T) {
	var mu sync.Mutex
	var sent map[string]bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/client/approvals/") && strings.HasSuffix(r.URL.Path, "/decision") {
			var body struct {
				Decision string `json:"decision"`
				Remember bool   `json:"remember"`
				Forever  bool   `json:"forever"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			sent = map[string]bool{"remember": body.Remember, "forever": body.Forever}
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.pushApproval(ApprovalState{ID: "ap-1", RunID: "run-1", Kind: "exec", Command: "npm test"})
	_, cmd := m.handleSlashCommand("/deny forever")
	if cmd == nil {
		t.Fatal("expected deny cmd")
	}
	_ = cmd()
	mu.Lock()
	defer mu.Unlock()
	if sent == nil || sent["remember"] || sent["forever"] {
		t.Fatalf("deny must never carry remember/forever: %+v", sent)
	}
}

func TestDecisionButton_SubmitsRunnerValue(t *testing.T) {
	var mu sync.Mutex
	var gotDecision string
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		mu.Unlock()
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/client/approvals/") && strings.HasSuffix(r.URL.Path, "/decision") {
			var body struct {
				Decision string `json:"decision"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			gotDecision = body.Decision
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.width, m.height = 140, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.pushApproval(ApprovalState{
		ID: "ap-1", RunID: "run-1", Kind: "exec", Command: "npm test",
		Decisions: []client.ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "approve_for_session", Label: "Approve for session"},
			{Value: "deny", Label: "Deny"},
		},
	})
	x, y, ok := findClickTarget(m, "adec:approve_for_session")
	if !ok {
		t.Fatalf("approve-for-session chip not clickable:\n%s", m.View())
	}
	m2, cmd := m.Update(clickLeft(x, y))
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected decision submit cmd")
	}
	if am.approval == nil {
		t.Fatal("must not clear before POST succeeds")
	}
	// Mouse clicks return tea.Batch(pulse, cmd); run the sub-cmds like the runtime.
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("click cmd returned %T, want tea.BatchMsg", msg)
	}
	for _, c := range batch {
		_ = c()
	}
	mu.Lock()
	defer mu.Unlock()
	if lastPath == "" {
		t.Fatalf("no POST reached server (click at %d,%d):\n%s", x, y, m.View())
	}
	if gotDecision != "approve_for_session" {
		t.Fatalf("submitted decision=%q (path=%s), want approve_for_session", gotDecision, lastPath)
	}
}

func TestApprovalDetailChips_DefaultWhenNoDecisions(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.pushApproval(ApprovalState{ID: "ap-1", RunID: "run-1"})
	view := m.View()
	if !strings.Contains(view, "Approve") || !strings.Contains(view, "Deny") {
		t.Fatalf("default chips missing:\n%s", view)
	}
	if strings.Contains(view, "Approve forever") {
		t.Fatalf("no-details card must not offer don't-ask-again:\n%s", view)
	}
	if _, _, ok := findClickTarget(m, "approve"); !ok {
		t.Fatal("default approve chip must stay clickable")
	}
}
