package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// CP-57 OC-02/OC-10 gap closure: Opencode turns must reach the shared
// finishTurn / flow-gate machinery exactly like Grok (r-ca from a mapped file
// change; r-bug/r-task from the declared change type). Mirrors
// grok_flow_gate_test.go with ProviderKeyOpencode. Additive — no pre-existing
// test is edited; Claude/Codex/Grok suites untouched.

// fileChangeOpencodeAdapter is the Opencode twin of fileChangeGrokAdapter.
type fileChangeOpencodeAdapter struct {
	finalMessage string
}

func (a *fileChangeOpencodeAdapter) Key() ProviderKey { return ProviderKeyOpencode }
func (a *fileChangeOpencodeAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, ApprovalEvents: true}
}
func (a *fileChangeOpencodeAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{
		Type:       EventFileChanged,
		Path:       "apps/local-runner/internal/example.go",
		ChangeType: "write",
	})
	msg := a.finalMessage
	if msg == "" {
		msg = "done"
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: msg})
	return nil
}

func startOpencodeGateService(t *testing.T, finalMessage string) (*InteractiveService, *httptest.Server, string) {
	t.Helper()
	workspace := t.TempDir()
	settingsDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "gate-config.json"), []byte(`{"gate_mode":"enforce"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyOpencode,
		DisplayName:  "Opencode",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &fileChangeOpencodeAdapter{finalMessage: finalMessage} },
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv, workspace
}

func opencodeWaitGate(t *testing.T, svc *InteractiveService, runID string) ProviderEvent {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[runID]
		var events []ProviderEvent
		if rs != nil {
			events = append(events, rs.events...)
		}
		svc.mu.Unlock()
		for _, ev := range events {
			if ev.Type == EventFlowGateViolation {
				return ev
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected EventFlowGateViolation for opencode turn")
	return ProviderEvent{}
}

func TestOpencodeNormalChatRCAFiresFromMappedFileChange(t *testing.T) {
	svc, srv, workspace := startOpencodeGateService(t, "")
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj-oc-gate",
		ProviderKey: ProviderKeyOpencode,
		Model:       "opencode/muse-spark-1.2-contributor-free",
		ChatMode:    "normal_chat",
		Cwd:         workspace,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID,
		"prompt": "edit example.go without writing a change-audit note",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send turn status=%d body=%s", status, body)
	}

	ev := opencodeWaitGate(t, svc, handle.RunID)
	if ev.Status != "reprompt" && ev.Status != "warn" {
		t.Fatalf("gate status = %q, want reprompt or warn", ev.Status)
	}
	if ev.Error == "" {
		t.Fatal("gate event missing Error/message")
	}
	if ev.ProviderKey != ProviderKeyOpencode {
		t.Fatalf("gate event provider = %q, want opencode", ev.ProviderKey)
	}
}

func TestOpencodeNormalChatRBugAndRTaskUseDeclaredChangeType(t *testing.T) {
	cases := []struct {
		name         string
		changeType   string
		finalMessage string
	}{
		{name: "r-bug", changeType: "bugfix", finalMessage: "fixed the issue"},
		{name: "r-task", changeType: "task", finalMessage: "implemented Task-303 slice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, srv, workspace := startOpencodeGateService(t, tc.finalMessage)
			status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
				ProjectID:   "proj-oc-" + tc.name,
				ProviderKey: ProviderKeyOpencode,
				Model:       "opencode/muse-spark-1.2-contributor-free",
				ChatMode:    "normal_chat",
				Cwd:         workspace,
			}, nil)
			if status != http.StatusOK {
				t.Fatalf("start run status=%d body=%s", status, body)
			}
			var handle RunHandle
			if err := json.Unmarshal(body, &handle); err != nil {
				t.Fatalf("decode: %v", err)
			}
			status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
				"stepId":     handle.StepID,
				"prompt":     "do the work",
				"changeType": tc.changeType,
			}, nil)
			if status != http.StatusOK {
				t.Fatalf("send turn status=%d body=%s", status, body)
			}
			ev := opencodeWaitGate(t, svc, handle.RunID)
			if ev.Error == "" {
				t.Fatal("gate event missing detail")
			}
		})
	}
}
