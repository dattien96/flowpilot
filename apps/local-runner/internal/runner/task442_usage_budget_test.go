package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Task-442 (CP-86 P-3): per-node REAL usage budget — cumulative
// Total.TotalTokens from the run's own token_usage_updated events, summed
// across legs (rotation never resets accounting). Exceed is a decision
// point evaluated post-turn only: extend/rotate/stop through the shared
// gate; extend is never auto-selectable.

const task442CoderUsageCap = 500000

// task442ProfiledChild builds a child node run under a flow parent whose
// task-harness "coder" profile carries maxUsageTokens (T-1). Returns
// (service, parentRunID, childRun).
func task442ProfiledChild(t *testing.T) (*InteractiveService, string, *interactiveRun) {
	t.Helper()
	svc, _ := newTestServer(t)
	svc.questionTTL = time.Hour // cards must stay answerable for the test
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	child, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", ContextProfile: "coder"}}
	prs.chatFlowRef = workingmode.PackPrefix + "task-harness"
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.stepID = "coder"
	crs.lastPrompt = strings.Repeat("p", 4000)
	svc.mu.Unlock()
	return svc, parent.RunID, crs
}

func task442EmitUsage(svc *InteractiveService, rs *interactiveRun, session string, total int64) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if session != "" {
		rs.providerSessionID = session
	}
	svc.emitLocked(rs, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Total: &TokenUsageBreakdown{TotalTokens: total},
		},
	})
}

func task442PendingUsageQuestion(svc *InteractiveService, runID string) *questionRecord {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, rec := range svc.questions {
		if rec.runID == runID && rec.kind == usageBudgetQuestionKind && rec.status == "pending" {
			return rec
		}
	}
	return nil
}

func task442OptionLabels(rec *questionRecord) []string {
	out := make([]string, 0, len(rec.options))
	for _, o := range rec.options {
		out = append(out, o.Label)
	}
	return out
}

func TestTask442_CumulativeUsageAccumulatesPerNode(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	task442EmitUsage(svc, child, "leg-a", 100)
	task442EmitUsage(svc, child, "leg-a", 250) // same leg: latest wins, not additive
	svc.mu.Lock()
	got := svc.nodeUsageTokensLocked(child)
	svc.mu.Unlock()
	if got != 250 {
		t.Fatalf("same-leg usage = %d, want latest total 250", got)
	}
	// Rotation → new provider session → its Total restarts; accounting must
	// carry across legs (sum of per-leg latest totals).
	task442EmitUsage(svc, child, "leg-b", 80)
	svc.mu.Lock()
	got = svc.nodeUsageTokensLocked(child)
	svc.mu.Unlock()
	if got != 330 {
		t.Fatalf("cross-leg usage = %d, want 250+80=330", got)
	}
}

func TestTask442_ExceedFiresAfterTurnNotMidTurn(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
	// Mid-turn (check not invoked): no card may exist.
	if rec := task442PendingUsageQuestion(svc, child.id); rec != nil {
		t.Fatal("usage card emitted mid-turn — the check must be post-turn only")
	}
	svc.checkUsageBudgetPostTurn(child, "turn-1")
	if rec := task442PendingUsageQuestion(svc, child.id); rec == nil {
		t.Fatal("post-turn exceed produced no usage_budget_exceeded card")
	}
}

func TestTask442_ExceedEmitsAskUserExtendRotateStop(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	svc.usageRouter = &fakeUsageRouter{}
	task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
	svc.checkUsageBudgetPostTurn(child, "turn-1")
	rec := task442PendingUsageQuestion(svc, child.id)
	if rec == nil {
		t.Fatal("no card emitted on exceed")
	}
	labels := task442OptionLabels(rec)
	want := []string{"extend", "rotate", "stop"}
	if len(labels) != len(want) {
		t.Fatalf("options = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("options = %v, want %v", labels, want)
		}
	}
}

func TestTask442_RotateDelegatesToRoutingGate(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	router := &fakeUsageRouter{}
	svc.usageRouter = router
	task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
	svc.checkUsageBudgetPostTurn(child, "turn-1")
	rec := task442PendingUsageQuestion(svc, child.id)
	if rec == nil {
		t.Fatal("no card emitted")
	}
	if e := svc.AnswerQuestion(rec.id, []string{"rotate"}); e != nil {
		t.Fatalf("AnswerQuestion rotate: %v", e)
	}
	if !router.called || router.runID != child.id {
		t.Fatalf("router not delegated: called=%v runID=%q", router.called, router.runID)
	}
	// Rotation implicitly applies the one-time extension or the next post-turn
	// check would re-fire immediately on the same carried-over total.
	if cap := svc.nodeUsageBudgetFor(child); cap != 2*task442CoderUsageCap {
		t.Fatalf("post-rotate budget = %d, want %d (base + one-time extension)", cap, 2*task442CoderUsageCap)
	}
}

func TestTask442_GateUnavailable_OffersExtendStopOnly(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	svc.usageRouter = nil
	task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
	svc.checkUsageBudgetPostTurn(child, "turn-1")
	rec := task442PendingUsageQuestion(svc, child.id)
	if rec == nil {
		t.Fatal("no card emitted")
	}
	for _, l := range task442OptionLabels(rec) {
		if l == "rotate" {
			t.Fatalf("options = %v, rotate must not appear without the routing gate", task442OptionLabels(rec))
		}
	}
}

func TestTask442_AutoModeNeverSelectsExtend(t *testing.T) {
	opts := []QuestionOption{{Label: "extend"}, {Label: "rotate"}, {Label: "stop"}}
	for _, o := range usageBudgetAutoSelectable(opts) {
		if o.Label == "extend" {
			t.Fatal("auto mode must never select extend — it mutates the user's declared budget")
		}
	}
}

func TestTask442_ExtendChoiceRaisesBudgetOnce(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
	svc.checkUsageBudgetPostTurn(child, "turn-1")
	rec := task442PendingUsageQuestion(svc, child.id)
	if rec == nil {
		t.Fatal("no card emitted")
	}
	if e := svc.AnswerQuestion(rec.id, []string{"extend"}); e != nil {
		t.Fatalf("AnswerQuestion extend: %v", e)
	}
	if cap := svc.nodeUsageBudgetFor(child); cap != 2*task442CoderUsageCap {
		t.Fatalf("budget after extend = %d, want %d", cap, 2*task442CoderUsageCap)
	}
	// Still over the raised cap after the NEXT turn → re-ask.
	task442EmitUsage(svc, child, "", 2*task442CoderUsageCap+1)
	svc.checkUsageBudgetPostTurn(child, "turn-2")
	if task442PendingUsageQuestion(svc, child.id) == nil {
		t.Fatal("second exceed past the raised cap must re-ask")
	}
}

func TestTask442_StopChoiceEscalatesNode(t *testing.T) {
	svc, parentID, child := task442ProfiledChild(t)
	task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
	svc.checkUsageBudgetPostTurn(child, "turn-1")
	rec := task442PendingUsageQuestion(svc, child.id)
	if rec == nil {
		t.Fatal("no card emitted")
	}
	if e := svc.AnswerQuestion(rec.id, []string{"stop"}); e != nil {
		t.Fatalf("AnswerQuestion stop: %v", e)
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID).Status; st != "blocked" {
		t.Fatalf("parent loop status = %q, want blocked (escalate edge)", st)
	}
}

func TestTask442_NoProfileOrNoCap_Uncapped(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.questionTTL = time.Hour
	rs, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	run := svc.runs[rs.RunID]
	svc.mu.Unlock()
	task442EmitUsage(svc, run, "", 9_999_999)
	svc.checkUsageBudgetPostTurn(run, "turn-1")
	if rec := task442PendingUsageQuestion(svc, run.id); rec != nil {
		t.Fatal("unprofiled run must never produce a usage card")
	}
}

func TestTask442_ProviderSilentOnUsage_CapNeverFires(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	svc.runner = &Runner{workspace: t.TempDir()}
	svc.checkUsageBudgetPostTurn(child, "turn-silent")
	if rec := task442PendingUsageQuestion(svc, child.id); rec != nil {
		t.Fatal("cap must never fire when the provider reports no usage")
	}
	line := task442ReadUsageAuditLine(t, svc.runner.workspace, child.id, "turn-silent")
	if line["usage_status"] != "no usage data" {
		t.Fatalf("audit usage_status = %v, want 'no usage data'", line["usage_status"])
	}
}

func TestTask442_AuditRecordsEstVsActual(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	svc.runner = &Runner{workspace: t.TempDir()}
	task442EmitUsage(svc, child, "", 777)
	svc.checkUsageBudgetPostTurn(child, "turn-a")
	line := task442ReadUsageAuditLine(t, svc.runner.workspace, child.id, "turn-a")
	if line["actual_usage_tokens"] != float64(777) {
		t.Fatalf("actual_usage_tokens = %v, want 777", line["actual_usage_tokens"])
	}
	if line["est_prompt_tokens"] != float64(1000) { // len(lastPrompt)=4000 → est 1000
		t.Fatalf("est_prompt_tokens = %v, want 1000", line["est_prompt_tokens"])
	}
	if line["usage_budget"] != float64(task442CoderUsageCap) {
		t.Fatalf("usage_budget = %v, want %d", line["usage_budget"], task442CoderUsageCap)
	}
}

func TestTask442_ClaudeCodexGrok_Parity(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		svc, _, child := task442ProfiledChild(t)
		svc.mu.Lock()
		child.providerKey = pk
		svc.mu.Unlock()
		task442EmitUsage(svc, child, "", task442CoderUsageCap+1)
		svc.checkUsageBudgetPostTurn(child, "turn-1")
		if task442PendingUsageQuestion(svc, child.id) == nil {
			t.Fatalf("provider %s: exceed produced no card — accounting must be adapter-agnostic", pk)
		}
	}
}

// TestTask442_GateEarlyReturnStillEnforcesBudget reproduces the live-miss
// found during CP-86 live verification: runChildArtifactOutputGateAtEpoch
// early-returns for a profiled child with no artifact bindings / no coding
// contract (the "no-op" guards) BEFORE the embedded post-turn budget check —
// so a scout/reviewer node burning past maxUsageTokens never produced the
// card. The budget check must live at the post-turn dispatch seam, not
// inside gate-rule evaluation.
func TestTask442_GateEarlyReturnStillEnforcesBudget(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	svc.runner = &Runner{workspace: t.TempDir()}
	svc.mu.Lock()
	child.workspaceCwd = t.TempDir()
	child.providerKey = ProviderKeyDevin
	svc.mu.Unlock()
	// Exceed the cap, then drive the REAL post-turn gate path — a child with
	// no artifact outputs and no coding contract hits the no-op early return.
	task442EmitUsage(svc, child, "leg-a", task442CoderUsageCap+1)
	fin := finalizeInput{RunID: child.id, TurnID: "turn-gate"}
	_ = svc.runChildArtifactOutputGateAtEpoch(context.Background(), child, "turn-gate", fin, -1)
	if task442PendingUsageQuestion(svc, child.id) == nil {
		t.Fatal("usage_budget_exceeded card missing after a no-op gate early return — the check must not live behind gate-rule guards")
	}
}

type fakeUsageRouter struct {
	called bool
	runID  string
}

func (f *fakeUsageRouter) RotateUsageBudgetRun(runID string) error {
	f.called = true
	f.runID = runID
	return nil
}

// task442ReadUsageAuditLine reads the appended usage record (second line)
// from the per-turn prompt-context-audit jsonl.
func task442ReadUsageAuditLine(t *testing.T, workspace, runID, turnID string) map[string]any {
	t.Helper()
	path := filepath.Join(workspace, ".flowpilot", "runs", "proj", runID, "prompt-context-audit-"+turnID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit %s: %v", path, err)
	}
	defer f.Close()
	var last map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err == nil {
			last = m
		}
	}
	if last == nil {
		t.Fatalf("audit %s has no jsonl lines", path)
	}
	return last
}
