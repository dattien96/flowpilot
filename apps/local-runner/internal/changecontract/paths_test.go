package changecontract

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestNormalizeDeclaredCodePathsNormalizesSeparators(t *testing.T) {
	ws := t.TempDir()
	got, err := NormalizeDeclaredCodePaths(ws, []string{`src\calc\Calc.go`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"src/calc/Calc.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v (case must be preserved)", got, want)
	}
}

func TestNormalizeDeclaredCodePathsSortsAndDeduplicates(t *testing.T) {
	ws := t.TempDir()
	got, err := NormalizeDeclaredCodePaths(ws, []string{"z/z.go", "a/a.go", "z/z.go", "./m/m.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a/a.go", "m/m.go", "z/z.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormalizeDeclaredCodePathsRejectsOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	for _, p := range []string{"../escape.go", "../../etc/passwd.go"} {
		if _, err := NormalizeDeclaredCodePaths(ws, []string{p}); err == nil {
			t.Errorf("%q should be rejected as outside workspace", p)
		}
	}
}

func TestNormalizeDeclaredCodePathsRejectsAbsoluteOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	abs := filepath.Join(outside, "foo.go")
	if _, err := NormalizeDeclaredCodePaths(ws, []string{filepath.ToSlash(abs)}); err == nil {
		t.Fatalf("absolute path outside workspace must be rejected")
	}
}

func TestNormalizeDeclaredCodePathsAcceptsAbsoluteInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	abs := filepath.Join(ws, "src", "calc.go")
	got, err := NormalizeDeclaredCodePaths(ws, []string{filepath.ToSlash(abs)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"src/calc.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormalizeDeclaredCodePathsRejectsGlobOnlyScope(t *testing.T) {
	ws := t.TempDir()
	if _, err := NormalizeDeclaredCodePaths(ws, []string{"internal/*.go", "src/?.ts"}); err == nil {
		t.Fatal("an all-glob scope must be rejected as having no concrete code path")
	}
}

func TestNormalizeDeclaredCodePathsRejectsDocumentOnlyWriterScope(t *testing.T) {
	ws := t.TempDir()
	if _, err := NormalizeDeclaredCodePaths(ws, []string{
		"requirements/08-Task/todo/Task-1.md",
		"change-audit/CA-1.md",
	}); err == nil {
		t.Fatal("an all-doc scope must be rejected as having no concrete code path")
	}
}

func TestNormalizeDeclaredCodePathsRejectsBucketOnlyScope(t *testing.T) {
	ws := t.TempDir()
	if _, err := NormalizeDeclaredCodePaths(ws, []string{"apps", "internal"}); err == nil {
		t.Fatal("directory-bucket-only scope must be rejected as having no concrete code path")
	}
}

func TestNormalizeDeclaredCodePathsEmptyInputIsError(t *testing.T) {
	ws := t.TempDir()
	if _, err := NormalizeDeclaredCodePaths(ws, nil); err == nil {
		t.Fatal("empty input must be rejected")
	}
}

func TestNormalizeDeclaredCodePathsMixedNoiseAndConcreteKeepsConcrete(t *testing.T) {
	ws := t.TempDir()
	got, err := NormalizeDeclaredCodePaths(ws, []string{
		"internal/*.go",
		"requirements/08-Task/todo/Task-1.md",
		"-rf",
		"apps",
		"src/keep.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"src/keep.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormalizeDeclaredCodePathsRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows CI")
	}
	ws := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "real.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ws, "link.go")
	if err := os.Symlink(filepath.Join(outside, "real.go"), link); err != nil {
		t.Skipf("cannot create symlink in this environment: %v", err)
	}
	if _, err := NormalizeDeclaredCodePaths(ws, []string{"link.go"}); err == nil {
		t.Fatal("a symlink resolving outside workspace must be rejected")
	}
}

func TestNormalizeDeclaredCodePathsToBeCreatedFileSkipsSymlinkCheck(t *testing.T) {
	ws := t.TempDir()
	// src/new.go does not exist on disk yet — EvalSymlinks will fail to
	// resolve it, which must not be treated as an escape.
	got, err := NormalizeDeclaredCodePaths(ws, []string{"src/new.go"})
	if err != nil {
		t.Fatalf("a not-yet-created path must not error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"src/new.go"}) {
		t.Fatalf("got %v, want [src/new.go]", got)
	}
}

func TestNormalizeDeclaredCodePathsBlankWorkspaceStillFiltersNoise(t *testing.T) {
	got, err := NormalizeDeclaredCodePaths("", []string{"internal/*.go", "src/keep.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"src/keep.go"}) {
		t.Fatalf("got %v, want [src/keep.go]", got)
	}
}
