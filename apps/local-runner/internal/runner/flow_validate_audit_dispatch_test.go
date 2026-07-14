package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func TestEdgeTargetFromMatchesExactTriple(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "validate", To: "audit", When: "done", Kind: "forward"},
		{From: "validate", To: "ask_user", When: "escalate", Kind: "forward"},
	}
	if got, ok := edgeTargetFrom(edges, "validate", "done", "forward"); !ok || got != "audit" {
		t.Fatalf("done/forward = %q, ok=%v, want audit", got, ok)
	}
	if got, ok := edgeTargetFrom(edges, "validate", "continue", "back"); !ok || got != "implement" {
		t.Fatalf("continue/back = %q, ok=%v, want implement", got, ok)
	}
	if got, ok := edgeTargetFrom(edges, "validate", "escalate", "forward"); !ok || got != "ask_user" {
		t.Fatalf("escalate/forward = %q, ok=%v, want ask_user", got, ok)
	}
	if _, ok := edgeTargetFrom(edges, "audit", "done", "forward"); ok {
		t.Fatal("expected no match for a from-node with no declared edge")
	}
}

func TestLoadValidateCommandReadsBaselineTestCommand(t *testing.T) {
	dir := t.TempDir()
	guardDir := filepath.Join(dir, ".flowpilot", "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	baseline := `{"captured_at":"2026-01-01T00:00:00Z","test_command":"go test ./..."}`
	if err := os.WriteFile(filepath.Join(guardDir, "test_baseline.json"), []byte(baseline), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	cmd, cwd := loadValidateCommand(dir)
	if cmd != "go test ./..." {
		t.Fatalf("command = %q, want %q", cmd, "go test ./...")
	}
	if cwd != dir {
		t.Fatalf("cwd = %q, want %q (no test_dir override)", cwd, dir)
	}
}

func TestLoadValidateCommandEmptyWhenNoBaseline(t *testing.T) {
	dir := t.TempDir()
	cmd, _ := loadValidateCommand(dir)
	if cmd != "" {
		t.Fatalf("command = %q, want empty (no baseline file)", cmd)
	}
}

// flowFixtureEdgesNodes returns rag-harness's exact context/implement/
// validate/audit topology (minus the context node, which callers set up
// separately since these tests start from "implement" already completed).
func flowFixtureEdgesNodes() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "implement", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "validate", To: "audit", When: "done", Kind: "forward"},
		{From: "validate", To: "ask_user", When: "escalate", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	return edges, nodes
}

// TestTryAdvanceFlowFromNodeRunsValidateSkippedNoCommandForwardsToAudit
// verifies BUG-243 F-0/F-1's "no command configured" degrade path: no
// .flowpilot/guard/test_baseline.json in the workspace must forward straight
// to audit (skipped_no_command, per NewFlowValidationRetryState — never a
// fabricated default command), and F-2's audit wiring must persist a real
// EventFlowAuditDraft (not the old stub's RawArgs echo) before settling the
// flow done.
func TestTryAdvanceFlowFromNodeRunsValidateSkippedNoCommandForwardsToAudit(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := flowFixtureEdgesNodes()

	dir := t.TempDir() // no .flowpilot/guard/test_baseline.json -> no command

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "fixed the bug") {
		t.Fatal("expected tryAdvanceFlowFromNode to reach validate->audit and return true")
	}

	events := store.events[parent.RunID]
	var draft *FlowAuditDraft
	for _, ev := range events {
		if ev.Type == EventFlowAuditDraft && ev.FlowAuditDraft != nil {
			draft = ev.FlowAuditDraft
		}
	}
	if draft == nil {
		t.Fatal("expected an EventFlowAuditDraft to be persisted")
	}
	if draft.ValidationResult != "skipped_no_command" {
		t.Fatalf("draft.ValidationResult = %q, want skipped_no_command", draft.ValidationResult)
	}
	// No feature key resolved in this minimal fixture (no context package) —
	// blocked_missing_feature_key is the CORRECT status (BuildAuditDraft's own
	// non-write invariant), proving the real builder ran, not an echo stub.
	if draft.Status == "" {
		t.Fatal("expected BuildAuditDraft's real status classification, not an empty/echoed value")
	}

	if got := svc.agentOrchestrator.loopStateFor(parent.RunID).Status; got != "done" {
		t.Fatalf("loop status = %q, want done (audit settled the flow via applyFlowControl)", got)
	}
}

// TestTryAdvanceFlowFromNodeValidatePassingCommandAdvancesToAudit verifies
// F-1's real command execution: a passing command ("go version") must
// produce status=passed and still reach audit.
func TestTryAdvanceFlowFromNodeValidatePassingCommandAdvancesToAudit(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := flowFixtureEdgesNodes()

	dir := t.TempDir()
	writeBaseline(t, dir, "go version")

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "fixed the bug") {
		t.Fatal("expected tryAdvanceFlowFromNode to run validate, pass, and reach audit")
	}

	var sawValidationResult, sawAuditDraft bool
	var validationStatus string
	for _, ev := range store.events[parent.RunID] {
		if ev.Type == EventFlowValidationResult && ev.FlowValidationResult != nil {
			sawValidationResult = true
			if ev.FlowValidationResult.ExitCode != 0 {
				t.Fatalf("go version exit code = %d, want 0", ev.FlowValidationResult.ExitCode)
			}
		}
		if ev.Type == EventFlowValidationRetry && ev.FlowValidationRetryState != nil {
			validationStatus = ev.FlowValidationRetryState.Status
		}
		if ev.Type == EventFlowAuditDraft {
			sawAuditDraft = true
		}
	}
	if !sawValidationResult {
		t.Fatal("expected a real EventFlowValidationResult from running the command")
	}
	if validationStatus != "passed" {
		t.Fatalf("retry state status = %q, want passed", validationStatus)
	}
	if !sawAuditDraft {
		t.Fatal("expected validate's pass to advance to audit and persist a draft")
	}
}

// TestTryAdvanceFlowFromNodeValidateFailingCommandRetriesCoder verifies F-1's
// retry loop: a failing command must NOT reach audit — it must respawn the
// back-edge target (implement) instead, and record status=retrying.
func TestTryAdvanceFlowFromNodeValidateFailingCommandRetriesCoder(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := flowFixtureEdgesNodes()

	dir := t.TempDir()
	writeBaseline(t, dir, "go build ./this-package-does-not-exist-xyz")

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "fixed the bug") {
		t.Fatal("expected tryAdvanceFlowFromNode to retry (return true), not bail")
	}

	var validationStatus string
	sawAuditDraft := false
	for _, ev := range store.events[parent.RunID] {
		if ev.Type == EventFlowValidationRetry && ev.FlowValidationRetryState != nil {
			validationStatus = ev.FlowValidationRetryState.Status
		}
		if ev.Type == EventFlowAuditDraft {
			sawAuditDraft = true
		}
	}
	if validationStatus != "retrying" {
		t.Fatalf("retry state status = %q, want retrying", validationStatus)
	}
	if sawAuditDraft {
		t.Fatal("a retrying validation must never reach audit")
	}

	svc.mu.Lock()
	got := svc.runs[parent.RunID].flowValidationRetryState
	svc.mu.Unlock()
	if got == nil || got.RetryAttempt != 1 {
		t.Fatalf("in-memory retry state = %#v, want RetryAttempt=1", got)
	}
}

// TestTryAdvanceFlowFromNodeValidateMaxRetriesEscalates verifies F-1's cap:
// once RetryAttempt reaches MaxRetries, the flow must escalate (blocked,
// awaiting user) rather than retry a 4th time or silently pass.
func TestTryAdvanceFlowFromNodeValidateMaxRetriesEscalates(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := flowFixtureEdgesNodes()

	dir := t.TempDir()
	writeBaseline(t, dir, "go build ./this-package-does-not-exist-xyz")

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	// Seed state at the edge of the cap: one more failure pushes RetryAttempt
	// from 2 to 3 (== MaxRetries), which AdvanceRetryState must classify as
	// failed_validation_max_retries, not another "retrying" round.
	rs.flowValidationRetryState = &FlowValidationRetryState{
		ValidationCommand: "go build ./this-package-does-not-exist-xyz",
		MaxRetries:        3,
		RetryAttempt:      2,
		Status:            "retrying",
	}
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "still broken") {
		t.Fatal("expected tryAdvanceFlowFromNode to handle the max-retries case (return true)")
	}

	if got := svc.agentOrchestrator.loopStateFor(parent.RunID).Status; got != "blocked" {
		t.Fatalf("loop status = %q, want blocked (escalated after max retries)", got)
	}

	var validationStatus string
	for _, ev := range store.events[parent.RunID] {
		if ev.Type == EventFlowValidationRetry && ev.FlowValidationRetryState != nil {
			validationStatus = ev.FlowValidationRetryState.Status
		}
	}
	if validationStatus != "failed_validation_max_retries" {
		t.Fatalf("retry state status = %q, want failed_validation_max_retries", validationStatus)
	}
}

// TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget
// verifies BUG-279's fix: a validate-retry back-edge into a node declared
// lifecycle: reinvoke (rag-harness's "implement") must reuse the existing
// child run/provider session for the retry turn, exactly like the
// forward-edge auto-advance path already does — not spawn a brand new child
// (which silently dropped the coder's own memory of the failed attempt).
func TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "fixed"})
				return nil
			})
		},
	})

	store := newFakeWorkflowStore()
	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	edges, nodes := flowFixtureEdgesNodes()
	for i := range nodes {
		if nodes[i].ID == "implement" {
			nodes[i].Lifecycle = "reinvoke"
		}
	}

	dir := t.TempDir()
	const failingCommand = "go build ./this-package-does-not-exist-xyz"
	writeBaseline(t, dir, failingCommand)
	// ensureBaseline (gate_hook.go) fires async on every runTurn and, since this
	// bare temp dir isn't a git repo, always sees the hand-written baseline above
	// as stale (its HeadSHA is empty) and re-captures — which, with no test-config
	// override, falls back to DetectTestRunner and finds nothing in an empty dir,
	// silently emptying the command out from under this test's own initial turn.
	// An explicit test-config.json (CaptureBaseline's higher-priority source)
	// keeps the recapture pinned to the same failing command regardless of when
	// that async goroutine runs relative to the rest of this test.
	settingsDir := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	testConfig := `{"test_command":"` + failingCommand + `"}`
	if err := os.WriteFile(filepath.Join(settingsDir, "test-config.json"), []byte(testConfig), 0o644); err != nil {
		t.Fatalf("write test-config.json: %v", err)
	}

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "initial implement", Label: "implement", Wait: false,
	}); err != nil {
		t.Fatalf("spawnChildRun(implement): %v", err)
	}
	waitLoop(t, "initial implement completed", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, run := range svc.runs {
			// Wait for turnInFlight to clear too, not just status: finishTurn (which
			// resets turnInFlight) runs some real work (git head capture, orchestrator
			// progress, session persistence) after status flips to Completed inside
			// the adapter's synchronous EventTurnCompleted emit, so a bare status
			// check can race ahead of reinvokeMatchingFlowChild's own turnInFlight
			// guard and make it think a turn is still in flight.
			if run.parentRunID == parent.RunID && run.label == "implement" && run.status == RunStatusCompleted && !run.turnInFlight {
				return true
			}
		}
		return false
	})

	before := countChildrenWithLabel(svc, parent.RunID, "implement")
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "fixed the bug") {
		t.Fatal("expected tryAdvanceFlowFromNode to retry (return true), not bail")
	}
	waitLoop(t, "implement reinvoked for retry", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 2
	})
	if after := countChildrenWithLabel(svc, parent.RunID, "implement"); after != before {
		t.Fatalf("implement child count = %d, want unchanged %d for lifecycle=reinvoke retry", after, before)
	}

	var validationStatus string
	for _, ev := range store.events[parent.RunID] {
		if ev.Type == EventFlowValidationRetry && ev.FlowValidationRetryState != nil {
			validationStatus = ev.FlowValidationRetryState.Status
		}
	}
	if validationStatus != "retrying" {
		t.Fatalf("retry state status = %q, want retrying", validationStatus)
	}
}

func writeBaseline(t *testing.T, workspace, command string) {
	t.Helper()
	guardDir := filepath.Join(workspace, ".flowpilot", "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		t.Fatalf("mkdir guard dir: %v", err)
	}
	body := `{"captured_at":"2026-01-01T00:00:00Z","test_command":"` + command + `"}`
	if err := os.WriteFile(filepath.Join(guardDir, "test_baseline.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
}
