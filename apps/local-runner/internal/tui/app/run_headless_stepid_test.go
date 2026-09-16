package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// headlessFakeRunner serves the minimal runner surface runHeadless needs:
// StartRun/ResumeRun (minting stepId), turns (capturing stepId), and an SSE
// stream ending in turn_completed. The captured stepId is the regression
// lock for BUG-373.
type headlessCapture struct {
	turnStepID string
	turns      int
	startProj  string
}

func headlessFakeRunner(t *testing.T, cap *headlessCapture, resumeStepID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case p == "/client/workflow-runs" && r.Method == http.MethodPost:
			var in struct {
				ProjectID string `json:"projectId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			cap.startProj = in.ProjectID
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runId": "run-h1", "providerSessionId": "s", "providerKey": "grok",
				"status": "idle", "stepId": "chat-run-h1",
			})
		case strings.HasSuffix(p, "/resume") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"runId": "run-h1", "providerSessionId": "s", "providerKey": "grok",
				"status": "idle", "stepId": resumeStepID,
			})
		case strings.HasSuffix(p, "/turns") && r.Method == http.MethodPost:
			var in struct {
				RunID  string `json:"runId"`
				StepID string `json:"stepId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode turn: %v", err)
			}
			cap.turnStepID = in.StepID
			cap.turns++
			_ = json.NewEncoder(w).Encode(map[string]any{"turnId": "turn-1"})
		case strings.Contains(p, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("id: 1\ndata: {\"id\":\"e1\",\"seq\":1,\"type\":\"turn_completed\",\"workflowRunId\":\"run-h1\",\"providerKey\":\"grok\",\"providerTurnId\":\"turn-1\",\"finalMessage\":\"hi\"}\n\n"))
		default:
			t.Errorf("unexpected %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestRunHeadlessSendsRunnerMintedStepID is the BUG-373 regression: headless
// (--print) must send the StartRun-minted stepId on its turn, exactly like
// the interactive path (resolveTurnStepID), or startTurn rejects with 400.
func TestRunHeadlessSendsRunnerMintedStepID(t *testing.T) {
	var cap headlessCapture
	srv := headlessFakeRunner(t, &cap, "chat-run-h1")
	defer srv.Close()
	// ProjectPath set so runHeadless skips the ListProjects fallback.
	m := New(config.ChatConfig{ProjectPath: t.TempDir()}, srv.URL)
	if err := runHeadless(m, "hello"); err != nil {
		t.Fatalf("runHeadless: %v", err)
	}
	if cap.turns != 1 {
		t.Fatalf("turns=%d want 1", cap.turns)
	}
	if cap.turnStepID != "chat-run-h1" {
		t.Fatalf("turn stepId=%q want runner-minted chat-run-h1", cap.turnStepID)
	}
}

// TestRunHeadlessSynthesizesLocalProjectID locks the BUG-373 offline
// fallback: with no catalog project, StartRun must still carry a stable
// non-empty projectId so turns don't 502 on dispatch_prepare_failed.
func TestRunHeadlessSynthesizesLocalProjectID(t *testing.T) {
	var cap headlessCapture
	srv := headlessFakeRunner(t, &cap, "chat-run-h1")
	defer srv.Close()
	cwd := t.TempDir()
	m := New(config.ChatConfig{ProjectPath: cwd}, srv.URL)
	if err := runHeadless(m, "hello"); err != nil {
		t.Fatalf("runHeadless: %v", err)
	}
	if cap.startProj == "" {
		t.Fatal("StartRun projectId must not be empty offline")
	}
	if !strings.HasPrefix(cap.startProj, "proj-local-") {
		t.Fatalf("projectId=%q want proj-local- prefix", cap.startProj)
	}
	// Deterministic per cwd, distinct across cwds.
	if a, b := localProjectID("/x/a"), localProjectID("/x/a"); a != b {
		t.Fatal("localProjectID must be stable per cwd")
	}
	if a, b := localProjectID("/x/a"), localProjectID("/x/b"); a == b {
		t.Fatal("localProjectID must differ across cwds")
	}
}

// TestRunHeadlessResumeWithoutStepIDFallsBack covers a resumed run whose
// handle carries no StepID: resolveTurnStepID must still produce the
// synthetic chat-<runId> step instead of an empty one.
func TestRunHeadlessResumeWithoutStepIDFallsBack(t *testing.T) {
	var cap headlessCapture
	srv := headlessFakeRunner(t, &cap, "")
	defer srv.Close()
	m := New(config.ChatConfig{ProjectPath: t.TempDir(), ResumeRunID: "run-h1"}, srv.URL)
	if err := runHeadless(m, "hello"); err != nil {
		t.Fatalf("runHeadless: %v", err)
	}
	if cap.turnStepID != "chat-run-h1" {
		t.Fatalf("resumed turn stepId=%q want synthetic chat-run-h1", cap.turnStepID)
	}
}
