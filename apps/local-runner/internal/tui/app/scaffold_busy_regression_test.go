package app

import (
	"net/http"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestScaffoldBusy_ShowsSpinnerAndDisablesComposerUntilTerminal(t *testing.T) {
	srv := newInitTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"done","platform":"react-native","message":"scaffold: done"}`))
	})
	m := newInitTestModel(t, srv.URL, "p1", "react-native")
	m.sessionDefaultsLoaded = true
	m.provider = "devin"
	m.model = "devin/swe-2-max"

	initMsg := m.cmdInitEngine("all")().(EngineInitMsg)
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd == nil {
		t.Fatal("expected scaffold command")
	}
	if !m.workIsLive() {
		t.Fatal("scaffold dispatch must count as live work so the spinner animates")
	}
	if got := m.renderInputLine(); !strings.Contains(got, "chat disabled") {
		t.Fatalf("composer while scaffold runs = %q, want chat disabled banner", got)
	}
	_, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("blocked")})
	if m.inputValue != "" {
		t.Fatalf("keyboard input while scaffold runs = %q, want ignored", m.inputValue)
	}

	before := len(m.messages)
	_, cmd := m.processInput("must not send during scaffold")
	if cmd != nil {
		t.Fatal("chat input during scaffold returned a command; want blocked send")
	}
	if len(m.messages) != before+1 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Scaffold") {
		t.Fatalf("blocked-send message = %+v, want Scaffold busy explanation", m.messages[before:])
	}

	scaffoldMsg := scaffoldCmd().(EngineScaffoldMsg)
	if scaffoldMsg.Err != nil {
		t.Fatalf("scaffold dispatch: %v", scaffoldMsg.Err)
	}
	_, _ = m.handleEngineScaffoldMsg(scaffoldMsg)
	if m.workIsLive() {
		t.Fatal("terminal scaffold result must stop the live-work spinner")
	}
	if got := m.renderInputLine(); strings.Contains(got, "chat disabled") {
		t.Fatalf("composer after scaffold = %q, want chat re-enabled", got)
	}
}

func TestScaffoldBusy_ErrorReEnablesComposer(t *testing.T) {
	srv := newInitTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	m := newInitTestModel(t, srv.URL, "p1", "react-native")
	m.sessionDefaultsLoaded = true

	initMsg := m.cmdInitEngine("all")().(EngineInitMsg)
	_, scaffoldCmd := m.handleEngineInitMsg(initMsg)
	if scaffoldCmd == nil || !m.workIsLive() {
		t.Fatal("expected live scaffold before the HTTP error")
	}
	scaffoldMsg := scaffoldCmd().(EngineScaffoldMsg)
	if scaffoldMsg.Err == nil {
		t.Fatal("expected scaffold HTTP error")
	}
	_, _ = m.handleEngineScaffoldMsg(scaffoldMsg)
	if m.workIsLive() {
		t.Fatal("scaffold error must stop the live-work spinner")
	}
	if got := m.renderInputLine(); strings.Contains(got, "chat disabled") {
		t.Fatalf("composer after scaffold error = %q, want chat re-enabled", got)
	}
}
