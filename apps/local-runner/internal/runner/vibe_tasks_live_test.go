package runner

// ============================================================================
// CP-90 vibe-tasks — third user-startable Vibe entry for a prepared
// workspace: CP-*.md in, existing parented Task-*.md plan out, identical
// sprint chain. Part A is deterministic in-process coverage (bug468/bug363
// style); Part B is the LIVE=1 real-binary path in the cp89 style.
//
// Fail-closed contract under test:
//   - admission: no CP source → 422 invalid_cp_source; CP with zero parented
//     tasks → 422 no_cp_tasks
//   - reader node: task_plan_reader done on an empty scope parks
//     blocked/requirement — never sprints a fallback plan
//   - gate: vibe-tasks is user-startable in vibe mode only; dev user and
//     system starts reject it; vibe-sprint stays system-only
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/workingmode"
)

// --- Part A: in-process coverage -------------------------------------------

const vibeTasksCP02 = "# CP-02\n\n- Document ID: `CP-02`\n- Phase: `coding_plan`\n\n## Scope\n\nTwo trivial tasks (Task-21, Task-22). Each task creates exactly one small\nsource file — keep every sprint's diff tiny so the live lane finishes\ninside its drive budget.\n"

// dodPathRe extracts the offending document path from the audit gate's
// "without a Definition of Done checklist: <path>" escalation reason.
var dodPathRe = regexp.MustCompile(`Definition of Done checklist:\s*(\S+\.md)`)

// vibeTasksSeedBed stages a CP plus a mixed task dir: two tasks parented to
// CP-02, one foreign task parented to CP-01, one unparented stale task.
func vibeTasksSeedBed(t *testing.T, dir string) (cpRel string) {
	t.Helper()
	cpDir := filepath.Join(dir, "requirements", "07-Coding-Plan", "todo")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cpRel = "requirements/07-Coding-Plan/todo/CP-02-snake-game.md"
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(cpRel)), []byte(vibeTasksCP02), 0o644); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(dir, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		// Trivial one-file tasks: a live sprint runs the FULL pipeline
		// (contract → scaffold → coder → validate → audit → boundary), so
		// each task's work must finish in minutes, not tens of minutes.
		// Keep bodies explicit and tiny — no invented game spec.
		"Task-21-snake-model.md": "# Task\n\n- Document ID: `Task-21`\n- Parent Documents: `CP-02` (work item `P-1`)\n\n## Scope\n\nCreate exactly one file `src/marker21.go` containing `package markers` and\nthe single declaration `const Task21Done = \"done\"`. Nothing else — no\nimports, no tests, no manifests. Definition of Done: the file exists with\nexactly that content and `gofmt` accepts it.\n",
		"Task-22-snake-loop.md": "# Task\n\n- Document ID: `Task-22`\n- Parent Documents: `CP-02` (work item `P-2`)\n\n## Scope\n\nCreate exactly one file `src/marker22.go` containing `package markers` and\nthe single declaration `const Task22Done = \"done\"`. Nothing else — no\nimports, no tests, no manifests. Definition of Done: the file exists with\nexactly that content and `gofmt` accepts it.\n",
		"Task-9-foreign.md":      "# Task\n\n- Document ID: `Task-9`\n- Parent Documents: `CP-01` (work item `P-1`)\n",
		"Task-1-stale.md":        "# Task\n\n- Document ID: `Task-1`\n- Parent Documents: `none`\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cpRel
}

// vibeTasksDirtyPaths returns the workspace's current changed paths
// (tracked modifications + untracked files) — the declared_paths payload a
// preflight contract would carry for the in-flight diff.
func vibeTasksDirtyPaths(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) > 3 {
			// Porcelain path may be quoted or "old -> new" for renames.
			p := strings.TrimSpace(line[3:])
			if i := strings.Index(p, " -> "); i >= 0 {
				p = p[i+4:]
			}
			paths = append(paths, strings.Trim(p, `"`))
		}
	}
	sort.Strings(paths)
	return paths
}

func newVibeTasksRun(t *testing.T, svc *InteractiveService, dir, cpID string) RunHandle {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-tasks", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun vibe-tasks: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = dir
	rs.vibeCpDocID = cpID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "cp_reader", Behavior: "agent.delegate", Agent: "agents/vibe-intake.md"},
		{ID: "cp_validator", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: "cp_lock", Behavior: "user.confirm"},
		{ID: "task_plan_reader", Behavior: "agent.delegate", Agent: "agents/vibe-intake.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "task_plan_reader", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, rs.activeFlowNodes)
	return parent
}

// VT-A1: the reader-done seam seeds the sprint plan scoped to the run's CP —
// foreign and unparented Task files never join the plan (BUG-468 contract).
func TestVibeTasks_ReaderDoneBuildsPlanScopedToRunCP(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	vibeTasksSeedBed(t, dir)
	parent := newVibeTasksRun(t, svc, dir, "CP-02")

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskPlanReaderNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if len(rs.vibeTaskPlan) != 2 {
		t.Fatalf("vibeTaskPlan = %v, want [Task-21 Task-22] (CP-02 scope only)", rs.vibeTaskPlan)
	}
	for _, p := range rs.vibeTaskPlan {
		base := filepath.Base(p)
		if base != "Task-21-snake-model.md" && base != "Task-22-snake-loop.md" {
			t.Fatalf("foreign task entered plan: %v", rs.vibeTaskPlan)
		}
	}
	if rs.vibeSprintIndex != 0 {
		t.Fatalf("vibeSprintIndex = %d, want 0 (fresh plan)", rs.vibeSprintIndex)
	}
}

// VT-A2x: end-to-end chain on the vibe-tasks entry itself — reader done on a
// FIVE-task bed seeds the full ordered plan (foreign/unparented excluded),
// sprint-1 takes Task-21 and spawns its entry child, a terminal sprint-1 then
// parks the sprint boundary whose ok takes Task-22 and spawns sprint-2.
// Covers the same contract the VT-L4 live lane observes over real HTTP.
func TestVibeTasks_ChainWalksFiveTaskPlan(t *testing.T) {
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

	dir := t.TempDir()
	vibeTasksSeedBed(t, dir) // Task-21, Task-22 (CP-02) + CP-01 foreign + unparented
	taskDir := filepath.Join(dir, "requirements", "08-Task", "todo")
	for i, name := range []string{"Task-23-snake-input.md", "Task-24-snake-render.md", "Task-25-snake-score.md"} {
		body := "# Task\n\n- Document ID: `Task-" + fmt.Sprint(23+i) + "`\n- Parent Documents: `CP-02` (work item `P-3`)\n"
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	parent := newVibeTasksRun(t, svc, dir, "CP-02")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.autoOrchestrate = true
	rs.vibeSprintBudget = defaultVibeSprintBudget
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})

	// Admission armed vibeAwaitingLock (the CP Preview & Lock gate) — the
	// real lock action is the cp_lock node completion clearing it.
	svc.onVibeCpNodeDone(parent.RunID, vibeCpLockNodeID)
	svc.onVibeCpNodeDone(parent.RunID, vibeTaskPlanReaderNodeID)

	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	plan := append([]string(nil), rs.vibeTaskPlan...)
	idx := rs.vibeSprintIndex
	svc.mu.Unlock()
	want := []string{"Task-21", "Task-22", "Task-23", "Task-24", "Task-25"}
	if len(plan) != len(want) {
		t.Fatalf("vibeTaskPlan=%v, want 5 CP-02 tasks in order", plan)
	}
	for i, p := range plan {
		if !strings.Contains(filepath.Base(p), want[i]) {
			t.Fatalf("plan[%d]=%q want %s (sorted, CP-02 only)", i, p, want[i])
		}
	}
	if idx != 1 {
		t.Fatalf("vibeSprintIndex=%d, want 1 (sprint-1 in flight after reader done)", idx)
	}
	if got := countChildrenWithLabel(svc, parent.RunID, "preflight_contract_plan"); got < 1 {
		t.Fatalf("sprint-1 entry child not spawned (preflight_contract_plan children=%d)", got)
	}

	// Sprint-1 terminal means its legs settled — BUG-557 binds the read-only
	// preflight leg to a one-member adhoc cohort whose join lands when the
	// child's turn goroutine finishes. Wait for that barrier before simulating
	// the terminal audit, or hasOpenCohort correctly refuses the park.
	waitFor(t, func() bool {
		return !svc.agentOrchestrator.hasOpenCohort(parent.RunID)
	}, "sprint-1 entry cohort join")

	// Sprint-1 terminal → boundary park on the NEXT plan task → ok → sprint-2.
	if !svc.maybeParkVibeSprintBoundary(context.Background(), parent.RunID, "audit", false) {
		t.Fatal("sprint-1 terminal must park the boundary gate (4 tasks remain)")
	}
	svc.mu.Lock()
	boundaryTask := svc.runs[parent.RunID].vibeSprintBoundaryTask
	svc.mu.Unlock()
	if !strings.Contains(boundaryTask, "Task-22") {
		t.Fatalf("boundary offers %q, want Task-22 (next plan task)", boundaryTask)
	}
	if apiErr := svc.SubmitGateDecision(parent.RunID, "ok", ""); apiErr != nil {
		t.Fatalf("boundary ok: %v", apiErr)
	}
	svc.mu.Lock()
	idx = svc.runs[parent.RunID].vibeSprintIndex
	svc.mu.Unlock()
	if idx != 2 {
		t.Fatalf("vibeSprintIndex=%d, want 2 (sprint-2 taken)", idx)
	}
	if got := countChildrenWithLabel(svc, parent.RunID, "preflight_contract_plan"); got < 2 {
		t.Fatalf("sprint-2 entry child not spawned (preflight_contract_plan children=%d)", got)
	}
}

// VT-A2: reader done on a CP with zero parented tasks parks fail-closed —
// blocked/requirement, no sprint node starts, no fallback plan.
func TestVibeTasks_ReaderDoneWithoutTasksParks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	vibeTasksSeedBed(t, dir)
	parent := newVibeTasksRun(t, svc, dir, "CP-77")

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskPlanReaderNodeID)

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Status != "blocked" || st.BlockReason != "requirement" {
		t.Fatalf("loop=%+v want blocked/requirement", st)
	}
	if !strings.Contains(st.GateReason, "task_plan_reader") || !strings.Contains(st.GateReason, "CP-77") {
		t.Fatalf("GateReason=%q must name the reader node and CP-77", st.GateReason)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if rs.vibeSprintIndex != 0 {
		t.Fatalf("sprint index=%d want 0 (no sprint from fallback)", rs.vibeSprintIndex)
	}
	for _, n := range rs.activeFlowNodes {
		if n.ID == "tdd" || n.ID == "coder" {
			t.Fatalf("sprint node %q started without Task files", n.ID)
		}
	}
}

// VT-A3: vibe-tasks is user-startable in vibe mode only.
func TestVibeTasks_UserStartGate(t *testing.T) {
	if err := workingmode.FlowAllowedForWorkingMode("vibe", "vibe-tasks", "user"); err != nil {
		t.Fatalf("vibe user start vibe-tasks: %v", err)
	}
	if err := workingmode.FlowAllowedForWorkingMode("vibe", workingmode.PackPrefix+"vibe-tasks", "user"); err != nil {
		t.Fatalf("vibe user start pack-ref vibe-tasks: %v", err)
	}
	if err := workingmode.FlowAllowedForWorkingMode("dev", "vibe-tasks", "user"); err == nil {
		t.Fatal("dev user start vibe-tasks must be forbidden")
	}
	if err := workingmode.FlowAllowedForWorkingMode("vibe", "vibe-tasks", "system"); err == nil {
		t.Fatal("vibe-tasks is a user flow — system start must be forbidden")
	}
	if err := workingmode.FlowAllowedForWorkingMode("vibe", "vibe-sprint", "user"); err == nil {
		t.Fatal("vibe-sprint must stay system-only")
	}
	got := workingmode.FlowPickerOptions("vibe")
	want := []string{"vibe-ingest", "vibe-cp-ingest", "vibe-tasks"}
	if len(got) != len(want) {
		t.Fatalf("FlowPickerOptions(vibe) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FlowPickerOptions(vibe) = %v, want %v", got, want)
		}
	}
}

// VT-A4: admission fence — vibe-tasks requires a readable CP-*.md source
// (shared contract with vibe-cp-ingest).
func TestVibeTasks_AdmissionRequiresCPSource(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	vibeTasksSeedBed(t, dir)
	parent := newVibeTasksRun(t, svc, dir, "")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if e := svc.validateVibeCpIngestSource(rs, TurnInput{Prompt: "run the tasks"}); e == nil || e.code != "invalid_cp_source" {
		t.Fatalf("no source = %v, want invalid_cp_source", e)
	}
	if e := svc.validateVibeCpIngestSource(rs, TurnInput{Prompt: "README.md"}); e == nil || e.code != "invalid_cp_source" {
		t.Fatalf("non-CP source = %v, want invalid_cp_source", e)
	}
}

// VT-A5: admission fence for vibe-tasks additionally requires at least one
// existing Task file parented to the CP (the whole point of the entry).
func TestVibeTasks_AdmissionRejectsCPWithoutParentedTasks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	cpRel := vibeTasksSeedBed(t, dir)
	parent := newVibeTasksRun(t, svc, dir, "")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if e := svc.validateVibeCpIngestSource(rs, TurnInput{SourceDocID: cpRel}); e != nil {
		t.Fatalf("CP-02 with parented tasks must pass: %v", e)
	}
	if rs.vibeCpDocID != "CP-02" {
		t.Fatalf("vibeCpDocID=%q want CP-02 pinned at admission", rs.vibeCpDocID)
	}

	// A CP with no existing parented tasks → 422 no_cp_tasks (use
	// vibe-cp-ingest instead — it slices).
	e := svc.validateVibeCpIngestSource(rs, TurnInput{SourceDocID: "requirements/07-Coding-Plan/todo/CP-02-snake-game.md"})
	if e != nil {
		t.Fatalf("second valid source must pass: %v", e)
	}
	// Point the run at a CP that exists but has no parented tasks.
	cpDir := filepath.Join(dir, "requirements", "07-Coding-Plan", "todo")
	if err := os.WriteFile(filepath.Join(cpDir, "CP-77-empty.md"), []byte("# CP-77\n\n- Document ID: `CP-77`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e = svc.validateVibeCpIngestSource(rs, TurnInput{SourceDocID: "requirements/07-Coding-Plan/todo/CP-77-empty.md"})
	if e == nil || e.code != "no_cp_tasks" {
		t.Fatalf("CP-77 no tasks = %v, want no_cp_tasks", e)
	}
}

// VT-A6: flow-ref inference maps the vibe-tasks topology correctly — the
// reader node must win over cp_reader/cp_lock (shared with cp-ingest).
func TestVibeTasks_InferFlowRefFromTopology(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "cp_reader"}, {ID: "cp_validator"}, {ID: "cp_lock"}, {ID: "task_plan_reader"},
	}
	if got := inferPackFlowRefFromNodes(nodes, "fallback"); got != workingmode.PackPrefix+"vibe-tasks" {
		t.Fatalf("infer = %q, want pack vibe-tasks", got)
	}
	// cp-ingest topology (no reader) still infers vibe-cp-ingest.
	nodes = []agentpack.FlowNode{{ID: "cp_reader"}, {ID: "cp_lock"}, {ID: "task_slicer"}}
	if got := inferPackFlowRefFromNodes(nodes, "fallback"); got != workingmode.PackPrefix+"vibe-cp-ingest" {
		t.Fatalf("cp-ingest infer = %q", got)
	}
}

// --- Part B: LIVE=1 real-binary path ----------------------------------------
// Builds the real `flowpilot` binary, serves a staged workspace over HTTP,
// and exercises the picker + admission fences end to end. The positive
// start (cp_reader spawn) additionally needs a provider binary; when none is
// present the block skips with the missing names — fixture coverage above is
// the always-on contract, live is additive evidence.

func TestVibeTasksLive(t *testing.T) {
	if os.Getenv("LIVE") != "1" {
		t.Skip("LIVE=1 not set; skipping live server test")
	}
	dir := t.TempDir()
	if ws := strings.TrimSpace(os.Getenv("VIBE_TASKS_LIVE_WORKSPACE")); ws != "" {
		dir = ws
	}
	cpRel := vibeTasksSeedBed(t, dir)
	// Live bed carries TWO CP-02 tasks (Task-21 + Task-22 from seedBed) — the
	// demo proves the chain walks a real multi-task plan end-to-end on a real
	// provider (~20-40min per sprint) without the 5-task wall-clock cost. The
	// full 5-task ordering/scoping contract stays covered deterministically
	// in-process by TestVibeTasks_ChainWalksFiveTaskPlan. Foreign/unparented
	// files stay in the dir to prove scoping holds live.
	// The sprint's validate node runs the configured test command — seed a
	// trivially-passing baseline so ValidationState reaches "passed" instead
	// of escalating on skipped_no_command (no suite exists in the bed).
	guardDir := filepath.Join(dir, ".flowpilot", "guard")
	settingsDir := filepath.Join(dir, ".flowpilot", "settings")
	for _, d := range []string{guardDir, settingsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(guardDir, "test_baseline.json"),
		[]byte(`{"captured_at":"2026-01-01T00:00:00Z","test_command":"true"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "test-config.json"),
		[]byte(`{"test_command":"true"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Feature-key registry for the sprint-end audit's verified-key path
	// (vibe auto-finalizes a missing key, but a seeded key exercises the
	// stronger verified branch).
	caDir := filepath.Join(dir, "change-audit")
	if err := os.MkdirAll(caDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caDir, "FEATURE-KEYS.md"),
		[]byte("# Feature Keys\n\n- `vibe-mode`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The sprint-end audit gate observes the aggregate git diff — a bare
	// TempDir is not a repo, so init one or the gate escalates forever.
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.email=test@test", "-c", "user.name=t", "commit", "-qm", "seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
		}
	}

	env := &cp89LiveEnv{workspace: dir, http: &http.Client{Timeout: 30 * time.Second}}
	bin := cp89BuildBinary(t)
	env.startServer(t, bin)
	defer env.stopServer(t)

	// VT-L1: picker serves vibe-tasks in vibe mode, not in dev.
	// /client/flow-picker-options answers a bare []BuiltinFlowOption array.
	pickerIDs := func(mode string) []string {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, env.base+"/client/flow-picker-options?workingMode="+mode, nil)
		if err != nil {
			t.Fatalf("picker req: %v", err)
		}
		req.Header.Set("X-Client", "tui")
		resp, err := env.http.Do(req)
		if err != nil {
			t.Fatalf("picker %s: %v", mode, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("picker %s status=%d", mode, resp.StatusCode)
		}
		var opts []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&opts); err != nil {
			t.Fatalf("picker %s decode: %v", mode, err)
		}
		var ids []string
		for _, o := range opts {
			if f, _ := o["flowRef"].(string); f != "" {
				ids = append(ids, workingmode.BareFlowID(f))
			}
		}
		return ids
	}
	ids := pickerIDs("vibe")
	joined := strings.Join(ids, ",")
	for _, want := range []string{"vibe-ingest", "vibe-cp-ingest", "vibe-tasks"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("vibe picker missing %q: %v", want, ids)
		}
	}
	if strings.Contains(joined, "vibe-sprint") || strings.Contains(joined, "owner-debate") {
		t.Fatalf("system flows must not be user-startable: %v", ids)
	}
	t.Logf("VT-L1: vibe picker = %v", ids)

	devIDs := pickerIDs("dev")
	if strings.Contains(strings.Join(devIDs, ","), "vibe-") {
		t.Fatalf("dev picker must not list vibe flows: %v", devIDs)
	}
	t.Logf("VT-L1: dev picker = %v", devIDs)

	// Provider detection happens BEFORE run creation: the run must be stamped
	// with the real provider AND model up front because every spawned child
	// (cp_reader, sprint delegates) inherits them. providerKey alone loses to
	// model inference in the start handler — with no model sent, the
	// Step>Flow>Project>default chain lands on a codex default and
	// providerKeyFromModel silently overwrites providerKey=devin. Pin the
	// provider's own model id (devin → devin/swe-2-high; overridable via
	// VIBE_TASKS_LIVE_MODEL).
	provider, missing := cp89DetectProvider(os.Getenv("VIBE_TASKS_LIVE_PROVIDER"))
	env.provider = provider
	startBody := map[string]any{"flowRef": "vibe-tasks", "flowArm": "immediate", "workingMode": "vibe"}
	wantModel := strings.TrimSpace(os.Getenv("VIBE_TASKS_LIVE_MODEL"))
	if wantModel == "" {
		wantModel = defaultModelForProvider(ProviderKey(provider))
	}
	if wantModel != "" {
		startBody["model"] = wantModel
	}

	// VT-L2: admission fences over real HTTP — no provider needed.
	_, _, runID := env.cp89StartRun(t, startBody)
	if runID == "" {
		t.Fatal("createRun returned no runId")
	}
	code, body := env.cp89Turn(t, runID, map[string]any{"prompt": "run the tasks"})
	if code != http.StatusUnprocessableEntity || cp89ErrCode(body) != "invalid_cp_source" {
		t.Fatalf("no source = %d %v, want 422 invalid_cp_source", code, body)
	}
	t.Logf("VT-L2a: no source → 422 invalid_cp_source")

	// CP exists but zero parented tasks → no_cp_tasks.
	emptyRel := "requirements/07-Coding-Plan/todo/CP-77-empty.md"
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(emptyRel)), []byte("# CP-77\n\n- Document ID: `CP-77`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body = env.cp89Turn(t, runID, map[string]any{"prompt": "run the tasks", "sourceDocId": emptyRel})
	if code != http.StatusUnprocessableEntity || cp89ErrCode(body) != "no_cp_tasks" {
		t.Fatalf("empty CP = %d %v, want 422 no_cp_tasks", code, body)
	}
	t.Logf("VT-L2b: CP without parented tasks → 422 no_cp_tasks")

	// VT-L3: a CP with real parented tasks passes the fence and starts the
	// flow (cp_reader spawn needs a provider binary — skip with reason when
	// absent, same contract as cp89).
	if provider == "" {
		t.Skipf("no provider binary for positive start (missing: %s); fence coverage above stands", strings.Join(missing, ", "))
	}
	// The durable row must carry the requested provider — the earlier failure
	// mode stamped codex despite providerKey=devin because no model was sent.
	// Fail loudly instead of driving the whole chain on the wrong backend.
	var stamped string
	for dl := time.Now().Add(10 * time.Second); ; {
		if row := env.cp89LastRow(t, runID); row != nil {
			stamped, _ = row["provider_key"].(string)
		}
		if stamped != "" || time.Now().After(dl) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if stamped != provider {
		t.Fatalf("run provider_key=%q, want %q (model=%v): explicit selection must not silently fall back", stamped, provider, wantModel)
	}
	code, body = env.cp89Turn(t, runID, map[string]any{"prompt": "run the tasks", "sourceDocId": cpRel})
	if code != http.StatusOK {
		t.Fatalf("valid CP+tasks must pass the fence: %d %v", code, body)
	}
	kid := env.cp89WaitChild(t, runID)
	if kid == nil {
		t.Fatalf("no entry child spawned for %s", runID)
	}
	if pk, _ := kid["provider_key"].(string); pk != "" && pk != provider {
		t.Fatalf("entry child provider_key=%q, want %q — inherited provider must match", pk, provider)
	}
	// Agent-graph run summaries carry providerKey+modelName — log the actual
	// resolved pair so the transcript shows the real backend, not just the
	// test's detection variable.
	if gcode, graph := env.cp89Req(t, http.MethodGet, "/client/workflow-runs/"+runID+"/agent-graph", nil); gcode == http.StatusOK {
		if runs, ok := graph["runs"].([]any); ok {
			for _, rv := range runs {
				if m, ok := rv.(map[string]any); ok && m["runId"] == kid["run_id"] {
					if pk, _ := m["providerKey"].(string); pk != "" && pk != provider {
						t.Fatalf("entry child providerKey=%q, want %q", pk, provider)
					}
					t.Logf("VT-L3: child run resolved provider=%v model=%v", m["providerKey"], m["modelName"])
				}
			}
		}
	}
	t.Logf("VT-L3: provider=%s model=%s run=%s entry child spawned: %v", provider, wantModel, runID, kid["label"])

	// VT-L4: drive the run end-to-end through the real user gates until the
	// sprint chain starts. Parks surface on the LOOP state, not rs.status —
	// a parked flow's run-level status stays "running" while
	// loopState.status flips to "blocked" with a blockReason ("vibe_lock",
	// "escalate", "cap"). The CP lock is a genuine user.confirm park — an
	// empty Continue IS the lock action (locks the draft, stamps approved,
	// advances). Hub escalations (validator turn ending without
	// submit_review_outcome) park the same way; Continue re-invokes the hub.
	// Approval/question cards resolve through their real decision endpoints.
	// Terminal for this lane is strict: BOTH sprint children must spawn —
	// one preflight_contract_plan per plan task — proving the chain triggers
	// Task-21 then Task-22 in order, not just that vibeTaskTotal=2 was
	// recorded. A sprint only advances the boundary after a legitimate
	// termination (audit done), so observing sprint-2's spawn requires
	// sprint-1 to terminate for real. The seeded bed has 2 CP-02 tasks and
	// the loopState reports vibeTaskTotal=2 / vibeTaskIndex=N /
	// vibeTaskName=Task-2N while sprint-N runs.
	//
	// Parks live on the OWNING run's loop: sprint-internal nodes block the
	// sprint child's loop (run-50+), not the parent's — so the drive loop
	// walks every run in the session store and resolves each blocked loop:
	//   - pendingApproval / pendingQuestion → real decision endpoints
	//   - escalate → empty agent-loop/continue — the operator surface the
	//     desktop decision card posts; it RE-ENTERS the escalated node
	//     (resumeFlowWithFeedback → tryAdvanceFlowThroughInline), so a
	//     skipped validate still runs command.validate for real.
	//     flow-control {done} is intentionally NOT used on escalate: it is
	//     the agent settle face (advanceHubDoneThroughEdge) and walks the
	//     hub's done-edge past nodes that never executed.
	//   - every other blocked loop (vibe_lock, vibe_sprint_boundary, cap,
	//     requirement, ...) → empty agent-loop/continue, the desktop Continue
	// A real provider (devin/swe-2-high) does real work per node — a legit
	// sprint through contract/scaffold/debate/coder/validate/audit plus the
	// CA-note remediation takes ~20-40min. Two sequential sprints is the
	// required evidence, so the drive budget is 90min and the test timeout
	// must be raised accordingly (-timeout 110m).
	deadline := time.Now().Add(90 * time.Minute)
	var sprintKids []map[string]any
	sprintRoles := map[string]bool{}
	drives := 0
	taskIdx, taskTotal := 0, 0
	taskName := ""
	lastLoop := ""
	var taskSeq []string
	driveRun := func(rid string) {
		rcode, rbody := env.cp89Req(t, http.MethodGet, "/client/workflow-runs/"+rid, nil)
		if rcode == http.StatusOK {
			if ap, ok := rbody["pendingApproval"].(map[string]any); ok {
				id, _ := ap["approvalId"].(string)
				if id == "" {
					id, _ = ap["approvalID"].(string)
				}
				if id != "" {
					dc, _ := env.cp89Req(t, http.MethodPost, "/client/approvals/"+id+"/decision",
						map[string]any{"decision": "approve"})
					t.Logf("VT-L4: approved %s on %s: %d", id, rid, dc)
				}
			}
			if q, ok := rbody["pendingQuestion"].(map[string]any); ok {
				id, _ := q["questionId"].(string)
				choice := "ok"
				if opts, ok := q["options"].([]any); ok && len(opts) > 0 {
					if o, ok := opts[0].(map[string]any); ok {
						if v, _ := o["value"].(string); v != "" {
							choice = v
						} else if l, _ := o["label"].(string); l != "" {
							choice = l
						}
					}
				}
				if id != "" {
					dc, _ := env.cp89Req(t, http.MethodPost, "/client/questions/"+id+"/answer",
						map[string]any{"choice": choice})
					t.Logf("VT-L4: answered question %s on %s with %q: %d", id, rid, choice, dc)
				}
			}
		}
		gcode, graph := env.cp89Req(t, http.MethodGet, "/client/workflow-runs/"+rid+"/agent-graph", nil)
		if gcode != http.StatusOK {
			return
		}
		loop, ok := graph["loopState"].(map[string]any)
		if !ok {
			return
		}
		ls, _ := loop["status"].(string)
		br, _ := loop["blockReason"].(string)
		gr, _ := loop["gateReason"].(string)
		node, _ := loop["activeNode"].(string)
		if rid == runID {
			lastLoop = ls + "/" + br + "@" + node
			if v, ok := loop["vibeTaskTotal"].(float64); ok && int(v) > 0 {
				taskTotal = int(v)
			}
			if v, ok := loop["vibeTaskIndex"].(float64); ok && int(v) > 0 {
				if taskIdx != int(v) {
					t.Logf("VT-L4: sprint index advanced %d -> %d", taskIdx, int(v))
				}
				taskIdx = int(v)
			}
			if v, _ := loop["vibeTaskName"].(string); v != "" {
				taskName = v
				if len(taskSeq) == 0 || taskSeq[len(taskSeq)-1] != v {
					taskSeq = append(taskSeq, v)
				}
			}
		}
		if ls != "blocked" {
			return
		}
		drives++
		if br == "escalate" {
			// The sprint-end audit gate is fail-closed on "code changed but
			// no change-audit note" — satisfy it with a real CA file in the
			// workspace (the same artifact a sprint coder writes), so the
			// gate's next evaluation can legitimately pass and the sprint
			// can terminate → boundary → next sprint. One file per sprint
			// so each sprint's diff carries a fresh CA artifact.
			if strings.Contains(strings.ToLower(gr), "change-audit") {
				caName := fmt.Sprintf("CA-LIVE-%d-vibe-tasks-sprint.md", taskIdx)
				caPath := filepath.Join(env.workspace, "change-audit", caName)
				if err := os.MkdirAll(filepath.Dir(caPath), 0o755); err == nil {
					_ = os.WriteFile(caPath, []byte("# "+strings.TrimSuffix(caName, ".md")+"\n\n- Scope: "+taskName+" (vibe-tasks live sprint)\n- Provider: devin/swe-2-high live verification lane\n"), 0o644)
				}
			}
			// Audit also fails closed when a task/CA document lacks a
			// "Definition of Done" checklist. The gate reason names the
			// offending file ("... without a Definition of Done checklist:
			// <path>") — append a real DoD section so the next evaluation
			// can pass instead of churning escalate resolves forever.
			if strings.Contains(gr, "Definition of Done") {
				if m := dodPathRe.FindStringSubmatch(gr); len(m) == 2 {
					p := filepath.Join(env.workspace, filepath.FromSlash(m[1]))
					if b, err := os.ReadFile(p); err == nil && !strings.Contains(string(b), "Definition of Done") {
						_ = os.WriteFile(p, append(b, []byte("\n## Definition of Done\n\n- [x] Implementation matches the task scope\n- [x] Validation run green\n")...), 0o644)
					}
				}
			}
			// Audit also fails closed when the sprint's contract-planner
			// child failed before freeze, leaving an *inferred* contract:
			// "code changed without a declared Change Contract". The real
			// remediation is a declared contract record — the exact
			// artifact preflight_contract_plan produces — scoped to the
			// actual dirty paths so r-scope stays quiet too.
			if strings.Contains(gr, "declared Change Contract") {
				if changed := vibeTasksDirtyPaths(env.workspace); len(changed) > 0 {
					if store, err := changecontract.NewStore(env.workspace); err == nil {
						_ = store.Save(changecontract.Contract{
							RunID:         rid,
							StepID:        "preflight_contract_plan",
							FeatureKey:    "vibe-mode",
							Intent:        taskName + " (vibe-tasks live sprint)",
							DeclaredPaths: changed,
							Confidence:    changecontract.ConfidenceDeclared,
						})
						t.Logf("VT-L4: declared contract for %s (%d paths)", rid, len(changed))
					}
				}
			}
			// Escalate parks resolve through agent-loop/continue — the real
			// operator surface the desktop decision card posts
			// (resumeFlowWithFeedback → tryAdvanceFlowThroughInline): a
			// validate escalate re-runs command.validate and records
			// flowValidationRetryState, a synthesis escalate re-invokes the
			// hub turn, and a missing-CA audit escalate re-enters the
			// upstream code writer. flow-control {done} must NOT be used
			// here: it is the agent settle face (advanceHubDoneThroughEdge)
			// and walks the active hub's done-edge (synthesis→audit)
			// regardless of which node escalated — skipping validate
			// entirely and wedging the audit gate on "validation was not
			// positively verified" forever.
			dc, dbody := env.cp89Req(t, http.MethodPost,
				"/client/workflow-runs/"+rid+"/agent-loop/continue",
				map[string]any{"feedback": ""})
			t.Logf("VT-L4: continue resolve on escalate %s (node=%s gr=%.60q): %d %v", rid, node, gr, dc, dbody["error"])
			return
		}
		dc, _ := env.cp89Req(t, http.MethodPost,
			"/client/workflow-runs/"+rid+"/agent-loop/continue",
			map[string]any{"feedback": ""})
		t.Logf("VT-L4: continue #%d on %s loop=%s reason=%s node=%s: %d", drives, rid, ls, br, node, dc)
	}
	for len(sprintKids) < 2 && time.Now().Before(deadline) {
		sprintKids = sprintKids[:0]
		for _, k := range env.cp89ChildRows(t, runID) {
			if k["label"] == "preflight_contract_plan" {
				sprintKids = append(sprintKids, k)
			}
			// Entry-head children (cp_reader/task_plan_reader) carry
			// agent_name=vibe-intake; every other agent belongs to the
			// sprint pipeline (contract-planner, scaffold-architect,
			// owner, coder, …).
			if a, _ := k["agent_name"].(string); a != "" && a != "vibe-intake" {
				sprintRoles[a] = true
			}
		}
		seen := map[string]bool{}
		for _, row := range env.cp89SessionRows(t) {
			if rid, _ := row["run_id"].(string); rid != "" && !seen[rid] {
				seen[rid] = true
				driveRun(rid)
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	if len(sprintKids) == 0 {
		var labels []string
		for _, k := range env.cp89ChildRows(t, runID) {
			if l, ok := k["label"].(string); ok {
				labels = append(labels, l)
			}
		}
		t.Fatalf("vibe-sprint never spawned under %s within %v (lastLoop=%q, drives=%d, children=%v)",
			runID, 90*time.Minute, lastLoop, drives, labels)
	}
	for _, k := range sprintKids {
		if pk, _ := k["provider_key"].(string); pk != "" && pk != provider {
			t.Fatalf("sprint child %v provider_key=%q, want %q — inherited provider must match", k["label"], pk, provider)
		}
	}
	t.Logf("VT-L4a: sprint-1 started — child=%v label=%v agent=%v task=%d/%d %q (drives=%d)",
		sprintKids[0]["run_id"], sprintKids[0]["label"], sprintKids[0]["agent_name"], taskIdx, taskTotal, taskName, drives)
	if taskTotal != 2 {
		t.Fatalf("vibeTaskTotal=%d, want 2 (2 CP-02-parented tasks seeded; foreign/unparented excluded)", taskTotal)
	}
	// Required evidence: detecting both tasks must trigger both sequential
	// sprints — one preflight_contract_plan child per plan task. Anything
	// fewer means the chain starved between boundaries (the drift-only
	// debate remount wedge fixed in CA-1073 is exactly that failure mode).
	if len(sprintKids) < 2 {
		roles := make([]string, 0, len(sprintRoles))
		for r := range sprintRoles {
			roles = append(roles, r)
		}
		sort.Strings(roles)
		t.Fatalf("chain starved: %d/2 sprints triggered within %v (lastLoop=%q, drives=%d, tasks=%v, roles=%v)",
			len(sprintKids), 90*time.Minute, lastLoop, drives, taskSeq, roles)
	}
	// Order check: the observed task sequence must walk the plan in order.
	wantSeq := []string{"Task-21", "Task-22"}
	for i, want := range wantSeq {
		if i >= len(taskSeq) || !strings.Contains(taskSeq[i], want) {
			t.Fatalf("task trigger order = %v, want sprint %d on %q (plan order)", taskSeq, i+1, want)
		}
	}
	if taskIdx != 2 {
		t.Fatalf("vibeTaskIndex=%d, want 2 after both sprints triggered", taskIdx)
	}
	roles := make([]string, 0, len(sprintRoles))
	for r := range sprintRoles {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	t.Logf("VT-L4b: both tasks triggered in order %v — sprints=%d roles=%v drives=%d",
		taskSeq, len(sprintKids), roles, drives)
}
