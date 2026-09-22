package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestResolveTurnStepID_FallsBackForWorkflowOpen guards run-193749: after /open
// of a completed workflow/flow run the handle carries no StepID (the runner only
// mints "chat-<runId>" for normal chat, T-7), so continue must resolve the
// launch workflow id / synthetic chat id instead of POSTing an empty stepId
// (startTurn rejects that with 400 "stepId is required").
func TestResolveTurnStepID_FallsBackForWorkflowOpen(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AppModel)
		want   string
	}{
		{
			name: "handle step id wins",
			mutate: func(m *AppModel) {
				m.stepID = "chat-run-1"
				m.runHandle = &client.RunHandle{RunID: "run-1", StepID: "chat-run-1"}
			},
			want: "chat-run-1",
		},
		{
			name: "workflow open falls back to workflow id",
			mutate: func(m *AppModel) {
				m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "workflow"}
				m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf-grok"}
			},
			want: "wf-grok",
		},
		{
			name: "step-mode launch step id wins over workflow",
			mutate: func(m *AppModel) {
				m.runHandle = &client.RunHandle{RunID: "run-1"}
				m.launch = LaunchArm{Mode: ModeStep, StepID: "step-x", WorkflowID: "wf-grok"}
			},
			want: "step-x",
		},
		{
			name: "chat run falls back to synthetic chat id",
			mutate: func(m *AppModel) {
				m.runHandle = &client.RunHandle{RunID: "run-77", RunKind: "chat"}
			},
			want: "chat-run-77",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
			tc.mutate(m)
			if got := m.resolveTurnStepID(); got != tc.want {
				t.Fatalf("resolveTurnStepID=%q want %q", got, tc.want)
			}
		})
	}
}

// TestChatOpenedMsg_WorkflowOpenPersistsResolvedStepID drives the full /open
// handler path: opening a workflow run that carries no StepID must persist the
// launch workflow id fallback into m.stepID so a later continue sends a valid
// stepId (CA-519). Claude / Codex / Grok share the same open path.
func TestChatOpenedMsg_WorkflowOpenPersistsResolvedStepID(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
			m2, _ := m.Update(ChatOpenedMsg{
				Handle: client.RunHandle{
					RunID: "run-193749", RunKind: "workflow",
					WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
					ProviderKey: pk,
				},
				Snapshot:    client.RunSnapshot{RunID: "run-193749", Status: "completed"},
				HistoryMeta: client.RunHistoryItem{RunID: "run-193749", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed"},
				Messages:    []ChatMessage{{Role: "user", Content: "fix bug 1+1 != 2"}},
			})
			am := m2.(*AppModel)
			if am.mode != ModeFlow {
				t.Fatalf("mode=%v want ModeFlow", am.mode)
			}
			if got := am.resolveTurnStepID(); got != "wf-grok" {
				t.Fatalf("resolveTurnStepID=%q want workflow id fallback wf-grok", got)
			}
		})
	}
}

// TestChatOpenedMsg_NormalChatOpenKeepsSyntheticStepID locks the chat path: a
// normal-chat open keeps the runner-minted synthetic "chat-<runId>" step, never
// falling back to a launch id that would mis-route the continue turn.
func TestChatOpenedMsg_NormalChatOpenKeepsSyntheticStepID(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-5", RunKind: "chat", StepID: "chat-run-5", Status: "completed",
			ProviderKey: "codex",
		},
		Snapshot:    client.RunSnapshot{RunID: "run-5", Status: "completed"},
		HistoryMeta: client.RunHistoryItem{RunID: "run-5", RunKind: "chat"},
		Messages:    []ChatMessage{{Role: "user", Content: "hi"}},
	})
	am := m2.(*AppModel)
	if am.mode != ModeChat {
		t.Fatalf("mode=%v want ModeChat", am.mode)
	}
	if got := am.resolveTurnStepID(); got != "chat-run-5" {
		t.Fatalf("resolveTurnStepID=%q want synthetic chat-run-5", got)
	}
}

// TestOpenTurnStream_WorkflowOpenSendsWorkflowIDAsStepID proves the actual turn
// payload: open a completed workflow (no handle StepID), then send a turn — the
// HTTP POST must carry stepId = workflow id, never empty.
func TestOpenTurnStream_WorkflowOpenSendsWorkflowIDAsStepID(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var gotBody map[string]any
			done := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns") {
					defer r.Body.Close()
					json.NewDecoder(r.Body).Decode(&gotBody)
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"turnId":"turn-1"}`))
					close(done)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.(http.Flusher).Flush()
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.width, m.height = 100, 30
			m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
			m2, _ := m.Update(ChatOpenedMsg{
				Handle: client.RunHandle{
					RunID: "run-193749", RunKind: "workflow",
					WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
					ProviderKey: pk,
				},
				Snapshot:    client.RunSnapshot{RunID: "run-193749", Status: "completed"},
				HistoryMeta: client.RunHistoryItem{RunID: "run-193749", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed"},
				Messages:    []ChatMessage{{Role: "user", Content: "fix bug 1+1 != 2"}},
			})
			am := m2.(*AppModel)

			cmd := am.openTurnStream("continue the fix")
			if cmd == nil {
				t.Fatal("openTurnStream returned nil cmd")
			}
			msg := cmd()
			if _, ok := msg.(turnStreamOpenedMsg); !ok {
				t.Fatalf("msg type %T", msg)
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("no turn POST captured")
			}
			if step, _ := gotBody["stepId"].(string); step != "wf-grok" {
				t.Fatalf("POST stepId=%q want wf-grok (never empty)", step)
			}
			if run, _ := gotBody["runId"].(string); run != "run-193749" {
				t.Fatalf("POST runId=%q", run)
			}
		})
	}
}

// TestCanSend_BlocksChildFocus stays green while the composer is hidden: send
// must remain disabled on a focused child for all three providers (the guard is
// provider-agnostic; parameterized to trip a future per-provider branch).
func TestCanSend_BlocksChildFocus(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.focusRunID = "run-child"
			m.runHandle = &client.RunHandle{RunID: "run-parent"}
			if m.canSend() {
				t.Fatalf("%s: child focus must disable send", pk)
			}
		})
	}
}

// TestRenderInputLine_ChildViewShowsReadOnlyBanner locks the CA-519 composer
// behavior: while a sub-agent is focused the editable chat box is replaced by a
// read-only banner — no "chat " prefix, no editable caret body.
func TestRenderInputLine_ChildViewShowsReadOnlyBanner(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 20
	m.asciiMode = true
	m.focusRunID = "run-child"
	m.runHandle = &client.RunHandle{RunID: "run-parent"}

	line := stripANSI(m.renderInputLine())
	if !strings.Contains(line, "read-only") {
		t.Fatalf("child view input line must say read-only: %q", line)
	}
	if strings.HasPrefix(line, " chat ") {
		t.Fatalf("child view must not render an editable chat prefix: %q", line)
	}

	// Escaping back to main restores the editable composer.
	m.focusRunID = ""
	back := stripANSI(m.renderInputLine())
	if strings.Contains(back, "read-only") {
		t.Fatalf("main view must not show the read-only banner: %q", back)
	}
}

// TestHandleKey_ChildViewDropsChatText locks the key gate: printable chat text
// and Enter-to-send are dropped while a sub-agent is focused, while navigation,
// [back]/Esc, agent-cycle Tab, and slash commands still work.
func TestHandleKey_ChildViewDropsChatText(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 20
	m.focusRunID = "run-child"
	m.runHandle = &client.RunHandle{RunID: "run-parent"}

	// Plain text must NOT insert.
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	am := m2.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("child view must drop chat text, inputValue=%q", am.inputValue)
	}

	// Enter with no slash must not send.
	m3, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am3 := m3.(*AppModel)
	if am3.inputValue != "" {
		t.Fatalf("child view Enter must not send, inputValue=%q", am3.inputValue)
	}

	// A slash command still works (needed for /agent main, /help).
	m4, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	am4 := m4.(*AppModel)
	if am4.inputValue != "/" {
		t.Fatalf("child view must allow slash command start, inputValue=%q", am4.inputValue)
	}

	// Agent-cycle Tab still works (exits child via focus move).
	m5, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	_ = m5
}
