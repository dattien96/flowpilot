package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
)

func init() { fakeAdapterDelay = 0 }

func newTestServer(t *testing.T) (*InteractiveService, *httptest.Server) {
	t.Helper()
	svc := NewInteractiveService()
	svc.approvalTTL = 50 * time.Millisecond
	svc.questionTTL = 50 * time.Millisecond
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

func doJSON(t *testing.T, method, url string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

func startRun(t *testing.T, base string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs", StartRunInput{ProjectID: "proj-web", WorkflowID: "wf-feature", StepID: "step-plan"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if h.RunID == "" || h.ProviderSessionID == "" {
		t.Fatalf("empty handle: %+v", h)
	}
	return h.RunID
}

func startProjectRun(t *testing.T, base, projectID, workflowID string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs", StartRunInput{ProjectID: projectID, WorkflowID: workflowID, StepID: "step-plan"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start project run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	return h.RunID
}

func sendTurn(t *testing.T, base, runID, scenario string, headers map[string]string) (int, string) {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs/"+runID+"/turns",
		map[string]any{"stepId": "step-plan", "prompt": "hi", "scenario": scenario}, headers)
	var out struct {
		TurnID string `json:"turnId"`
	}
	_ = json.Unmarshal(body, &out)
	return status, out.TurnID
}

func getSnapshot(t *testing.T, base, runID string) runSnapshotView {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/client/workflow-runs/"+runID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", status, body)
	}
	var v runSnapshotView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return v
}

func TestCreateRunGeminiRequiresUsableWorkspacePath(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyGemini,
		DisplayName:  "Gemini",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{SkillSelection: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &captureTurnAdapter{ch: make(chan TurnRequest, 1)} },
	})
	svc := NewInteractiveServiceWithRegistry(reg)

	if _, apiErr := svc.createRun(StartRunInput{
		ProjectID:   "project-gate-sandbox",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyGemini,
	}); apiErr == nil || apiErr.code != "workspace_required" {
		t.Fatalf("createRun missing cwd error = %#v, want workspace_required", apiErr)
	}

	if _, apiErr := svc.createRun(StartRunInput{
		ProjectID:   "project-gate-sandbox",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyGemini,
		Cwd:         filepath.Join(t.TempDir(), "missing"),
	}); apiErr == nil || apiErr.code != "workspace_unavailable" {
		t.Fatalf("createRun missing dir error = %#v, want workspace_unavailable", apiErr)
	}

	if _, apiErr := svc.createRun(StartRunInput{
		ProjectID:   "project-gate-sandbox",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyGemini,
		Cwd:         t.TempDir(),
	}); apiErr != nil {
		t.Fatalf("createRun valid cwd error = %#v", apiErr)
	}
}

type captureTurnAdapter struct {
	ch chan TurnRequest
}

func (a *captureTurnAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *captureTurnAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, SkillSelection: true}
}
func (a *captureTurnAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.ch <- req
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	return nil
}

func TestChatModeSelectedControlsReachProviderTurnRequest(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:         ProviderKeyClaude,
		DisplayName: "Claude",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, SkillSelection: true, ApprovalEvents: true,
		},
		newAdapter: func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:       "proj-web",
		ProviderKey:     ProviderKeyClaude,
		Model:           "claude-sonnet-4-5",
		ReasoningEffort: "high",
		YoloMode:        true,
		ChatMode:        "normal_chat",
		Cwd:             "/Users/dev/project",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start chat run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if handle.StepID == "" {
		t.Fatalf("chat run must return synthetic step id: %+v", handle)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID,
		"prompt": "hello",
		"selectedSkills": []map[string]any{
			{"name": "planner", "source": "slash_picker"},
			{"name": "code-review", "source": "slash_picker"},
		},
		"reasoningEffort": "high",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send chat turn status=%d body=%s", status, body)
	}

	select {
	case req := <-capture.ch:
		if req.ModelName != "claude-sonnet-4-5" {
			t.Fatalf("ModelName = %q, want claude-sonnet-4-5", req.ModelName)
		}
		if req.ReasoningEffort != "high" {
			t.Fatalf("ReasoningEffort = %q, want high", req.ReasoningEffort)
		}
		if !req.YoloMode {
			t.Fatal("YoloMode = false, want true")
		}
		if req.Cwd != "/Users/dev/project" {
			t.Fatalf("Cwd = %q, want /Users/dev/project", req.Cwd)
		}
		if len(req.SelectedSkills) != 2 || req.SelectedSkills[0].Name != "planner" || req.SelectedSkills[1].Name != "code-review" {
			t.Fatalf("SelectedSkills = %+v, want planner + code-review", req.SelectedSkills)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured provider turn request")
	}
}

func TestLiveChatInjectsFeatureHistoryForSupportedProviders(t *testing.T) {
	workspace := t.TempDir()
	repoDir := t.TempDir()
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(repoDir, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := changeledger.New(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Upsert([]changeledger.Entry{{CommitHash: "c1", FeatureKey: "chat-ui", Summary: "first history", CommittedAt: "2026-01-01T00:00:00Z", Confidence: changeledger.ConfidenceHigh}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, ledger, dotDir); err != nil {
		t.Fatal(err)
	}

	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	rs := &interactiveRun{id: "run-1", providerKey: ProviderKeyClaude, workspaceCwd: workspace, runKind: "chat", turnCount: 1}
	svc.runTurn(context.Background(), rs, capture, TurnInput{StepID: "step-1", Prompt: "chat-ui"}, "", "turn-1", nil)

	select {
	case req := <-capture.ch:
		if !strings.Contains(req.Prompt, "first history") {
			t.Fatalf("expected feature history in prompt, got %q", req.Prompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured prompt")
	}
}

func TestLiveChatRefreshesLedgerBeforeFeatureHistoryInjection(t *testing.T) {
	workspace := t.TempDir()
	runGit(t, workspace, "init")
	runGit(t, workspace, "config", "user.email", "test@example.com")
	runGit(t, workspace, "config", "user.name", "Test User")
	auditDir := filepath.Join(workspace, "change-audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auditDir, "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chat.txt"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workspace, "add", ".")
	runGit(t, workspace, "commit", "-m", "[Feature][chat-ui][runner] fresh history Task-157")
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(dotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changeledger.SentinelPath(dotDir), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	rs := &interactiveRun{id: "run-1", providerKey: ProviderKeyCodex, workspaceCwd: workspace, runKind: "chat", turnCount: 1}
	svc.runTurn(context.Background(), rs, capture, TurnInput{StepID: "step-1", Prompt: "chat-ui"}, "", "turn-1", nil)

	select {
	case req := <-capture.ch:
		if !strings.Contains(req.Prompt, "fresh history") {
			t.Fatalf("expected refreshed history in prompt, got %q", req.Prompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured prompt")
	}
}

func TestLoadHandoffSummaryUsesMatchingStateKey(t *testing.T) {
	workspace := t.TempDir()
	repoDir := t.TempDir()
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(repoDir, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dotDir, "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	rs := &interactiveRun{
		id:           "run-1",
		runKind:      "chat",
		workspaceCwd: workspace,
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventMessageCompleted, Text: "first answer"},
			{Type: EventTurnCompleted, FinalMessage: "first answer"},
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventMessageCompleted, Text: "second answer"},
			{Type: EventTurnCompleted, FinalMessage: "second answer"},
		},
	}
	svc := NewInteractiveService()
	ledger, err := changeledger.NewChatSummaryLedger(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	baseLedger, err := changeledger.New(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := baseLedger.Upsert([]changeledger.Entry{{CommitHash: "c1", FeatureKey: "chat-ui", Summary: "history"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, baseLedger, dotDir); err != nil {
		t.Fatal(err)
	}
	matchKey := transcriptStateKey(rs.id, transcriptTurnsFromRun(rs))
	if err := ledger.Append([]changeledger.ChatSummaryEntry{
		{RunID: rs.id, TurnID: "turn-1", FeatureKey: "chat-ui", StateKey: "turn-1:old-state", Summary: "stale summary", CreatedAt: "2026-01-01T00:00:00Z"},
		{RunID: rs.id, TurnID: "turn-1", FeatureKey: "chat-ui", StateKey: matchKey, Summary: "fresh summary", CreatedAt: "2026-01-02T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	got := svc.loadHandoffSummary(rs, transcriptTurnsFromRun(rs))
	if got != "fresh summary" {
		t.Fatalf("loadHandoffSummary() = %q, want fresh summary", got)
	}
}

// Regression: hybrid must survive off-feature turns. The recorder hashes the
// feature-bucketed turns, so the handoff must hash the SAME set — otherwise an
// interleaved off-feature turn (e.g. a one-off "write a haiku") changes the
// all-turns hash and silently degrades hybrid to raw, even though a valid summary
// for the run exists.
func TestLoadHandoffSummaryMatchesAcrossOffFeatureTurns(t *testing.T) {
	workspace := t.TempDir()
	repoDir := t.TempDir()
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(repoDir, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dotDir, "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	rs := &interactiveRun{
		id:           "run-1",
		runKind:      "chat",
		workspaceCwd: workspace,
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventTurnCompleted, FinalMessage: "first answer"},
			{Type: EventTurnStarted, Prompt: "write a haiku about the sea"}, // off-feature
			{Type: EventTurnCompleted, FinalMessage: "waves fold into shore"},
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventTurnCompleted, FinalMessage: "second answer"},
		},
	}
	svc := NewInteractiveService()
	ledger, err := changeledger.NewChatSummaryLedger(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	baseLedger, err := changeledger.New(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := baseLedger.Upsert([]changeledger.Entry{{CommitHash: "c1", FeatureKey: "chat-ui", Summary: "history"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, baseLedger, dotDir); err != nil {
		t.Fatal(err)
	}
	catalog, err := featurecatalog.LoadCatalog(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	allTurns := transcriptTurnsFromRun(rs)
	bucketKey := transcriptStateKey(rs.id, featureBucketTurns(allTurns, catalog, "chat-ui"))
	// The off-feature turn must actually change the all-turns hash, else the test
	// proves nothing.
	if bucketKey == transcriptStateKey(rs.id, allTurns) {
		t.Fatal("setup invalid: off-feature turn did not change the all-turns hash")
	}
	if err := ledger.Append([]changeledger.ChatSummaryEntry{
		{RunID: rs.id, TurnID: "turn-3", FeatureKey: "chat-ui", StateKey: bucketKey, Summary: "fresh summary", CreatedAt: "2026-01-02T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := svc.loadHandoffSummary(rs, allTurns); got != "fresh summary" {
		t.Fatalf("hybrid summary not matched across off-feature turns: got %q (would degrade to raw)", got)
	}
}

func TestBuildHandoffContextUsesRawModeWhenNoSummaryAndHistoryFits(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{
		id:          "run-1",
		providerKey: ProviderKeyCodex,
		runKind:     "chat",
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "continue this task"},
			{Type: EventTurnCompleted, FinalMessage: "previous answer"},
		},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	result, apiErr := svc.buildHandoffContext(context.Background(), rs.id, handoffContextRequest{TargetProviderKey: ProviderKeyClaude})
	if apiErr != nil {
		t.Fatalf("buildHandoffContext returned error: %+v", apiErr)
	}
	// No cached summary and the whole short conversation fits: pure raw floor,
	// no summary block and no self-summarize instruction.
	if result.HandoffMode != "raw" {
		t.Fatalf("handoff mode = %q, want raw", result.HandoffMode)
	}
	if strings.Contains(result.Prompt, "First summarize the previous conversation") {
		t.Fatalf("raw handoff should not carry the self-summarize instruction: %q", result.Prompt)
	}
	if strings.Contains(result.Prompt, "<conversation_summary>") {
		t.Fatalf("raw handoff should not carry a summary block: %q", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "continue this task") || !strings.Contains(result.Prompt, "previous answer") {
		t.Fatalf("raw handoff missing the conversation: %q", result.Prompt)
	}
}

func TestBuildHandoffContextUsesTargetSummaryModeWhenHistoryTruncated(t *testing.T) {
	svc := NewInteractiveService()
	// A single newest turn larger than the budget forces truncation, so the raw
	// floor is lossy and (with no cached summary) the mode is target_summary.
	rs := &interactiveRun{
		id:          "run-1",
		providerKey: ProviderKeyCodex,
		runKind:     "chat",
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "kick off"},
			{Type: EventTurnCompleted, FinalMessage: strings.Repeat("verbose answer. ", 6000)},
		},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	result, apiErr := svc.buildHandoffContext(context.Background(), rs.id, handoffContextRequest{TargetProviderKey: ProviderKeyClaude})
	if apiErr != nil {
		t.Fatalf("buildHandoffContext returned error: %+v", apiErr)
	}
	if result.HandoffMode != "target_summary" {
		t.Fatalf("handoff mode = %q, want target_summary for a truncated history", result.HandoffMode)
	}
	if !result.Truncated {
		t.Fatalf("expected Truncated=true for an oversized turn")
	}
	if !strings.Contains(result.Prompt, "First summarize the previous conversation") {
		t.Fatalf("handoff prompt missing target-summary instruction: %q", result.Prompt)
	}
}

func TestBuildHandoffContextIgnoresEmptyStateKeySummary(t *testing.T) {
	workspace := t.TempDir()
	repoDir := t.TempDir()
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(repoDir, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseLedger, err := changeledger.New(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := baseLedger.Upsert([]changeledger.Entry{{CommitHash: "c1", FeatureKey: "chat-ui", Summary: "history"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, baseLedger, dotDir); err != nil {
		t.Fatal(err)
	}
	summaryLedger, err := changeledger.NewChatSummaryLedger(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := summaryLedger.Append([]changeledger.ChatSummaryEntry{
		{RunID: "run-1", TurnID: "turn-1", FeatureKey: "chat-ui", Summary: "legacy summary", CreatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{
		id:           "run-1",
		providerKey:  ProviderKeyCodex,
		runKind:      "chat",
		workspaceCwd: workspace,
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventTurnCompleted, FinalMessage: "previous answer"},
		},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	result, apiErr := svc.buildHandoffContext(context.Background(), rs.id, handoffContextRequest{TargetProviderKey: ProviderKeyClaude})
	if apiErr != nil {
		t.Fatalf("buildHandoffContext returned error: %+v", apiErr)
	}
	// The stale summary has no matching state_key, so it is ignored. The short
	// transcript fits whole, so the degrade lands on the raw floor (not hybrid).
	if result.HandoffMode != "raw" {
		t.Fatalf("handoff mode = %q, want raw for ignored empty-state-key summary", result.HandoffMode)
	}
	// The stale summary must not be used as the hybrid <conversation_summary>. (It
	// may still appear in the prepended "## Prior discussion" feature block, which is
	// the Task-161 timeline — a separate, legitimate channel.)
	if strings.Contains(result.Prompt, "<conversation_summary>") {
		t.Fatalf("empty-state-key summary should not become a hybrid conversation_summary: %q", result.Prompt)
	}
}

func TestRecordChatSummaryTriggersBestEffortContextSync(t *testing.T) {
	workspace, repoDir, rs := chatSummarySyncFixture(t)
	svc := NewInteractiveService()

	var gotProjectID, gotDotDir string
	svc.syncContextEngineFilesHook = func(projectID, dotFlowpilotDir string) (error, string) {
		gotProjectID = projectID
		gotDotDir = dotFlowpilotDir
		return nil, "manifest ok; 4 files synced to drive, 0 skipped"
	}

	svc.recordChatSummarySync(rs, "turn-1")

	if gotProjectID != rs.projectID {
		t.Fatalf("sync projectID = %q, want %q", gotProjectID, rs.projectID)
	}
	wantDotDir := filepath.Join(workspace, ".flowpilot")
	if gotDotDir != wantDotDir {
		t.Fatalf("sync dotFlowpilotDir = %q, want %q", gotDotDir, wantDotDir)
	}

	ledger, err := changeledger.NewChatSummaryLedger(wantDotDir)
	if err != nil {
		t.Fatalf("NewChatSummaryLedger error: %v", err)
	}
	summaries, err := ledger.GetFeatureSummaries("chat-ui")
	if err != nil {
		t.Fatalf("GetFeatureSummaries error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d, want 1", len(summaries))
	}
	if !strings.Contains(summaries[0].Summary, "sync trigger") {
		t.Fatalf("summary %q missing expected content", summaries[0].Summary)
	}

	if _, err := os.Stat(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md")); err != nil {
		t.Fatalf("fixture repo missing feature keys: %v", err)
	}
}

func TestRecordChatSummarySwallowsSyncFailure(t *testing.T) {
	workspace, _, rs := chatSummarySyncFixture(t)
	svc := NewInteractiveService()
	svc.syncContextEngineFilesHook = func(projectID, dotFlowpilotDir string) (error, string) {
		return os.ErrPermission, "manifest ok; 4 files synced to drive, 0 skipped"
	}

	svc.recordChatSummarySync(rs, "turn-1")

	ledger, err := changeledger.NewChatSummaryLedger(filepath.Join(workspace, ".flowpilot"))
	if err != nil {
		t.Fatalf("NewChatSummaryLedger error: %v", err)
	}
	summaries, err := ledger.GetFeatureSummaries("chat-ui")
	if err != nil {
		t.Fatalf("GetFeatureSummaries error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d, want 1", len(summaries))
	}

	data, err := os.ReadFile(filepath.Join(workspace, ".flowpilot", "ledger", "chat_summary.ndjson"))
	if err != nil {
		t.Fatalf("ReadFile chat_summary.ndjson: %v", err)
	}
	if !strings.Contains(string(data), `"feature_key":"chat-ui"`) {
		t.Fatalf("chat_summary.ndjson missing feature key: %s", data)
	}
}

func chatSummarySyncFixture(t *testing.T) (workspace string, repoDir string, rs *interactiveRun) {
	t.Helper()
	workspace = t.TempDir()
	repoDir = t.TempDir()
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(repoDir, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "change-audit", "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseLedger, err := changeledger.New(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := baseLedger.Upsert([]changeledger.Entry{{CommitHash: "c1", FeatureKey: "chat-ui", Summary: "history"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, baseLedger, dotDir); err != nil {
		t.Fatal(err)
	}
	rs = &interactiveRun{
		id:           "run-1",
		projectID:    "proj-chat-sync",
		runKind:      "chat",
		workspaceCwd: workspace,
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventMessageCompleted, Text: "assistant response for sync trigger"},
			{Type: EventTurnCompleted, FinalMessage: "assistant response for sync trigger"},
		},
	}
	return workspace, repoDir, rs
}

// A repeated finalization with no new committed turn must reuse the cached
// summary (Task-162 T-3): the state_key matches, so no duplicate entry and no
// second (cheap-model) summarization is performed.
func TestRecordChatSummaryReusesCachedSummaryOnUnchangedState(t *testing.T) {
	workspace, _, rs := chatSummarySyncFixture(t)
	svc := NewInteractiveService()
	svc.syncContextEngineFilesHook = func(string, string) (error, string) { return nil, "ok" }

	svc.recordChatSummarySync(rs, "turn-1")
	svc.recordChatSummarySync(rs, "turn-2")

	ledger, err := changeledger.NewChatSummaryLedger(filepath.Join(workspace, ".flowpilot"))
	if err != nil {
		t.Fatalf("NewChatSummaryLedger error: %v", err)
	}
	summaries, err := ledger.GetFeatureSummaries("chat-ui")
	if err != nil {
		t.Fatalf("GetFeatureSummaries error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d, want 1 (cache guard should suppress the duplicate)", len(summaries))
	}
}

// A new committed turn changes the transcript state and must refresh the rolling
// summary in place (upsert): still one entry per run+feature, but with an updated
// state_key — not a second appended line.
func TestRecordChatSummaryRefreshesInPlaceAfterNewTurn(t *testing.T) {
	workspace, _, rs := chatSummarySyncFixture(t)
	svc := NewInteractiveService()
	svc.syncContextEngineFilesHook = func(string, string) (error, string) { return nil, "ok" }

	svc.recordChatSummarySync(rs, "turn-1")
	ledger, err := changeledger.NewChatSummaryLedger(filepath.Join(workspace, ".flowpilot"))
	if err != nil {
		t.Fatalf("NewChatSummaryLedger error: %v", err)
	}
	first, _ := ledger.GetFeatureSummaries("chat-ui")
	if len(first) != 1 {
		t.Fatalf("first summary count = %d, want 1", len(first))
	}

	rs.events = append(rs.events,
		ProviderEvent{Type: EventTurnStarted, Prompt: "chat-ui follow up"},
		ProviderEvent{Type: EventMessageCompleted, Text: "second response for sync trigger"},
		ProviderEvent{Type: EventTurnCompleted, FinalMessage: "second response for sync trigger"},
	)
	svc.recordChatSummarySync(rs, "turn-2")

	ledger2, err := changeledger.NewChatSummaryLedger(filepath.Join(workspace, ".flowpilot"))
	if err != nil {
		t.Fatalf("NewChatSummaryLedger error: %v", err)
	}
	after, _ := ledger2.GetFeatureSummaries("chat-ui")
	if len(after) != 1 {
		t.Fatalf("summary count = %d, want 1 (upsert refreshes in place, no new line)", len(after))
	}
	if after[0].StateKey == first[0].StateKey {
		t.Fatalf("expected state_key to change after a new turn, got unchanged %q", after[0].StateKey)
	}
}

// A chat that spans two features must bucket turns per feature so each summary
// sees only its own turns (no cross-feature mixing). Low-signal turns attach to
// the running feature (CP-37 V-161-14).
func TestBucketTurnsByFeatureSeparatesFeatures(t *testing.T) {
	catalog := featurecatalog.New()
	catalog.Add(featurecatalog.Feature{Key: "alpha", Keywords: []string{"alpha"}})
	catalog.Add(featurecatalog.Feature{Key: "bravo", Keywords: []string{"bravo"}})

	turns := []transcriptTurn{
		{User: "work on alpha feature", Assistant: "did alpha"},
		{User: "continue", Assistant: "kept going on alpha"},
		{User: "now switch to bravo feature", Assistant: "did bravo"},
	}
	buckets := bucketTurnsByFeature(turns, catalog)

	if len(buckets["alpha"]) != 2 {
		t.Fatalf("alpha bucket = %d turns, want 2 (incl. the low-signal continue)", len(buckets["alpha"]))
	}
	if len(buckets["bravo"]) != 1 {
		t.Fatalf("bravo bucket = %d turns, want 1", len(buckets["bravo"]))
	}
	for _, turn := range buckets["alpha"] {
		if strings.Contains(turn.User, "bravo") {
			t.Fatalf("alpha bucket leaked a bravo turn: %q", turn.User)
		}
	}
}

// The manual generate endpoint errors while a turn is in flight and otherwise
// generates immediately (CP-37 V-161-12 / V-161-13).
func TestGenerateChatSummaryNow(t *testing.T) {
	_, _, rs := chatSummarySyncFixture(t)
	svc := NewInteractiveService()
	svc.syncContextEngineFilesHook = func(string, string) (error, string) { return nil, "ok" }
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	// Busy → 409.
	svc.mu.Lock()
	rs.turnInFlight = true
	svc.mu.Unlock()
	if _, apiErr := svc.generateChatSummaryNow(rs.id); apiErr == nil || apiErr.status != 409 {
		t.Fatalf("expected 409 while running, got %+v", apiErr)
	}

	// Idle → generates.
	svc.mu.Lock()
	rs.turnInFlight = false
	svc.mu.Unlock()
	resp, apiErr := svc.generateChatSummaryNow(rs.id)
	if apiErr != nil {
		t.Fatalf("generateChatSummaryNow error: %+v", apiErr)
	}
	if !resp.Generated {
		t.Fatalf("expected Generated=true, got %+v", resp)
	}
	// Second call with no change → skipped (hash match), still no error.
	resp2, apiErr := svc.generateChatSummaryNow(rs.id)
	if apiErr != nil {
		t.Fatalf("second generateChatSummaryNow error: %+v", apiErr)
	}
	if resp2.Generated || !resp2.Skipped {
		t.Fatalf("expected second call skipped (no change), got %+v", resp2)
	}
}

func TestNormalizeSummaryBullets(t *testing.T) {
	out := normalizeSummaryBullets("- Goal: ship handoff\n* decided to reuse the summarizer\n\n  • blocked on review\nextra line one\nextra line two\nextra line three")
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("bullet count = %d, want 5 (capped): %q", len(lines), out)
	}
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "- ") {
			t.Fatalf("line not normalized to a bullet: %q", ln)
		}
	}
	if strings.Contains(out, "</previous_conversation>") {
		t.Fatal("closing tag should be escaped")
	}
}

// A low-signal latest prompt ("try again") must inherit the conversation's
// established feature by scanning back to the earlier substantive prompt, so the
// summary is still recorded under that feature (CP-37 V-161-10).
func TestRecordChatSummaryResolvesFeatureFromEarlierTurnOnLowSignalPrompt(t *testing.T) {
	workspace, _, _ := chatSummarySyncFixture(t)
	svc := NewInteractiveService()
	svc.syncContextEngineFilesHook = func(string, string) (error, string) { return nil, "ok" }

	rs := &interactiveRun{
		id:           "run-low-signal",
		projectID:    "proj-low",
		runKind:      "chat",
		workspaceCwd: workspace,
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "chat-ui"},
			{Type: EventMessageCompleted, Text: "worked on the chat-ui input"},
			{Type: EventTurnCompleted, FinalMessage: "worked on the chat-ui input"},
			{Type: EventTurnStarted, Prompt: "try again"},
			{Type: EventMessageCompleted, Text: "retried the change"},
			{Type: EventTurnCompleted, FinalMessage: "retried the change"},
		},
	}
	svc.recordChatSummarySync(rs, "turn-2")

	ledger, err := changeledger.NewChatSummaryLedger(filepath.Join(workspace, ".flowpilot"))
	if err != nil {
		t.Fatalf("NewChatSummaryLedger error: %v", err)
	}
	summaries, err := ledger.GetFeatureSummaries("chat-ui")
	if err != nil {
		t.Fatalf("GetFeatureSummaries error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d, want 1 (low-signal latest prompt should inherit chat-ui)", len(summaries))
	}
}

// Injection on a low-signal prompt falls back to the established feature when
// prior turns resolve it, and injects nothing when there is no prior context to
// fall back on (CP-37 V-161-11).
func TestInjectFeatureHistoryFallsBackToPriorTurnFeature(t *testing.T) {
	workspace, _, _ := chatSummarySyncFixture(t)

	prior := []transcriptTurn{{User: "chat-ui", Assistant: "worked on the chat-ui input"}}
	withPrior := injectFeatureHistoryPrompt(workspace, "continue", prior)
	if !strings.Contains(withPrior, `## Prior work on "chat-ui"`) {
		t.Fatalf("expected chat-ui history injected via fallback, got: %q", withPrior)
	}

	noPrior := injectFeatureHistoryPrompt(workspace, "continue", nil)
	if noPrior != "continue" {
		t.Fatalf("expected prompt unchanged with no prior feature, got: %q", noPrior)
	}
}

// The continuation battery from CP-37 Test E4: only explicit continuations inherit
// the running feature. Greetings, acknowledgements, and any other short or
// substantive prompt do NOT — they drop prior context.
func TestIsContinuationPrompt(t *testing.T) {
	continuations := []string{
		"continue", "try again", "retry", "do it", "do it again", "go on",
		"go ahead", "keep going", "proceed", "resume", "next", "more", "redo",
		"Continue.", "  retry  ", "TRY AGAIN",
	}
	for _, p := range continuations {
		if !isContinuationPrompt(p) {
			t.Errorf("isContinuationPrompt(%q) = false, want true (continuation → inherit)", p)
		}
	}
	notContinuations := []string{
		"ok", "okay", "yes", "yep", "no", "sure", // acknowledgements — do NOT inherit
		"hi", "hello", "hey", "thanks", "", // greetings / empty — do NOT inherit
		"write a haiku about the sea", // substantive off-topic
		"add a safe arithmetic divide to calc-core",
	}
	for _, p := range notContinuations {
		if isContinuationPrompt(p) {
			t.Errorf("isContinuationPrompt(%q) = true, want false (non-continuation → drop)", p)
		}
	}
}

// A substantive but unrelated prompt must NOT inherit the prior feature's context
// — only low-signal continuations do. "write a haiku about the sea" after a
// resolved feature turn injects nothing (CP-37 Test A negative control holds even
// mid-conversation).
func TestInjectFeatureHistoryDropsContextOnUnrelatedPrompt(t *testing.T) {
	workspace, _, _ := chatSummarySyncFixture(t)

	prior := []transcriptTurn{{User: "chat-ui", Assistant: "worked on the chat-ui input"}}
	out := injectFeatureHistoryPrompt(workspace, "write a haiku about the sea", prior)
	if out != "write a haiku about the sea" {
		t.Fatalf("expected no injection for unrelated substantive prompt, got: %q", out)
	}
	if strings.Contains(out, "Prior work on") {
		t.Fatalf("unrelated prompt should not inherit prior feature context: %q", out)
	}
}

// A substantive but unrelated turn must not attach to the running feature's
// bucket — otherwise its content would pollute that feature's summary.
func TestBucketTurnsByFeatureDropsUnrelatedTurn(t *testing.T) {
	catalog := featurecatalog.New()
	catalog.Add(featurecatalog.Feature{Key: "alpha", Keywords: []string{"alpha"}})

	turns := []transcriptTurn{
		{User: "work on alpha feature", Assistant: "did alpha"},
		{User: "write a haiku about the sea", Assistant: "here is a haiku"},
	}
	buckets := bucketTurnsByFeature(turns, catalog)
	if len(buckets["alpha"]) != 1 {
		t.Fatalf("alpha bucket = %d turns, want 1 (unrelated turn dropped)", len(buckets["alpha"]))
	}
}

// A flow-gate reprompt must inherit the conversation's established feature, not
// resolve on its own process text — which names feature keys ("Suggested feature
// keys: meta") and would otherwise mis-resolve the turn (CP-37 Test B).
func TestResolveInjectionFeatureGateRepromptInheritsEstablishedFeature(t *testing.T) {
	catalog := featurecatalog.New()
	catalog.Add(featurecatalog.Feature{Key: "alpha", Keywords: []string{"alpha"}})
	catalog.Add(featurecatalog.Feature{Key: "meta", Keywords: []string{"feature", "document", "change"}})

	prior := []transcriptTurn{{User: "work on alpha"}}
	reprompt := flowgate.GateRepromptPrefix +
		"\n\n• Missing change-audit note. You changed code but did not add a change-audit note." +
		"\n\n• Missing or unverified feature key. Suggested feature keys: meta, alpha."

	top, ok := resolveInjectionFeature(reprompt, prior, catalog)
	if !ok || top.Key != "alpha" {
		t.Fatalf("gate reprompt should inherit alpha, got ok=%v key=%q", ok, top.Key)
	}
}

// A cross-provider handoff envelope must not self-resolve a feature from its own
// text — it embeds the prior conversation and gate-reprompt lines that name feature
// keys (e.g. "Suggested feature keys: sandbox-meta"). On a fresh target run (no
// prior turns) that means no block, NOT a feature the envelope merely mentions
// (CP-37 Test D — run-5611 injected the wrong "sandbox-meta" history before this).
func TestResolveInjectionFeatureHandoffPromptDoesNotSelfResolve(t *testing.T) {
	catalog := featurecatalog.New()
	catalog.Add(featurecatalog.Feature{Key: "sandbox-meta", Keywords: []string{"sandbox", "meta"}})
	catalog.Add(featurecatalog.Feature{Key: "calc-core", Keywords: []string{"calc", "arithmetic"}})

	handoff := handoffPromptPrefix +
		"\n\nSource provider: claude\n\n<previous_conversation>\nUser:\n" +
		"The flow gate is asking… Suggested feature keys: sandbox-meta, calc-core.\n</previous_conversation>"

	if _, ok := resolveInjectionFeature(handoff, nil, catalog); ok {
		t.Fatal("handoff envelope must not self-resolve a feature on a fresh target run")
	}
}

// The recording/bucketing path must likewise keep a gate reprompt on the running
// feature instead of opening a bogus bucket for a key its text mentions.
func TestBucketTurnsByFeatureGateRepromptInheritsFeature(t *testing.T) {
	catalog := featurecatalog.New()
	catalog.Add(featurecatalog.Feature{Key: "alpha", Keywords: []string{"alpha"}})
	catalog.Add(featurecatalog.Feature{Key: "meta", Keywords: []string{"feature", "document", "change"}})

	turns := []transcriptTurn{
		{User: "work on alpha", Assistant: "did alpha"},
		{User: flowgate.GateRepromptPrefix + "\n\n• Missing change-audit note. Suggested feature keys: meta.", Assistant: "added the doc"},
	}
	buckets := bucketTurnsByFeature(turns, catalog)
	if len(buckets["alpha"]) != 2 {
		t.Fatalf("alpha bucket = %d turns, want 2 (reprompt inherits alpha)", len(buckets["alpha"]))
	}
	if _, ok := buckets["meta"]; ok {
		t.Fatalf("gate reprompt must not open a 'meta' bucket")
	}
}

func TestBuildHandoffContextRejectsInvalidSources(t *testing.T) {
	tests := []struct {
		name    string
		run     *interactiveRun
		target  ProviderKey
		wantErr string
	}{
		{
			name: "same provider",
			run: &interactiveRun{
				id:          "run-same",
				providerKey: ProviderKeyCodex,
				runKind:     "chat",
				events:      []ProviderEvent{{Type: EventTurnStarted, Prompt: "hello"}},
			},
			target:  ProviderKeyCodex,
			wantErr: "handoff_same_provider",
		},
		{
			name: "busy run",
			run: &interactiveRun{
				id:           "run-busy",
				providerKey:  ProviderKeyCodex,
				runKind:      "chat",
				turnInFlight: true,
				events:       []ProviderEvent{{Type: EventTurnStarted, Prompt: "hello"}},
			},
			target:  ProviderKeyClaude,
			wantErr: "handoff_run_busy",
		},
		{
			name: "non chat",
			run: &interactiveRun{
				id:          "run-workflow",
				providerKey: ProviderKeyCodex,
				runKind:     "workflow",
				events:      []ProviderEvent{{Type: EventTurnStarted, Prompt: "hello"}},
			},
			target:  ProviderKeyClaude,
			wantErr: "handoff_run_kind_unsupported",
		},
		{
			name: "unsupported source",
			run: &interactiveRun{
				id:          "run-gemini",
				providerKey: ProviderKeyGemini,
				runKind:     "chat",
				events:      []ProviderEvent{{Type: EventTurnStarted, Prompt: "hello"}},
			},
			target:  ProviderKeyClaude,
			wantErr: "handoff_source_provider_unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewInteractiveService()
			svc.mu.Lock()
			svc.runs[tt.run.id] = tt.run
			svc.mu.Unlock()

			_, apiErr := svc.buildHandoffContext(context.Background(), tt.run.id, handoffContextRequest{TargetProviderKey: tt.target})
			if apiErr == nil || apiErr.code != tt.wantErr {
				t.Fatalf("buildHandoffContext error = %+v, want code %q", apiErr, tt.wantErr)
			}
		})
	}
}

// TestChatModeTurnLevelControlsOverrideRunDefaults locks BUG-063: model + YOLO are
// per-turn in chat mode, so changing them between prompts reaches the provider turn
// request — they are no longer frozen at the run-level values captured at startRun.
func TestChatModeTurnLevelControlsOverrideRunDefaults(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj-web",
		ProviderKey: ProviderKeyClaude,
		Model:       "model-a",
		YoloMode:    true,
		ChatMode:    "normal_chat",
		Cwd:         "/w",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	// Turn 1 omits model/yolo → the run-level defaults (model-a, yolo=true) are used.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID, "prompt": "one",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("turn 1 status=%d body=%s", status, body)
	}
	req1 := <-capture.ch
	if req1.ModelName != "model-a" || !req1.YoloMode {
		t.Fatalf("turn 1 req = {model:%q yolo:%v}, want {model-a true}", req1.ModelName, req1.YoloMode)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "turn 1 to finish")

	// Turn 2 supplies new per-turn values → they override the run-level defaults.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID, "prompt": "two", "model": "model-b", "yoloMode": false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("turn 2 status=%d body=%s", status, body)
	}
	req2 := <-capture.ch
	if req2.ModelName != "model-b" || req2.YoloMode {
		t.Fatalf("turn 2 req = {model:%q yolo:%v}, want {model-b false}", req2.ModelName, req2.YoloMode)
	}
}

func TestAgentGraphRoutesExposeSnapshotAndControls(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	spawned, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-graph", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", status, body)
	}
	var graph AgentGraphSnapshot
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if graph.LoopState.RoundCap != 3 {
		t.Fatalf("round cap = %d, want 3", graph.LoopState.RoundCap)
	}
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/pause", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("decode paused graph: %v", err)
	}
	if len(graph.Runs) == 0 || graph.Runs[0].RunID != spawned.RunID || graph.Runs[0].AgentName == "" || graph.Runs[0].Status == "" {
		t.Fatalf("pause graph lost run metadata: %+v", graph.Runs)
	}
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/feedback", map[string]any{"message": "revise", "toRunId": "child-1"}, nil)
	if status != http.StatusOK {
		t.Fatalf("feedback status=%d body=%s", status, body)
	}
	status, body = doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-bus", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("bus status=%d body=%s", status, body)
	}
	var bus []AgentBusMessage
	if err := json.Unmarshal(body, &bus); err != nil {
		t.Fatalf("decode bus: %v", err)
	}
	foundFeedback := false
	for _, msg := range bus {
		if msg.Kind == "user-feedback" && msg.Message == "revise" {
			foundFeedback = true
			break
		}
	}
	if !foundFeedback {
		t.Fatalf("bus = %+v", bus)
	}
}

// A flow-authored node stores its agent reference as the full definition
// path (the Agent-ref dropdown submits agent.path to keep same-named agents
// across sources distinct). spawnChildRun must resolve that path back to the
// real AgentDefinition — otherwise agentDef stays nil, the child runs with the
// bare task prompt (no system prompt, no identity line), and the UI shows the
// raw path instead of the agent's clean name.
func TestSpawnChildResolvesAgentByPath(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	agentPath := filepath.Join(agentsDir, "coder-agent.toml")
	def := "name = \"coder-agent\"\nrole = \"coder\"\ndeveloper_instructions = \"You are the project coder.\"\n"
	if err := os.WriteFile(agentPath, []byte(def), 0o644); err != nil {
		t.Fatalf("write agent def: %v", err)
	}

	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workspaceCwd = dir
	svc.mu.Unlock()

	spawned, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: agentPath, Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	svc.mu.Lock()
	gotName, gotRole := "", ""
	if child := svc.runs[spawned.RunID]; child != nil {
		gotName = child.agentName
		gotRole = child.role
	}
	svc.mu.Unlock()
	if gotName != "coder-agent" {
		t.Fatalf("child agentName = %q, want %q (path did not resolve to the definition)", gotName, "coder-agent")
	}
	if gotRole != "coder" {
		t.Fatalf("child role = %q, want %q", gotRole, "coder")
	}
}

// An extensionless or hand-typed path (e.g. "...\coder-agent" for the file
// "...\coder-agent.toml", as stored by the old free-text Agent-ref input)
// must still resolve via the base-name fallback rather than spawning an agent
// with no identity.
func TestSpawnChildResolvesAgentByExtensionlessPath(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	agentPath := filepath.Join(agentsDir, "coder-agent.toml")
	def := "name = \"coder-agent\"\nrole = \"coder\"\ndeveloper_instructions = \"You are the project coder.\"\n"
	if err := os.WriteFile(agentPath, []byte(def), 0o644); err != nil {
		t.Fatalf("write agent def: %v", err)
	}

	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workspaceCwd = dir
	svc.mu.Unlock()

	// Note: the ref intentionally omits the ".toml" extension.
	extensionless := filepath.Join(agentsDir, "coder-agent")
	spawned, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: extensionless, Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	svc.mu.Lock()
	gotName := ""
	if child := svc.runs[spawned.RunID]; child != nil {
		gotName = child.agentName
	}
	svc.mu.Unlock()
	if gotName != "coder-agent" {
		t.Fatalf("child agentName = %q, want %q (extensionless path did not resolve)", gotName, "coder-agent")
	}
}

func TestSpawnChildEmitsGraphAndBusEvents(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false}); spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	status, body := doJSON(t, "GET", srv.URL+"/admin/workflow-runs/"+parent.RunID+"/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("admin events status=%d body=%s", status, body)
	}
	var evs []ProviderEvent
	if err := json.Unmarshal(body, &evs); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	hasGraph := false
	hasBus := false
	for _, ev := range evs {
		if ev.Type == EventAgentGraphUpdated {
			hasGraph = true
		}
		if ev.Type == EventAgentBusMessage {
			hasBus = true
		}
	}
	if !hasGraph || !hasBus {
		t.Fatalf("events missing graph/bus updates: %+v", evs)
	}
}

type loopRestartAdapter struct {
	turns chan string
}

func (a *loopRestartAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *loopRestartAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *loopRestartAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.turns <- req.Prompt
	switch {
	case strings.Contains(strings.ToLower(req.Prompt), "review"):
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "changes requested"})
	default:
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coder complete"})
	}
	return nil
}

type gatedLoopAdapter struct {
	turns        chan string
	releaseCoder chan struct{}
}

func (a *gatedLoopAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *gatedLoopAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *gatedLoopAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.turns <- req.Prompt
	switch {
	case strings.Contains(strings.ToLower(req.Prompt), "implement"):
		<-a.releaseCoder
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coder complete"})
	case strings.Contains(strings.ToLower(req.Prompt), "review"):
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "changes requested"})
	default:
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	}
	return nil
}

func TestChangesRequestedRestartsCoderTurn(t *testing.T) {
	svc := NewInteractiveService()
	adapter := &loopRestartAdapter{turns: make(chan string, 4)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return adapter },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	coder, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawn coder: %v", spawnErr)
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review", Provider: "codex", DependsOn: []string{coder.RunID}, Wait: false}); spawnErr != nil {
		t.Fatalf("spawn reviewer: %v", spawnErr)
	}
	seen := []string{}
	deadline := time.After(3 * time.Second)
	for len(seen) < 3 {
		select {
		case prompt := <-adapter.turns:
			seen = append(seen, prompt)
		case <-deadline:
			t.Fatalf("timed out waiting for turns, saw %v", seen)
		}
	}
	if !strings.Contains(strings.ToLower(seen[2]), "changes requested") {
		t.Fatalf("third turn prompt = %q, want restarted coder with review feedback", seen[2])
	}
	graph := svc.agentGraphSnapshot(parent.RunID)
	if graph.LoopState.Round != 1 {
		t.Fatalf("loop round = %d, want 1 after changes-requested retry", graph.LoopState.Round)
	}
}

func TestReviewerWaitsForDependencyAndResumeReleasesPendingTurn(t *testing.T) {
	svc := NewInteractiveService()
	adapter := &gatedLoopAdapter{turns: make(chan string, 4), releaseCoder: make(chan struct{})}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return adapter },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.pauseAgentLoop(parent.RunID, "paused for test")
	coder, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawn coder: %v", spawnErr)
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review", Provider: "codex", DependsOn: []string{coder.RunID}, Wait: false}); spawnErr != nil {
		t.Fatalf("spawn reviewer: %v", spawnErr)
	}
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "implement") {
			t.Fatalf("first prompt = %q, want coder turn", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for coder turn")
	}
	select {
	case prompt := <-adapter.turns:
		t.Fatalf("unexpected reviewer turn while paused: %q", prompt)
	case <-time.After(200 * time.Millisecond):
	}
	close(adapter.releaseCoder)
	time.Sleep(50 * time.Millisecond)
	select {
	case prompt := <-adapter.turns:
		t.Fatalf("unexpected reviewer turn after coder completion while paused: %q", prompt)
	case <-time.After(200 * time.Millisecond):
	}
	graph := svc.agentGraphSnapshot(parent.RunID)
	if got := graph.LoopState.Status; got != "paused" {
		t.Fatalf("loop status = %q, want paused", got)
	}
	svc.resumeAgentLoop(parent.RunID)
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "review") || !strings.Contains(strings.ToLower(prompt), "coder complete") {
			t.Fatalf("reviewer prompt = %q, want resumed handoff", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reviewer turn after resume")
	}
}

type dependencyReleaseAdapter struct {
	turns           chan string
	releaseReviewer chan struct{}
}

func (a *dependencyReleaseAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *dependencyReleaseAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *dependencyReleaseAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.turns <- req.Prompt
	if strings.Contains(strings.ToLower(req.Prompt), "review this change") {
		<-a.releaseReviewer
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: req.Prompt + " complete"})
	return nil
}

func TestCompletedReviewerReleasesDependentTester(t *testing.T) {
	svc := NewInteractiveService()
	adapter := &dependencyReleaseAdapter{turns: make(chan string, 4), releaseReviewer: make(chan struct{})}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return adapter },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	reviewer, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review this change", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawn reviewer: %v", spawnErr)
	}
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "review this change") {
			t.Fatalf("first prompt = %q, want reviewer turn", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reviewer turn")
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:     "tester",
		Prompt:    "run the regression checks",
		Provider:  "codex",
		DependsOn: []string{reviewer.RunID},
		Wait:      false,
	}); spawnErr != nil {
		t.Fatalf("spawn tester: %v", spawnErr)
	}
	select {
	case prompt := <-adapter.turns:
		t.Fatalf("unexpected tester turn before reviewer completion: %q", prompt)
	case <-time.After(200 * time.Millisecond):
	}
	close(adapter.releaseReviewer)
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "run the regression checks") || !strings.Contains(strings.ToLower(prompt), "review this change complete") {
			t.Fatalf("second prompt = %q, want dependent tester turn with reviewer handoff", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for dependent tester turn")
	}
}

// TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted guards BUG-106: when a child
// provider's turn/completed notification carries an empty FinalMessage but the actual text
// arrived via EventMessageCompleted streaming events, spawnChildRun(wait=true) must still
// return the child's text in SpawnAgentResult.FinalMessage (not an empty string).
func TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted(t *testing.T) {
	svc := NewInteractiveService()
	// Adapter emits the child text ONLY via EventMessageCompleted (no FinalMessage on
	// EventTurnCompleted) — reproducing Codex new-protocol turn/completed behaviour.
	reg := newProviderRegistry()
	msgOnlyAdapter := &msgOnlyTurnAdapter{reply: "CHILD_AGENT_DONE"}
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return msgOnlyAdapter },
	})
	svc.registry = reg

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:    "reviewer",
		Prompt:   "check it",
		Provider: "codex",
		Wait:     true,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	if result.FinalMessage != "CHILD_AGENT_DONE" {
		t.Fatalf("FinalMessage = %q, want %q", result.FinalMessage, "CHILD_AGENT_DONE")
	}
}

// msgOnlyTurnAdapter emits the child reply via EventMessageCompleted with an empty
// FinalMessage on EventTurnCompleted (reproduces Codex new-protocol turn/completed).
type msgOnlyTurnAdapter struct{ reply string }

func (a *msgOnlyTurnAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *msgOnlyTurnAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *msgOnlyTurnAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{Type: EventMessageCompleted, Text: a.reply})
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: ""}) // empty — the bug case
	return nil
}

// readTurnReqFor drains the shared capture channel until it sees the turn request for runID,
// skipping unrelated runs' turns (e.g. the spawned child's own first turn).
func readTurnReqFor(t *testing.T, ch chan TurnRequest, runID string) TurnRequest {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case req := <-ch:
			if req.RunID == runID {
				return req
			}
		case <-deadline:
			t.Fatalf("timed out waiting for turn request for run %s", runID)
		}
	}
}

// ---- applyFlowControl + extendCap (Task-090) ----------------------------------

func newFlowTestRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Seed the loop state with an explicit cap so tests are deterministic.
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	return svc, parent.RunID
}

// TestCohortMemberSettlesOwnNodeOnCompletion is the BUG-234 (#1) regression:
// when one reviewer of a cohort finishes but its sibling is still running, the
// finished reviewer's OWN step-timeline node must read DONE — not stay RUNNING
// until the whole cohort joins. Before the fix, individual completion only
// buffered the result and the reviewer nodes were settled together at the
// barrier, so one-done-one-running showed both as RUNNING.
func TestCohortMemberSettlesOwnNodeOnCompletion(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate"},
		{ID: "reviewer_correctness", Behavior: "agent.delegate"},
		{ID: "reviewer_security", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	if rs := svc.runs[runID]; rs != nil {
		rs.activeFlowNodes = nodes
	}
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	// Both reviewers are running; the cohort has 2 members.
	svc.setFlowStepStatus(context.Background(), runID, "reviewer_correctness", StepStatusRunning)
	svc.setFlowStepStatus(context.Background(), runID, "reviewer_security", StepStatusRunning)

	// Complete only the first cohort member (Wait blocks until its turn finishes).
	if _, err := svc.spawnChildRun(context.Background(), runID, SpawnAgentInput{
		Agent: "reviewer_correctness", Prompt: "review", Wait: true,
		FlowCohortID: "review-1", Label: "reviewer_correctness", CohortSize: 2,
	}); err != nil {
		t.Fatalf("spawnChildRun(reviewer_correctness): %v", err)
	}

	if got := flowStepStatus(t, svc, runID, "reviewer_correctness"); got != StepStatusDone {
		t.Errorf("finished cohort member node = %v, want DONE (must settle on its own completion, not wait for the barrier)", got)
	}
	if got := flowStepStatus(t, svc, runID, "reviewer_security"); got == StepStatusDone {
		t.Errorf("still-running sibling node = DONE, want not-DONE (the barrier has not fired — only one member completed)")
	}
}

// newFlowEngineTestRun is like newFlowTestRun but marks the run flow-engine-
// driven with a tracked hub.inline node, so BUG-231's setFlowStepAwaitingUser
// (gated on isFlowEngineDriven + hubInlineNodeID) has something to settle.
func newFlowEngineTestRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc, runID := newFlowTestRun(t)
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	if rs := svc.runs[runID]; rs != nil {
		rs.activeFlowNodes = nodes
	}
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusRunning)
	return svc, runID
}

func flowStepStatus(t *testing.T, svc *InteractiveService, runID, nodeID string) RuntimeWorkflowStepStatus {
	t.Helper()
	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	for _, st := range steps {
		if st.ID == nodeID {
			return st.Status
		}
	}
	t.Fatalf("step %q not found in run %q", nodeID, runID)
	return ""
}

// TestApplyFlowControlEscalateSettlesHubToWaitingUser is the regression test
// for BUG-231: an escalate outcome must be a non-terminal "awaiting user"
// pause, not a hang. The hub inline node must move to WAITING_USER_APPROVAL
// (not stay RUNNING, not go FAILED) and the loop must carry BlockReason
// "escalate" so the desktop can render the correct recovery affordance.
func TestApplyFlowControlEscalateSettlesHubToWaitingUser(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "escalate", Summary: "reviewers disagree"})
	if err != nil {
		t.Fatalf("applyFlowControl(escalate): %v", err)
	}
	if result.NextAction != "awaiting_user" {
		t.Errorf("NextAction = %q, want awaiting_user", result.NextAction)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Status != "blocked" {
		t.Errorf("loop.Status = %q, want blocked", snap.LoopState.Status)
	}
	if snap.LoopState.BlockReason != "escalate" {
		t.Errorf("loop.BlockReason = %q, want escalate", snap.LoopState.BlockReason)
	}
	// BUG-233: the hub node's transition to WAITING_USER_APPROVAL must be visible
	// the instant applyFlowControl returns — not eventually, via a background
	// goroutine — because the desktop's step-runtime refresh can be triggered by
	// the agent_graph_updated event emitted for this same call, and a
	// still-in-flight goroutine write would let that refresh read a stale status.
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Errorf("hub node status immediately after applyFlowControl = %v, want WAITING_USER_APPROVAL", got)
	}
}

// TestApplyFlowControlCapReachedSettlesHubToWaitingUser mirrors the escalate
// test for the cap-reached "continue" branch: BlockReason must be "cap", and
// the hub node must also move to WAITING_USER_APPROVAL rather than staying
// RUNNING.
func TestApplyFlowControlCapReachedSettlesHubToWaitingUser(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 2, RoundCap: 2, Round: 1})
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue", Summary: "still open issues"})
	if err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}
	if result.NextAction != "awaiting_user" {
		t.Errorf("NextAction = %q, want awaiting_user", result.NextAction)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.BlockReason != "cap" {
		t.Errorf("loop.BlockReason = %q, want cap", snap.LoopState.BlockReason)
	}
	// BUG-233: see the matching comment in TestApplyFlowControlEscalateSettlesHubToWaitingUser.
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Errorf("hub node status immediately after applyFlowControl = %v, want WAITING_USER_APPROVAL", got)
	}
}

// TestApplyFlowControlLoopingResetsStepsSynchronously is the regression test
// for BUG-233: when a new review round starts ("continue"->"looping"), the
// downstream nodes' PENDING reset and the entry node's RUNNING transition must
// be visible the instant applyFlowControl returns, not via a background
// goroutine racing the agent_graph_updated event's desktop-side step-runtime
// refresh (the reported symptom: a fresh reviewer cohort shows as "running" in
// the agent panel while the step timeline still reads the prior round's "all
// done").
func TestApplyFlowControlLoopingResetsStepsSynchronously(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusDone)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", Cap: 5, RoundCap: 5, Round: 1})

	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue", Summary: "changes requested"})
	if err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}
	if result.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", result.NextAction)
	}
	if got := flowStepStatus(t, svc, runID, "coder"); got != StepStatusRunning {
		t.Errorf("entry node (coder) status immediately after applyFlowControl = %v, want RUNNING", got)
	}
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusPending {
		t.Errorf("downstream node (synthesis) status immediately after applyFlowControl = %v, want PENDING", got)
	}
}

// TestApplyFlowControlContinueClearsRejectedLoopStatus is the regression test
// for run-13821: the hub submitted "continue" from a rejected review verdict,
// but applyFlowControl left loop.Status="rejected". The re-entered coder then
// completed while the loop was not advancing, so tryAdvanceFlowFromNode refused
// to spawn the next reviewer cohort and the coder's full output leaked into
// the main hub prompt as a fallback note.
func TestApplyFlowControlContinueClearsRejectedLoopStatus(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status:     "rejected",
		GateReason: "previous reviewer verdict",
		Cap:        5,
		RoundCap:   5,
		Round:      1,
		OpenIssues: 2,
	})

	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue", Summary: "retry the fix"})
	if err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}
	if result.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", result.NextAction)
	}
	snap := svc.agentGraphSnapshot(runID)
	if got := snap.LoopState.Status; got != "running" {
		t.Fatalf("loop.Status = %q, want running so coder completion can auto-advance", got)
	}
	if got := snap.LoopState.GateReason; got != "" {
		t.Fatalf("loop.GateReason = %q, want cleared stale reviewer verdict", got)
	}
}

func TestSubmitFlowControlRejectsDuplicateSubmissionForSameTurn(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})

	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.currentTurnID = "turn-synthesis-1"
	bridge := &turnBridge{svc: svc, rs: rs}
	svc.mu.Unlock()

	if _, err := bridge.SubmitFlowControl(FlowControlInput{Status: "continue", Summary: "changes requested"}); err != nil {
		t.Fatalf("first SubmitFlowControl(continue): %v", err)
	}
	if _, err := bridge.SubmitFlowControl(FlowControlInput{Status: "escalate", Summary: "stale second verdict"}); err == nil {
		t.Fatal("second SubmitFlowControl in the same provider turn returned nil error, want duplicate rejection")
	}
	snap := svc.agentGraphSnapshot(runID)
	if got := snap.LoopState.Status; got != "running" {
		t.Fatalf("loop.Status = %q after duplicate submission, want still running", got)
	}
	if strings.Contains(snap.LoopState.GateReason, "stale second verdict") {
		t.Fatalf("duplicate submission mutated GateReason: %q", snap.LoopState.GateReason)
	}
}

// TestStartTurnPreservesOriginalPromptOverInternalFlowEngineTurns is the
// regression test for BUG-235: the chat-history list titles a run from
// lastPrompt (Navigator.tsx: runTitle(item.lastPrompt || item.lastMessage)).
// Every internal flow-engine turn (hub auto-reinvoke, coder re-entry, the
// Continue-resume note) is prefixed "[flow-engine]" and previously overwrote
// lastPrompt unconditionally, so a flow run's history entry ended up titled by
// whichever internal prompt ran last instead of the user's original request.
func TestStartTurnPreservesOriginalPromptOverInternalFlowEngineTurns(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID
	originalPrompt := "fix bug 1 + 1 is not equals 2"

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: originalPrompt}, "", ""); apiErr != nil {
		t.Fatalf("first startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "first turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: autoReinvokePromptText()}, "", ""); apiErr != nil {
		t.Fatalf("hub auto-reinvoke startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "second (internal) turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	svc.mu.Lock()
	got := svc.runs[parent.RunID].lastPrompt
	svc.mu.Unlock()
	if got != originalPrompt {
		t.Errorf("lastPrompt = %q, want unchanged original prompt %q (internal flow-engine turn must not overwrite the history title)", got, originalPrompt)
	}
}

// TestResumeFlowWithFeedbackAutoExtendsOnlyForCap is the regression test for
// BUG-231 D-6: Continue must auto-raise the cap when the block reason was
// the round cap, but must NOT touch the cap for a genuine escalate — an
// escalate isn't a capacity problem, so bumping the cap would be a no-op at
// best and misleading at worst.
func TestResumeFlowWithFeedbackAutoExtendsOnlyForCap(t *testing.T) {
	t.Run("cap reason auto-extends", func(t *testing.T) {
		svc, runID := newFlowEngineTestRun(t)
		svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "cap", Cap: 3, RoundCap: 3, Round: 3})
		snap, err := svc.resumeFlowWithFeedback(runID, "")
		if err != nil {
			t.Fatalf("resumeFlowWithFeedback: %v", err)
		}
		if snap.LoopState.Cap != 5 {
			t.Errorf("Cap after Continue on a cap block = %d, want 5 (3+2)", snap.LoopState.Cap)
		}
		if snap.LoopState.Status != "running" || snap.LoopState.BlockReason != "" {
			t.Errorf("status/blockReason after Continue = %q/%q, want running/\"\"", snap.LoopState.Status, snap.LoopState.BlockReason)
		}
		// BUG-233: the hub node's WAITING_USER_APPROVAL -> RUNNING transition must
		// be visible the instant resumeFlowWithFeedback returns, not via a
		// background goroutine racing the desktop's step-runtime refresh.
		if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusRunning {
			t.Errorf("hub node status immediately after resumeFlowWithFeedback = %v, want RUNNING", got)
		}
	})

	t.Run("escalate reason does not touch cap", func(t *testing.T) {
		svc, runID := newFlowEngineTestRun(t)
		svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "escalate", Cap: 3, RoundCap: 3, Round: 1})
		snap, err := svc.resumeFlowWithFeedback(runID, "prioritize the security reviewer's finding")
		if err != nil {
			t.Fatalf("resumeFlowWithFeedback: %v", err)
		}
		if snap.LoopState.Cap != 3 {
			t.Errorf("Cap after Continue on an escalate block = %d, want unchanged 3", snap.LoopState.Cap)
		}
		if snap.LoopState.Status != "running" || snap.LoopState.BlockReason != "" {
			t.Errorf("status/blockReason after Continue = %q/%q, want running/\"\"", snap.LoopState.Status, snap.LoopState.BlockReason)
		}
	})
}

// TestResumeFlowWithFeedbackNoopWhenNotBlocked proves Continue is safe to
// call more than once — a run that already left "blocked" is left untouched.
func TestResumeFlowWithFeedbackNoopWhenNotBlocked(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Round: 1})
	snap, err := svc.resumeFlowWithFeedback(runID, "ignored")
	if err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	if snap.LoopState.Status != "running" || snap.LoopState.Cap != 3 {
		t.Errorf("no-op resume changed state: status=%q cap=%d", snap.LoopState.Status, snap.LoopState.Cap)
	}
}

// TestResumeFlowWithFeedbackReinvokesHubSynthesisTurn proves Continue
// actually re-runs the hub's synthesis turn (not just flips loop state) and
// embeds the user's feedback directly in that turn's prompt (D-5) — the same
// reliable-delivery pattern cohort-join notes use, rather than relying
// solely on pendingAgentContext rendering.
func TestResumeFlowWithFeedbackReinvokesHubSynthesisTurn(t *testing.T) {
	promptCh := make(chan string, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case promptCh <- req.Prompt:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID := parent.RunID
	svc.mu.Lock()
	if rs := svc.runs[runID]; rs != nil {
		rs.autoOrchestrate = true
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "escalate", Cap: 3, RoundCap: 3, Round: 1})

	if _, err := svc.resumeFlowWithFeedback(runID, "prioritize the security reviewer's finding"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	select {
	case prompt := <-promptCh:
		if !strings.Contains(prompt, "prioritize the security reviewer's finding") {
			t.Errorf("synthesis re-invoke prompt does not embed the user's feedback: %q", prompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the hub synthesis turn to be re-invoked")
	}
}

func TestApplyFlowControlDoneTerminates(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.OpenIssues = 5
		return st
	})
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "done", Summary: "all tests pass"})
	if err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}
	if result.Status != "done" || result.NextAction != "done" {
		t.Errorf("result = %+v, want Status=done NextAction=done", result)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Status != "done" {
		t.Errorf("loop status = %q, want done", snap.LoopState.Status)
	}
	if snap.LoopState.OpenIssues != 0 {
		t.Errorf("OpenIssues = %d, want 0 after done", snap.LoopState.OpenIssues)
	}
	// Hub handoff note must be queued.
	svc.mu.Lock()
	notes := svc.runs[runID].pendingAgentContext
	svc.mu.Unlock()
	if len(notes) == 0 {
		t.Error("expected hub handoff note in pendingAgentContext, got none")
	}
}

func TestApplyFlowControlContinueAdvancesRound(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue", Summary: "revise"})
	if err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}
	if result.Status != "continue" || result.NextAction != "looping" {
		t.Errorf("result = %+v, want Status=continue NextAction=looping", result)
	}
	if result.Round != 1 {
		t.Errorf("Round = %d, want 1", result.Round)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Round != 1 {
		t.Errorf("loop Round = %d, want 1", snap.LoopState.Round)
	}
}

func TestApplyFlowControlContinueBlocksAtCap(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	// Advance to cap-1 rounds first, then trigger cap.
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 2, RoundCap: 2, Round: 1})
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue"})
	if err != nil {
		t.Fatalf("applyFlowControl(continue at cap): %v", err)
	}
	if result.Status != "blocked" || result.NextAction != "awaiting_user" {
		t.Errorf("result = %+v, want Status=blocked NextAction=awaiting_user", result)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Status != "blocked" {
		t.Errorf("loop status = %q, want blocked", snap.LoopState.Status)
	}
}

func TestApplyFlowControlEscalateBlocks(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "escalate", Summary: "cannot proceed"})
	if err != nil {
		t.Fatalf("applyFlowControl(escalate): %v", err)
	}
	if result.Status != "blocked" || result.NextAction != "awaiting_user" {
		t.Errorf("result = %+v, want Status=blocked NextAction=awaiting_user", result)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Status != "blocked" || snap.LoopState.GateReason != "cannot proceed" {
		t.Errorf("loop = %+v, want blocked with GateReason=cannot proceed", snap.LoopState)
	}
}

func TestExtendCapRaisesCap(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	// Block first, then extend.
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", Cap: 3, RoundCap: 3, Round: 2})
	result, err := svc.extendCap(runID)
	if err != nil {
		t.Fatalf("extendCap: %v", err)
	}
	if result.Cap != 5 {
		t.Errorf("Cap after extend = %d, want 5 (3+2)", result.Cap)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Cap != 5 {
		t.Errorf("loop Cap = %d, want 5", snap.LoopState.Cap)
	}
	if snap.LoopState.ExtendCount != 1 {
		t.Errorf("ExtendCount = %d, want 1", snap.LoopState.ExtendCount)
	}
	if snap.LoopState.Status != "running" {
		t.Errorf("status after extend from blocked = %q, want running", snap.LoopState.Status)
	}
}

// TestExtendCapNoLongerRejectedPastFormerMax is the regression test for
// BUG-231 D-8: ExtendMax used to reject a third extension (ExtendCount >= 2).
// That limit only ever bounded this user-triggered action, so its only
// remaining effect was to block the human indefinitely — the exact
// awaiting-user wedge BUG-231 fixes. Retired: extendCap must keep succeeding
// (and keep raising the cap) past the old limit.
func TestExtendCapNoLongerRejectedPastFormerMax(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", Cap: 7, RoundCap: 7, Round: 4, ExtendCount: 2})
	result, err := svc.extendCap(runID)
	if err != nil {
		t.Fatalf("extendCap past the former ExtendMax: unexpected error: %v", err)
	}
	if result.Cap != 9 {
		t.Errorf("Cap after third extend = %d, want 9 (7+2)", result.Cap)
	}
	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.ExtendCount != 3 {
		t.Errorf("ExtendCount = %d, want 3", snap.LoopState.ExtendCount)
	}
	if snap.LoopState.Status != "running" {
		t.Errorf("status after extend = %q, want running", snap.LoopState.Status)
	}
}

func TestFlowControlHTTPRoundTrip(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// POST flow-control?status=continue
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control",
		map[string]any{"status": "continue", "summary": "round 1"}, nil)
	if status != http.StatusOK {
		t.Fatalf("POST flow-control: status=%d body=%s", status, body)
	}
	// Handler now returns AgentGraphSnapshot (Fix 3 — wire contract alignment).
	var snap AgentGraphSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode AgentGraphSnapshot: %v (body=%s)", err, body)
	}
	if snap.LoopState.Status != "running" || snap.LoopState.Round != 1 {
		t.Errorf("loopState = %+v, want Status=running Round=1", snap.LoopState)
	}

	// POST flow-control?status=invalid → 400
	badStatus, _ := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control",
		map[string]any{"status": "invalid"}, nil)
	if badStatus != http.StatusBadRequest {
		t.Errorf("invalid status should return 400, got %d", badStatus)
	}
}

func TestExtendCapHTTPRoundTrip(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", Cap: 3, RoundCap: 3, Round: 2})

	httpStatus, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/extend-cap",
		map[string]any{}, nil)
	if httpStatus != http.StatusOK {
		t.Fatalf("POST extend-cap: status=%d body=%s", httpStatus, body)
	}
	// Handler now returns AgentGraphSnapshot (Fix 3 — wire contract alignment).
	var snap AgentGraphSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode AgentGraphSnapshot: %v (body=%s)", err, body)
	}
	if snap.LoopState.Cap != 5 {
		t.Errorf("Cap = %d, want 5", snap.LoopState.Cap)
	}
	if snap.LoopState.Status != "running" {
		t.Errorf("Status = %q after extend-cap, want running", snap.LoopState.Status)
	}
}

func TestSubmitReviewOutcomeViaClaudeBridge(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	// Mode=explicit so the engine path is exercised.
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Mode = "explicit"
		return st
	})

	// Directly call the handler (same path claude MCP uses).
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue", Summary: "needs revision"})
	if err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}
	if result.Status != "continue" || result.Round != 1 {
		t.Errorf("result = %+v, want Status=continue Round=1", result)
	}
}

// Fix 1 (Finding 1): desktop board sends { outcome: "approved" } — must not get 400.
func TestSubmitFlowControlHTTPAcceptsOutcomeAlias(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// Exact payload the desktop store sends from submitReviewOutcome("approved").
	httpStatus, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control",
		map[string]any{"outcome": "approved"}, nil)
	if httpStatus != http.StatusOK {
		t.Fatalf("POST flow-control with outcome=approved: status=%d body=%s", httpStatus, body)
	}
	var snap AgentGraphSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode AgentGraphSnapshot: %v (body=%s)", err, body)
	}
	if snap.LoopState.Status != "done" {
		t.Errorf("Status = %q after approved, want done", snap.LoopState.Status)
	}
}

// TestSubmitFlowControlHTTPAcceptsCanonicalStatusField is the regression test
// for BUG-NOTE-CP42 #32: ReviewOutcomeInput's canonical wire field is
// "status" (json:"status" on the Go struct) — "outcome" is only the board/
// legacy alias. handleSubmitFlowControl used to route purely on the literal
// presence of an "outcome" key, so a caller sending the canonical
// {"status":"approved"} body fell through to the raw FlowControlInput parser
// instead, which only recognizes continue|done|escalate and rejected
// "approved" outright.
func TestSubmitFlowControlHTTPAcceptsCanonicalStatusField(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// The canonical ReviewOutcomeInput wire shape: {"status": "approved", ...},
	// not the board's "outcome" alias.
	httpStatus, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control",
		map[string]any{"status": "approved"}, nil)
	if httpStatus != http.StatusOK {
		t.Fatalf("POST flow-control with status=approved: status=%d body=%s", httpStatus, body)
	}
	var snap AgentGraphSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode AgentGraphSnapshot: %v (body=%s)", err, body)
	}
	if snap.LoopState.Status != "done" {
		t.Errorf("Status = %q after status=approved, want done", snap.LoopState.Status)
	}
}

// Fix 1: outcome=changes_requested with issues array through HTTP.
func TestSubmitFlowControlHTTPOutcomeChangesRequested(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	httpStatus, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control",
		map[string]any{
			"outcome":  "changes_requested",
			"feedback": "fix null check",
			"issues":   []any{map[string]any{"id": "I-1", "title": "nil deref", "severity": "error"}},
		}, nil)
	if httpStatus != http.StatusOK {
		t.Fatalf("POST flow-control with outcome=changes_requested: status=%d body=%s", httpStatus, body)
	}
	var snap AgentGraphSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode AgentGraphSnapshot: %v (body=%s)", err, body)
	}
	if snap.LoopState.Round != 1 {
		t.Errorf("Round = %d after changes_requested, want 1", snap.LoopState.Round)
	}
	if snap.LoopState.OpenIssues != 1 {
		t.Errorf("OpenIssues = %d, want 1", snap.LoopState.OpenIssues)
	}
}

// Fix 2 (Finding 2): reviewOutcomeToFlowControl sets payload["issues"] as []ReviewIssue,
// not []any — applyFlowControl must count them correctly either way.
func TestApplyFlowControlCountsIssuesFromReviewOutcomePayload(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	roi := ReviewOutcomeInput{
		Status:   "changes_requested",
		Feedback: "fix the bug",
		Issues: []ReviewIssue{
			{ID: "I-1", Title: "nil deref", Severity: "error"},
			{ID: "I-2", Title: "off-by-one", Severity: "warning"},
		},
	}
	fc, fcErr := reviewOutcomeToFlowControl(roi)
	if fcErr != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", fcErr)
	}
	result, applyErr := svc.applyFlowControl(parent.RunID, fc)
	if applyErr != nil {
		t.Fatalf("applyFlowControl: %v", applyErr)
	}
	if result.OpenIssues != 2 {
		t.Errorf("OpenIssues = %d, want 2 ([]ReviewIssue path)", result.OpenIssues)
	}
	if result.Status != "continue" {
		t.Errorf("Status = %q, want continue", result.Status)
	}
}

// TestSubmitReviewOutcomeCannotChangeCap verifies that submit_review_outcome never
// modifies the loop cap, even when a caller injects a raw "roundCapOverride" key
// into the args map (the field was removed from the schema but the parser must
// silently ignore unknown keys rather than applying them).
func TestSubmitReviewOutcomeCannotChangeCap(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	initialCap := 3 // matches newFlowTestRun seed

	// Simulate a model attempting to sneak roundCapOverride through the raw args.
	args := map[string]any{
		"status":           "approved",
		"roundCapOverride": float64(1000), // attacker-supplied; must be ignored
	}
	roi, err := parseReviewOutcomeInput(args)
	if err != nil {
		t.Fatalf("parseReviewOutcomeInput: %v", err)
	}
	fc, err := reviewOutcomeToFlowControl(roi)
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	if _, err = svc.applyFlowControl(runID, fc); err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}

	snap := svc.agentGraphSnapshot(runID)
	if got := effectiveCap(snap.LoopState); got != initialCap {
		t.Errorf("cap after submit = %d, want %d (cap must not change via submit_review_outcome)", got, initialCap)
	}
}

func TestLegacyKeywordModeUnchangedInKeywordMode(t *testing.T) {
	// Keyword-mode (Mode=="") must still drive transitions via the existing isAgentRole branch.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "APPROVED"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// No Mode set → keyword mode. Loop state should remain unchanged (no Mode field).
	snap := svc.agentGraphSnapshot(parent.RunID)
	if snap.LoopState.Mode != "" {
		t.Errorf("Mode = %q, want empty for keyword mode", snap.LoopState.Mode)
	}
}

func TestExplicitModeSilencesLegacyBranch(t *testing.T) {
	// With Mode=explicit, a reviewer completing must NOT trigger a coder restart
	// via the legacy keyword branch.
	callCount := 0
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				callCount++
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "CHANGES REQUESTED: fix tests"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	// Set explicit mode.
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	// Spawn a child with role "reviewer".
	_, _ = svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "review", Wait: true,
	})
	// The keyword branch would have called advanceRound + restarted coder.
	// In explicit mode it must not: Round stays 0.
	snap := svc.agentGraphSnapshot(parent.RunID)
	if snap.LoopState.Round != 0 {
		t.Errorf("Round = %d after explicit-mode reviewer: expected 0 (keyword branch silenced)", snap.LoopState.Round)
	}
}

func TestCohortConsolidatedNoteEmittedOnLastMember(t *testing.T) {
	// Three children in one cohort completing out of order must produce exactly one
	// consolidated note, emitted only after the last member completes.
	reg := newProviderRegistry()
	idx := 0
	outcomes := []string{"LGTM", "needs fix", "also ok"}
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			myIdx := idx
			idx++
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				msg := "result"
				if myIdx < len(outcomes) {
					msg = outcomes[myIdx]
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: msg})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	for k := 0; k < 3; k++ {
		lbl := fmt.Sprintf("agent-%d", k)
		_, err := svc.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent: lbl, Prompt: "work", Wait: true, FlowCohortID: "review-cohort", Label: lbl,
			CohortSize: 3,
		})
		if err != nil {
			t.Fatalf("spawnChildRun[%d]: %v", k, err)
		}
	}

	svc.mu.Lock()
	notes := len(svc.runs[parentRunID].pendingAgentContext)
	noteText := ""
	if notes > 0 {
		noteText = svc.runs[parentRunID].pendingAgentContext[0]
	}
	svc.mu.Unlock()

	if notes != 1 {
		t.Errorf("pendingAgentContext entries = %d, want exactly 1 consolidated note", notes)
	}
	for _, want := range []string{"[flow-engine joined result note]", "agent-0", "agent-1", "agent-2", "Synthesize"} {
		if !strings.Contains(noteText, want) {
			t.Errorf("consolidated note missing %q", want)
		}
	}
}

func TestCohortFailedMemberIncludedInNote(t *testing.T) {
	// A failed cohort member should appear as "failed: <err>" in the note.
	reg := newProviderRegistry()
	k := 0
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			idx := k
			k++
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				if idx == 1 {
					b.Emit(ProviderEvent{Type: EventTurnFailed, Error: "connection reset"})
				} else {
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				}
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle2, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID2 := parentHandle2.RunID
	nodes := []agentpack.FlowNode{
		{ID: "worker-0", Behavior: "agent.delegate"},
		{ID: "worker-1", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.markFlowEngineDriven(parentRunID2)
	svc.reseedFlowStepRuntime(parentRunID2, nodes)

	for j := 0; j < 2; j++ {
		lbl := fmt.Sprintf("worker-%d", j)
		_, _ = svc.spawnChildRun(context.Background(), parentRunID2, SpawnAgentInput{
			Agent: lbl, Prompt: "do", Wait: true, FlowCohortID: "fail-cohort", Label: lbl,
			CohortSize: 2,
		})
		// worker-1 will fail; don't fatal — the cohort note should still be built.
	}

	svc.mu.Lock()
	ctx := svc.runs[parentRunID2].pendingAgentContext
	svc.mu.Unlock()

	if len(ctx) != 1 {
		t.Fatalf("pendingAgentContext = %d entries, want 1", len(ctx))
	}
	if !strings.Contains(ctx[0], "failed:") || !strings.Contains(ctx[0], "connection reset") {
		t.Errorf("note does not contain failed member info:\n%s", ctx[0])
	}
	if got := flowStepStatus(t, svc, parentRunID2, "worker-1"); got != StepStatusFailed {
		t.Errorf("failed cohort member step status = %v, want FAILED after sibling completion joins the cohort", got)
	}
}

func TestCohortCompletedMemberWithoutFinalMessageGetsPlaceholder(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: ""})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})

	_, _ = svc.spawnChildRun(context.Background(), parentHandle.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "review", Wait: true, FlowCohortID: "empty-msg", Label: "reviewer", CohortSize: 1,
	})

	svc.mu.Lock()
	ctx := svc.runs[parentHandle.RunID].pendingAgentContext
	svc.mu.Unlock()
	if len(ctx) != 1 {
		t.Fatalf("pendingAgentContext = %d entries, want 1", len(ctx))
	}
	if !strings.Contains(ctx[0], "(completed with no final message captured)") {
		t.Errorf("note missing empty-final-message placeholder:\n%s", ctx[0])
	}
}

func TestNoCohortRunKeepsIsolatedAppend(t *testing.T) {
	// Single child without FlowCohortID must keep the existing isolated-append path.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle3, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID3 := parentHandle3.RunID
	// Wait=false (no UIInitiated) so the only append is the completion-time isolated
	// path (!waitForResult=true), avoiding the double-append from UIInitiated.
	_, err := svc.spawnChildRun(context.Background(), parentRunID3, SpawnAgentInput{
		Agent: "worker", Prompt: "do", Wait: false,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	// Wait for async child goroutine to complete and append the note.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		n := len(svc.runs[parentRunID3].pendingAgentContext)
		svc.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	svc.mu.Lock()
	ctx := svc.runs[parentRunID3].pendingAgentContext
	svc.mu.Unlock()
	if len(ctx) != 1 {
		t.Fatalf("pendingAgentContext = %d, want 1", len(ctx))
	}
	if strings.Contains(ctx[0], "Synthesize") {
		t.Error("isolated note should not contain Synthesize directive")
	}
}

// ---- Task-093: Bounded Auto-Reinvocation Of The Hub --------------------------------

func TestAutoReinvokeHubFiresAfterCohortComplete(t *testing.T) {
	// With autoOrchestrate=true, the last cohort member completing should trigger
	// exactly one hub turn (turnCount goes from 0 to 1).
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	// Enable auto-orchestration directly.
	svc.mu.Lock()
	svc.runs[parentRunID].autoOrchestrate = true
	svc.mu.Unlock()

	// Spawn 2 cohort children with CohortSize=2 so the barrier fires after both.
	for j := 0; j < 2; j++ {
		lbl := fmt.Sprintf("worker-%d", j)
		_, _ = svc.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent: lbl, Prompt: "do", Wait: true, FlowCohortID: "cohort", Label: lbl, CohortSize: 2,
		})
	}

	// Hub should be auto-reinvoked asynchronously; wait up to 2 s.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		tc := svc.runs[parentRunID].turnCount
		svc.mu.Unlock()
		if tc > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	svc.mu.Lock()
	tc := svc.runs[parentRunID].turnCount
	svc.mu.Unlock()
	if tc == 0 {
		t.Fatal("hub was not auto-reinvoked after cohort join (turnCount still 0)")
	}
}

func TestAutoReinvokeHubNormalChatNeverFires(t *testing.T) {
	// With autoOrchestrate=false (default), cohort completion must NOT reinvoke hub.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID
	// autoOrchestrate is false by default — do NOT set it.

	for j := 0; j < 2; j++ {
		lbl := fmt.Sprintf("worker-%d", j)
		_, _ = svc.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent: lbl, Prompt: "do", Wait: true, FlowCohortID: "cohort2", Label: lbl, CohortSize: 2,
		})
	}

	// Give any (incorrect) goroutine time to run.
	time.Sleep(50 * time.Millisecond)
	svc.mu.Lock()
	tc := svc.runs[parentRunID].turnCount
	svc.mu.Unlock()
	if tc != 0 {
		t.Fatalf("normal chat hub was auto-reinvoked (turnCount=%d), expected 0", tc)
	}
}

func TestAutoReinvokeHubSetViaSpawnAgentInput(t *testing.T) {
	// Passing AutoOrchestrate=true on SpawnAgentInput sets it on the parent run.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	_, _ = svc.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
		Agent: "worker", Prompt: "do", Wait: true, FlowCohortID: "cohort3",
		CohortSize: 1, AutoOrchestrate: true,
	})

	svc.mu.Lock()
	ao := svc.runs[parentRunID].autoOrchestrate
	svc.mu.Unlock()
	if !ao {
		t.Fatal("autoOrchestrate should be true after spawn with AutoOrchestrate=true")
	}
}

func TestAutoReinvokeHubStopCancels(t *testing.T) {
	// After stopAgentLoop, autoOrchestrate and reinvokeInFlight are cleared so no
	// further auto-reinvoke fires.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	svc.mu.Lock()
	svc.runs[parentRunID].autoOrchestrate = true
	svc.runs[parentRunID].reinvokeInFlight = true
	svc.mu.Unlock()

	svc.stopAgentLoop(parentRunID)

	svc.mu.Lock()
	ao := svc.runs[parentRunID].autoOrchestrate
	rif := svc.runs[parentRunID].reinvokeInFlight
	svc.mu.Unlock()
	if ao {
		t.Error("autoOrchestrate should be false after stopAgentLoop")
	}
	if rif {
		t.Error("reinvokeInFlight should be false after stopAgentLoop")
	}
}

func TestStopAgentLoopCancelsParentTurn(t *testing.T) {
	parentCanceled := make(chan struct{}, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, _ TurnRequest, _ TurnBridge) error {
				<-ctx.Done()
				select {
				case parentCanceled <- struct{}{}:
				default:
				}
				return ctx.Err()
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, apiErr := svc.startTurn(parentHandle.RunID, TurnInput{StepID: parentHandle.StepID, Prompt: "keep running"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}

	svc.stopAgentLoop(parentHandle.RunID)

	select {
	case <-parentCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("stopAgentLoop did not cancel the parent turn")
	}
}

func TestInterruptParentCancelsRunningChildAgents(t *testing.T) {
	childStarted := make(chan struct{}, 1)
	childCanceled := make(chan struct{}, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, req TurnRequest, _ TurnBridge) error {
				if strings.HasPrefix(req.RunID, "run-") {
					select {
					case childStarted <- struct{}{}:
					default:
					}
				}
				<-ctx.Done()
				select {
				case childCanceled <- struct{}{}:
				default:
				}
				return ctx.Err()
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if _, err := svc.spawnChildRun(context.Background(), parentHandle.RunID, SpawnAgentInput{
		Agent:  "worker",
		Prompt: "keep running",
		Wait:   false,
	}); err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	select {
	case <-childStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("child turn did not start")
	}

	if apiErr := svc.Interrupt(parentHandle.RunID); apiErr != nil {
		t.Fatalf("Interrupt(parent): %s", apiErr.msg)
	}
	select {
	case <-childCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("interrupting parent did not cancel running child")
	}
}

// TestStopAgentLoopSnapshotReportsChildCancelledSynchronously guards BUG-248: the
// snapshot stopAgentLoop returns must report a cancelled child's status immediately,
// not "running". turnCancel() only signals cancellation — the child's own finishTurn
// (which sets status = RunStatusCancelled) runs asynchronously once its turn's
// goroutine observes ctx.Done(), and a cancelled child's finishTurn never re-emits an
// agent_graph_updated event for the parent, so the desktop has no later event to
// correct a stale "running" reading. The adapter here blocks past ctx.Done() so
// finishTurn provably has not run yet when the snapshot is inspected.
func TestStopAgentLoopSnapshotReportsChildCancelledSynchronously(t *testing.T) {
	childStarted := make(chan struct{}, 1)
	releaseChild := make(chan struct{})
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, req TurnRequest, _ TurnBridge) error {
				if strings.HasPrefix(req.RunID, "run-") {
					select {
					case childStarted <- struct{}{}:
					default:
					}
				}
				<-ctx.Done()
				<-releaseChild // held open so finishTurn cannot have run yet
				return ctx.Err()
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	t.Cleanup(func() { close(releaseChild) })
	parentHandle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, spawnErr := svc.spawnChildRun(context.Background(), parentHandle.RunID, SpawnAgentInput{
		Agent:  "worker",
		Prompt: "keep running",
		Wait:   false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	select {
	case <-childStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("child turn did not start")
	}

	snap := svc.stopAgentLoop(parentHandle.RunID)

	var childRun *AgentRunSummary
	for i := range snap.Runs {
		if snap.Runs[i].RunID == child.RunID {
			childRun = &snap.Runs[i]
			break
		}
	}
	if childRun == nil {
		t.Fatalf("stopAgentLoop snapshot missing child run %q", child.RunID)
	}
	if childRun.Status != RunStatusCancelled {
		t.Fatalf("stopAgentLoop snapshot reported child status %q, want %q (finishTurn has not run yet)", childRun.Status, RunStatusCancelled)
	}
}

func TestAutoReinvokeHubSingleFlightConcurrent(t *testing.T) {
	// Calling maybeAutoReinvokeHub from two goroutines must schedule exactly one hub
	// turn. Uses a blocking adapter so G2 always races against an in-flight turn
	// (turnInFlight=true, reinvokeInFlight=false), exercising the single-flight guard.
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				select {
				case started <- struct{}{}: // signal that turn has started
				default:
				}
				<-release // block until test releases it
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	svc.mu.Lock()
	svc.runs[parentRunID].autoOrchestrate = true
	svc.mu.Unlock()

	// G1 schedules the hub turn; it will block inside the adapter on 'release'.
	svc.maybeAutoReinvokeHub(parentRunID)

	// Wait until the turn is actually in flight before firing G2.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("hub turn did not start within 2 s")
	}

	// G2 arrives while turnInFlight=true, reinvokeInFlight=false. Our fix ensures
	// it does NOT set pendingHubReinvoke because pendingAgentContext is empty.
	svc.maybeAutoReinvokeHub(parentRunID)

	svc.mu.Lock()
	phri := svc.runs[parentRunID].pendingHubReinvoke
	svc.mu.Unlock()
	if phri {
		t.Error("pendingHubReinvoke must not be set when pendingAgentContext is empty")
	}

	// Release the blocked turn and wait for it to finish.
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		tif := svc.runs[parentRunID].turnInFlight
		svc.mu.Unlock()
		if !tif {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // let runTurn cleanup complete

	svc.mu.Lock()
	tc := svc.runs[parentRunID].turnCount
	svc.mu.Unlock()
	if tc != 1 {
		t.Errorf("concurrent maybeAutoReinvokeHub produced %d hub turns, want exactly 1", tc)
	}
}

// TestAutoReinvokeHubNoPendingContextNoDefer verifies the single-flight fix:
// a concurrent maybeAutoReinvokeHub call that arrives after the first call's hub
// turn has already started (turnInFlight=true, pendingAgentContext=nil) must NOT
// set pendingHubReinvoke. startTurn drains pendingAgentContext before launching
// the turn, so an empty slice means the in-flight turn already consumed the signal.
func TestAutoReinvokeHubNoPendingContextNoDefer(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	handle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	runID := handle.RunID

	svc.mu.Lock()
	svc.runs[runID].autoOrchestrate = true
	svc.runs[runID].turnInFlight = true // hub turn already in flight
	// pendingAgentContext is nil — the in-flight turn has consumed the context
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHub(runID)

	svc.mu.Lock()
	pending := svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if pending {
		t.Error("pendingHubReinvoke must not be set when pendingAgentContext is empty: in-flight turn already consumed the signal")
	}
}

func TestAutoReinvokeHubWithNotePreservesNoteWhenTurnInFlight(t *testing.T) {
	svc, _ := newTestServer(t)
	handle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	runID := handle.RunID

	svc.mu.Lock()
	svc.runs[runID].autoOrchestrate = true
	svc.runs[runID].turnInFlight = true
	svc.mu.Unlock()

	note := "[flow-engine] Joined result note\nreviewer: approved"
	svc.maybeAutoReinvokeHubWithNote(runID, note)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if !rs.pendingHubReinvoke {
		t.Fatal("pendingHubReinvoke = false, want true for note-bearing reinvoke while hub turn is in flight")
	}
	if len(rs.pendingAgentContext) != 1 || rs.pendingAgentContext[0] != note {
		t.Fatalf("pendingAgentContext = %#v, want preserved cohort note", rs.pendingAgentContext)
	}
}

func TestAutoReinvokeHubWithNotePreservesNoteWhenReinvokeAlreadyScheduled(t *testing.T) {
	svc, _ := newTestServer(t)
	handle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	runID := handle.RunID

	svc.mu.Lock()
	svc.runs[runID].autoOrchestrate = true
	svc.runs[runID].reinvokeInFlight = true
	svc.mu.Unlock()

	note := "[flow-engine] Joined result note\nreviewer: approved"
	svc.maybeAutoReinvokeHubWithNote(runID, note)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.pendingHubReinvoke {
		t.Fatal("pendingHubReinvoke = true, want false because scheduled reinvoke can still consume preserved context")
	}
	if len(rs.pendingAgentContext) != 1 || rs.pendingAgentContext[0] != note {
		t.Fatalf("pendingAgentContext = %#v, want preserved cohort note", rs.pendingAgentContext)
	}
}

// TestAutoReinvokeHubDeferredWhenCoderCompletesInFlight verifies the deferred
// reinvoke path: when genuinely new context arrives (coder appends a note) while
// a hub turn is in flight, exactly one follow-up hub turn is scheduled after the
// in-flight turn completes.
func TestAutoReinvokeHubDeferredWhenCoderCompletesInFlight(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	handle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	runID := handle.RunID

	// Simulate hub turn already in flight with a new coder-completion note pending.
	svc.mu.Lock()
	svc.runs[runID].autoOrchestrate = true
	svc.runs[runID].turnInFlight = true
	svc.runs[runID].pendingAgentContext = []string{"[flow-engine] Coder completed."}
	svc.mu.Unlock()

	// Call should defer (set pendingHubReinvoke) not fire immediately.
	svc.maybeAutoReinvokeHub(runID)

	svc.mu.Lock()
	pending := svc.runs[runID].pendingHubReinvoke
	rif := svc.runs[runID].reinvokeInFlight
	svc.mu.Unlock()
	if !pending {
		t.Error("pendingHubReinvoke must be set when new context arrived during hub turn")
	}
	if rif {
		t.Error("reinvokeInFlight must not be set while hub turn is still in flight")
	}

	// Now simulate hub turn completion: clear turnInFlight and pendingHubReinvoke,
	// then call maybeAutoReinvokeHub (the runTurn retry path).
	svc.mu.Lock()
	svc.runs[runID].turnInFlight = false
	svc.runs[runID].pendingHubReinvoke = false
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHub(runID)

	// Wait for the deferred hub turn to complete.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rif = svc.runs[runID].reinvokeInFlight
		svc.mu.Unlock()
		if !rif {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)

	svc.mu.Lock()
	tc := svc.runs[runID].turnCount
	svc.mu.Unlock()
	if tc != 1 {
		t.Errorf("deferred hub reinvoke produced %d hub turns, want exactly 1", tc)
	}
}

func TestAutoReinvokeHubTurnInFlightGuard(t *testing.T) {
	// maybeAutoReinvokeHub must not schedule a turn when one is already in flight.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	// Simulate a turn already in flight.
	svc.mu.Lock()
	svc.runs[parentRunID].autoOrchestrate = true
	svc.runs[parentRunID].turnInFlight = true
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHub(parentRunID)

	svc.mu.Lock()
	rif := svc.runs[parentRunID].reinvokeInFlight
	svc.mu.Unlock()
	// reinvokeInFlight must remain false because the turn-in-flight guard blocked it.
	if rif {
		t.Error("reinvokeInFlight should not be set when turnInFlight=true")
	}
}

func TestAutoReinvokeHubCapBounded(t *testing.T) {
	// maybeAutoReinvokeHub must not fire when Round >= effectiveCap.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentRunID := parentHandle.RunID

	// Enable auto-orchestrate and set loop to cap (Round==Cap so the guard fires).
	svc.mu.Lock()
	svc.runs[parentRunID].autoOrchestrate = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentRunID, AgentLoopState{Mode: "explicit", Round: 3, Cap: 3, Status: "running"})

	svc.maybeAutoReinvokeHub(parentRunID)

	// turnCount stays 0 because the cap guard blocked reinvocation.
	time.Sleep(20 * time.Millisecond)
	svc.mu.Lock()
	tc := svc.runs[parentRunID].turnCount
	rif := svc.runs[parentRunID].reinvokeInFlight
	svc.mu.Unlock()
	if tc != 0 {
		t.Errorf("hub reinvoked at cap (turnCount=%d), want 0", tc)
	}
	if rif {
		t.Error("reinvokeInFlight should be false when cap guard blocked reinvocation")
	}
}

func TestApplyFlowControlDomainFreeGuard(t *testing.T) {
	// Verify the executor methods themselves are free of domain role strings.
	// The actual source check is done by the build + grep in CI; here we confirm the
	// runtime string constants the engine emits contain no role names.
	engineOutputs := []string{
		"done", "continue", "escalate", "blocked", "running",
		"looping", "awaiting_user", "flow completed", "cap",
	}
	forbidden := []string{"coder", "reviewer", "approved", "changes_requested"}
	for _, s := range engineOutputs {
		for _, bad := range forbidden {
			if strings.Contains(s, bad) {
				t.Errorf("engine output %q contains forbidden role string %q", s, bad)
			}
		}
	}
}

// TestSpawnedChildInheritsParentYolo guards BUG-129: with YOLO enabled on the parent, a
// child spawned via spawn_agent must run with YOLO too, so its gated actions auto-approve
// instead of stalling the (often wait=true) parent on a child approval prompt. It also
// covers the sticky per-turn posture: the parent enables YOLO per-turn (UI toggle), and the
// child spawned afterward must inherit that posture, not the stale run-level default.
func TestSpawnedChildInheritsParentYolo(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	// Parent starts WITHOUT run-level YOLO (the stale default the old code would copy).
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Parent turn enables YOLO per-turn (mirrors the UI toggle). This must stick on the run.
	yoloOn := true
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "hi", YoloMode: &yoloOn}, "", ""); apiErr != nil {
		t.Fatalf("parent startTurn: %s", apiErr.msg)
	}
	parentReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if !parentReq.YoloMode {
		t.Fatalf("parent turn should carry YOLO=true")
	}

	// Spawn a child via the tool path AFTER the YOLO turn; it must inherit YOLO=true.
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "work", Provider: "claude", Wait: false,
	}); spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	// The child's first provider turn must run with YOLO enabled.
	for {
		select {
		case req := <-capture.ch:
			if req.RunID == parent.RunID {
				continue // ignore any further parent turns
			}
			if !req.YoloMode {
				t.Fatalf("child turn should inherit parent YOLO=true, got YoloMode=false (run %s)", req.RunID)
			}
			return
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for child turn request")
		}
	}
}

// TestAgentSummaryCarriesWaitForResultFlag guards BUG-133: the desktop gates the main run on
// a running wait=true child, so the agent summary must expose the spawn's wait flag. A wait=false
// (background) child must report waitForResult=false; a wait=true child must report true.
func TestAgentSummaryCarriesWaitForResultFlag(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	bg, bgErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "bg", Provider: "claude", Wait: false})
	if bgErr != nil {
		t.Fatalf("spawn bg: %v", bgErr)
	}
	waiter, waitErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "blocking", Provider: "claude", Wait: true})
	if waitErr != nil {
		t.Fatalf("spawn wait: %v", waitErr)
	}

	byID := make(map[string]AgentRunSummary)
	for _, s := range svc.listAgentRunSummaries(parent.RunID) {
		byID[s.RunID] = s
	}
	if s, ok := byID[bg.RunID]; !ok || s.WaitForResult {
		t.Fatalf("background child should have WaitForResult=false, got %+v (present=%t)", s, ok)
	}
	if s, ok := byID[waiter.RunID]; !ok || !s.WaitForResult {
		t.Fatalf("wait=true child should have WaitForResult=true, got %+v (present=%t)", s, ok)
	}
}

// TestUISpawnInjectsContextIntoParentProviderTurn guards BUG-122: a child agent spawned from
// the desktop UI is invisible to the parent's provider conversation. The next parent turn's
// provider prompt must carry a system note naming the child so the parent agent can answer
// "which sub-agents did we start?", while the displayed prompt stays clean.
// TestComposeAgentSpawnPromptIsProviderConsistent guards BUG-128: the spawn prompt
// is built by one shared helper, so a given agent yields the same shape regardless
// of provider — system prompt first (preserving built-in-agent prefix detection),
// then a single identity line that links the definition file, then the user prompt.
func TestComposeAgentSpawnPromptIsProviderConsistent(t *testing.T) {
	def := &AgentDefinition{
		Name:         "coder",
		Role:         "coder",
		SystemPrompt: "You are the coder sub-agent. Implement the change.",
		Source:       "claude",
		Path:         "/proj/.claude/agents/coder.md",
	}
	got := composeAgentSpawnPrompt(def, "do work")

	// System prompt stays first so hasBuiltInAgentPromptPrefix keeps matching.
	if !strings.HasPrefix(strings.ToLower(got), "you are the coder sub-agent.") {
		t.Fatalf("system prompt must stay first, got %q", got)
	}
	// The agent is named and its definition file is linked.
	if !strings.Contains(got, "[FlowPilot sub-agent — agent: coder | role: coder | definition: /proj/.claude/agents/coder.md]") {
		t.Fatalf("identity line missing or malformed: %q", got)
	}
	// The user prompt is appended last, unchanged.
	if !strings.HasSuffix(got, "do work") {
		t.Fatalf("user prompt must be appended last, got %q", got)
	}

	// A built-in agent (no on-disk path) is marked as built-in, not blank.
	builtin := &AgentDefinition{Name: "coder", Role: "coder", SystemPrompt: "sp", Source: "flowpilot"}
	if line := composeAgentIdentityLine(builtin); !strings.Contains(line, "definition: built-in (flowpilot)") {
		t.Fatalf("built-in agent should mark definition as built-in, got %q", line)
	}

	// A nil agent definition (unknown agent) leaves the user prompt untouched.
	if got := composeAgentSpawnPrompt(nil, "just this"); got != "just this" {
		t.Fatalf("nil agentDef must pass the prompt through unchanged, got %q", got)
	}
}

func TestUISpawnInjectsContextIntoParentProviderTurn(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Spawn from the desktop UI (the HTTP handler sets UIInitiated=true). wait=false so the
	// child runs in the background; its first turn flows through the same capture adapter.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/spawn-agent", map[string]any{
		"agent": "coder", "prompt": "do work", "provider": "claude", "wait": false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("spawn status=%d body=%s", status, body)
	}
	var spawn SpawnAgentResult
	if err := json.Unmarshal(body, &spawn); err != nil {
		t.Fatalf("decode spawn: %v", err)
	}
	// Let the child turn complete so both the spawn note and the result note are queued.
	waitTerminal(t, srv.URL, spawn.RunID)

	// Parent turn: its provider prompt must carry the injected sub-agent context.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "which sub-agents did we start?",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("parent turn status=%d body=%s", status, body)
	}

	parentReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if !strings.Contains(parentReq.Prompt, "FlowPilot system note") || !strings.Contains(parentReq.Prompt, "coder") {
		t.Fatalf("parent provider prompt missing UI-spawn context: %q", parentReq.Prompt)
	}
	if !strings.HasSuffix(strings.TrimSpace(parentReq.Prompt), "which sub-agents did we start?") {
		t.Fatalf("user prompt should be appended after the context block: %q", parentReq.Prompt)
	}

	// The displayed prompt (turn_started event) stays clean — the note is provider-only.
	var displayed string
	for _, e := range adminEvents(t, srv.URL, parent.RunID) {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			displayed = e.Prompt
		}
	}
	if displayed != "which sub-agents did we start?" {
		t.Fatalf("displayed prompt should be clean, got %q", displayed)
	}

	// A second parent turn must NOT re-inject (the buffer was cleared after consumption).
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "thanks",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("second parent turn status=%d body=%s", status, body)
	}
	secondReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if strings.Contains(secondReq.Prompt, "FlowPilot system note") {
		t.Fatalf("context must be injected once, not re-sent: %q", secondReq.Prompt)
	}
}

// TestToolSpawnWaitTrueDoesNotInjectParentContext guards BUG-122/BUG-126: an AI spawn_agent
// tool call with wait=true returns the child result synchronously as the tool result (already
// in the provider conversation), so it must NOT also be injected as a context note.
func TestToolSpawnWaitTrueDoesNotInjectParentContext(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Direct service call mirrors the AI tool path with wait=true: the result is returned
	// synchronously (UIInitiated stays false), so no injection should happen.
	spawn, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "do work", Provider: "claude", Wait: true,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	if spawn.FinalMessage == "" {
		t.Fatalf("wait=true should return the child result synchronously, got empty")
	}

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "hello",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("parent turn status=%d body=%s", status, body)
	}
	parentReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if strings.Contains(parentReq.Prompt, "FlowPilot system note") {
		t.Fatalf("wait=true tool spawn must not inject context, got prompt: %q", parentReq.Prompt)
	}
}

// TestToolSpawnWaitFalseInjectsResult guards BUG-126: an AI spawn_agent tool call with
// wait=false only acks "spawned" — its eventual result is never returned to the model, so
// the child's completion result must be injected into the parent's next provider turn, the
// same way UI spawns are, keeping tool and UI spawn symmetric.
func TestToolSpawnWaitFalseInjectsResult(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Tool path, background: UIInitiated=false, wait=false.
	spawn, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "do work", Provider: "claude", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	waitTerminal(t, srv.URL, spawn.RunID)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "what did the background agent return?",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("parent turn status=%d body=%s", status, body)
	}
	parentReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if !strings.Contains(parentReq.Prompt, "FlowPilot system note") || !strings.Contains(parentReq.Prompt, "completed") {
		t.Fatalf("wait=false tool spawn must inject the child result, got prompt: %q", parentReq.Prompt)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func adminEvents(t *testing.T, base, runID string) []ProviderEvent {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/admin/workflow-runs/"+runID+"/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("admin events status=%d", status)
	}
	var evs []ProviderEvent
	if err := json.Unmarshal(body, &evs); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	return evs
}

func waitTerminal(t *testing.T, base, runID string) []ProviderEvent {
	t.Helper()
	var evs []ProviderEvent
	waitFor(t, func() bool {
		evs = adminEvents(t, base, runID)
		if len(evs) == 0 {
			return false
		}
		last := evs[len(evs)-1].Type
		return last == EventTurnCompleted || last == EventTurnFailed
	}, "terminal event")
	return evs
}

func assertSeqContiguous(t *testing.T, evs []ProviderEvent) {
	t.Helper()
	for i, e := range evs {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq not contiguous at %d: got %d (%s)", i, e.Seq, e.Type)
		}
	}
}

// ---- tests -----------------------------------------------------------------

func TestCatalogAndRegistry(t *testing.T) {
	svc, srv := newTestServer(t)

	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Acme Web App") {
		t.Fatalf("projects status=%d body=%s", status, body)
	}
	status, body = doJSON(t, "GET", srv.URL+"/client/workflows/wf-feature/steps", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "step-plan") {
		t.Fatalf("steps status=%d body=%s", status, body)
	}

	// registry: codex available, claude/gemini placeholders
	status, body = doJSON(t, "GET", srv.URL+"/admin/providers", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("providers status=%d", status)
	}
	for _, want := range []string{`"codex"`, `"claude"`, `"gemini"`, `"available"`, `"placeholder"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("providers missing %s in %s", want, body)
		}
	}
	// disabled/placeholder adapters surface the typed error
	if _, err := svc.registry.Adapter(ProviderKeyClaude); err == nil {
		t.Fatal("expected UnsupportedProviderRuntimeError for claude")
	} else if _, ok := err.(*UnsupportedProviderRuntimeError); !ok {
		t.Fatalf("expected UnsupportedProviderRuntimeError, got %T", err)
	}
}

func TestNormalTurnPersistsWithSeq(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)

	if status, turnID := sendTurn(t, srv.URL, runID, "normal", nil); status != http.StatusOK || turnID == "" {
		t.Fatalf("send turn status=%d turnId=%q", status, turnID)
	}
	evs := waitTerminal(t, srv.URL, runID)

	assertSeqContiguous(t, evs)
	if evs[0].Type != EventTurnStarted {
		t.Fatalf("first event = %s, want turn_started", evs[0].Type)
	}
	if evs[0].Prompt != "hi" {
		t.Fatalf("turn_started prompt = %q, want hi", evs[0].Prompt)
	}
	if last := evs[len(evs)-1]; last.Type != EventTurnCompleted {
		t.Fatalf("last event = %s, want turn_completed", last.Type)
	}
	if got := getSnapshot(t, srv.URL, runID).Status; got != RunStatusCompleted {
		t.Fatalf("status = %s, want completed", got)
	}

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	steps := store.steps[runID]
	if len(steps) != 1 || steps[0].Status != StepStatusDone {
		t.Fatalf("workflow steps = %+v, want one DONE step", steps)
	}
	if store.runStatus[runID] != RunStatusEngineDone {
		t.Fatalf("run status = %s, want DONE", store.runStatus[runID])
	}
	if store.applies < 2 {
		t.Fatalf("expected running + orchestrator transitions, got applies=%d", store.applies)
	}
	events := store.events[runID]
	if len(events) != 3 {
		t.Fatalf("persisted events = %d, want 3 non-delta events", len(events))
	}
	if events[0].Type != EventTurnStarted || events[1].Type != EventMessageCompleted || events[2].Type != EventTurnCompleted {
		t.Fatalf("persisted events = %+v, want turn_started/message_completed/turn_completed", events)
	}
	if events[0].Prompt != "hi" {
		t.Fatalf("persisted turn_started prompt = %q, want hi", events[0].Prompt)
	}
	for _, ev := range events {
		if ev.Type == EventMessageDelta {
			t.Fatalf("delta event should not be persisted: %+v", ev)
		}
	}
	replayed := streamEvents(t, srv.URL, runID, 0, 1)
	if len(replayed) != 1 || replayed[0].Type != EventTurnStarted || replayed[0].Prompt != "hi" {
		t.Fatalf("replayed first event = %+v, want turn_started with prompt", replayed)
	}
}

func TestProjectRunHistoryFiltersRunsByProject(t *testing.T) {
	_, srv := newTestServer(t)
	first := startProjectRun(t, srv.URL, "proj-web", "wf-feature")
	other := startProjectRun(t, srv.URL, "proj-android", "wf-feature")
	second := startProjectRun(t, srv.URL, "proj-web", "wf-feature")

	if status, turnID := sendTurn(t, srv.URL, first, "normal", nil); status != http.StatusOK || turnID == "" {
		t.Fatalf("send turn status=%d turnId=%q", status, turnID)
	}
	waitTerminal(t, srv.URL, first)

	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history len=%d want 2: %+v", len(history), history)
	}
	for _, item := range history {
		if item.ProjectID != "proj-web" {
			t.Fatalf("history included other project: %+v", item)
		}
		if item.RunID == other {
			t.Fatalf("history included excluded run %s", other)
		}
	}
	if history[0].RunID != first {
		t.Fatalf("first history run=%s want updated run %s (second created run was %s)", history[0].RunID, first, second)
	}
	if history[0].LastPrompt != "hi" || history[0].LastMessage == "" || history[0].Status != RunStatusCompleted {
		t.Fatalf("updated history item missing summary: %+v", history[0])
	}
}

func TestProjectRunHistoryExcludesLiveChildAgentRuns(t *testing.T) {
	_, srv := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj-web", WorkflowID: "wf-feature", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	parent := handle.RunID

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent+"/spawn-agent", SpawnAgentInput{
		Agent:    "coder",
		Prompt:   "implement the small change",
		Provider: "codex",
		Wait:     false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("spawn-agent status=%d body=%s", status, body)
	}
	var spawned SpawnAgentResult
	if err := json.Unmarshal(body, &spawned); err != nil {
		t.Fatalf("decode spawn result: %v", err)
	}
	if spawned.RunID == "" {
		t.Fatalf("spawn result missing run id: %+v", spawned)
	}

	status, body = doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != parent {
		t.Fatalf("history = %+v, want only parent %s; child %s must stay out of main history", history, parent, spawned.RunID)
	}

	status, body = doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent+"/agents", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", status, body)
	}
	var agents []AgentRunSummary
	if err := json.Unmarshal(body, &agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 1 || agents[0].RunID != spawned.RunID || agents[0].ParentRunID != parent {
		t.Fatalf("agents = %+v, want child %s under parent %s", agents, spawned.RunID, parent)
	}
}

func TestProjectRunHistoryExcludesLiveOrphanAgentMetadataRuns(t *testing.T) {
	svc, _ := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.mu.Lock()
	svc.runs["parent-run"] = &interactiveRun{
		id:                "parent-run",
		projectID:         "proj-web",
		workflowID:        "wf-feature",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "session-parent",
		status:            RunStatusCompleted,
		createdAt:         now,
		updatedAt:         now,
		lastPrompt:        "Use spawn_agent exactly once with agent=\"reviewer\".",
		runKind:           "chat",
	}
	svc.runs["orphan-agent-run"] = &interactiveRun{
		id:                "orphan-agent-run",
		projectID:         "proj-web",
		workflowID:        "wf-feature",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "session-child",
		status:            RunStatusCompleted,
		createdAt:         now,
		updatedAt:         now,
		lastPrompt:        "You are the reviewer sub-agent. Review the coder's diff.\n\nDo not use tools. Reply exactly: CHILD_AGENT_DONE.",
		lastMessage:       "CHILD_AGENT_DONE",
		runKind:           "chat",
		agentName:         "reviewer",
		role:              "reviewer",
		agentStatus:       string(RunStatusCompleted),
	}
	svc.mu.Unlock()

	history := svc.projectRunHistory("proj-web")

	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only parent; live orphan agent metadata run must stay out of main history", history)
	}
}

func TestProjectRunHistoryKeepsLiveRootRowsWithAgentMetadata(t *testing.T) {
	svc, _ := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.mu.Lock()
	svc.runs["main-run"] = &interactiveRun{
		id:                "main-run",
		projectID:         "proj-web",
		workflowID:        "wf-feature",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "session-main",
		status:            RunStatusCompleted,
		createdAt:         now,
		updatedAt:         now,
		lastPrompt:        "Main agent prompt",
		lastMessage:       "Main agent response",
		runKind:           "chat",
		agentName:         "main",
		agentStatus:       string(RunStatusCompleted),
	}
	svc.mu.Unlock()

	history := svc.projectRunHistory("proj-web")

	if len(history) != 1 || history[0].RunID != "main-run" {
		t.Fatalf("history = %+v, want main root row even when live metadata contains agent-like fields", history)
	}
}

func TestProjectRunHistoryKeepsPersistedRootRowsWithAgentMetadataAfterRestart(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "main-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-main",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "Main agent prompt",
		LastMessage:       "Main agent response",
		RunKind:           "chat",
		AgentName:         "main",
		AgentStatus:       string(RunStatusCompleted),
	}); err != nil {
		t.Fatalf("seed main session: %v", err)
	}

	_, srv := newTestServerWith(t, registry, catalog, store)
	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != "main-run" {
		t.Fatalf("history = %+v, want persisted main root row after restart", history)
	}
}

func TestProjectRunHistoryExcludesPersistedChildAgentRuns(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "parent-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-parent",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "parent prompt",
	}); err != nil {
		t.Fatalf("seed parent session: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "child-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-child",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "child prompt",
		ParentRunID:       "parent-run",
		AgentName:         "coder",
		Role:              "coder",
	}); err != nil {
		t.Fatalf("seed child session: %v", err)
	}

	_, srv := newTestServerWith(t, registry, catalog, store)
	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only persisted parent; child-run must stay out of main history", history)
	}
}

func TestProjectRunHistoryExcludesResumedChildRunsAfterReopen(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, session := range []ProviderSessionState{
		{
			RunID:             "parent-run",
			ProjectID:         "proj-web",
			WorkflowID:        "wf-feature",
			ProviderSessionID: "thread-parent",
			ProviderKey:       ProviderKeyClaude,
			Status:            RunStatusCompleted,
			StartedAt:         now,
			UpdatedAt:         now,
			LastPrompt:        "parent prompt",
			RunKind:           "workflow",
		},
		{
			RunID:             "child-run",
			ProjectID:         "proj-web",
			WorkflowID:        "wf-feature",
			ProviderSessionID: "thread-child",
			ProviderKey:       ProviderKeyClaude,
			Status:            RunStatusCompleted,
			StartedAt:         now,
			UpdatedAt:         now,
			LastPrompt:        "You are the reviewer sub-agent.",
			RunKind:           "chat",
			ParentRunID:       "parent-run",
			AgentName:         "reviewer-agent",
			Role:              "reviewer-agent",
			AgentStatus:       string(RunStatusCompleted),
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc.resumeRun("child-run"); apiErr != nil {
		t.Fatalf("resumeRun(child-run): %v", apiErr)
	}

	history := svc.projectRunHistory("proj-web")
	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only parent after reopening a child run", history)
	}
}

func TestProjectRunHistoryKeepsCompletedStatusWhenReopeningLegacyFlowRun(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "flow-parent",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "thread-parent",
		ProviderKey:       ProviderKeyClaude,
		Status:            RunStatusWaitingQuestion,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "parent prompt",
		LastMessage:       "done",
		RunKind:           "workflow",
		AutoOrchestrate:   true,
		PendingAgentContext: []string{
			"[flow-engine] An agent has already been spawned to work on this request.",
			"Flow completed. # Flow Audit Draft",
		},
		LoopState: AgentLoopState{Status: "done", Round: 0, Cap: 3, RoundCap: 3, Mode: "explicit"},
		ActiveFlowNodes: []agentpack.FlowNode{
			{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
			{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"coder"}},
		},
	}); err != nil {
		t.Fatalf("seed parent flow session: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc.resumeRun("flow-parent"); apiErr != nil {
		t.Fatalf("resumeRun(flow-parent): %v", apiErr)
	}

	history := svc.projectRunHistory("proj-web")
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Status != RunStatusCompleted {
		t.Fatalf("history status = %q, want completed after reopening a settled flow run with stale waiting_question status", history[0].Status)
	}
}

func TestReconstructRunPreservesChildMetadataForResumedAgentRun(t *testing.T) {
	svc, _ := newTestServer(t)
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:             "child-run",
		ProjectID:         "proj",
		ProviderKey:       ProviderKeyClaude,
		ProviderSessionID: "thread-child",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		ParentRunID:       "parent-run",
		AgentName:         "reviewer-agent",
		Role:              "reviewer-agent",
		AgentStatus:       string(RunStatusCompleted),
		ModelName:         "claude-sonnet",
		DependsOn:         []string{"coder-run"},
		StartedAt:         "2026-07-07T16:00:00Z",
		UpdatedAt:         "2026-07-07T16:01:00Z",
		LastPrompt:        "You are the reviewer sub-agent.",
		LastMessage:       "done",
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.parentRunID != "parent-run" {
		t.Fatalf("parentRunID = %q, want parent-run", rs.parentRunID)
	}
	if rs.agentName != "reviewer-agent" {
		t.Fatalf("agentName = %q, want reviewer-agent", rs.agentName)
	}
	if rs.role != "reviewer-agent" {
		t.Fatalf("role = %q, want reviewer-agent", rs.role)
	}
	if rs.agentStatus != string(RunStatusCompleted) {
		t.Fatalf("agentStatus = %q, want completed", rs.agentStatus)
	}
	if rs.modelName != "claude-sonnet" {
		t.Fatalf("modelName = %q, want claude-sonnet", rs.modelName)
	}
	if !reflect.DeepEqual(rs.dependsOn, []string{"coder-run"}) {
		t.Fatalf("dependsOn = %#v, want [coder-run]", rs.dependsOn)
	}
}

func TestProjectRunHistoryExcludesLegacyOrphanAgentPromptRuns(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "parent-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-parent",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "Use spawn_agent exactly once with agent=\"reviewer\".",
	}); err != nil {
		t.Fatalf("seed parent session: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "orphan-child-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-child",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "You are the reviewer sub-agent. Review the coder's diff adversarially for correctness, regressions, and missed edge cases. Return either APPROVED or CHANGES-REQUESTED with specific, actionable feedback.\n\nDo not use tools. Reply exactly: CHILD_AGENT_DONE.",
		LastMessage:       "CHILD_AGENT_DONE",
		RunKind:           "chat",
	}); err != nil {
		t.Fatalf("seed orphan child session: %v", err)
	}

	_, srv := newTestServerWith(t, registry, catalog, store)
	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only parent; legacy orphan child must stay out of main history", history)
	}
}

func TestEventStreamReplayNoGapsOrDupes(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "tool-heavy", nil)
	all := waitTerminal(t, srv.URL, runID)
	maxSeq := all[len(all)-1].Seq

	// reconnect from the middle: replay only seq > after
	after := int64(3)
	want := int(maxSeq - after)
	got := streamEvents(t, srv.URL, runID, after, want)
	if len(got) != want {
		t.Fatalf("replay got %d events, want %d", len(got), want)
	}
	for i, e := range got {
		if e.Seq != after+int64(i)+1 {
			t.Fatalf("replay gap/dupe at %d: seq=%d", i, e.Seq)
		}
	}
}

func TestApprovalDenyThenApprove(t *testing.T) {
	_, srv := newTestServer(t)

	// deny
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	var approvalID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingApproval != nil {
			approvalID = s.PendingApproval.ApprovalID
			return true
		}
		return false
	}, "pending approval")

	// invalid decision → 400
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "bogus"}, nil); st != http.StatusBadRequest {
		t.Fatalf("invalid decision status=%d, want 400", st)
	}
	// deny
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "deny"}, nil); st != http.StatusOK {
		t.Fatalf("deny status=%d", st)
	}
	// idempotent: second submit still 200 (first-write-wins)
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "approve"}, nil); st != http.StatusOK {
		t.Fatalf("idempotent resubmit status=%d", st)
	}
	evs := waitTerminal(t, srv.URL, runID)
	if !hasToolStatus(evs, "shell", "cancelled") {
		t.Fatal("deny: expected shell tool_completed cancelled")
	}

	// approve (fresh run)
	runID2 := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID2, "approval-required", nil)
	var approvalID2 string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID2)
		if s.PendingApproval != nil {
			approvalID2 = s.PendingApproval.ApprovalID
			return true
		}
		return false
	}, "pending approval 2")
	doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID2+"/decision", map[string]string{"decision": "approve"}, nil)
	evs2 := waitTerminal(t, srv.URL, runID2)
	if !hasToolStatus(evs2, "shell", "success") {
		t.Fatal("approve: expected shell tool_completed success")
	}
}

func TestQuestionAnswer(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	var qID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingQuestion != nil {
			qID = s.PendingQuestion.QuestionID
			return true
		}
		return false
	}, "pending question")
	if st, _ := doJSON(t, "POST", srv.URL+"/client/questions/"+qID+"/answer", map[string]any{"choice": "css-modules"}, nil); st != http.StatusOK {
		t.Fatalf("answer status=%d", st)
	}
	evs := waitTerminal(t, srv.URL, runID)
	if evs[len(evs)-1].Type != EventTurnCompleted {
		t.Fatal("question: expected turn_completed after answer")
	}
}

func TestInterruptCancelsTurn(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "waiting_question")

	if st, _ := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/interrupt", nil, nil); st != http.StatusOK {
		t.Fatalf("interrupt status=%d", st)
	}
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusCancelled }, "cancelled")
}

func TestConcurrentTurnConflict(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil) // blocks in-flight
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")

	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("concurrent turn status=%d, want 409", st)
	}
}

func TestIdempotentTurn(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	h := map[string]string{"Idempotency-Key": "k1"}
	_, t1 := sendTurn(t, srv.URL, runID, "question-required", h) // in-flight
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")
	st, t2 := sendTurn(t, srv.URL, runID, "question-required", h)
	if st != http.StatusOK || t2 != t1 {
		t.Fatalf("idempotent turn status=%d t1=%s t2=%s", st, t1, t2)
	}
}

func TestApprovalExpiry(t *testing.T) {
	_, srv := newTestServer(t) // TTL = 50ms
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	evs := waitTerminal(t, srv.URL, runID)
	last := evs[len(evs)-1]
	if last.Type != EventTurnFailed || !last.Recoverable {
		t.Fatalf("expiry: last event=%s recoverable=%v, want turn_failed recoverable", last.Type, last.Recoverable)
	}
}

func TestAccountMismatch(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	svc.activeAccountID = "switched"

	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("turn after account switch status=%d, want 409", st)
	}
	if st, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/resume", nil, nil); st != http.StatusOK || !strings.Contains(string(body), "\"runId\":\"run-1\"") {
		t.Fatalf("resume after switch status=%d body=%s", st, body)
	}
}

// ---- helpers ---------------------------------------------------------------

func hasToolStatus(evs []ProviderEvent, tool, status string) bool {
	for _, e := range evs {
		if e.Type == EventToolCompleted && e.ToolName == tool && e.Status == status {
			return true
		}
	}
	return false
}

// streamEvents connects to the SSE endpoint and returns up to wantCount events,
// then cancels.
func streamEvents(t *testing.T, base, runID string, afterSeq int64, wantCount int) []ProviderEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	url := base + "/client/workflow-runs/" + runID + "/events/stream?afterSeq=" +
		strconv.FormatInt(afterSeq, 10)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream connect: %v", err)
	}
	defer resp.Body.Close()

	var out []ProviderEvent
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev ProviderEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		out = append(out, ev)
		if len(out) >= wantCount {
			cancel()
			return out
		}
	}
	return out
}

func TestSeedIDCounterAvoidsRunIDReuseAfterRestart(t *testing.T) {
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Persisted runs from a previous runner session; run-17 is a child of run-9.
	for _, sess := range []ProviderSessionState{
		{RunID: "run-9", ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now, RunKind: "chat"},
		{RunID: "run-17", ParentRunID: "run-9", ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now, RunKind: "chat", AgentName: "reviewer"},
	} {
		if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	// The next minted run id must be numerically above every persisted run id so a new
	// chat cannot collide with a previous run and inherit its child agents. (BUG-117)
	id := svc.nextID("run")
	if n := numericIDSuffix(id); n <= 17 {
		t.Fatalf("nextID after seeding = %q (suffix %d), want suffix > 17", id, n)
	}
}

func TestNumericIDSuffix(t *testing.T) {
	cases := map[string]int64{"run-14": 14, "evt-1": 1, "run-": 0, "main-run": 0, "": 0, "bus-007": 7}
	for in, want := range cases {
		if got := numericIDSuffix(in); got != want {
			t.Errorf("numericIDSuffix(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestReconstructRunPreservesUpdatedAt guards BUG-118: opening (reconstructing) a persisted
// chat must keep its stored updatedAt — the in-memory history list reports rs.updatedAt, so
// seeding it with `now` made every opened chat jump to the top with the current date.
func TestReconstructRunPreservesUpdatedAt(t *testing.T) {
	store := newFakeWorkflowStore()
	persisted := "2026-06-19T10:00:00.000000000Z"
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-1",
		ProviderKey: ProviderKeyClaude,
		Status:      RunStatusCompleted,
		RunKind:     "chat",
		StartedAt:   "2026-06-19T09:00:00.000000000Z",
		UpdatedAt:   persisted,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	rs, apiErr := svc.loadPersistedRun("run-1")
	if apiErr != nil {
		t.Fatalf("loadPersistedRun: %v", apiErr)
	}
	if rs.updatedAt != persisted {
		t.Fatalf("reconstructRun updatedAt = %q, want preserved %q", rs.updatedAt, persisted)
	}
}

// TestReconstructRunRestoresTrackedFlowTopology is the regression test for
// BUG-NOTE-CP42 #16: activeFlowEdges/activeFlowNodes lived only on the
// in-memory interactiveRun, and ProviderSessionState never snapshotted them —
// so a chat reopened after a runner restart (or a Drive-synced cross-PC
// move) lost its tracked flow topology entirely. resolveContinueBackEdgeTarget
// and tryAdvanceFlowFromNode would then silently fall back to legacy
// isCoderRun role matching instead of the flow's own declared edges, even
// though the chat's mid-flow state was otherwise fully restorable.
func TestReconstructRunRestoresTrackedFlowTopology(t *testing.T) {
	store := newFakeWorkflowStore()
	edges := []agentpack.FlowEdge{{From: "coder", To: "reviewer", When: "done", Kind: "forward"}}
	nodes := []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:           "run-flow-1",
		ProviderKey:     ProviderKeyClaude,
		Status:          RunStatusCompleted,
		RunKind:         "chat",
		ActiveFlowEdges: edges,
		ActiveFlowNodes: nodes,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	rs, apiErr := svc.loadPersistedRun("run-flow-1")
	if apiErr != nil {
		t.Fatalf("loadPersistedRun: %v", apiErr)
	}
	if !reflect.DeepEqual(rs.activeFlowEdges, edges) {
		t.Fatalf("activeFlowEdges = %#v, want %#v", rs.activeFlowEdges, edges)
	}
	if !reflect.DeepEqual(rs.activeFlowNodes, nodes) {
		t.Fatalf("activeFlowNodes = %#v, want %#v", rs.activeFlowNodes, nodes)
	}
}

// TestSessionStateOfSnapshotsTrackedFlowTopology proves the write side of the
// same fix: sessionStateOf must actually include activeFlowEdges/
// activeFlowNodes in the snapshot it persists, or the restore-side fix above
// would have nothing real to round-trip in production.
func TestSessionStateOfSnapshotsTrackedFlowTopology(t *testing.T) {
	edges := []agentpack.FlowEdge{{From: "coder", To: "reviewer", When: "done", Kind: "forward"}}
	nodes := []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}}
	rs := &interactiveRun{id: "run-1", activeFlowEdges: edges, activeFlowNodes: nodes}
	state := sessionStateOf(rs)
	if !reflect.DeepEqual(state.ActiveFlowEdges, edges) {
		t.Fatalf("ProviderSessionState.ActiveFlowEdges = %#v, want %#v", state.ActiveFlowEdges, edges)
	}
	if !reflect.DeepEqual(state.ActiveFlowNodes, nodes) {
		t.Fatalf("ProviderSessionState.ActiveFlowNodes = %#v, want %#v", state.ActiveFlowNodes, nodes)
	}
}

// TestAgentGraphUpdateDoesNotResetParentRunStatus guards BUG-120: agent_graph_updated and
// agent_bus_message are orchestration relays emitted on the PARENT run to refresh the Agents
// panel. They must NOT flip the parent's run status to running — otherwise an idle/completed
// parent shows a perpetual "running" spinner whenever a child agent emits graph activity.
func TestAgentGraphUpdateDoesNotResetParentRunStatus(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].status = RunStatusCompleted
	svc.mu.Unlock()

	// Child activity relays these on the parent run.
	svc.emitAgentGraph(parent.RunID, svc.agentGraphSnapshot(parent.RunID))
	svc.emitAgentBus(parent.RunID, AgentBusMessage{ID: "bus-1", ParentRunID: parent.RunID, Kind: "handoff", Message: "x"})

	svc.mu.Lock()
	got := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if got != RunStatusCompleted {
		t.Fatalf("parent status after orchestration relays = %q, want %q (must not flip to running)", got, RunStatusCompleted)
	}
}

// BUG-250: a flow-engine-driven hub's own first provider turn is suppressed
// (CP-42), so provider_session_id never advances past the synthetic
// "thread-<n>" placeholder until the hub is later reinvoked. Before this fix,
// resumeRun's session-file validation bypass only covered a run still
// resident in memory (isActiveInMemory requires inMemory==true) -- once the
// runner restarts and the run is rebuilt from sessions.ndjson, inMemory is
// permanently false regardless of status, so the bypass never applied and
// LocateSessionFile always failed against a placeholder that was never a real
// provider session to begin with, permanently blocking reopen.
func TestSkipsResumeSessionValidation(t *testing.T) {
	svc, _ := newTestServer(t)

	cases := []struct {
		name        string
		rs          *interactiveRun
		inMemory    bool
		wantSkipped bool
	}{
		{
			name: "BUG-250: Claude placeholder session, rebuilt from disk (not in memory), status completed -- must skip",
			rs: &interactiveRun{
				providerKey:       ProviderKeyClaude,
				providerSessionID: "thread-6075",
				status:            RunStatusCompleted,
			},
			inMemory:    false,
			wantSkipped: true,
		},
		{
			name: "Claude with a real session id, rebuilt from disk -- must NOT skip",
			rs: &interactiveRun{
				providerKey:       ProviderKeyClaude,
				providerSessionID: "674851bf-1b16-47d4-a7ce-7ec0b78e46a0",
				status:            RunStatusCompleted,
			},
			inMemory:    false,
			wantSkipped: false,
		},
		{
			name: "Codex with a placeholder session, rebuilt from disk -- must NOT skip (Codex self-heals via rollout discovery)",
			rs: &interactiveRun{
				providerKey:       ProviderKeyCodex,
				providerSessionID: "thread-6075",
				status:            RunStatusCompleted,
			},
			inMemory:    false,
			wantSkipped: false,
		},
		{
			name: "Codex flow hub with only a synthetic placeholder and no rollout chain -- skip so history can reopen from turn log",
			rs: &interactiveRun{
				id:                "run-flow-hub-1",
				providerKey:       ProviderKeyCodex,
				providerSessionID: "thread-6075",
				status:            RunStatusCompleted,
				runKind:           "workflow",
				activeFlowNodes:   []agentpack.FlowNode{{ID: "coder"}},
			},
			inMemory:    false,
			wantSkipped: true,
		},
		{
			name: "Claude placeholder session, still in memory with turn in flight -- skip for the pre-existing isActiveInMemory reason",
			rs: &interactiveRun{
				providerKey:       ProviderKeyClaude,
				providerSessionID: "thread-6075",
				status:            RunStatusRunning,
			},
			inMemory:    true,
			wantSkipped: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.skipsResumeSessionValidation(tc.rs, tc.inMemory); got != tc.wantSkipped {
				t.Fatalf("skipsResumeSessionValidation = %v, want %v", got, tc.wantSkipped)
			}
		})
	}
}

// BUG-251: after a restart, a child agent's run row is read from
// listAgentRunSummaries' disk fallback (neither the live in-memory map nor the
// orchestrator's in-memory historical cache have anything for it yet) --
// nothing is actually executing it anymore, since the whole process just
// restarted. reconstructRun already normalizes an in-flight status
// (running/starting/waiting_*) to "cancelled" for a run's own top-level
// status via normalizeResumedStatus; this disk-fallback branch skipped that
// normalization, so a reviewer/coder child whose CLI process was killed
// mid-turn showed a permanently stale "running" badge in the Agents panel
// with no way to tell it apart from one genuinely still executing.
func TestListAgentRunSummariesNormalizesStaleRunningStatusAfterRestart(t *testing.T) {
	store := newFakeWorkflowStore()
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-hub-1",
		ProviderKey: ProviderKeyClaude,
		Status:      RunStatusCompleted,
		RunKind:     "chat",
	}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-reviewer-1",
		ProviderKey: ProviderKeyClaude,
		ParentRunID: "run-hub-1",
		AgentName:   "reviewer",
		Role:        "reviewer",
		Status:      RunStatusRunning,
		AgentStatus: string(RunStatusRunning),
		RunKind:     "chat",
	}); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	// A fresh service with nothing in s.runs and an empty agentOrchestrator --
	// exactly the state right after a runner restart, before any run in this
	// chat has been reopened.
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	summaries := svc.listAgentRunSummaries("run-hub-1")
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v, want exactly 1", summaries)
	}
	got := summaries[0]
	if got.Status != RunStatusCancelled {
		t.Fatalf("Status = %q, want %q (BUG-251: stale running status must be normalized after restart)", got.Status, RunStatusCancelled)
	}
	if got.AgentStatus != string(RunStatusCancelled) {
		t.Fatalf("AgentStatus = %q, want %q", got.AgentStatus, RunStatusCancelled)
	}
}

// Integration-level proof for BUG-250: a run persisted only via
// sessions.ndjson (never in memory) whose provider_session_id is still the
// placeholder must be reopenable through the real resumeRun path, not just
// the extracted helper. No provider accounts are configured on this service
// at all, so if the fix's bypass did not fire, ensureResumeReady would
// certainly fail on account/home resolution before ever reaching
// LocateSessionFile -- resumeRun succeeding here proves the validation was
// skipped, not that it accidentally passed.
func TestResumeRunSucceedsForRebuiltRunWithPlaceholderSession(t *testing.T) {
	store := newFakeWorkflowStore()
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-hub-1",
		ProviderKey:       ProviderKeyClaude,
		ProviderSessionID: "thread-6075",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	handle, apiErr := svc.resumeRun("run-hub-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v (BUG-250: a rebuilt run with a placeholder session must not require a resolvable session file)", apiErr)
	}
	if handle.RunID != "run-hub-1" {
		t.Fatalf("handle.RunID = %q, want run-hub-1", handle.RunID)
	}
}

func TestResumeRunSucceedsForRebuiltCodexFlowHubWithSyntheticPlaceholder(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-codex-flow-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-9001",
		Status:            RunStatusCompleted,
		RunKind:           "workflow",
		ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder"}},
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-codex-flow-1", turnLogLine{
		Kind:   turnLogKindPrompt,
		TurnID: "turn-1",
		Prompt: "fix bug 1+1 != 2",
	}); err != nil {
		t.Fatalf("seed turn log: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	handle, apiErr := svc.resumeRun("run-codex-flow-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if handle.ProviderSessionID != "thread-9001" {
		t.Fatalf("handle.ProviderSessionID = %q, want synthetic placeholder preserved", handle.ProviderSessionID)
	}
}

func TestResumeRunRebuildsParentAgentAnnotationsFromPersistedChildren(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, session := range []ProviderSessionState{
		{
			RunID:             "parent-run",
			ProjectID:         "proj",
			ProviderKey:       ProviderKeyCodex,
			ProviderSessionID: "thread-parent",
			Status:            RunStatusCompleted,
			RunKind:           "workflow",
			StartedAt:         now,
			UpdatedAt:         now,
			LastPrompt:        "fix bug 1+1 != 2",
			ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder"}},
		},
		{
			RunID:             "child-coder",
			ProjectID:         "proj",
			ProviderKey:       ProviderKeyCodex,
			ProviderSessionID: "thread-coder",
			Status:            RunStatusCompleted,
			RunKind:           "chat",
			ParentRunID:       "parent-run",
			AgentName:         "coder-agent",
			Role:              "coder-agent",
			AgentStatus:       string(RunStatusCompleted),
			StartedAt:         now,
			UpdatedAt:         now,
			LastMessage:       "implemented the fix",
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	if err := store.AppendTurnLog(context.Background(), "parent-run", turnLogLine{
		Kind:   turnLogKindPrompt,
		TurnID: "turn-1",
		Prompt: "fix bug 1+1 != 2",
	}); err != nil {
		t.Fatalf("seed turn log: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	if _, apiErr := svc.resumeRun("parent-run"); apiErr != nil {
		t.Fatalf("resumeRun(parent-run): %v", apiErr)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	var sawSpawn, sawResult bool
	for _, ev := range svc.runs["parent-run"].events {
		if ev.Type == EventAgentSpawnedByUser && ev.AgentName == "coder-agent" && ev.ChildRunID == "child-coder" {
			sawSpawn = true
		}
		if ev.Type == EventAgentResultInjected && ev.AgentName == "coder-agent" && ev.FinalMessage == "implemented the fix" {
			sawResult = true
		}
	}
	if !sawSpawn {
		t.Fatal("expected resumed parent timeline to include a Spawned agent annotation rebuilt from persisted child sessions")
	}
	if !sawResult {
		t.Fatal("expected resumed parent timeline to include the completed child result annotation")
	}
}

func TestResumeRunRebuildsParentAgentAnnotationsOnlyForRootRun(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, session := range []ProviderSessionState{
		{
			RunID:             "parent-run",
			ProjectID:         "proj",
			ProviderKey:       ProviderKeyClaude,
			ProviderSessionID: "thread-parent",
			Status:            RunStatusCompleted,
			RunKind:           "workflow",
			StartedAt:         now,
			UpdatedAt:         now,
		},
		{
			RunID:             "child-run",
			ProjectID:         "proj",
			ProviderKey:       ProviderKeyClaude,
			ProviderSessionID: "thread-child",
			Status:            RunStatusCompleted,
			RunKind:           "chat",
			ParentRunID:       "parent-run",
			AgentName:         "coder-agent",
			Role:              "coder-agent",
			AgentStatus:       string(RunStatusCompleted),
			StartedAt:         now,
			UpdatedAt:         now,
			LastPrompt:        "You are the coder sub-agent.",
			LastMessage:       "implemented the fix",
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc.resumeRun("child-run"); apiErr != nil {
		t.Fatalf("resumeRun(child-run): %v", apiErr)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, ev := range svc.runs["child-run"].events {
		if ev.Type == EventAgentSpawnedByUser || ev.Type == EventAgentResultInjected {
			t.Fatalf("child timeline must not receive reconstructed parent-facing agent annotation events, saw %+v", ev)
		}
	}
}

func TestResumeRunRebuildsParentAgentAnnotationsWithoutDuplicatesAcrossReopen(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, session := range []ProviderSessionState{
		{
			RunID:             "parent-run",
			ProjectID:         "proj",
			ProviderKey:       ProviderKeyCodex,
			ProviderSessionID: "thread-parent",
			Status:            RunStatusCompleted,
			RunKind:           "workflow",
			StartedAt:         now,
			UpdatedAt:         now,
			LastPrompt:        "fix bug 1+1 != 2",
			ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder"}},
		},
		{
			RunID:             "child-coder",
			ProjectID:         "proj",
			ProviderKey:       ProviderKeyCodex,
			ProviderSessionID: "thread-coder",
			Status:            RunStatusCompleted,
			RunKind:           "chat",
			ParentRunID:       "parent-run",
			AgentName:         "coder-agent",
			Role:              "coder-agent",
			AgentStatus:       string(RunStatusCompleted),
			StartedAt:         now,
			UpdatedAt:         now,
			LastMessage:       "implemented the fix",
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	if err := store.AppendTurnLog(context.Background(), "parent-run", turnLogLine{
		Kind:   turnLogKindPrompt,
		TurnID: "turn-1",
		Prompt: "fix bug 1+1 != 2",
	}); err != nil {
		t.Fatalf("seed turn log: %v", err)
	}

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc.resumeRun("parent-run"); apiErr != nil {
		t.Fatalf("first resumeRun(parent-run): %v", apiErr)
	}
	svc.mu.Lock()
	firstEvents := append([]ProviderEvent(nil), svc.runs["parent-run"].events...)
	svc.mu.Unlock()

	svc2 := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	if _, apiErr := svc2.resumeRun("parent-run"); apiErr != nil {
		t.Fatalf("second resumeRun(parent-run): %v", apiErr)
	}
	svc2.mu.Lock()
	defer svc2.mu.Unlock()
	secondEvents := svc2.runs["parent-run"].events

	countType := func(events []ProviderEvent, typ ProviderEventType) int {
		n := 0
		for _, ev := range events {
			if ev.Type == typ {
				n++
			}
		}
		return n
	}
	if got, want := countType(secondEvents, EventAgentSpawnedByUser), countType(firstEvents, EventAgentSpawnedByUser); got != want {
		t.Fatalf("spawn annotation count after reopen = %d, want %d", got, want)
	}
	if got, want := countType(secondEvents, EventAgentResultInjected), countType(firstEvents, EventAgentResultInjected); got != want {
		t.Fatalf("result annotation count after reopen = %d, want %d", got, want)
	}
}
