package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// CA-688 follow-up (operator report: "mở lại chỉ có prompt không có answer"):
// opencode has no provider-owned transcript file, so the durable turn log must
// carry each turn's prompt/assistant pair. The transcript_turn gate used to be
// Gemini/Grok-only — opencode turns logged prompt lines and an opencode_session
// id but never the assistant text, so /open replayed prompts without answers.
// With the gate extended, resumeRun replays both sides.

func TestOpencodeOpenReplaysPromptAndAnswer(t *testing.T) {
	// Real local-file store so the turn log persists (fakeWorkflowStore lacks
	// the TurnLogStore capability).
	chatsDir := filepath.Join(t.TempDir(), ".flowpilot", "chats")
	store, err := NewLocalFileSessionStore(chatsDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyOpencode,
		DisplayName:  "Opencode",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &fileChangeOpencodeAdapter{finalMessage: "Tôi là hy3 model"}
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), store)

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj-oc-replay",
		ProviderKey: ProviderKeyOpencode,
		Model:       "opencode-go/hy3",
		ChatMode:    "normal_chat",
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
		"prompt": "chao hy3",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send turn status=%d body=%s", status, body)
	}
	for i := 0; i < 100; i++ {
		svc.mu.Lock()
		rs := svc.runs[handle.RunID]
		settled := rs != nil && !rs.turnInFlight
		svc.mu.Unlock()
		if settled {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The durable turn log must carry the prompt/assistant pair.
	entries, err := store.ReadTurnLog(context.Background(), handle.RunID)
	if err != nil {
		t.Fatalf("read turn log: %v", err)
	}
	var sawAssistant bool
	var assistantText string
	for _, e := range entries {
		if e.Kind == turnLogKindTranscriptTurn && strings.Contains(e.Prompt, "chao hy3") {
			assistantText = e.Assistant
			if strings.TrimSpace(e.Assistant) != "" {
				sawAssistant = true
			}
		}
	}
	if !sawAssistant {
		t.Fatalf("turn log must record the opencode assistant answer, got entries=%+v (assistant=%q)", entries, assistantText)
	}
	if !strings.Contains(assistantText, "hy3") {
		t.Fatalf("assistant text = %q", assistantText)
	}

	// Reopen via the real resume path: the replay must contain BOTH sides.
	if _, apiErr := svc.resumeRun(handle.RunID); apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	svc.mu.Lock()
	replayed := svc.runs[handle.RunID]
	var sawPromptReplay, sawAnswerReplay bool
	if replayed != nil {
		for _, ev := range replayed.events {
			if ev.Type == EventTurnStarted && strings.Contains(ev.Prompt, "chao hy3") {
				sawPromptReplay = true
			}
			if ev.Type == EventTurnCompleted && strings.Contains(ev.FinalMessage, "hy3") {
				sawAnswerReplay = true
			}
			if ev.Type == EventMessageDelta && strings.Contains(ev.Text, "hy3") {
				sawAnswerReplay = true
			}
		}
	}
	svc.mu.Unlock()
	if !sawPromptReplay || !sawAnswerReplay {
		t.Fatalf("replay after open: prompt=%v answer=%v", sawPromptReplay, sawAnswerReplay)
	}
}
