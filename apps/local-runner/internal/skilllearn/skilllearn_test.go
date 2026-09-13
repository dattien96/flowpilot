package skilllearn

// Task-336 §10 Test Signature Guide — the 9 required Test functions are
// implemented below with their exact names, plus additive coverage for the
// store lifecycle (PromoterService), the CA-838 dedupe note, the DOD item 5
// provider dirs, and the Compact Rule Card (CP-23 T-3).
//
// Safe-fix contract: this file is NEW (no pre-existing test is edited) and it
// never writes into the repo's real skillpack/flow-pack or .agents trees —
// every export targets a t.TempDir() root via SkillExportOptions.WorkspaceRoot
// / FlowPackRoot / SkillDirs.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"flowpilot-runner/internal/driftdetect"
	"flowpilot-runner/internal/skillpack"
)

// driftEvent builds a Task-335 DriftEvent with deterministic per-turn note.
func driftEvent(runID, turnID string, signals ...string) driftdetect.DriftEvent {
	return driftdetect.DriftEvent{
		RunID:            runID,
		StepID:           "step-" + turnID,
		TurnID:           turnID,
		DriftScore:       30,
		TriggeredSignals: signals,
		CorrectionAction: driftdetect.ActionInjectSystemNote,
		SystemNotePrompt: "[FlowPilot Drift Detector] drift_score=30 for " + turnID,
	}
}

// goroutineCandidate is a clean (non-conflicting) hand-built candidate whose
// slug is "goroutine-safety", matching the Task-336 §10 example layout.
func goroutineCandidate() LessonCandidate {
	return LessonCandidate{
		ID:                "lc-test-goroutine",
		ProjectID:         "proj-1",
		Title:             "Goroutine safety",
		Group:             "golang",
		TriggerPattern:    "turns that spawn goroutines without a cancellation path",
		AntiPattern:       "Spawning goroutines without a cancellation or timeout path.",
		PreferredBehavior: "Always pass a context with a cancel func and defer the cancel; review every goroutine for a bounded lifetime.",
		RepeatCount:       2,
		SupportingRunIDs:  []string{"run-1", "run-2"},
		Status:            StatusCandidate,
	}
}

// repeatedTestFailureEvents returns two same-pattern drift events on distinct
// runs/turns (the >= 2 threshold of CP-23 D-4).
func repeatedTestFailureEvents() []driftdetect.DriftEvent {
	return []driftdetect.DriftEvent{
		driftEvent("run-1", "turn-1", driftdetect.SignalRepeatedTestFailure),
		driftEvent("run-2", "turn-2", driftdetect.SignalRepeatedTestFailure),
	}
}

// findCandidate returns the candidate whose TriggerPattern contains substr.
func findCandidate(t *testing.T, candidates []LessonCandidate, substr string) LessonCandidate {
	t.Helper()
	for _, c := range candidates {
		if strings.Contains(c.TriggerPattern, substr) {
			return c
		}
	}
	t.Fatalf("no candidate with trigger pattern containing %q in %+v", substr, candidates)
	return LessonCandidate{}
}

// --- Task-336 §10 required signatures -------------------------------------

// Scenario: Sự cố xảy ra lần đầu -> Chưa đủ điều kiện tạo candidate
// Input: 1 DriftEvent duy nhất với pattern "goroutine_leak"
// Expect: Không tạo LessonCandidate mới
func TestCandidateAggregator_SingleEvent_NoCandidate(t *testing.T) {
	events := []driftdetect.DriftEvent{
		driftEvent("run-1", "turn-1", driftdetect.SignalRepeatedTestFailure),
	}
	if got := AggregateDriftEvents(events, 2); len(got) != 0 {
		t.Fatalf("expected no LessonCandidate from a single incident (CP-23 D-4), got %d: %+v", len(got), got)
	}
}

// Scenario: Sự cố lặp lại lần 2 -> Tự động sinh LessonCandidate
// Input: 2 DriftEvent có cùng pattern (repeated_test_failure) trên cùng project
// Expect: Sinh ra 1 LessonCandidate với RepeatCount=2, Status="candidate"
func TestCandidateAggregator_RepeatedEvent_GeneratesCandidate(t *testing.T) {
	events := repeatedTestFailureEvents()

	got := AggregateDriftEvents(events, 2)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 LessonCandidate, got %d: %+v", len(got), got)
	}
	c := got[0]

	if c.RepeatCount != 2 {
		t.Errorf("RepeatCount = %d, want 2", c.RepeatCount)
	}
	if c.Status != StatusCandidate {
		t.Errorf("Status = %q, want %q", c.Status, StatusCandidate)
	}
	if want := []string{"run-1", "run-2"}; !reflect.DeepEqual(c.SupportingRunIDs, want) {
		t.Errorf("SupportingRunIDs = %v, want %v", c.SupportingRunIDs, want)
	}
	if c.ID == "" || !strings.HasPrefix(c.ID, "lc-") {
		t.Errorf("ID = %q, want a non-empty \"lc-\" prefixed id", c.ID)
	}
	// Documented mapping defaults (Task-336): DriftEvent has no project/group
	// identity, so Group defaults to "common" and ProjectID is caller-filled.
	if c.Group != "common" {
		t.Errorf("Group = %q, want default %q", c.Group, "common")
	}
	if c.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty (caller fills it)", c.ProjectID)
	}
	// Deterministic derivation from signal names (signalLessons mapping).
	wantTrigger := "turns that re-run the same failing test without changing strategy"
	if c.TriggerPattern != wantTrigger {
		t.Errorf("TriggerPattern = %q, want %q", c.TriggerPattern, wantTrigger)
	}
	if !strings.Contains(c.AntiPattern, "Re-running the same failing test") {
		t.Errorf("AntiPattern = %q, want the repeated-test-failure anti-pattern sentence", c.AntiPattern)
	}
	if !strings.Contains(c.PreferredBehavior, "change strategy") {
		t.Errorf("PreferredBehavior = %q, want the preferred-behavior sentence", c.PreferredBehavior)
	}
	// Note-text contribution: the first non-empty SystemNotePrompt of the
	// group (input order) is appended deterministically.
	if !strings.Contains(c.PreferredBehavior, "Drift correction note: [FlowPilot Drift Detector] drift_score=30 for turn-1") {
		t.Errorf("PreferredBehavior = %q, want the first group note appended", c.PreferredBehavior)
	}

	// Provider-agnostic parity: pure deterministic Go — same input, same bytes.
	again := AggregateDriftEvents(events, 2)
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("AggregateDriftEvents is not deterministic:\n%+v\nvs\n%+v", got, again)
	}
}

// Scenario: Người dùng phê duyệt candidate -> Xuất bản file SKILL.md vào target project
// Input: Candidate đã duyệt, TargetDir="<project>/.agents/skills", Group="golang"
// Expect: File .agents/skills/golang/goroutine-safety/SKILL.md được tạo với đúng format
func TestSkillExporter_ApproveToTargetProject_WritesValidSkill(t *testing.T) {
	project := t.TempDir()
	candidate := goroutineCandidate()

	if err := PromoteCandidateToSkill(candidate, SkillExportOptions{WorkspaceRoot: project}); err != nil {
		t.Fatalf("PromoteCandidateToSkill returned error: %v", err)
	}

	skillFile := filepath.Join(project, ".agents", "skills", "golang", "goroutine-safety", "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("exported skill not found at %s: %v", skillFile, err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\nname: goroutine-safety\ndescription: ") {
		t.Errorf("SKILL.md must start with YAML frontmatter (name, description), got:\n%.120s", content)
	}
	if !strings.Contains(content, "\n---\n") {
		t.Errorf("SKILL.md frontmatter is not closed, got:\n%s", content)
	}
	if !strings.Contains(content, "description: FlowPilot lesson learned from 2 repeated drift incident(s)") {
		t.Errorf("SKILL.md description missing, got:\n%s", content)
	}
	for _, want := range []string{
		"# Goroutine safety",
		"## Core Rules",
		"## Anti-Pattern",
		"## Preferred Behavior",
		"## Example",
		"- Wrong: Spawning goroutines without a cancellation or timeout path.",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SKILL.md missing required section/line %q, got:\n%s", want, content)
		}
	}

	// Deterministic rendering (provider-agnostic parity evidence).
	if RenderSkillMarkdown(candidate) != RenderSkillMarkdown(candidate) {
		t.Errorf("RenderSkillMarkdown is not deterministic")
	}
}

// Scenario: Xuất bản vào kho Core Flow-Pack
// Input: Candidate đã duyệt, ExportToCore=true, Group="common"
// Expect: File được ghi vào <root>/internal/skillpack/flow-pack/common/ với đúng cấu trúc
func TestSkillExporter_ExportToCoreFlowPack_Success(t *testing.T) {
	root := t.TempDir() // test root — never the repo's real flow-pack tree
	candidate := goroutineCandidate()
	candidate.Title = "Slow test triage"
	candidate.Group = "common"

	opts := SkillExportOptions{WorkspaceRoot: root, ExportToCore: true}
	if err := PromoteCandidateToSkill(candidate, opts); err != nil {
		t.Fatalf("PromoteCandidateToSkill (core) returned error: %v", err)
	}

	skillFile := filepath.Join(root, "internal", "skillpack", "flow-pack", "common", "slow-test-triage", "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("core-exported skill not found at %s: %v", skillFile, err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\nname: slow-test-triage\n") {
		t.Errorf("core SKILL.md frontmatter name mismatch, got:\n%.80s", content)
	}
	if !strings.Contains(content, "# Slow test triage") {
		t.Errorf("core SKILL.md title mismatch, got:\n%s", content)
	}
}

// [Edge] Scenario: Hai pattern khác nhau, mỗi pattern chỉ xuất hiện 1 lần
// Input: DriftEvent pattern "goroutine_leak" x1, DriftEvent pattern "nil_pointer" x1
// Expect: Không tạo LessonCandidate nào (mỗi pattern chưa đủ ngưỡng 2)
func TestCandidateAggregator_DifferentPatterns_NoCandidates(t *testing.T) {
	events := []driftdetect.DriftEvent{
		driftEvent("run-1", "turn-1", driftdetect.SignalRepeatedTestFailure),
		driftEvent("run-1", "turn-2", driftdetect.SignalApologyLoop),
	}
	if got := AggregateDriftEvents(events, 2); len(got) != 0 {
		t.Fatalf("expected no candidates (each pattern below threshold 2), got %d: %+v", len(got), got)
	}
	// CP-23 D-4 invariant: a threshold below 2 is clamped to 2 — never a
	// candidate from a single incident.
	if got := AggregateDriftEvents(events, 1); len(got) != 0 {
		t.Fatalf("threshold 1 must be clamped to 2 (D-4), got %d candidates: %+v", len(got), got)
	}
}

// [Edge] Scenario: Skill trùng tên đã tồn tại trong target project
// Input: File .agents/skills/golang/goroutine-safety/SKILL.md đã có
// Expect: Tạo version mới (goroutine-safety-v2) thay vì ghi đè
func TestSkillExporter_DuplicateName_CreatesNewVersion(t *testing.T) {
	project := t.TempDir()
	opts := SkillExportOptions{WorkspaceRoot: project}

	v1 := filepath.Join(project, ".agents", "skills", "golang", "goroutine-safety", "SKILL.md")
	if err := PromoteCandidateToSkill(goroutineCandidate(), opts); err != nil {
		t.Fatalf("first export failed: %v", err)
	}
	v1Bytes, err := os.ReadFile(v1)
	if err != nil {
		t.Fatalf("read v1: %v", err)
	}

	// Same candidate again: must create goroutine-safety-v2, never overwrite.
	if err := PromoteCandidateToSkill(goroutineCandidate(), opts); err != nil {
		t.Fatalf("second export failed: %v", err)
	}
	v2 := filepath.Join(project, ".agents", "skills", "golang", "goroutine-safety-v2", "SKILL.md")
	if _, err := os.Stat(v2); err != nil {
		t.Fatalf("expected versioned skill at %s: %v", v2, err)
	}
	v1After, err := os.ReadFile(v1)
	if err != nil {
		t.Fatalf("read v1 after second export: %v", err)
	}
	if !bytes.Equal(v1Bytes, v1After) {
		t.Errorf("existing skill file was modified by the duplicate export")
	}

	// And a third export walks to -v3.
	if err := PromoteCandidateToSkill(goroutineCandidate(), opts); err != nil {
		t.Fatalf("third export failed: %v", err)
	}
	v3 := filepath.Join(project, ".agents", "skills", "golang", "goroutine-safety-v3", "SKILL.md")
	if _, err := os.Stat(v3); err != nil {
		t.Fatalf("expected third version at %s: %v", v3, err)
	}
}

// [Error] Scenario: WorkspaceRoot không tồn tại hoặc readonly
// Input: WorkspaceRoot="/nonexistent/path"
// Expect: Trả về error rõ ràng, không panic
func TestSkillExporter_InvalidWorkspace_ReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	candidate := goroutineCandidate()

	err := PromoteCandidateToSkill(candidate, SkillExportOptions{WorkspaceRoot: missing})
	if err == nil {
		t.Fatalf("expected an error for a missing workspace root")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should clearly mention the missing workspace root, got: %v", err)
	}
	// Validation happens before any mkdir: the missing root stays missing.
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Errorf("exporter must not create the missing workspace root, stat err: %v", statErr)
	}

	// Core mode with a missing root fails the same way.
	if err := PromoteCandidateToSkill(candidate, SkillExportOptions{WorkspaceRoot: missing, ExportToCore: true}); err == nil {
		t.Fatalf("expected an error for a missing core workspace root")
	}

	// Empty root.
	if err := PromoteCandidateToSkill(candidate, SkillExportOptions{WorkspaceRoot: ""}); err == nil {
		t.Fatalf("expected an error for an empty workspace root")
	}

	// Root exists but is a file, not a directory.
	notDir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	err = PromoteCandidateToSkill(candidate, SkillExportOptions{WorkspaceRoot: notDir})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected a clear not-a-directory error, got: %v", err)
	}
}

// [Error] Scenario: Skill mới mâu thuẫn với skill cốt lõi (safe-fix-contract)
// Input: Candidate có AntiPattern trùng với rule trong safe-fix-contract
// Expect: Cảnh báo conflict, yêu cầu user xác nhận thêm
func TestSkillExporter_ConflictWithCoreSkill_WarnsUser(t *testing.T) {
	project := t.TempDir()
	conflicting := LessonCandidate{
		ID:                "lc-test-conflict",
		Title:             "Fast test pass",
		Group:             "golang",
		TriggerPattern:    "turns that churn tests to go green",
		AntiPattern:       "When a test fails, edit existing tests until the suite passes.",
		PreferredBehavior: "Speed up the suite by deleting old tests that keep failing.",
		RepeatCount:       2,
		Status:            StatusCandidate,
	}

	err := PromoteCandidateToSkill(conflicting, SkillExportOptions{WorkspaceRoot: project})
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
	if !strings.Contains(conflictErr.Error(), "safe-fix-contract") {
		t.Errorf("conflict error must cite the core skills, got: %v", err)
	}
	if !strings.Contains(conflictErr.Reason, "edit existing tests") {
		t.Errorf("conflict reason should name the forbidden phrase, got: %q", conflictErr.Reason)
	}
	// The conflicting skill must NOT be written before explicit confirmation.
	skillFile := filepath.Join(project, ".agents", "skills", "golang", "fast-test-pass", "SKILL.md")
	if _, statErr := os.Stat(skillFile); !os.IsNotExist(statErr) {
		t.Fatalf("conflicting skill must not be written without Force, stat err: %v", statErr)
	}

	// Explicit user confirmation (Force) lets the export proceed.
	if err := PromoteCandidateToSkill(conflicting, SkillExportOptions{WorkspaceRoot: project, Force: true}); err != nil {
		t.Fatalf("export with Force=true should succeed: %v", err)
	}
	if _, statErr := os.Stat(skillFile); statErr != nil {
		t.Fatalf("expected the skill after Force confirmation: %v", statErr)
	}

	// Name collision with a protected core skill is also a conflict.
	nameCollision := goroutineCandidate()
	nameCollision.Title = "Safe Fix Contract" // slugify -> "safe-fix-contract"
	err = PromoteCandidateToSkill(nameCollision, SkillExportOptions{WorkspaceRoot: project})
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *ConflictError for a protected core skill name, got %v", err)
	}
	if !strings.Contains(conflictErr.Reason, "safe-fix-contract") {
		t.Errorf("name-collision reason should cite the protected skill, got: %q", conflictErr.Reason)
	}
}

// [Edge] Scenario: Kiểm tra skill sau install có thể được AI phát hiện
// Input: Xuất bản skill vào skillpack, chạy skillpack.Install()
// Expect: File SKILL.md xuất hiện đúng vị trí trong target project
func TestSkillExporter_InstalledSkillIsDiscoverable(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	project := filepath.Join(tmp, "target-project")
	flowPackRoot := filepath.Join(tmp, "flow-pack") // isolated core root (never the repo tree)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("setup workspace: %v", err)
	}

	// 1. Publish the lesson into the (temp) core flow-pack root.
	exported, err := SkillExporter{}.Export(goroutineCandidate(), SkillExportOptions{
		WorkspaceRoot: workspace,
		ExportToCore:  true,
		FlowPackRoot:  flowPackRoot,
	})
	if err != nil {
		t.Fatalf("core export failed: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("expected exactly 1 exported file, got %v", exported)
	}
	if want := filepath.Join(flowPackRoot, "golang", "goroutine-safety", "SKILL.md"); exported[0] != want {
		t.Fatalf("core export path = %s, want %s", exported[0], want)
	}

	// 2. Run the REAL skillpack install entry point against the published
	// root (skillpack.InstallFromRoot reuses the embedded Install's group
	// selection and provider roots — the embedded FS itself cannot see a temp
	// root). No mocks of the install logic.
	result, err := skillpack.InstallFromRoot(project, "golang", flowPackRoot)
	if err != nil {
		t.Fatalf("InstallFromRoot failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("InstallFromRoot reported errors: %v", result.Errors)
	}

	// 3. The skill is discoverable in the target project's provider dirs.
	installed := filepath.Join(project, ".agents", "skills", "goroutine-safety", "SKILL.md")
	got, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("installed skill not discoverable at %s: %v", installed, err)
	}
	exportedBytes, err := os.ReadFile(exported[0])
	if err != nil {
		t.Fatalf("read exported skill: %v", err)
	}
	if !bytes.Equal(got, exportedBytes) {
		t.Errorf("installed SKILL.md differs from the published core skill")
	}
	for _, providerFile := range []string{
		filepath.Join(project, ".claude", "skills", "goroutine-safety", "SKILL.md"),
		filepath.Join(project, ".grok", "skills", "goroutine-safety", "SKILL.md"),
	} {
		if _, err := os.Stat(providerFile); err != nil {
			t.Errorf("provider skill missing at %s: %v", providerFile, err)
		}
	}

	// 4. Re-install never overwrites an existing target-project skill.
	again, err := skillpack.InstallFromRoot(project, "golang", flowPackRoot)
	if err != nil {
		t.Fatalf("re-install failed: %v", err)
	}
	if len(again.Installed) != 0 || len(again.Skipped) == 0 {
		t.Errorf("re-install must skip (never overwrite) existing skills, got installed=%v skipped=%v", again.Installed, again.Skipped)
	}
}

// --- Additive coverage (Task-336 store lifecycle, DOD items 3/5, T-3) ------

// The full approval lifecycle: upsert -> list -> approve -> promoted (with a
// real SKILL.md on disk) and reject -> rejected; both persisted immediately.
func TestPromoterService_ApproveAndRejectLifecycle(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	svc := NewPromoterService(workspace)

	events := append(repeatedTestFailureEvents(),
		driftEvent("run-1", "turn-3", driftdetect.SignalApologyLoop),
		driftEvent("run-2", "turn-4", driftdetect.SignalApologyLoop),
	)
	if err := svc.UpsertCandidates(ctx, AggregateDriftEvents(events, 2)); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}

	list, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(list), list)
	}
	for _, c := range list {
		if c.Status != StatusCandidate {
			t.Errorf("ingested candidate %q must stay %q (T-1), got %q", c.ID, StatusCandidate, c.Status)
		}
	}

	// Approve + export one candidate into a temp target project.
	candidate := findCandidate(t, list, "re-run the same failing test")
	project := t.TempDir()
	if err := svc.ApproveCandidateAndExport(ctx, candidate.ID, SkillExportOptions{WorkspaceRoot: project}); err != nil {
		t.Fatalf("ApproveCandidateAndExport: %v", err)
	}
	after, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates after approve: %v", err)
	}
	promoted := findCandidate(t, after, "re-run the same failing test")
	if promoted.Status != StatusPromoted {
		t.Errorf("status = %q, want %q (persisted)", promoted.Status, StatusPromoted)
	}
	slug := Slugify(promoted.Title)
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", promoted.Group, slug, "SKILL.md")); err != nil {
		t.Errorf("approved skill must exist in the target project: %v", err)
	}
	if err := svc.ApproveCandidateAndExport(ctx, candidate.ID, SkillExportOptions{WorkspaceRoot: project}); err == nil {
		t.Errorf("approving an already-promoted candidate must fail")
	}

	// Reject the other candidate (idempotent; approving it then fails).
	other := findCandidate(t, after, "apology or filler loop")
	if err := svc.RejectCandidate(ctx, other.ID); err != nil {
		t.Fatalf("RejectCandidate: %v", err)
	}
	if err := svc.RejectCandidate(ctx, other.ID); err != nil {
		t.Fatalf("RejectCandidate must be idempotent: %v", err)
	}
	afterReject, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates after reject: %v", err)
	}
	rejected := findCandidate(t, afterReject, "apology or filler loop")
	if rejected.Status != StatusRejected {
		t.Errorf("status = %q, want %q (persisted)", rejected.Status, StatusRejected)
	}
	if err := svc.ApproveCandidateAndExport(ctx, other.ID, SkillExportOptions{WorkspaceRoot: project}); err == nil {
		t.Errorf("approving a rejected candidate must fail")
	}
}

// Export failure after approval keeps StatusApproved (no silent loss) and a
// retry — including a Force retry after a conflict — promotes cleanly.
func TestPromoterService_ApproveExportFailure_KeepsApproved(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	svc := NewPromoterService(workspace)
	if err := svc.UpsertCandidates(ctx, AggregateDriftEvents(repeatedTestFailureEvents(), 2)); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}
	list, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	id := list[0].ID

	// Failing export (missing workspace root): error returned, status stays
	// approved — approval is never lost.
	missing := filepath.Join(workspace, "missing-target")
	if err := svc.ApproveCandidateAndExport(ctx, id, SkillExportOptions{WorkspaceRoot: missing}); err == nil {
		t.Fatalf("expected the export to fail for a missing workspace root")
	}
	after, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if after[0].ID != id || after[0].Status != StatusApproved {
		t.Fatalf("status = %q for %q, want approved with no silent loss", after[0].Status, after[0].ID)
	}

	// Conflict path: approve succeeds but the export raises a *ConflictError;
	// the candidate stays approved until Force acknowledges the conflict.
	conflicting := LessonCandidate{
		ID:                "lc-test-conflict-store",
		Title:             "Fast test pass",
		TriggerPattern:    "turns that churn tests",
		AntiPattern:       "Edit existing tests to force them green.",
		PreferredBehavior: "Delete old tests for speed.",
		RepeatCount:       2,
	}
	if err := svc.UpsertCandidates(ctx, []LessonCandidate{conflicting}); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}
	project := t.TempDir()
	err = svc.ApproveCandidateAndExport(ctx, conflicting.ID, SkillExportOptions{WorkspaceRoot: project})
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
	afterConflict, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	found := findCandidate(t, afterConflict, "turns that churn tests")
	if found.Status != StatusApproved {
		t.Fatalf("status after conflict = %q, want approved (retryable)", found.Status)
	}

	// Force retry: same ID, export succeeds, status promoted.
	if err := svc.ApproveCandidateAndExport(ctx, conflicting.ID, SkillExportOptions{WorkspaceRoot: project, Force: true}); err != nil {
		t.Fatalf("Force retry failed: %v", err)
	}
	final, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if got := findCandidate(t, final, "turns that churn tests"); got.Status != StatusPromoted {
		t.Fatalf("status after Force retry = %q, want promoted", got.Status)
	}
}

// DOD item 3: the Edit right — refine Title/AntiPattern/PreferredBehavior (and
// Group) before approval; the candidate stays StatusCandidate and evidence is
// preserved.
func TestPromoterService_UpdateCandidate_EditsBeforeApproval(t *testing.T) {
	ctx := context.Background()
	svc := NewPromoterService(t.TempDir())
	if err := svc.UpsertCandidates(ctx, AggregateDriftEvents(repeatedTestFailureEvents(), 2)); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}
	list, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	id := list[0].ID
	originalTrigger := list[0].TriggerPattern
	originalRuns := list[0].SupportingRunIDs

	if err := svc.UpdateCandidate(ctx, id, LessonCandidate{
		Title:             "Flaky test triage",
		Group:             "golang",
		AntiPattern:       "Edited anti-pattern for the flaky suite.",
		PreferredBehavior: "Edited preferred behavior: quarantine and report.",
	}); err != nil {
		t.Fatalf("UpdateCandidate: %v", err)
	}
	after, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	edited := after[0]
	if edited.ID != id {
		t.Fatalf("unexpected candidate %q", edited.ID)
	}
	if edited.Title != "Flaky test triage" || edited.Group != "golang" {
		t.Errorf("Title/Group not edited: %q / %q", edited.Title, edited.Group)
	}
	if edited.AntiPattern != "Edited anti-pattern for the flaky suite." {
		t.Errorf("AntiPattern not edited: %q", edited.AntiPattern)
	}
	if edited.PreferredBehavior != "Edited preferred behavior: quarantine and report." {
		t.Errorf("PreferredBehavior not edited: %q", edited.PreferredBehavior)
	}
	if edited.TriggerPattern != originalTrigger {
		t.Errorf("TriggerPattern must be preserved when not edited, got %q", edited.TriggerPattern)
	}
	if edited.RepeatCount != 2 || !reflect.DeepEqual(edited.SupportingRunIDs, originalRuns) {
		t.Errorf("evidence fields must be preserved: count=%d runs=%v", edited.RepeatCount, edited.SupportingRunIDs)
	}
	if edited.Status != StatusCandidate {
		t.Errorf("editing must keep the candidate pending, got %q", edited.Status)
	}

	// ID mismatch and unknown IDs are errors.
	if err := svc.UpdateCandidate(ctx, id, LessonCandidate{ID: "other"}); err == nil {
		t.Errorf("mismatched edited.ID must fail")
	}
	if err := svc.UpdateCandidate(ctx, "no-such-id", LessonCandidate{Title: "x"}); err == nil {
		t.Errorf("unknown candidate ID must fail")
	}

	// Editing after promotion is refused.
	project := t.TempDir()
	if err := svc.ApproveCandidateAndExport(ctx, id, SkillExportOptions{WorkspaceRoot: project}); err != nil {
		t.Fatalf("ApproveCandidateAndExport: %v", err)
	}
	if err := svc.UpdateCandidate(ctx, id, LessonCandidate{Title: "Too late"}); err == nil {
		t.Errorf("editing a promoted candidate must fail")
	}
}

// Store contract: missing file = empty list; corrupt file = error (no panic);
// upsert creates the .flowpilot directory hierarchy.
func TestPromoterService_MissingAndCorruptStore(t *testing.T) {
	ctx := context.Background()
	svc := NewPromoterService(t.TempDir())

	list, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("missing store must be an empty list, got error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("missing store must yield no candidates, got %d", len(list))
	}

	// Corrupt store: clear error, no panic.
	if err := os.MkdirAll(filepath.Dir(svc.StorePath), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(svc.StorePath, []byte("{ definitely not json"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := svc.ListCandidates(ctx); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt store must return a corrupt-store error, got: %v", err)
	}

	// Upsert into a fresh nested workspace creates the store.
	fresh := NewPromoterService(filepath.Join(t.TempDir(), "nested", "workspace"))
	if err := fresh.UpsertCandidates(ctx, AggregateDriftEvents(repeatedTestFailureEvents(), 2)); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}
	if _, err := os.Stat(fresh.StorePath); err != nil {
		t.Fatalf("store not created at %s: %v", fresh.StorePath, err)
	}
	if got, err := fresh.ListCandidates(ctx); err != nil || len(got) != 1 {
		t.Fatalf("ListCandidates after upsert: %d candidates, err %v", len(got), err)
	}
}

// Upsert refreshes evidence for pending candidates (stable ID), respects human
// decisions (rejected patterns are never resurrected) and enforces T-1
// (ingestion can never smuggle a promoted status).
func TestPromoterService_UpsertCandidates_RefreshesAndRespectsDecisions(t *testing.T) {
	ctx := context.Background()
	svc := NewPromoterService(t.TempDir())

	events := repeatedTestFailureEvents()
	if err := svc.UpsertCandidates(ctx, AggregateDriftEvents(events, 2)); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}
	first, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(first) != 1 || first[0].RepeatCount != 2 {
		t.Fatalf("expected 1 candidate with count 2, got %+v", first)
	}
	stableID := first[0].ID

	// Newer aggregation with more evidence refreshes the same entry.
	events = append(events, driftEvent("run-3", "turn-3", driftdetect.SignalRepeatedTestFailure))
	if err := svc.UpsertCandidates(ctx, AggregateDriftEvents(events, 2)); err != nil {
		t.Fatalf("UpsertCandidates (refresh): %v", err)
	}
	second, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("same pattern must upsert, got %d entries", len(second))
	}
	if second[0].ID != stableID {
		t.Errorf("refresh must keep the candidate ID, got %q want %q", second[0].ID, stableID)
	}
	if second[0].RepeatCount != 3 {
		t.Errorf("RepeatCount = %d, want 3", second[0].RepeatCount)
	}
	if want := []string{"run-1", "run-2", "run-3"}; !reflect.DeepEqual(second[0].SupportingRunIDs, want) {
		t.Errorf("SupportingRunIDs = %v, want %v", second[0].SupportingRunIDs, want)
	}

	// A rejected pattern is never resurrected or duplicated by re-aggregation.
	if err := svc.RejectCandidate(ctx, stableID); err != nil {
		t.Fatalf("RejectCandidate: %v", err)
	}
	events = append(events, driftEvent("run-4", "turn-4", driftdetect.SignalRepeatedTestFailure))
	if err := svc.UpsertCandidates(ctx, AggregateDriftEvents(events, 2)); err != nil {
		t.Fatalf("UpsertCandidates (after reject): %v", err)
	}
	third, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(third) != 1 || third[0].Status != StatusRejected || third[0].RepeatCount != 3 {
		t.Fatalf("rejected entry must be left untouched, got %+v", third)
	}

	// T-1: ingestion forces StatusCandidate even if the caller lies.
	if err := svc.UpsertCandidates(ctx, []LessonCandidate{{
		ID: "lc-smuggled", Title: "Smuggled", TriggerPattern: "a unique fresh pattern", Status: StatusPromoted,
	}}); err != nil {
		t.Fatalf("UpsertCandidates (smuggled): %v", err)
	}
	final, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	smuggled := findCandidate(t, final, "unique fresh pattern")
	if smuggled.Status != StatusCandidate {
		t.Errorf("ingested candidate must stay %q (T-1), got %q", StatusCandidate, smuggled.Status)
	}
}

// CA-838 reviewer note: gate-resume replays append duplicate JSONL events for
// the same (run, turn) — the aggregator must count each turn once.
func TestAggregateDriftEvents_DedupesGateResumeReplays(t *testing.T) {
	events := []driftdetect.DriftEvent{
		driftEvent("run-1", "turn-1", driftdetect.SignalRepeatedTestFailure),
		driftEvent("run-1", "turn-1", driftdetect.SignalRepeatedTestFailure), // replay duplicate
		driftEvent("run-1", "turn-2", driftdetect.SignalRepeatedTestFailure),
	}
	got := AggregateDriftEvents(events, 2)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if got[0].RepeatCount != 2 {
		t.Errorf("RepeatCount = %d, want 2 (duplicates of the same turn count once)", got[0].RepeatCount)
	}
}

// DOD item 5: exports support .agents/.claude/.grok provider dirs.
func TestSkillExporter_MultiProviderSkillDirs(t *testing.T) {
	project := t.TempDir()
	if err := PromoteCandidateToSkill(goroutineCandidate(), SkillExportOptions{
		WorkspaceRoot: project,
		SkillDirs:     ProjectSkillDirsAll,
	}); err != nil {
		t.Fatalf("PromoteCandidateToSkill: %v", err)
	}
	for _, dir := range ProjectSkillDirsAll {
		providerFile := filepath.Join(project, dir, "golang", "goroutine-safety", "SKILL.md")
		if _, err := os.Stat(providerFile); err != nil {
			t.Errorf("provider skill missing at %s: %v", providerFile, err)
		}
	}
}

// CP-23 T-3: the compact rule card is 3-5 deterministic lines (library-only
// until the Task-334 BudgetPacker wiring).
func TestCompactRuleCard_LinesAndDeterminism(t *testing.T) {
	card := CompactRuleCard(goroutineCandidate())
	lines := strings.Split(card, "\n")
	if len(lines) < 3 || len(lines) > 5 {
		t.Fatalf("compact rule card must be 3-5 lines, got %d:\n%s", len(lines), card)
	}
	for _, want := range []string{
		"- Rule: Always pass a context with a cancel func",
		"- Avoid: Spawning goroutines without a cancellation or timeout path.",
		"- Trigger: turns that spawn goroutines without a cancellation path",
		"- Skill: goroutine-safety",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("card missing %q, got:\n%s", want, card)
		}
	}
	if card != CompactRuleCard(goroutineCandidate()) {
		t.Errorf("CompactRuleCard is not deterministic")
	}
	if got := CompactRuleCard(LessonCandidate{}); got != "" {
		t.Errorf("empty candidate must yield an empty card, got %q", got)
	}
}

// Task-336 §6 Acceptance Check end-to-end run (matches the
// `go test ./internal/skilllearn/ -run TestSkillPromotion` command): 2 drift
// events of the same pattern -> 1 candidate -> human approve -> SKILL.md in the
// target project -> core export -> discoverable via the real skillpack install.
func TestSkillPromotion_EndToEnd_CandidateToSkill(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	target := filepath.Join(tmp, "target")
	coreRoot := filepath.Join(tmp, "flow-pack")
	for _, dir := range []string{workspace, target} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("setup %s: %v", dir, err)
		}
	}

	// 1. Two same-pattern drift events -> exactly 1 LessonCandidate.
	candidates := AggregateDriftEvents([]driftdetect.DriftEvent{
		driftEvent("run-1", "turn-1", driftdetect.SignalOutOfScopeEdit),
		driftEvent("run-2", "turn-2", driftdetect.SignalOutOfScopeEdit),
	}, 2)
	if len(candidates) != 1 || candidates[0].Status != StatusCandidate || candidates[0].RepeatCount != 2 {
		t.Fatalf("aggregation produced %+v", candidates)
	}

	// 2. Persist (forced to candidate — T-1), approve and export to the target.
	svc := NewPromoterService(workspace)
	if err := svc.UpsertCandidates(ctx, candidates); err != nil {
		t.Fatalf("UpsertCandidates: %v", err)
	}
	if err := svc.ApproveCandidateAndExport(ctx, candidates[0].ID, SkillExportOptions{
		WorkspaceRoot: target,
		SkillDirs:     ProjectSkillDirsAll,
	}); err != nil {
		t.Fatalf("ApproveCandidateAndExport: %v", err)
	}

	// 3. Core export for the platform pack, then the real install path.
	published, err := SkillExporter{}.Export(candidates[0], SkillExportOptions{
		WorkspaceRoot: workspace, ExportToCore: true, FlowPackRoot: coreRoot,
	})
	if err != nil {
		t.Fatalf("core export: %v", err)
	}
	if _, err := skillpack.InstallFromRoot(target, "golang", coreRoot); err != nil {
		t.Fatalf("InstallFromRoot: %v", err)
	}

	// 4. Assert everything landed where the CP-23 ladder says it must.
	final, err := svc.ListCandidates(ctx)
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if final[0].Status != StatusPromoted {
		t.Errorf("status = %q, want promoted", final[0].Status)
	}
	slug := Slugify(candidates[0].Title) // "repeated-drift-out-of-scope-edit"
	for _, want := range append([]string{published[0]}, filepath.Join(target, ".agents", "skills", "common", slug, "SKILL.md")) {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected artifact missing: %s (%v)", want, err)
		}
	}
}

// Review-hardening regression (Task-336 §8 follow-up): a hostile Group value
// must never escape the skills directory — separators and dot-dot segments
// collapse to a safe slug, while ordinary groups pass through unchanged.
func TestGroupDirName_PathSafety(t *testing.T) {
	cases := map[string]string{
		"golang":    "golang", // ordinary groups are untouched
		"common":    "common",
		"kmm":       "kmm",
		"":          "common",    // empty → default group
		"../..":     "group",     // traversal collapses away
		"a/b":       "a-b-group", // separators collapse
		"..":        "group",
		"android x": "android-x-group",
	}
	for in, want := range cases {
		if got := GroupDirName(in); got != want {
			t.Fatalf("GroupDirName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := GroupDirName("../.."); strings.Contains(got, "..") || strings.ContainsRune(got, '/') {
		t.Fatalf("hostile group must not survive, got %q", got)
	}
}

// Review-hardening regression: an end-to-end export with a hostile Group
// writes INSIDE the skills root only.
func TestSkillExporter_HostileGroup_StaysInsideSkillsRoot(t *testing.T) {
	root := t.TempDir()
	cand := goroutineCandidate()
	cand.Group = "../.."
	exp := SkillExporter{}
	written, err := exp.Export(cand, SkillExportOptions{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(written) == 0 {
		t.Fatalf("expected at least one written skill file")
	}
	escaped := false
	for _, p := range written {
		if !strings.HasPrefix(filepath.Clean(p), filepath.Clean(root)) {
			escaped = true
		}
	}
	if escaped {
		t.Fatalf("export escaped the workspace root: %v", written)
	}
}
