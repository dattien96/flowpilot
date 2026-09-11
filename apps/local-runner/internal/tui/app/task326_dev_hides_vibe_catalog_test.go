package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/workingmode"
)

func TestTUIFlowSuggest_DevOmitsVibeCatalogClones(t *testing.T) {
	m := tuiModelWithFlows(t)
	m.flowWorkflows = []client.Workflow{
		{ID: "2588bdc5-00a8-43bf-9823-97491e95361c", Name: "Vibe Cp Ingest"},
		{ID: "22bc8919-42f3-4fe7-93e6-28e7bf89373a", Name: "Vibe Owner Debate"},
		{ID: "67793e12-ce53-456d-89db-4d24c0d205b9", Name: "Vibe Sprint"},
		{ID: "e1balcda-99ea-4b7-83eb-catalog-task", Name: "Task Harness"},
	}
	m2, _ := m.handleSlashCommand("/vibe off")
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Dev {
		t.Fatalf("mode=%q", am.workingMode)
	}
	for _, it := range collectFlowLine(am) {
		blob := strings.ToLower(it.value + " " + it.detail)
		if strings.Contains(blob, "vibe") {
			t.Fatalf("dev /flow leaked vibe catalog: %+v", it)
		}
	}
}
