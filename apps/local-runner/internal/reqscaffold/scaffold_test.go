package reqscaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldCreatesRequiredFolders(t *testing.T) {
	dir := t.TempDir()
	result, err := Scaffold(dir)
	if err != nil {
		t.Fatalf("Scaffold error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected scaffold errors: %v", result.Errors)
	}

	wantDirs := []string{
		filepath.Join(dir, "requirements", "05-System-Specs"),
		filepath.Join(dir, "requirements", "06-System-Tech-Design"),
		filepath.Join(dir, "requirements", "07-Coding-Plan"),
		filepath.Join(dir, "requirements", "07-Coding-Plan", "todo"),
		filepath.Join(dir, "requirements", "07-Coding-Plan", "inprogress"),
		filepath.Join(dir, "requirements", "07-Coding-Plan", "done"),
		filepath.Join(dir, "requirements", "08-Task"),
		filepath.Join(dir, "requirements", "08-Task", "todo"),
		filepath.Join(dir, "requirements", "08-Task", "done"),
		filepath.Join(dir, "requirements", "09-BugFix"),
		filepath.Join(dir, "requirements", "09-BugFix", "todo"),
		filepath.Join(dir, "requirements", "09-BugFix", "done"),
	}
	for _, d := range wantDirs {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			t.Errorf("expected directory %s to exist", d)
		}
	}
}

func TestScaffoldCopiesFormatReferenceFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Scaffold(dir); err != nil {
		t.Fatalf("Scaffold error: %v", err)
	}

	wantFiles := []string{
		filepath.Join(dir, "requirements", "05-System-Specs", "FORMAT-REFERENCE-SS.md"),
		filepath.Join(dir, "requirements", "06-System-Tech-Design", "FORMAT-REFERENCE-SD.md"),
		filepath.Join(dir, "requirements", "07-Coding-Plan", "FORMAT-REFERENCE-CP.md"),
		filepath.Join(dir, "requirements", "08-Task", "FORMAT-REFERENCE-TASK.md"),
		filepath.Join(dir, "requirements", "09-BugFix", "FORMAT-REFERENCE-BUGFIX.md"),
	}
	for _, f := range wantFiles {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected file %s to exist: %v", f, err)
		}
	}
}

func TestScaffoldSkipsExistingFormatFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create one of the folders and its FORMAT-REFERENCE with custom content.
	customDir := filepath.Join(dir, "requirements", "08-Task")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatal(err)
	}
	customContent := []byte("# My custom task format\n")
	customFile := filepath.Join(customDir, "FORMAT-REFERENCE-TASK.md")
	if err := os.WriteFile(customFile, customContent, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Scaffold(dir)
	if err != nil {
		t.Fatalf("Scaffold error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected scaffold errors: %v", result.Errors)
	}

	// The custom file must not have been overwritten.
	got, err := os.ReadFile(customFile)
	if err != nil {
		t.Fatalf("read custom file: %v", err)
	}
	if string(got) != string(customContent) {
		t.Errorf("existing FORMAT-REFERENCE-TASK.md was overwritten; got %q", string(got))
	}

	// The file must appear in Skipped.
	found := false
	for _, s := range result.Skipped {
		if s == customFile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s in Skipped, got Skipped=%v", customFile, result.Skipped)
	}
}

func TestIsScaffolded(t *testing.T) {
	dir := t.TempDir()

	if IsScaffolded(dir) {
		t.Error("expected false before scaffolding")
	}

	if _, err := Scaffold(dir); err != nil {
		t.Fatalf("Scaffold error: %v", err)
	}

	if !IsScaffolded(dir) {
		t.Error("expected true after scaffolding")
	}
}

func TestScaffoldIdempotent(t *testing.T) {
	dir := t.TempDir()

	r1, err := Scaffold(dir)
	if err != nil || len(r1.Errors) != 0 {
		t.Fatalf("first Scaffold failed: err=%v errs=%v", err, r1.Errors)
	}

	r2, err := Scaffold(dir)
	if err != nil || len(r2.Errors) != 0 {
		t.Fatalf("second Scaffold failed: err=%v errs=%v", err, r2.Errors)
	}

	// Second run should create nothing new (everything was Skipped or already there).
	if len(r2.Created) != 0 {
		// Subdirectories are always "created" via MkdirAll (no error if present),
		// but files must not be re-created.
		for _, c := range r2.Created {
			info, statErr := os.Stat(c)
			if statErr == nil && !info.IsDir() {
				t.Errorf("second run created a file it should have skipped: %s", c)
			}
		}
	}
}
