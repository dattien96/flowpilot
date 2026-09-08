package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/workingmode"
)

const ca781CPPath = "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md"

func vibeCpFlowModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.workingMode = workingmode.Vibe
	m.flowBuiltins = mixedFlowBuiltins()
	m.inputValue = "/flow "
	return m
}

func indexFlowSuggestion(items []suggestItem, id string) int {
	for i, it := range items {
		if workingmode.BareFlowID(it.value) == id {
			return i
		}
	}
	return -1
}

func TestCA781_VibeFlowListsCpIngest(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			ids := suggestionIDs(collectFlowLine(vibeCpFlowModel(pk)))
			if _, ok := ids["vibe-ingest"]; !ok {
				t.Fatalf("missing ingest: %v", ids)
			}
			if _, ok := ids["vibe-cp-ingest"]; !ok {
				t.Fatalf("missing cp-ingest: %v", ids)
			}
		})
	}
}

func TestCA781_TabCpIngestOpensFilePicker(t *testing.T) {
	m := vibeCpFlowModel("codex")
	items := m.collectSuggestions()
	idx := indexFlowSuggestion(items, "vibe-cp-ingest")
	if idx < 0 {
		t.Fatalf("no vibe-cp-ingest in %+v", items)
	}
	m.suggIdx = idx
	m.applySuggestion(items)
	if !strings.Contains(m.inputValue, "/flow vibe-cp-ingest") || !strings.Contains(m.inputValue, "@") {
		t.Fatalf("input=%q want /flow vibe-cp-ingest @", m.inputValue)
	}
	files := m.collectSuggestions()
	if len(files) == 0 || files[0].kind != "file" {
		t.Fatalf("want file picker, got %+v", files)
	}
}

func TestCA781_EnterCpIngestOpensFilePicker(t *testing.T) {
	m := vibeCpFlowModel("codex")
	items := m.collectSuggestions()
	idx := indexFlowSuggestion(items, "vibe-cp-ingest")
	if idx < 0 {
		t.Fatal("no vibe-cp-ingest")
	}
	m.suggIdx = idx
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.launch.IsArmed() {
		t.Fatal("Enter must not arm without a CP path")
	}
	if !strings.Contains(am.inputValue, "@") {
		t.Fatalf("input=%q", am.inputValue)
	}
	files := am.collectSuggestions()
	if len(files) == 0 || files[0].kind != "file" {
		t.Fatalf("want file picker, got %+v", files)
	}
}

func TestCA781_BareFlowCpIngestOpensPicker(t *testing.T) {
	m := vibeCpFlowModel("codex")
	m2, _ := m.handleSlashCommand("/flow vibe-cp-ingest")
	am := m2.(*AppModel)
	if am.launch.IsArmed() {
		t.Fatal("bare /flow vibe-cp-ingest must not arm")
	}
	if !strings.Contains(am.inputValue, "@") {
		t.Fatalf("input=%q", am.inputValue)
	}
}

func TestCA781_FlowCpPathArms(t *testing.T) {
	m := vibeCpFlowModel("codex")
	m2, _ := m.handleSlashCommand("/flow vibe-cp-ingest " + ca781CPPath)
	am := m2.(*AppModel)
	if am.launch.FlowRef != "vibe-cp-ingest" {
		t.Fatalf("flowRef=%q", am.launch.FlowRef)
	}
	if am.launch.SourceDocID != ca781CPPath {
		t.Fatalf("source=%q", am.launch.SourceDocID)
	}
}

func TestCA781_FlowReadmeRejected(t *testing.T) {
	m := vibeCpFlowModel("codex")
	m2, _ := m.handleSlashCommand("/flow vibe-cp-ingest README.md")
	am := m2.(*AppModel)
	if am.launch.IsArmed() {
		t.Fatal("README must not arm")
	}
	if !strings.Contains(am.View(), "vibe-cp requires") {
		t.Fatalf("view:\n%s", am.View())
	}
}

func TestCA781_DevOmitsCpIngest(t *testing.T) {
	m := vibeCpFlowModel("codex")
	m.workingMode = workingmode.Dev
	ids := suggestionIDs(collectFlowLine(m))
	if _, ok := ids["vibe-cp-ingest"]; ok {
		t.Fatalf("dev leaked cp-ingest: %v", ids)
	}
}

func TestCA781_AcceptValueCpIngestIsAtPicker(t *testing.T) {
	got := suggestionAcceptValue(suggestItem{kind: "flow", value: "vibe-cp-ingest"})
	if got != "/flow vibe-cp-ingest @" {
		t.Fatalf("got %q", got)
	}
}
