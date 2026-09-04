package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestFlowPickerTabRefreshesStaleCache is the BUG-351 regression: the Tab
// picker renders from the cached flow list, and a session-start prefetch can
// race runner mirror-sync (partial list stuck for the whole session because
// the old maybe-prefetch skipped non-empty caches). While the picker is open,
// a keypress must dispatch a silent background refresh; the stale cache keeps
// showing until the fresh FlowListMsg merges.
func TestFlowPickerTabRefreshesStaleCache(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	// Stale session-start snapshot: rag-harness only, no task-harness yet.
	m.flowWorkflows = []client.Workflow{{ID: "wf-old", Name: "Rag Harness"}}
	m.inputValue = "/flow "
	m.inputCursor = -1

	// Picker still renders the stale cache meanwhile (no empty flash).
	subs := m.collectSuggestions()
	if len(subs) == 0 {
		t.Fatal("picker must keep showing the stale cache while refreshing")
	}
	for _, s := range subs {
		if s.kind == "flow" && (s.value == "wf-old" || strings.Contains(strings.ToLower(s.detail), "rag harness")) {
			goto haveStale
		}
	}
	t.Fatalf("stale cache row missing from picker: %+v", subs)
haveStale:

	// A keypress with the picker open dispatches exactly one refresh.
	if cmd := m.cmdMaybePrefetchFlows(); cmd == nil {
		t.Fatal("expected a silent background refresh while the picker is open")
	}
	if !m.flowListInflight {
		t.Fatal("expected flowListInflight while the refresh is running")
	}
	if cmd := m.cmdMaybePrefetchFlows(); cmd != nil {
		t.Fatal("in-flight refresh must not double-fire per keypress")
	}

	// Fresh list arrives (mirror sync finished): cache converges, flag clears.
	fresh := []client.Workflow{
		{ID: "wf-old", Name: "Rag Harness"},
		{ID: "wf-task", Name: "Task Harness"},
	}
	m2, _ := m.Update(FlowListMsg{Workflows: fresh, Silent: true})
	am := m2.(*AppModel)
	if am.flowListInflight {
		t.Fatal("expected flowListInflight cleared after FlowListMsg")
	}
	if len(am.flowWorkflows) != 2 {
		t.Fatalf("workflows=%d, want converged 2", len(am.flowWorkflows))
	}
	am.inputValue = "/flow task"
	am.inputCursor = -1
	found := false
	for _, s := range am.collectSuggestions() {
		if s.kind == "flow" && s.value == "wf-task" {
			found = true
		}
	}
	if !found {
		t.Fatalf("task-harness missing after refresh: %+v", am.collectSuggestions())
	}
}

// TestFlowListMsgErrorKeepsGoodCache pins the BUG-351 companion rule: a failed
// background refresh must not wipe the last good list.
func TestFlowListMsgErrorKeepsGoodCache(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.flowWorkflows = []client.Workflow{{ID: "wf-old", Name: "Rag Harness"}}
	m.flowListInflight = true

	m2, _ := m.Update(FlowListMsg{CatalogErr: "boom", Silent: true})
	am := m2.(*AppModel)
	if am.flowListInflight {
		t.Fatal("expected flowListInflight cleared even on error")
	}
	if len(am.flowWorkflows) != 1 || am.flowWorkflows[0].ID != "wf-old" {
		t.Fatalf("good cache must survive a failed refresh: %+v", am.flowWorkflows)
	}
}

// TestFlowPickerRefreshIntervalBound keeps per-keypress refreshes cheap while
// the picker stays open: a second refresh right after a completed one waits
// out flowPickerRefreshInterval instead of firing per keystroke.
func TestFlowPickerRefreshIntervalBound(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.flowWorkflows = []client.Workflow{{ID: "wf-old", Name: "Rag Harness"}}
	m.inputValue = "/flow "
	m.inputCursor = -1

	m2, _ := m.Update(FlowListMsg{Workflows: m.flowWorkflows, Silent: true})
	am := m2.(*AppModel)
	// Fresh response just landed: no immediate second fetch.
	if cmd := am.cmdMaybePrefetchFlows(); cmd != nil {
		t.Fatal("refresh must wait out the interval instead of firing per keystroke")
	}
}
