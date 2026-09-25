package flowgate

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-395 (live CP-61 run-2688/run-4464, CP-58): r-dod-present demanded a
// "## Definition of Done" section that no writer prompt or format reference
// defines — plan-task.md/task-splitter.md/FORMAT-REFERENCE-TASK.md specify the
// checklist under "## 6. Acceptance Check". The gate must accept the
// spec-mandated acceptance checklist as the DoD-equivalent section.
func TestBug395_AcceptanceCheckSectionSatisfiesDoD(t *testing.T) {
	doc := "# Task-912\n\n## 6. Acceptance Check\n\n- [ ] login works\n- [ ] refresh rotates\n\n## 7. Notes\n"
	st := ParseDefinitionOfDone(doc)
	if !st.Present || st.Total != 2 {
		t.Fatalf("spec-mandated Acceptance Check checklist must count as DoD, got %+v", st)
	}
}

func TestBug395_AcceptanceCheckWithoutCheckboxesStillMissing(t *testing.T) {
	doc := "# Task-912\n\n## 6. Acceptance Check\n\nEverything should work.\n\n## 7. Notes\n"
	st := ParseDefinitionOfDone(doc)
	if st.Present {
		t.Fatalf("Acceptance Check heading with no checkboxes must still be missing, got %+v", st)
	}
}

func TestBug395_MissingDodDocsAcceptsAcceptanceCheck(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Task-912-x.md")
	if err := os.WriteFile(p, []byte("# Task\n\n## 6. Acceptance Check\n\n- [ ] verified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := MissingDodDocs(dir, []string{"Task-912-x.md"})
	if len(missing) != 0 {
		t.Fatalf("Task doc with spec-mandated Acceptance Check must not be flagged, got %v", missing)
	}
}
