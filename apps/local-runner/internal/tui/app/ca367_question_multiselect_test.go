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

// G3: the TUI ignored MultiSelect/description/Answer on question cards — it
// always submitted a single string and re-showed interactive forms for
// questions the runner had already answered. These tests lock in the
// desktop-parity surface: multi-select toggling, description text, an explicit
// submit that posts a string array, and read-only replay of answered questions.

func TestQuestionReplayAnswered_ReadOnlyNoCard(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "user_question_required", QuestionID: "q-rep", WorkflowRunID: "run-1",
		Prompt: "Pick", Answer: []string{"alpha", "beta"},
	}})
	am := m2.(*AppModel)
	if am.question != nil || len(am.questions) != 0 {
		t.Fatalf("replayed answered question must not mount a card: %+v", am.question)
	}
	if am.connStatus != ConnRunning {
		t.Fatalf("status=%v, want ConnRunning", am.connStatus)
	}
	if !strings.Contains(am.View(), "already answered: alpha, beta") {
		t.Fatalf("transcript must surface the recorded answer:\n%s", am.View())
	}
}

func TestQuestionMultiSelect_PushCarriesFlagAndHint(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m2, _ := m.Update(orchStreamEventMsg{Ev: client.ProviderEvent{
		Type: "user_question_required", QuestionID: "q-ms", WorkflowRunID: "run-1",
		Prompt: "Pick files", MultiSelect: true,
		Options: []map[string]string{{"value": "a", "label": "Alpha", "description": "first file"}},
	}})
	am := m2.(*AppModel)
	if am.question == nil || !am.question.MultiSelect {
		t.Fatalf("multi flag not carried: %+v", am.question)
	}
	view := am.View()
	if !strings.Contains(view, "first file") {
		t.Fatalf("option description missing from transcript:\n%s", view)
	}
	if !strings.Contains(view, "[multi-select]") || !strings.Contains(view, "[Submit]") {
		t.Fatalf("bar must show multi hint + Submit chip:\n%s", view)
	}
}

func TestQuestionMultiSelect_TypingTogglesNotSubmits(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.pushQuestion(QuestionState{
		ID: "q-t", Prompt: "Pick", MultiSelect: true,
		Options: []map[string]string{
			{"value": "alpha", "label": "Alpha"},
			{"value": "beta", "label": "Beta"},
		},
	})
	m2, cmd := m.submitQuestionAnswer("1")
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("toggle must not submit")
	}
	if len(am.question.Selected) != 1 || am.question.Selected[0] != "alpha" {
		t.Fatalf("selected=%v", am.question.Selected)
	}
	if !strings.Contains(am.View(), "[x] Alpha") {
		t.Fatalf("bar must show checked marker:\n%s", am.View())
	}
	m2, cmd = am.submitQuestionAnswer("beta")
	am = m2.(*AppModel)
	if cmd != nil || len(am.question.Selected) != 2 {
		t.Fatalf("second toggle selected=%v", am.question.Selected)
	}
	m2, cmd = am.submitQuestionAnswer("1")
	am = m2.(*AppModel)
	if cmd != nil || len(am.question.Selected) != 1 || am.question.Selected[0] != "beta" {
		t.Fatalf("re-toggle must deselect: %v", am.question.Selected)
	}
}

func TestQuestionMultiSelect_SubmitSendsArray(t *testing.T) {
	var mu sync.Mutex
	var lastPath string
	var lastBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&lastBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.yolo = false
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.pushQuestion(QuestionState{
		ID: "q-ms", Prompt: "Pick files", MultiSelect: true,
		Options: []map[string]string{
			{"value": "alpha", "label": "Alpha", "description": "first"},
			{"value": "beta", "label": "Beta", "description": "second"},
		},
	})
	m2, cmd := m.submitQuestionAnswer("1")
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("toggle must not submit")
	}
	m2, cmd = am.submitQuestionAnswer("beta")
	am = m2.(*AppModel)
	if cmd != nil {
		t.Fatal("toggle must not submit")
	}
	m2, cmd = am.handleSlashCommand("/submit")
	am = m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected submit cmd")
	}
	if am.question == nil {
		t.Fatal("must not clear question until POST succeeds")
	}
	_ = cmd()
	mu.Lock()
	defer mu.Unlock()
	if lastPath != "/client/questions/q-ms/answer" {
		t.Fatalf("path=%s", lastPath)
	}
	var choice []string
	if err := json.Unmarshal(lastBody["choice"], &choice); err != nil {
		t.Fatalf("choice=%s err=%v", lastBody["choice"], err)
	}
	if len(choice) != 2 || choice[0] != "alpha" || choice[1] != "beta" {
		t.Fatalf("choice=%v", choice)
	}
}

func TestQuestionMultiSelect_SubmitEmptyRejected(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, "http://127.0.0.1:4317")
	m.yolo = false
	m.pushQuestion(QuestionState{
		ID: "q-e", Prompt: "Pick", MultiSelect: true,
		Options: []map[string]string{{"value": "a", "label": "Alpha"}},
	})
	m2, cmd := m.handleSlashCommand("/submit")
	if cmd != nil {
		t.Fatal("must not submit an empty selection")
	}
	if !strings.Contains(m2.(*AppModel).View(), "Select at least one option") {
		t.Fatalf("missing empty-selection hint:\n%s", m2.(*AppModel).View())
	}
}

func TestQuestionMultiSelect_ClickTargets(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.question = &QuestionState{
		ID: "q-c", MultiSelect: true,
		Options: []map[string]string{
			{"value": "a", "label": "Alpha"},
			{"value": "b", "label": "Beta"},
		},
	}
	if _, _, ok := findClickTarget(m, "qtoggle:1"); !ok {
		t.Fatal("expected clickable multi option")
	}
	if _, _, ok := findClickTarget(m, "qsubmit"); !ok {
		t.Fatal("expected clickable Submit chip")
	}
}

func TestQuestionMultiSelect_ClickTogglesThenSubmit(t *testing.T) {
	var mu sync.Mutex
	var lastPath string
	var lastBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&lastBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.yolo = false
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionLoading = false
	m.question = &QuestionState{
		ID: "q-x", MultiSelect: true,
		Options: []map[string]string{
			{"value": "a", "label": "Alpha"},
			{"value": "b", "label": "Beta"},
		},
	}
	x, y, ok := findClickTarget(m, "qtoggle:1")
	if !ok {
		t.Fatal("no qtoggle target")
	}
	m2, _ := m.Update(clickLeft(x, y))
	am := m2.(*AppModel)
	if len(am.question.Selected) != 1 || am.question.Selected[0] != "b" {
		t.Fatalf("click did not toggle: %v", am.question.Selected)
	}
	xs, ys, ok := findClickTarget(am, "qsubmit")
	if !ok {
		t.Fatal("no qsubmit target")
	}
	m3, cmd := am.Update(clickLeft(xs, ys))
	if cmd == nil {
		t.Fatal("expected submit cmd from click")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("click cmd returned %T, want tea.BatchMsg", msg)
	}
	for _, c := range batch {
		_ = c()
	}
	_ = m3
	mu.Lock()
	defer mu.Unlock()
	if lastPath != "/client/questions/q-x/answer" {
		t.Fatalf("path=%s", lastPath)
	}
	var choice []string
	if err := json.Unmarshal(lastBody["choice"], &choice); err != nil {
		t.Fatalf("choice=%s err=%v", lastBody["choice"], err)
	}
	if len(choice) != 1 || choice[0] != "b" {
		t.Fatalf("choice=%v", choice)
	}
}

func TestQuestionSingleSelect_StillImmediateSubmit(t *testing.T) {
	var mu sync.Mutex
	var lastPath string
	var lastBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&lastBody)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer srv.Close()

	m := New(config.ChatConfig{Provider: "grok", Yolo: false}, srv.URL)
	m.yolo = false
	m.width, m.height = 100, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.question = &QuestionState{
		ID: "q-s",
		Options: []map[string]string{
			{"value": "alpha", "label": "Alpha"},
		},
	}
	x, y, ok := findClickTarget(m, "qopt:0")
	if !ok {
		t.Fatal("no qopt target")
	}
	m2, cmd := m.Update(clickLeft(x, y))
	if cmd == nil {
		t.Fatal("expected immediate submit cmd")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("click cmd returned %T, want tea.BatchMsg", msg)
	}
	for _, c := range batch {
		_ = c()
	}
	_ = m2
	mu.Lock()
	defer mu.Unlock()
	if lastPath != "/client/questions/q-s/answer" {
		t.Fatalf("path=%s", lastPath)
	}
	var choice string
	if err := json.Unmarshal(lastBody["choice"], &choice); err != nil {
		t.Fatalf("choice=%s err=%v", lastBody["choice"], err)
	}
	if choice != "alpha" {
		t.Fatalf("choice=%q", choice)
	}
}
