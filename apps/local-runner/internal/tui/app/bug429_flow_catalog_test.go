package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-429 (live BUG-LIVE-UI-6): /flow built its catalog from
// GET /client/chat/builtin-orchestration-options?subMode=bug — the chat-mode
// orchestration picker, which returns [] — instead of the flow-picker endpoint
// the arming resolver uses (workingmode.FlowPickerOptions). Dev harnesses were
// unarmable and the list rendered "(none)" while /flow vibe-ingest still armed.
// cmdFetchFlows must populate m.flowBuiltins from /client/flow-picker-options
// with the session's working mode so list and resolver share one catalog.
func TestBug429_FetchFlowsUsesFlowPickerEndpoint(t *testing.T) {
	var sawPicker, sawOrchOptions bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/client/flow-picker-options"):
			sawPicker = true
			_ = json.NewEncoder(w).Encode([]client.BuiltinFlowOption{
				{FlowRef: "task-harness", Label: "task-harness"},
				{FlowRef: "bug-harness", Label: "bug-harness"},
			})
		case strings.HasPrefix(r.URL.Path, "/client/chat/builtin-orchestration-options"):
			sawOrchOptions = true
			_ = json.NewEncoder(w).Encode([]client.BuiltinFlowOption{})
		case strings.HasPrefix(r.URL.Path, "/client/workflows"):
			_ = json.NewEncoder(w).Encode([]client.Workflow{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	raw := m.cmdFetchFlows(false)()
	msg, ok := raw.(FlowListMsg)
	if !ok {
		t.Fatalf("cmdFetchFlows returned %T, want FlowListMsg", raw)
	}
	if !sawPicker {
		t.Fatal("cmdFetchFlows must query /client/flow-picker-options")
	}
	if sawOrchOptions {
		t.Fatal("cmdFetchFlows must not populate the flow catalog from the chat-orchestration endpoint")
	}
	found := false
	for _, b := range msg.Builtins {
		if b.FlowRef == "task-harness" {
			found = true
		}
	}
	if !found {
		t.Fatalf("picker flows missing from FlowListMsg.Builtins: %+v", msg.Builtins)
	}
}

// BUG-429 resolver parity: once flowBuiltins carries the picker catalog,
// flowCatalogForWorkingMode (dev) surfaces task-harness so /flow task-harness
// arms instead of "not found".
func TestBug429_DevHarnessArmableViaResolver(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.workingMode = "dev"
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "task-harness", Label: "task-harness"},
		{FlowRef: "bug-harness", Label: "bug-harness"},
	}
	builtins, _ := m.flowCatalogForWorkingMode()
	found := false
	for _, b := range builtins {
		if b.FlowRef == "task-harness" {
			found = true
		}
	}
	if !found {
		t.Fatal("dev-mode catalog must surface task-harness from the picker list")
	}
}
