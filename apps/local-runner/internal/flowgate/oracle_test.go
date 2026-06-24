package flowgate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTestFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"internal/foo/foo_test.go", true},
		{"internal/foo/foo.go", false},
		{"src/util.test.ts", true},
		{"src/util.spec.tsx", true},
		{"src/util.test.js", true},
		{"src/util.spec.jsx", true},
		{"src/main.ts", false},
		{"tests/test_utils.py", true},
		{"lib/test_helper.py", true},
		{"app/utils.py", false},
		{"pkg/test_something/test_it.py", true},
	}
	for _, c := range cases {
		if got := IsTestFile(c.path); got != c.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestDetectTestCommandGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := DetectTestCommand(dir)
	if cmd != "go test -v ./..." {
		t.Errorf("expected 'go test -v ./...', got %q", cmd)
	}
}

func TestDetectTestCommandPackageJSON(t *testing.T) {
	dir := t.TempDir()
	content := `{"name":"app","scripts":{"test":"jest"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := DetectTestCommand(dir)
	if cmd != "npm test" {
		t.Errorf("expected 'npm test', got %q", cmd)
	}
}

func TestDetectTestCommandPackageJSONNoTestScript(t *testing.T) {
	dir := t.TempDir()
	content := `{"name":"app","scripts":{"build":"tsc"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := DetectTestCommand(dir)
	if cmd != "" {
		t.Errorf("expected empty string, got %q", cmd)
	}
}

func TestDetectTestCommandPytestIni(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pytest.ini"), []byte("[pytest]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := DetectTestCommand(dir)
	if cmd != "pytest -v" {
		t.Errorf("expected 'pytest -v', got %q", cmd)
	}
}

func TestDetectTestCommandPyproject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := DetectTestCommand(dir)
	if cmd != "pytest -v" {
		t.Errorf("expected 'pytest -v', got %q", cmd)
	}
}

func TestDetectTestCommandEmpty(t *testing.T) {
	dir := t.TempDir()
	cmd := DetectTestCommand(dir)
	if cmd != "" {
		t.Errorf("expected empty string for unknown project, got %q", cmd)
	}
}

// A monorepo whose root has no usable runner but a nested module does (e.g. flowpilot
// with go.mod under apps/local-runner) must still be detected, with TestDir pointing
// at the module directory. (CP-35)
func TestDetectTestRunnerNestedGoMod(t *testing.T) {
	root := t.TempDir()
	// Root package.json with no test script — the old root-only detector returned "".
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"mono"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "apps", "local-runner")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := DetectTestRunner(root)
	if r.Cmd != "go test -v ./..." {
		t.Errorf("Cmd = %q, want 'go test -v ./...'", r.Cmd)
	}
	if r.Dir != "apps/local-runner" {
		t.Errorf("Dir = %q, want 'apps/local-runner'", r.Dir)
	}
}

// node_modules and other vendored trees must not be walked — a package.json with a
// test script buried in node_modules must never be selected. (CP-35)
func TestDetectNestedRunnerSkipsVendored(t *testing.T) {
	root := t.TempDir()
	buried := filepath.Join(root, "node_modules", "somepkg")
	if err := os.MkdirAll(buried, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buried, "package.json"), []byte(`{"scripts":{"test":"jest"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := DetectTestRunner(root); r.Cmd != "" {
		t.Errorf("expected no runner (node_modules skipped), got %q in %q", r.Cmd, r.Dir)
	}
}

func TestRunOracleNilBaseline(t *testing.T) {
	result := RunOracle("/some/dir", nil, nil)
	if result.HasRegression {
		t.Error("nil baseline should produce no regression")
	}
	if result.HasTampering {
		t.Error("nil baseline should produce no tampering")
	}
	if len(result.Regressed) != 0 {
		t.Error("nil baseline should produce empty Regressed")
	}
}

func TestRunOracleEmptyTestCmd(t *testing.T) {
	bl := &Baseline{CapturedAt: "2024-01-01T00:00:00Z", GreenTests: []string{"TestFoo"}, TestCmd: ""}
	result := RunOracle("/some/dir", bl, nil)
	if result.HasRegression {
		t.Error("empty TestCmd should produce no regression")
	}
}

func TestOracleResultHasRegressionOnlyWhenNonEmpty(t *testing.T) {
	r1 := OracleResult{Regressed: []string{}, HasRegression: len([]string{}) > 0}
	if r1.HasRegression {
		t.Error("empty Regressed should mean HasRegression=false")
	}

	r2 := OracleResult{Regressed: []string{"TestFoo"}, HasRegression: len([]string{"TestFoo"}) > 0}
	if !r2.HasRegression {
		t.Error("non-empty Regressed should mean HasRegression=true")
	}
}

func TestIsInBaseline(t *testing.T) {
	baseline := []string{"TestFoo", "TestBar/subtest"}
	cases := []struct {
		name string
		want bool
	}{
		{"TestFoo", true},
		{"TestBar/subtest", true},
		{"subtest", true},
		{"TestBaz", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isInBaseline(c.name, baseline); got != c.want {
			t.Errorf("isInBaseline(%q, baseline) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLoadBaselineMissingFile(t *testing.T) {
	dir := t.TempDir()
	bl, err := LoadBaseline(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl != nil {
		t.Error("expected nil baseline for missing file")
	}
}

func TestCaptureAndLoadBaseline(t *testing.T) {
	repoDir := t.TempDir()
	dotDir := t.TempDir()

	// No go.mod, package.json, or pytest config → empty baseline
	bl, err := CaptureBaseline(repoDir, dotDir)
	if err != nil {
		t.Fatalf("CaptureBaseline: %v", err)
	}
	if bl == nil {
		t.Fatal("expected non-nil baseline even for empty project")
	}
	if bl.TestCmd != "" {
		t.Errorf("expected empty TestCmd for unknown project, got %q", bl.TestCmd)
	}

	// Create go.mod so DetectTestCommand returns "go test -v ./..."
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bl2, err := CaptureBaseline(repoDir, dotDir)
	if err != nil {
		t.Fatalf("CaptureBaseline with go.mod: %v", err)
	}
	if bl2.TestCmd != "go test -v ./..." {
		t.Errorf("expected 'go test -v ./...', got %q", bl2.TestCmd)
	}
	if bl2.CapturedAt == "" {
		t.Error("CapturedAt should be set")
	}

	// LoadBaseline should round-trip
	loaded, err := LoadBaseline(dotDir)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded baseline")
	}
	if loaded.TestCmd != bl2.TestCmd {
		t.Errorf("loaded TestCmd = %q, want %q", loaded.TestCmd, bl2.TestCmd)
	}
}

func TestRunOracleTampering(t *testing.T) {
	repoDir := t.TempDir() // empty dir: runTests returns [] quickly (no .go files)
	bl := &Baseline{
		CapturedAt: "2024-01-01T00:00:00Z",
		GreenTests: []string{},
		TestCmd:    "go test ./...",
	}
	diff := []ChangedFile{
		{Path: "internal/foo/foo_test.go", Status: "M"},
		{Path: "internal/foo/foo.go", Status: "M"},
	}
	result := RunOracle(repoDir, bl, diff)
	if !result.HasTampering {
		t.Error("expected HasTampering=true when a test file is Modified in diff")
	}
	if len(result.Tampered) != 1 || result.Tampered[0] != "internal/foo/foo_test.go" {
		t.Errorf("Tampered = %v, want [internal/foo/foo_test.go]", result.Tampered)
	}
}

func TestRunOracleNewTestFileNotTampered(t *testing.T) {
	repoDir := t.TempDir()
	bl := &Baseline{
		CapturedAt: "2024-01-01T00:00:00Z",
		GreenTests: []string{},
		TestCmd:    "go test ./...",
	}
	diff := []ChangedFile{
		{Path: "internal/foo/foo_test.go", Status: "A"}, // Added, not Modified
	}
	result := RunOracle(repoDir, bl, diff)
	if result.HasTampering {
		t.Error("newly added test file should not be considered tampered")
	}
}
