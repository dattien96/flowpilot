package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// CA-1216 / Task-459 (vibe-adopt): verify-first adoption of code written
// outside FlowPilot. The wrapper parks adopt_select behind engine question
// cards (scope -> task/cp candidate list), builds the sprint plan from the
// pick, and chains vibe-adopt-sprint graphs instead of vibe-sprint.

func adoptTestTaskDoc(t *testing.T, dir, lane, name, parentCP string) string {
	t.Helper()
	rel := filepath.Join("requirements", "08-Task", lane, name)
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# " + name + "\n\n## Metadata\n\n- Parent Documents: " + parentCP + "\n"
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

func adoptTestCPDoc(t *testing.T, dir, name string) string {
	t.Helper()
	rel := filepath.Join("requirements", "07-Coding-Plan", "todo", name)
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

func adoptTestParentRun(t *testing.T, svc *InteractiveService, dir string) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workspaceCwd = dir
	rs.flowEngineDriven = true
	rs.chatFlowRef = workingmode.PackPrefix + vibeAdoptFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: vibeAdoptSelectNodeID, Behavior: "user.confirm"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: vibeAdoptSelectNodeID, To: "done", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	return parent.RunID
}

// Topology (CA-1221): the adopt sprint enters through the characterization
// scaffold — context.produce may only forward to a spawnable delegate/writer,
// so the suite entry is context→tdd→validate, never context→validate. coder
// stays reachable only through remediation back-edges plus the inert
// `remediate` forward anchor that keeps it non-entry.
func TestVibeAdoptSprint_TopologyEntersAtValidate(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def *agentpack.FlowDefinition
	for i := range pack.Flows {
		if pack.Flows[i].ID == vibeAdoptSprintFlowID {
			def = &pack.Flows[i]
			break
		}
	}
	if def == nil {
		t.Fatal("vibe-adopt-sprint not in builtin pack")
	}
	var contextToTdd, tddToValidate, tddToCoder, coderAnchor, remediateBack bool
	for _, e := range def.Edges {
		if e.From == "context" && e.To == "tdd" && e.When == "done" && e.Kind == "forward" {
			contextToTdd = true
		}
		if e.From == "tdd" && e.To == "validate" && e.When == "done" && e.Kind == "forward" {
			tddToValidate = true
		}
		if e.From == "tdd" && e.To == "coder" {
			tddToCoder = true
		}
		if e.From == "synthesis_negotiation" && e.To == "coder" && e.When == "remediate" && e.Kind == "forward" {
			coderAnchor = true
		}
		if e.From == "synthesis_negotiation" && e.To == "tdd" && e.When == "continue" && e.Kind == "back" {
			remediateBack = true
		}
	}
	if !contextToTdd {
		t.Fatal("adopt sprint must enter through tdd from context (spawnable target)")
	}
	if !tddToValidate {
		t.Fatal("tdd must hand off to validate so the suite runs before the review cohort")
	}
	if tddToCoder {
		t.Fatal("adopt sprint must not route tdd into coder on the happy path")
	}
	if !coderAnchor {
		t.Fatal("coder needs the inert remediate forward anchor to stay non-entry")
	}
	if !remediateBack {
		t.Fatal("defect remediation must keep the continue back-edge into tdd")
	}
}

// Sprint flow ref selection: adopt parents mount vibe-adopt-sprint; every
// other vibe run keeps vibe-sprint.
func TestVibeAdopt_SprintFlowRefSelection(t *testing.T) {
	normal := &interactiveRun{chatFlowRef: workingmode.PackPrefix + vibeTasksFlowID}
	if got := vibeSprintFlowRefFor(normal); got != workingmode.PackPrefix+vibeSprintFlowID {
		t.Fatalf("non-adopt ref = %q", got)
	}
	for _, ref := range []string{workingmode.PackPrefix + vibeAdoptFlowID, workingmode.PackPrefix + vibeAdoptSprintFlowID} {
		rs := &interactiveRun{chatFlowRef: ref}
		if got := vibeSprintFlowRefFor(rs); got != workingmode.PackPrefix+vibeAdoptSprintFlowID {
			t.Fatalf("adopt ref %q -> %q, want vibe-adopt-sprint", ref, got)
		}
	}
}

// inferPackFlowRefFromNodes: the wrapper resolves to vibe-adopt; an explicit
// adopt-sprint ref is authoritative on the shared sprint topology.
func TestVibeAdopt_FlowRefInference(t *testing.T) {
	if got := inferPackFlowRefFromNodes([]agentpack.FlowNode{{ID: vibeAdoptSelectNodeID}}, "fallback"); got != workingmode.PackPrefix+vibeAdoptFlowID {
		t.Fatalf("adopt_select topology -> %q", got)
	}
	sprintNodes := []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}
	adoptRef := workingmode.PackPrefix + vibeAdoptSprintFlowID
	if got := inferPackFlowRefFromNodes(sprintNodes, adoptRef); got != adoptRef {
		t.Fatalf("stored adopt ref on sprint topology must stick, got %q", got)
	}
	if got := inferPackFlowRefFromNodes(sprintNodes, "stale-ref"); got != workingmode.PackPrefix+vibeSprintFlowID {
		t.Fatalf("stale ref on sprint topology must correct to vibe-sprint, got %q", got)
	}
}

// The scope card parks adopt_select with the two-scope option set.
func TestVibeAdopt_ScopeSelectEmitsCard(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parentID := adoptTestParentRun(t, svc, dir)

	svc.parkVibeAdoptSelect(parentID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !svc.vibeAdoptSelectPendingLocked(parentID) {
		t.Fatal("scope question must be pending")
	}
	var rec *questionRecord
	for _, q := range svc.questions {
		if q.runID == parentID {
			rec = q
		}
	}
	if rec == nil || len(rec.options) != 2 || rec.options[0].Value != "task" || rec.options[1].Value != "cp" {
		t.Fatalf("scope card options wrong: %+v", rec)
	}
	if got := vibeAdoptSelectStageFromPrompt(rec.prompt); got != "scope" {
		t.Fatalf("stage = %q, want scope", got)
	}
	if loop := svc.agentOrchestrator.loopStateFor(parentID); loop.Status != "blocked" || loop.BlockReason != vibeAdoptSelectBlockReason {
		t.Fatalf("loop = %+v, want blocked on vibe_adopt_select", loop)
	}
}

// scope=task -> task-list card -> pick -> single-task plan + chain start.
func TestVibeAdopt_TaskPickSetsSinglePlanAndChains(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parentID := adoptTestParentRun(t, svc, dir)
	taskRel := adoptTestTaskDoc(t, dir, "done", "Task-051-x.md", "CP-05")

	svc.parkVibeAdoptSelect(parentID)
	rec := &questionRecord{runID: parentID, prompt: vibeAdoptSelectQuestionKind + ":scope: pick", choice: []string{"task"}}
	svc.applyVibeAdoptSelectAnswer(svc.runs[parentID], rec)

	svc.mu.Lock()
	stage2 := (*questionRecord)(nil)
	for _, q := range svc.questions {
		// The synthetic answer record never resolved the stored scope card —
		// match the pending record by stage so map order can't pick it.
		if q.runID == parentID && q.status == "pending" && vibeAdoptSelectStageFromPrompt(q.prompt) == "task" {
			stage2 = q
		}
	}
	svc.mu.Unlock()
	if stage2 == nil {
		t.Fatal("task scope must emit the task-list card")
	}
	found := false
	for _, o := range stage2.options {
		if o.Value == taskRel {
			found = true
		}
	}
	if !found {
		t.Fatalf("task list must contain %q, got %+v", taskRel, stage2.options)
	}

	rec2 := &questionRecord{runID: parentID, prompt: vibeAdoptSelectQuestionKind + ":task: pick", choice: []string{taskRel}}
	svc.applyVibeAdoptSelectAnswer(svc.runs[parentID], rec2)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parentID]
	if len(rs.vibeTaskPlan) != 1 || rs.vibeTaskPlan[0] != taskRel {
		t.Fatalf("plan = %v, want [%s]", rs.vibeTaskPlan, taskRel)
	}
	// The chain seam consumed the take: index advanced for sprint 1.
	if rs.vibeSprintIndex != 1 {
		t.Fatalf("sprint index = %d, want 1 (first adopt sprint taken)", rs.vibeSprintIndex)
	}
}

// scope=cp -> CP-list card -> pick -> union plan over todo+done tasks of the
// CP, ordered by task id; foreign tasks excluded.
func TestVibeAdopt_CpPickBuildsUnionPlan(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parentID := adoptTestParentRun(t, svc, dir)
	adoptTestCPDoc(t, dir, "CP-05-x.md")
	t1 := adoptTestTaskDoc(t, dir, "done", "Task-051-a.md", "CP-05")
	t2 := adoptTestTaskDoc(t, dir, "done", "Task-052-b.md", "CP-05")
	t3 := adoptTestTaskDoc(t, dir, "todo", "Task-053-c.md", "CP-05")
	adoptTestTaskDoc(t, dir, "todo", "Task-042-z.md", "CP-04") // foreign

	svc.parkVibeAdoptSelect(parentID)
	svc.applyVibeAdoptSelectAnswer(svc.runs[parentID],
		&questionRecord{runID: parentID, prompt: vibeAdoptSelectQuestionKind + ":scope: pick", choice: []string{"cp"}})

	var stage2 *questionRecord
	svc.mu.Lock()
	for _, q := range svc.questions {
		// The synthetic answer record never resolved the stored scope card —
		// match the pending record by stage so map order can't pick it.
		if q.runID == parentID && q.status == "pending" && vibeAdoptSelectStageFromPrompt(q.prompt) == "cp" {
			stage2 = q
		}
	}
	svc.mu.Unlock()
	if stage2 == nil {
		t.Fatal("cp scope must emit the CP-list card")
	}
	var cpOpt *QuestionOption
	for i, o := range stage2.options {
		if o.Value == "CP-05" {
			cpOpt = &stage2.options[i]
		}
	}
	if cpOpt == nil {
		t.Fatalf("CP-05 not offered: %+v", stage2.options)
	}

	svc.applyVibeAdoptSelectAnswer(svc.runs[parentID],
		&questionRecord{runID: parentID, prompt: vibeAdoptSelectQuestionKind + ":cp: pick", choice: []string{cpOpt.Value}})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parentID]
	want := []string{t1, t2, t3}
	if len(rs.vibeTaskPlan) != len(want) {
		t.Fatalf("plan = %v, want %v", rs.vibeTaskPlan, want)
	}
	for i := range want {
		if rs.vibeTaskPlan[i] != want[i] {
			t.Fatalf("plan[%d] = %q, want %q", i, rs.vibeTaskPlan[i], want[i])
		}
	}
	if rs.vibeCpDocID != "CP-05" {
		t.Fatalf("vibeCpDocID = %q, want CP-05", rs.vibeCpDocID)
	}
}

// A CP pick with zero parented tasks re-emits the CP card instead of
// dead-ending the flow.
func TestVibeAdopt_CpPickEmptyReemits(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parentID := adoptTestParentRun(t, svc, dir)
	adoptTestCPDoc(t, dir, "CP-09-x.md")

	svc.parkVibeAdoptSelect(parentID)
	svc.applyVibeAdoptSelectAnswer(svc.runs[parentID],
		&questionRecord{runID: parentID, prompt: vibeAdoptSelectQuestionKind + ":cp: pick", choice: []string{"CP-09"}})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !svc.vibeAdoptSelectPendingLocked(parentID) {
		t.Fatal("empty CP must re-emit the select card, not dead-end")
	}
	if rs := svc.runs[parentID]; len(rs.vibeTaskPlan) != 0 {
		t.Fatalf("empty CP must not set a plan, got %v", rs.vibeTaskPlan)
	}
}

// A stray gate-decision while the select card owns the surface is rejected,
// not routed into the generic blocked-loop path.
func TestVibeAdopt_GateDecisionRejectedWhileSelecting(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parentID := adoptTestParentRun(t, svc, dir)
	svc.parkVibeAdoptSelect(parentID)

	if e := svc.SubmitGateDecision(parentID, "custom", "anything"); e == nil || e.code != "adopt_select_pending" {
		t.Fatalf("gate decision while selecting = %v, want adopt_select_pending", e)
	}
}

// The characterization contract marker waives RED only under adopt.
func TestVibeAdopt_CharacterizationScopedToAdopt(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	sigDir := filepath.Join(dir, filepath.FromSlash("requirements/.flowpilot/vibe"))
	if err := os.MkdirAll(sigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sig := "# TDD signatures — adopt-one (Task-051)\n\n## RED gate expectation\n- red_tests: `[]`\n- failure_type: `characterization`\n\n# TDD signatures — adopt-two (Task-052)\n\n## RED gate expectation\n- red_tests: `[\"X\"]`\n- failure_type: `not_implemented`\n\n# TDD signatures — adopt-three (Task-053)\n\n## RED gate expectation\n- red_tests: `[\"Y\"]`\n- failure_type: `characterization`\n"
	if err := os.WriteFile(filepath.Join(sigDir, "tdd-signatures.md"), []byte(sig), 0o644); err != nil {
		t.Fatal(err)
	}
	if !vibeScaffoldCharacterizationForTask(dir, "Task-051") {
		t.Fatal("characterization contract must be detected for its task")
	}
	if vibeScaffoldCharacterizationForTask(dir, "Task-052") {
		t.Fatal("characterization must not leak across task sections")
	}
	if vibeScaffoldCharacterizationForTask(dir, "Task-053") {
		t.Fatal("characterization with red_tests declared is a defect remediation, not a waiver")
	}
	adopt := &interactiveRun{chatFlowRef: workingmode.PackPrefix + vibeAdoptSprintFlowID}
	normal := &interactiveRun{chatFlowRef: workingmode.PackPrefix + vibeSprintFlowID}
	if !vibeSprintIsAdopt(adopt) || vibeSprintIsAdopt(normal) {
		t.Fatal("adopt detection must key on adopt flow refs only")
	}
}

// Stage parsing round-trips through the prompt prefix (durable path).
func TestVibeAdopt_StagePrefixRoundTrip(t *testing.T) {
	for _, want := range []string{"scope", "task", "cp"} {
		p := vibeAdoptSelectQuestionKind + ":" + want + ": human text"
		if got := vibeAdoptSelectStageFromPrompt(p); got != want {
			t.Fatalf("stage(%q) = %q, want %q", p, got, want)
		}
	}
	if got := vibeAdoptSelectStageFromPrompt("unrelated prompt"); got != "" {
		t.Fatalf("foreign prompt stage = %q", got)
	}
}
