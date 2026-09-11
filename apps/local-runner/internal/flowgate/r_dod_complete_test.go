package flowgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task-331 (CP-47 P-3/P-4): gate tests for r-dod-complete and the done
// transition signal. New file only — no pre-existing test is modified.

const (
	// doneDocAllChecked is a done Task document whose DOD is fully ticked.
	doneDocAllChecked = `# Task-300: Sample

## Metadata

- Status: done

## Definition of Done

- [x] Parser handles missing section
- [x] Parser counts checkboxes
- [x] Gate fires on missing DOD
`
	// doneDocOpenItems is a done Task document with one open checkbox and no
	// explanation anywhere (metadata done; the checkbox text carries no
	// explanation phrase).
	doneDocOpenItems = `# Task-301: Sample

## Metadata

- Status: done

## Definition of Done

- [x] Parser handles missing section
- [x] Parser counts checkboxes
- [ ] Gate fires on missing DOD
`
	// doneDocPathOnlyDraft is a doc sitting in a done/ directory whose
	// metadata still says draft (path-based done detection must win).
	doneDocPathOnlyDraft = `# Task-302: Sample

## Metadata

- Status: draft

## Definition of Done

- [x] Step one
- [ ] Step two pending review
`
	// doneDocDeferredSection explains its open item via a Deferred section.
	doneDocDeferredSection = `# Task-303: Sample

## Metadata

- Status: done

## Definition of Done

- [x] Step one
- [ ] Step two

## Deferred

Step two moves to the next sprint (blocked by an upstream API).
`
)

// writeDodCompleteDoc writes content at rel (slash-separated) under dir.
func writeDodCompleteDoc(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findRDodCompleteViolation returns the r-dod-complete violation, or nil.
func findRDodCompleteViolation(violations []Violation) *Violation {
	for i := range violations {
		if violations[i].Rule.ID == "r-dod-complete" {
			return &violations[i]
		}
	}
	return nil
}

// findRDodPresentViolationIn returns the r-dod-present violation, or nil.
func findRDodPresentViolationIn(violations []Violation) *Violation {
	for i := range violations {
		if violations[i].Rule.ID == "r-dod-present" {
			return &violations[i]
		}
	}
	return nil
}

// Scenario: Đánh dấu done khi tất cả checkbox đã tick [x]
// Input: DodTransitionedToDone=true, DodStatus{Total: 3, Checked: 3, OpenItems: []}
// Expect: Không phát sinh vi phạm (Pass)
func TestRDodComplete_AllChecked_Pass(t *testing.T) {
	tr := TurnResult{
		DodTransitionedToDone: true,
		DodStatus: DodStatus{
			Present: true,
			Total:   3,
			Checked: 3,
		},
	}
	if v := findRDodCompleteViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("r-dod-complete must pass when every DOD checkbox is ticked, got %+v", v)
	}
}

// Scenario: Đánh dấu done nhưng còn checkbox [ ] mở và không giải trình
// Input: DodTransitionedToDone=true, DodStatus{Total: 3, Checked: 2, OpenItems: ["Item 3"]}, FinalMessage="Xong task"
// Expect: Phát sinh vi phạm Rule="r-dod-complete", Action="block", Detail chứa "Item 3"
func TestRDodComplete_OpenItemsNoExplanation_Block(t *testing.T) {
	tr := TurnResult{
		DodTransitionedToDone: true,
		DodStatus: DodStatus{
			Present:   true,
			Total:     3,
			Checked:   2,
			OpenItems: []string{"Item 3"},
		},
		FinalMessage: "Xong task",
	}
	v := findRDodCompleteViolation(Evaluate(tr, DefaultRules()))
	if v == nil {
		t.Fatal("expected r-dod-complete violation when done with open DOD items and no explanation")
	}
	if v.Rule.Action != "block" {
		t.Fatalf("Action = %q, want block (no explanation anywhere)", v.Rule.Action)
	}
	if !strings.Contains(v.Detail, "Item 3") {
		t.Fatalf("detail %q must name the open checkbox verbatim", v.Detail)
	}
}

// Scenario: Đánh dấu done, còn checkbox [ ] mở nhưng có giải trình hợp lệ
// Input: DodTransitionedToDone=true, DodStatus{Total: 3, Checked: 2, OpenItems: ["Item 3"]}, FinalMessage="Hoãn Item 3 sang phase sau vì lý do phụ thuộc API"
// Expect: Phát sinh vi phạm Action="warn" (hạ cấp từ block), cho phép qua
func TestRDodComplete_OpenItemsWithExplanation_Warn(t *testing.T) {
	tr := TurnResult{
		DodTransitionedToDone: true,
		DodStatus: DodStatus{
			Present:   true,
			Total:     3,
			Checked:   2,
			OpenItems: []string{"Item 3"},
		},
		FinalMessage: "Hoãn Item 3 sang phase sau vì lý do phụ thuộc API",
	}
	v := findRDodCompleteViolation(Evaluate(tr, DefaultRules()))
	if v == nil {
		t.Fatal("expected a downgraded r-dod-complete violation when an explanation exists")
	}
	if v.Rule.Action != "warn" {
		t.Fatalf("Action = %q, want warn (block downgraded by valid explanation)", v.Rule.Action)
	}
	if !strings.Contains(v.Detail, "Item 3") {
		t.Fatalf("detail %q must still name the open item", v.Detail)
	}
	// "Cho phép qua": the warn violation must not resolve to block under the
	// default enforce gate mode.
	if result := Enforce(Evaluate(tr, DefaultRules()), "enforce"); result.Action != "warn" {
		t.Fatalf("Enforce action = %q, want warn (explained open items pass with warning)", result.Action)
	}
}

// Scenario: Lượt chạy bình thường, doc chưa chuyển sang done
// Input: DodTransitionedToDone=false, DodStatus{Total: 3, Checked: 1}
// Expect: Không kích hoạt rule r-dod-complete
func TestRDodComplete_NotTransitionedToDone_Ignore(t *testing.T) {
	tr := TurnResult{
		DodTransitionedToDone: false,
		DodStatus: DodStatus{
			Present:   true,
			Total:     3,
			Checked:   1,
			OpenItems: []string{"Item 2", "Item 3"},
		},
	}
	if v := findRDodCompleteViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("r-dod-complete must not fire when no doc transitioned to done, got %+v", v)
	}
}

// [Edge] Scenario: Section DOD tồn tại nhưng Total == 0 (heading mà không có checkbox)
// Input: DodTransitionedToDone=true, DodStatus{Present: false, Total: 0}
// Expect: Trả về vi phạm r-dod-present (thiếu DOD), không phải r-dod-complete
func TestRDodComplete_ZeroDodItems_DelegatesToPresent(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/done/Task-700.md"
	// DOD heading with zero checkboxes — the parser reports Total == 0.
	writeDodCompleteDoc(t, dir, rel, "# Task-700: Empty DOD\n\n## Definition of Done\n\nNo checkboxes here.\n")
	status, ok := DodDoneTransition(dir, []string{rel})
	if !ok {
		t.Fatal("done/ path segment must mark the doc transitioned")
	}
	if status.Present || status.Total != 0 {
		t.Fatalf("DodStatus = %+v, want Present=false Total=0", status)
	}
	tr := TurnResult{
		WorkspaceCwd:          dir,
		WrittenPaths:          []string{rel},
		DodTransitionedToDone: true,
		DodStatus:             status,
	}
	violations := Evaluate(tr, DefaultRules())
	if v := findRDodCompleteViolation(violations); v != nil {
		t.Fatalf("r-dod-complete must not fire for Total == 0 (delegates to r-dod-present), got %+v", v)
	}
	if v := findRDodPresentViolationIn(violations); v == nil {
		t.Fatal("expected r-dod-present to own the zero-checkbox DOD doc")
	}
}

// [Edge] Scenario: File nằm trong thư mục done/ nhưng metadata Status vẫn là draft
// Input: WrittenPaths=["requirements/08-Task/done/Task-001.md"], metadata Status=draft
// Expect: Vẫn kích hoạt r-dod-complete vì đường dẫn chứa done/ là tín hiệu đủ
func TestRDodComplete_PathBasedDoneDetection(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/done/Task-702.md"
	writeDodCompleteDoc(t, dir, rel, doneDocPathOnlyDraft)
	status, ok := DodDoneTransition(dir, []string{rel})
	if !ok {
		t.Fatal("a done/ path segment is a sufficient done signal even with Status: draft metadata")
	}
	if status.Total != 2 || status.Checked != 1 || len(status.OpenItems) != 1 {
		t.Fatalf("DodStatus = %+v, want 2 total / 1 checked / 1 open", status)
	}
	tr := TurnResult{
		WorkspaceCwd:          dir,
		WrittenPaths:          []string{rel},
		DodTransitionedToDone: true,
		DodStatus:             status,
	}
	v := findRDodCompleteViolation(Evaluate(tr, DefaultRules()))
	if v == nil {
		t.Fatal("expected r-dod-complete to fire for a done/-path doc with an open checkbox")
	}
	if v.Rule.Action != "block" {
		t.Fatalf("Action = %q, want block (no explanation in doc or message)", v.Rule.Action)
	}
	if !strings.Contains(v.Detail, "Step two pending review") {
		t.Fatalf("detail %q must name the open checkbox", v.Detail)
	}
}

// [Error] Scenario: File không thể đọc nội dung khi kiểm tra DOD
// Input: WrittenPaths chứa file hợp lệ nhưng I/O error khi đọc
// Expect: Gate degrade graceful - log cảnh báo, không panic, không block
func TestRDodComplete_FileReadError_GracefulDegradation(t *testing.T) {
	dir := t.TempDir() // deliberately does NOT contain the doc
	rel := "requirements/08-Task/done/Task-705.md"
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("gate must not panic on unreadable file, got panic: %v", r)
		}
	}()
	// The wiring contract: the doc cannot be read, so DodDoneTransition
	// yields no signal and the runner leaves the TurnResult fields zero —
	// r-dod-complete no-ops instead of blocking on unverifiable state.
	if _, ok := DodDoneTransition(dir, []string{rel}); ok {
		t.Fatal("unreadable doc must contribute no done signal")
	}
	tr := TurnResult{
		WorkspaceCwd:          dir,
		WrittenPaths:          []string{rel},
		DodTransitionedToDone: true,
		DodStatus:             DodStatus{}, // zero: nothing could be parsed
	}
	if v := findRDodCompleteViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("unreadable doc must degrade gracefully (no block), got %+v", v)
	}
	// The explanation scanner likewise skips unreadable docs without
	// panicking and stays deterministic (no doc-side explanation found).
	open := TurnResult{
		WorkspaceCwd:          dir,
		WrittenPaths:          []string{rel},
		DodTransitionedToDone: true,
		DodStatus: DodStatus{
			Present:   true,
			Total:     2,
			Checked:   1,
			OpenItems: []string{"Item B"},
		},
	}
	if hasValidDodExplanation(open) {
		t.Fatal("no explanation source is reachable when the doc cannot be read and the message is silent")
	}
}

// Task-331 §9: r-dod-complete must be registered with the exact rule shape
// and join the tier-1 doc family next to r-dod-present (CP-47 §5).
func TestRDodCompleteRegisteredInDefaultRules(t *testing.T) {
	found := false
	for _, r := range DefaultRules() {
		if r.ID == "r-dod-complete" {
			found = true
			if r.Trigger != "marked_done_with_open_dod" {
				t.Fatalf("r-dod-complete trigger = %q, want marked_done_with_open_dod", r.Trigger)
			}
			if r.RequiredOutput != "dod_all_checked_or_explained" {
				t.Fatalf("r-dod-complete required_output = %q, want dod_all_checked_or_explained", r.RequiredOutput)
			}
			if r.Action != "block" {
				t.Fatalf("r-dod-complete action = %q, want block", r.Action)
			}
			if r.Scope != "step" {
				t.Fatalf("r-dod-complete scope = %q, want step", r.Scope)
			}
			if !r.Enabled {
				t.Fatal("r-dod-complete must be enabled")
			}
		}
	}
	if !found {
		t.Fatal("DefaultRules must contain r-dod-complete (Task-331)")
	}
	if !IsDocScopeRule("r-dod-complete") {
		t.Fatal("r-dod-complete must be in DocScopeRuleIDs (CP-47 §5 tier-1 doc family)")
	}
}

// Task-331 §9: MergeDefaultRules must auto-add r-dod-complete to a legacy
// workspace flow-rules.json that predates the rule.
func TestMergeDefaultRulesAddsRDodComplete(t *testing.T) {
	legacy := []Rule{
		{ID: "r-ca", Scope: "step", Trigger: "code_changed", RequiredOutput: "change_audit_note", Action: "reprompt", Enabled: true},
	}
	dir := t.TempDir()
	if err := SaveRules(dir, legacy); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	loaded, err := LoadRules(dir)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	found := false
	for _, r := range loaded {
		if r.ID == "r-dod-complete" {
			found = true
		}
	}
	if !found {
		t.Fatal("LoadRules must merge r-dod-complete into a legacy flow-rules.json")
	}
	merged := MergeDefaultRules(legacy)
	found = false
	for _, r := range merged {
		if r.ID == "r-dod-complete" {
			found = true
		}
	}
	if !found {
		t.Fatal("MergeDefaultRules must append r-dod-complete when missing")
	}
}

// Metadata priority (CP-47 D-2): `- Status: done` alone marks the transition
// even when the doc lives outside a done/ directory, and the checklist is
// still parsed from the same doc.
func TestDodDoneTransition_MetadataStatusDone(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/todo/Task-706.md"
	writeDodCompleteDoc(t, dir, rel, doneDocOpenItems)
	status, ok := DodDoneTransition(dir, []string{rel})
	if !ok {
		t.Fatal("metadata Status: done must mark the transition without a done/ path")
	}
	if status.Total != 3 || status.Checked != 2 || len(status.OpenItems) != 1 {
		t.Fatalf("DodStatus = %+v, want 3 total / 2 checked / 1 open", status)
	}
}

// No done signal anywhere (no metadata, no done/ path) — the gate must not
// fire for a plain in-progress doc write.
func TestDodDoneTransition_NoSignal(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/todo/Task-707.md" // todo/, metadata draft
	writeDodCompleteDoc(t, dir, rel, doneDocPathOnlyDraft)
	if _, ok := DodDoneTransition(dir, []string{rel}); ok {
		t.Fatal("a draft doc outside done/ must not be treated as transitioned")
	}
}

// Fail-closed bias: when one transitioned doc is fully checked and another
// still has open items, the open one wins so the gate cannot be masked by a
// completed sibling.
func TestDodDoneTransition_PrefersOpenChecklist(t *testing.T) {
	dir := t.TempDir()
	okRel := "requirements/08-Task/done/Task-708.md"
	openRel := "requirements/09-BugFix/done/BUG-709.md"
	writeDodCompleteDoc(t, dir, okRel, doneDocAllChecked)
	writeDodCompleteDoc(t, dir, openRel, doneDocOpenItems)
	status, ok := DodDoneTransition(dir, []string{okRel, openRel})
	if !ok {
		t.Fatal("expected a transitioned doc")
	}
	if status.Checked >= status.Total {
		t.Fatalf("DodStatus = %+v, want the open checklist to win (fail-closed bias)", status)
	}
}

// Explanation detection (Task-331 T-2): a "## Deferred" / "## Open Items"
// section in the written doc downgrades the block to warn; a bare message
// without phrases does not.
func TestHasValidDodExplanation_DocSectionDowngradesToWarn(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/done/Task-710.md"
	writeDodCompleteDoc(t, dir, rel, doneDocDeferredSection)
	tr := TurnResult{
		WorkspaceCwd:          dir,
		WrittenPaths:          []string{rel},
		DodTransitionedToDone: true,
		DodStatus: DodStatus{
			Present:   true,
			Total:     2,
			Checked:   1,
			OpenItems: []string{"Step two"},
		},
	}
	v := findRDodCompleteViolation(Evaluate(tr, DefaultRules()))
	if v == nil {
		t.Fatal("expected a r-dod-complete violation for the open checkbox")
	}
	if v.Rule.Action != "warn" {
		t.Fatalf("Action = %q, want warn (Deferred section is a valid explanation)", v.Rule.Action)
	}
}

// Explanation detection via the open checkbox line itself: the OpenItems text
// already parsed into DodStatus carries the phrase, so no message or section
// is needed.
func TestHasValidDodExplanation_OpenItemPhrase(t *testing.T) {
	tr := TurnResult{
		DodTransitionedToDone: true,
		DodStatus: DodStatus{
			Present:   true,
			Total:     2,
			Checked:   1,
			OpenItems: []string{"Mục X (Hoãn sang sprint sau do phụ thuộc Y)"},
		},
	}
	v := findRDodCompleteViolation(Evaluate(tr, DefaultRules()))
	if v == nil {
		t.Fatal("expected a r-dod-complete violation for the open checkbox")
	}
	if v.Rule.Action != "warn" {
		t.Fatalf("Action = %q, want warn (checkbox-adjacent explanation)", v.Rule.Action)
	}
}
