package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-399 (live run-3439): vibe-cp-ingest fed a plain README.md drafted SS
// files and a sprint plan and parked at a lock card. The intake must fail
// closed — reject the flow start unless the source is a CP-shaped document
// (requirements/07-Coding-Plan/**/CP-*.md carrying `Document ID: CP-*`).

func bug399Service(t *testing.T, cwd string) (*InteractiveService, string) {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui", Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("createRun: status=%d code=%q msg=%q", err.status, err.code, err.msg)
	}
	return svc, run.RunID
}

func bug399WriteCP(t *testing.T, cwd, name, body string) string {
	t.Helper()
	rel := filepath.Join("requirements", "07-Coding-Plan", "todo", name)
	abs := filepath.Join(cwd, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

func TestBug399_VibeCpIngestRejectsNonCPPrompt(t *testing.T) {
	cwd := t.TempDir()
	svc, runID := bug399Service(t, cwd)
	if err := os.WriteFile(filepath.Join(cwd, "README.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Live shape: flowRef=vibe-cp-ingest, prompt is the raw non-CP file name.
	turnID, apiErr := svc.startTurn(runID, TurnInput{
		Prompt: "README.md", FlowRef: "vibe-cp-ingest", StepID: "s1",
	}, "", "")
	if apiErr == nil {
		t.Fatalf("non-CP prompt must be rejected, got turnID=%q", turnID)
	}
	if apiErr.status != 422 || !strings.Contains(apiErr.msg, "CP") {
		t.Fatalf("want typed 422 not-a-CP rejection, got status=%d code=%q msg=%q", apiErr.status, apiErr.code, apiErr.msg)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	nodes := rs.activeFlowNodes
	svc.mu.Unlock()
	if len(nodes) != 0 {
		t.Fatalf("rejected ingest must not start flow nodes, got %+v", nodes)
	}
}

func TestBug399_VibeCpIngestRejectsMarkdownWithoutCPDocID(t *testing.T) {
	cwd := t.TempDir()
	svc, runID := bug399Service(t, cwd)
	// CP-shaped path but the body is a README — Document ID: CP-* absent.
	rel := bug399WriteCP(t, cwd, "CP-77-Not-A-Plan.md", "# Readme\n\nSome prose.\n")
	turnID, apiErr := svc.startTurn(runID, TurnInput{
		Prompt: rel, FlowRef: "vibe-cp-ingest", StepID: "s1",
	}, "", "")
	if apiErr == nil {
		t.Fatalf("CP path without Document ID must be rejected, got turnID=%q", turnID)
	}
	if apiErr.status != 422 {
		t.Fatalf("want 422, got status=%d code=%q msg=%q", apiErr.status, apiErr.code, apiErr.msg)
	}
}

func TestBug399_VibeCpIngestRejectsMissingFile(t *testing.T) {
	cwd := t.TempDir()
	svc, runID := bug399Service(t, cwd)
	// CP-shaped path but the file does not exist — unverifiable, fail closed.
	_, apiErr := svc.startTurn(runID, TurnInput{
		SourceDocID: "requirements/07-Coding-Plan/todo/CP-88-Missing.md",
		Prompt:      "implement the plan",
		FlowRef:     "vibe-cp-ingest", StepID: "s1",
	}, "", "")
	if apiErr == nil {
		t.Fatal("missing CP file must be rejected, not drafted from nothing")
	}
	if apiErr.status != 422 {
		t.Fatalf("want 422, got status=%d code=%q msg=%q", apiErr.status, apiErr.code, apiErr.msg)
	}
}

func TestBug399_VibeCpIngestAcceptsRealCP(t *testing.T) {
	cwd := t.TempDir()
	svc, runID := bug399Service(t, cwd)
	rel := bug399WriteCP(t, cwd, "CP-60-Test-Steps.md",
		"# CP-60\n\n- Document ID: `CP-60`\n- Feature Keys: vibe-mode\n\n## AI Quick View\n\n## 1. Overview\n")
	turnID, apiErr := svc.startTurn(runID, TurnInput{
		SourceDocID: rel, Prompt: "implement " + rel,
		FlowRef: "vibe-cp-ingest", StepID: "s1",
	}, "", "")
	if apiErr != nil {
		t.Fatalf("valid CP must be admitted: status=%d code=%q msg=%q", apiErr.status, apiErr.code, apiErr.msg)
	}
	if turnID == "" {
		t.Fatal("valid CP start returned no turnID")
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[runID]
		var nodes []agentpack.FlowNode
		if rs != nil {
			nodes = rs.activeFlowNodes
		}
		svc.mu.Unlock()
		if len(nodes) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("valid CP start never spawned the flow topology")
}

func TestBug399_VibeCpIngestFollowupTurnNotRevalidated(t *testing.T) {
	cwd := t.TempDir()
	svc, runID := bug399Service(t, cwd)
	rel := bug399WriteCP(t, cwd, "CP-60-Test-Steps.md",
		"# CP-60\n\n- Document ID: `CP-60`\n- Feature Keys: vibe-mode\n")
	if _, apiErr := svc.startTurn(runID, TurnInput{
		SourceDocID: rel, Prompt: "implement " + rel, FlowRef: "vibe-cp-ingest", StepID: "s1",
	}, "", ""); apiErr != nil {
		t.Fatalf("first turn: %v", apiErr)
	}
	// A second turn with a free-form prompt is a normal follow-up — the CP
	// source check only gates the flow-starting turn.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[runID]
		inFlight := rs != nil && rs.turnInFlight
		svc.mu.Unlock()
		if !inFlight {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}
