package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestTurnStreamClosed_PlainChatStartsOrchStream(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-97624", LastEventSeq: 14}
	m.mode = ModeChat
	m.connStatus = ConnRunning
	m2, cmd := m.Update(turnStreamClosedMsg{})
	am := m2.(*AppModel)
	if am.shouldPollStepsRuntime() {
		t.Fatal("plain chat must not flip shouldPollStepsRuntime")
	}
	if cmd == nil {
		t.Fatal("plain chat turn close must start orch listen + snapshot hydrate")
	}
}

func TestTurnStreamClosed_LatePermissionRequiredMountsApproval(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.yolo = false
	m.mode = ModeChat
	m.runHandle = &client.RunHandle{RunID: "run-97624"}
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, cmd := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type:          "permission_required",
		ApprovalID:    "appr-97698",
		WorkflowRunID: "run-97624",
	}})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("orch poll must continue")
	}
	if am.approval == nil || am.approval.ID != "appr-97698" {
		t.Fatalf("approval=%+v", am.approval)
	}
	if !am.sendBlocked() {
		t.Fatal("pending late approval must block a new chat turn")
	}
	if strings.Contains(am.View(), "click Approve or Deny") {
		t.Fatal("must keep CA-477 waiting copy")
	}
}

func TestTurnStreamClosed_YoloOnLateCardAutoApproves(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", Yolo: true}, "http://127.0.0.1:4317")
	m.yolo = true
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, cmd := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type:          "permission_required",
		ApprovalID:    "appr-late",
		WorkflowRunID: "run-1",
	}})
	am := m2.(*AppModel)
	if am.approval != nil {
		t.Fatal("YOLO=on must not mount late approval (CA-476)")
	}
	if cmd == nil {
		t.Fatal("expected auto-approve + orch poll")
	}
}

func TestApplyPendingFromSnapshot_WaitingApproval(t *testing.T) {
	for _, provider := range []string{"grok", "claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: provider, Yolo: false}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.yolo = false
			m.runHandle = &client.RunHandle{RunID: "run-97624", ProviderKey: provider}
			cmd := m.applyPendingFromSnapshot(client.RunSnapshot{
				RunID:  "run-97624",
				Status: "waiting_approval",
				PendingApproval: &client.ApprovalInfo{ID: "appr-97698"},
			})
			if cmd != nil {
				t.Fatal("YOLO=off must wait")
			}
			if m.approval == nil || m.approval.ID != "appr-97698" {
				t.Fatalf("approval=%+v", m.approval)
			}
			if !m.sendBlocked() {
				t.Fatal("hydrated approval must block hello")
			}
			if _, cmd := m.processInput("hello"); cmd != nil {
				t.Fatal("hello must not POST a turn while approval is pending")
			}
		})
	}
}

func TestApplyPendingFromSnapshot_CompletedDoesNotMountReplayCard(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.yolo = false
	m.applyPendingFromSnapshot(client.RunSnapshot{
		RunID:  "run-done",
		Status: "completed",
		PendingApproval: &client.ApprovalInfo{ID: "appr-stale"},
	})
	if m.approval != nil {
		t.Fatal("CA-089: completed snapshot must not remount Approve")
	}
}

func TestApplyPendingFromSnapshot_WaitingQuestion(t *testing.T) {
	m := New(config.ChatConfig{Provider: "claude"}, "http://127.0.0.1:4317")
	m.yolo = false
	m.applyPendingFromSnapshot(client.RunSnapshot{
		RunID:  "run-q",
		Status: "waiting_question",
		PendingQuestion: &client.QuestionInfo{ID: "q-1", Prompt: "Pick one"},
	})
	if m.approval != nil {
		t.Fatal("must not treat question as approval")
	}
	if m.question == nil || m.question.ID != "q-1" {
		t.Fatalf("question=%+v", m.question)
	}
}

func TestChatOpenedMsg_HydratesWaitingApproval(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m.yolo = false
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{RunID: "run-97624", ProviderKey: "grok", Status: "waiting_approval"},
		Messages: []ChatMessage{{Role: "user", Content: "ghi file"}},
		Snapshot: client.RunSnapshot{
			RunID:  "run-97624",
			Status: "waiting_approval",
			PendingApproval: &client.ApprovalInfo{ID: "appr-97698"},
		},
	})
	am := m2.(*AppModel)
	if am.approval == nil || am.approval.ID != "appr-97698" {
		t.Fatalf("open hydrate approval=%+v", am.approval)
	}
	if !strings.Contains(am.View(), "appr-97698") {
		t.Fatalf("open view missing waiting approval:\n%s", am.View())
	}
}

func TestChatOpenedMsg_CompletedIgnoresStalePending(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.yolo = false
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{RunID: "run-old", Status: "completed"},
		Snapshot: client.RunSnapshot{
			RunID:  "run-old",
			Status: "completed",
			PendingApproval: &client.ApprovalInfo{ID: "appr-old"},
		},
	})
	am := m2.(*AppModel)
	if am.approval != nil {
		t.Fatal("open of completed run must not remount stale pending")
	}
}

func TestCmdOpenChat_GetRunFailureStillOpens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/resume") {
			_ = json.NewEncoder(w).Encode(client.RunHandle{RunID: "run-open", LastEventSeq: 0})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	m := New(config.ChatConfig{}, srv.URL)
	msg := m.cmdOpenChat("run-open")()
	opened, ok := msg.(ChatOpenedMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if opened.Err != "" {
		t.Fatalf("open err=%s", opened.Err)
	}
	if opened.Handle.RunID != "run-open" {
		t.Fatalf("handle=%+v", opened.Handle)
	}
}

func TestTurnIsActive_PlainChatOrchListenerDoesNotArmStop(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-97624"}
	m.mode = ModeChat
	m.connStatus = ConnIdle
	m.orchStream = &orchStreamState{}
	if m.turnIsActive() {
		t.Fatal("plain-chat late-gate listener must not keep [stop] armed")
	}
}

func TestTurnIsActive_FlowOrchStillActive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-flow"}
	m.mode = ModeFlow
	m.connStatus = ConnIdle
	m.orchStream = &orchStreamState{}
	if !m.turnIsActive() {
		t.Fatal("flow orch stream must still count as active")
	}
}

func TestRunSnapshotMsg_EmptySnapNoPanic(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, cmd := m.Update(runSnapshotMsg{})
	if m2 == nil {
		t.Fatal("nil model")
	}
	if cmd != nil && cmd() != nil {
		// applyPending on empty snap returns nil cmd; Batch may still be nil
	}
	_ = tea.Batch()
}
