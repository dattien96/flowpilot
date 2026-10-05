package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// CA-1205 (live run-183756): vault_metadata_test.cpp was written but never
// added to CMakeLists.txt — the suite ran green on already-landed tests and
// r-scaffold-red reprompted "implemented stubs" instead of the registration
// gap. The helper must flag C/C++ test sources no CMake manifest references.

func TestCA1205_UnregisteredTestFileFlagged(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "CMakeLists.txt"),
		[]byte("add_executable(tests existing_test.cpp)\ngtest_discover_tests(tests)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &InteractiveService{}
	got := svc.scaffoldUnregisteredTestFiles(cwd, []string{
		"core/vault/vault_metadata_test.cpp", // never registered
		"tests/existing_test.cpp",            // registered by basename
		"core/vault/vault.cpp",               // not a test file
	})
	if len(got) != 1 || got[0] != "core/vault/vault_metadata_test.cpp" {
		t.Fatalf("unregistered = %v, want [core/vault/vault_metadata_test.cpp]", got)
	}
}

func TestCA1205_RegisteredTestFileClean(t *testing.T) {
	cwd := t.TempDir()
	sub := filepath.Join(cwd, "tests")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "CMakeLists.txt"),
		[]byte("target_sources(tests PRIVATE vault_metadata_test.cpp)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &InteractiveService{}
	if got := svc.scaffoldUnregisteredTestFiles(cwd, []string{"tests/vault_metadata_test.cpp"}); len(got) != 0 {
		t.Fatalf("registered test must not be flagged, got %v", got)
	}
}

func TestCA1205_NoCMakeProjectIsNoSignal(t *testing.T) {
	cwd := t.TempDir() // Kotlin/Gradle-shaped workspace — no manifests at all.
	svc := &InteractiveService{}
	if got := svc.scaffoldUnregisteredTestFiles(cwd, []string{"x_test.cpp"}); len(got) != 0 {
		t.Fatalf("no CMake manifest → fail-open, got %v", got)
	}
}

func TestCA1205_NonCppTestFilesSkipped(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "CMakeLists.txt"), []byte("project(x)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &InteractiveService{}
	// Go/Kotlin tests auto-discover — the CMake check must not flag them.
	if got := svc.scaffoldUnregisteredTestFiles(cwd, []string{"vault_test.go", "VaultTest.kt"}); len(got) != 0 {
		t.Fatalf("non-C/C++ test files must be skipped, got %v", got)
	}
}
