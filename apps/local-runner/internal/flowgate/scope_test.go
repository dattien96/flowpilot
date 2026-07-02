package flowgate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Go scoping (DOD-02, DOD-11) ──────────────────────────────────────────────

func TestScopeGoSinglePackage(t *testing.T) {
	// DOD-11: changed .go file maps to ./pkg/... arg
	diff := []ChangedFile{{Path: "internal/foo/bar.go", Status: "M"}}
	got := scopeTestCommand("", "", "go test -v ./...", diff)
	want := "go test -v ./internal/foo/..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGoDeduplication(t *testing.T) {
	// DOD-11: two files in same package → one package arg
	diff := []ChangedFile{
		{Path: "internal/foo/bar.go", Status: "M"},
		{Path: "internal/foo/baz.go", Status: "A"},
	}
	got := scopeTestCommand("", "", "go test -v ./...", diff)
	want := "go test -v ./internal/foo/..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGoMultiPackageSorted(t *testing.T) {
	// DOD-02: multiple packages deduped and sorted; non-.go files skipped
	diff := []ChangedFile{
		{Path: "internal/runner/gate_hook.go", Status: "M"},
		{Path: "internal/flowgate/oracle.go", Status: "M"},
		{Path: "apps/web/README.md", Status: "M"}, // not .go — skipped
	}
	got := scopeTestCommand("", "", "go test -v ./...", diff)
	want := "go test -v ./internal/flowgate/... ./internal/runner/..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGoFlagsPreserved(t *testing.T) {
	// DOD-02: flags from original command are preserved
	diff := []ChangedFile{{Path: "pkg/auth/login.go", Status: "M"}}
	got := scopeTestCommand("", "", "go test -v -race ./...", diff)
	want := "go test -v -race ./pkg/auth/..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGoThresholdFallback(t *testing.T) {
	// DOD-11: >10 distinct packages → fallback to ""
	diff := make([]ChangedFile, 11)
	for i := range diff {
		diff[i] = ChangedFile{Path: fmt.Sprintf("pkg%d/file.go", i), Status: "M"}
	}
	got := scopeTestCommand("", "", "go test -v ./...", diff)
	if got != "" {
		t.Errorf("expected fallback (empty string), got %q", got)
	}
}

func TestScopeGoNoGoFiles(t *testing.T) {
	// DOD-07: no .go files in diff → fallback
	diff := []ChangedFile{
		{Path: "README.md", Status: "M"},
		{Path: "docs/guide.md", Status: "M"},
	}
	got := scopeTestCommand("", "", "go test -v ./...", diff)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestScopeGoExplicitScopeInTestCmd(t *testing.T) {
	// DOD-07: testCmd already contains specific packages → fallback
	diff := []ChangedFile{{Path: "internal/foo/bar.go", Status: "M"}}
	got := scopeTestCommand("", "", "go test -v ./internal/flowgate/...", diff)
	if got != "" {
		t.Errorf("expected fallback for already-scoped testCmd, got %q", got)
	}
}

func TestScopeGoWithTestDir(t *testing.T) {
	// DOD-02: monorepo with testDir strips prefix so packages are relative to runner dir
	diff := []ChangedFile{
		{Path: "apps/local-runner/internal/flowgate/oracle.go", Status: "M"},
		{Path: "apps/web/README.md", Status: "M"}, // outside testDir — skipped
	}
	got := scopeTestCommand("", "apps/local-runner", "go test -v ./...", diff)
	want := "go test -v ./internal/flowgate/..."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── Gradle scoping (DOD-03, DOD-12) ──────────────────────────────────────────

func TestScopeGradleSingleModule(t *testing.T) {
	// DOD-12: walk up to build.gradle, convert path to :module format
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "feature-auth/src/main/java/com/example")
	mustWriteFile(t, filepath.Join(repoDir, "feature-auth", "build.gradle"), "")

	diff := []ChangedFile{
		{Path: "feature-auth/src/main/java/com/example/AuthRepo.kt", Status: "M"},
	}
	got := scopeTestCommand(repoDir, "", "./gradlew test", diff)
	want := "./gradlew :feature-auth:test"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGradleNestedModule(t *testing.T) {
	// DOD-12: nested module path core/network → :core:network
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "core/network/src")
	mustWriteFile(t, filepath.Join(repoDir, "core", "network", "build.gradle"), "")

	diff := []ChangedFile{
		{Path: "core/network/src/HttpClient.kt", Status: "M"},
	}
	got := scopeTestCommand(repoDir, "", "./gradlew test", diff)
	want := "./gradlew :core:network:test"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGradleMultiModuleSorted(t *testing.T) {
	// DOD-03: multiple modules deduped and sorted
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "feature-auth/src")
	mustWriteFile(t, filepath.Join(repoDir, "feature-auth", "build.gradle"), "")
	mustMkdir(t, repoDir, "core/network/src")
	mustWriteFile(t, filepath.Join(repoDir, "core", "network", "build.gradle.kts"), "")

	diff := []ChangedFile{
		{Path: "feature-auth/src/AuthRepo.kt", Status: "M"},
		{Path: "core/network/src/HttpClient.kt", Status: "M"},
	}
	got := scopeTestCommand(repoDir, "", "./gradlew test", diff)
	want := "./gradlew :core:network:test :feature-auth:test"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGradleKtsFile(t *testing.T) {
	// DOD-03: build.gradle.kts also recognised
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "feature-pay/src")
	mustWriteFile(t, filepath.Join(repoDir, "feature-pay", "build.gradle.kts"), "")

	diff := []ChangedFile{{Path: "feature-pay/src/PayService.kt", Status: "M"}}
	got := scopeTestCommand(repoDir, "", "./gradlew test", diff)
	want := "./gradlew :feature-pay:test"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGradleThresholdFallback(t *testing.T) {
	// DOD-07: >5 modules → fallback
	repoDir := t.TempDir()
	for i := 0; i < 6; i++ {
		modDir := filepath.Join(repoDir, fmt.Sprintf("module%d", i))
		mustMkdir(t, repoDir, fmt.Sprintf("module%d/src", i))
		mustWriteFile(t, filepath.Join(modDir, "build.gradle"), "")
	}
	diff := make([]ChangedFile, 6)
	for i := range diff {
		diff[i] = ChangedFile{Path: fmt.Sprintf("module%d/src/Main.kt", i), Status: "M"}
	}
	got := scopeTestCommand(repoDir, "", "./gradlew test", diff)
	if got != "" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestScopeGradleExplicitScopeFallback(t *testing.T) {
	// DOD-07: testCmd already has :module: args → fallback
	repoDir := t.TempDir()
	diff := []ChangedFile{{Path: "feature-auth/src/AuthRepo.kt", Status: "M"}}
	got := scopeTestCommand(repoDir, "", "./gradlew :feature-auth:test", diff)
	if got != "" {
		t.Errorf("expected fallback for already-scoped testCmd, got %q", got)
	}
}

func TestScopeGradleNonTestTaskFallback(t *testing.T) {
	// Gradle lint/build/assemble must not be scoped — only "test" is safe to rewrite
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "feature-auth/src")
	mustWriteFile(t, filepath.Join(repoDir, "feature-auth", "build.gradle"), "")

	diff := []ChangedFile{{Path: "feature-auth/src/AuthRepo.kt", Status: "M"}}
	for _, task := range []string{"./gradlew lint", "./gradlew build", "./gradlew assemble", "./gradlew check"} {
		got := scopeTestCommand(repoDir, "", task, diff)
		if got != "" {
			t.Errorf("task=%q: expected fallback, got %q", task, got)
		}
	}
}

func TestScopeGradleFlagsPreserved(t *testing.T) {
	// Extra flags like --no-daemon must survive in the scoped command
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "feature-auth/src")
	mustWriteFile(t, filepath.Join(repoDir, "feature-auth", "build.gradle"), "")

	diff := []ChangedFile{{Path: "feature-auth/src/AuthRepo.kt", Status: "M"}}
	got := scopeTestCommand(repoDir, "", "./gradlew test --no-daemon", diff)
	want := "./gradlew :feature-auth:test --no-daemon"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeGradleNoBuildGradleFallback(t *testing.T) {
	// DOD-07: no build.gradle found for changed files → fallback
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "feature-auth/src")
	// No build.gradle created

	diff := []ChangedFile{{Path: "feature-auth/src/AuthRepo.kt", Status: "M"}}
	got := scopeTestCommand(repoDir, "", "./gradlew test", diff)
	if got != "" {
		t.Errorf("expected fallback when no build.gradle found, got %q", got)
	}
}

func TestScopeGradleWrapperBat(t *testing.T) {
	// DOD-03: gradlew.bat wrapper preserved
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "app/src")
	mustWriteFile(t, filepath.Join(repoDir, "app", "build.gradle"), "")

	diff := []ChangedFile{{Path: "app/src/Main.kt", Status: "M"}}
	got := scopeTestCommand(repoDir, "", "gradlew.bat test", diff)
	want := "gradlew.bat :app:test"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── pytest scoping (DOD-05, DOD-13) ──────────────────────────────────────────

func TestScopePytestDirExtraction(t *testing.T) {
	// DOD-13: unique dirs from changed .py files
	diff := []ChangedFile{
		{Path: "src/auth/service.py", Status: "M"},
		{Path: "src/auth/models.py", Status: "M"},
	}
	got := scopeTestCommand("", "", "pytest -v", diff)
	want := "pytest -v src/auth/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopePytestMultiDirSorted(t *testing.T) {
	diff := []ChangedFile{
		{Path: "src/payments/service.py", Status: "M"},
		{Path: "src/auth/service.py", Status: "M"},
	}
	got := scopeTestCommand("", "", "pytest -v", diff)
	want := "pytest -v src/auth/ src/payments/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopePytestConftestFallback(t *testing.T) {
	// DOD-13: conftest.py changed → full suite fallback
	diff := []ChangedFile{
		{Path: "tests/conftest.py", Status: "M"},
		{Path: "src/auth/service.py", Status: "M"},
	}
	got := scopeTestCommand("", "", "pytest -v", diff)
	if got != "" {
		t.Errorf("conftest.py change must fall back, got %q", got)
	}
}

func TestScopePytestNoPyFiles(t *testing.T) {
	// DOD-07: no .py files → fallback
	diff := []ChangedFile{{Path: "README.md", Status: "M"}}
	got := scopeTestCommand("", "", "pytest -v", diff)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestScopePytestWithTestDir(t *testing.T) {
	// testDir strips prefix so dirs are relative to runner cwd
	diff := []ChangedFile{
		{Path: "apps/api/src/auth/service.py", Status: "M"},
		{Path: "other/README.md", Status: "M"}, // outside testDir — skipped
	}
	got := scopeTestCommand("", "apps/api", "pytest -v", diff)
	want := "pytest -v src/auth/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopePytestThresholdFallback(t *testing.T) {
	// DOD-07: >8 dirs → fallback
	diff := make([]ChangedFile, 9)
	for i := range diff {
		diff[i] = ChangedFile{Path: fmt.Sprintf("src/mod%d/service.py", i), Status: "M"}
	}
	got := scopeTestCommand("", "", "pytest -v", diff)
	if got != "" {
		t.Errorf("expected fallback, got %q", got)
	}
}

// ── Jest scoping (DOD-06) ─────────────────────────────────────────────────────

func TestScopeJestTestPathPattern(t *testing.T) {
	// DOD-06: --testPathPattern from changed TS/JS files
	diff := []ChangedFile{
		{Path: "src/components/Button.tsx", Status: "M"},
		{Path: "src/utils/format.ts", Status: "M"},
	}
	got := scopeTestCommand("", "", "jest", diff)
	want := "jest --testPathPattern=src/components|src/utils"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeJestNpxPrefix(t *testing.T) {
	// DOD-06: npx jest prefix preserved
	diff := []ChangedFile{{Path: "src/auth/LoginForm.tsx", Status: "M"}}
	got := scopeTestCommand("", "", "npx jest", diff)
	if !strings.HasPrefix(got, "npx jest") {
		t.Errorf("expected npx jest prefix, got %q", got)
	}
}

func TestScopeJestNoJsFiles(t *testing.T) {
	// DOD-07: no JS/TS files → fallback
	diff := []ChangedFile{{Path: "src/styles.css", Status: "M"}}
	got := scopeTestCommand("", "", "jest", diff)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestScopeJestWithTestDir(t *testing.T) {
	// testDir strips prefix so dirs are relative to runner cwd
	diff := []ChangedFile{
		{Path: "apps/web/src/components/Button.tsx", Status: "M"},
		{Path: "docs/README.md", Status: "M"}, // outside testDir — skipped
	}
	got := scopeTestCommand("", "apps/web", "jest", diff)
	want := "jest --testPathPattern=src/components"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeJestThresholdFallback(t *testing.T) {
	// DOD-07: >8 dirs → fallback
	diff := make([]ChangedFile, 9)
	for i := range diff {
		diff[i] = ChangedFile{Path: fmt.Sprintf("src/mod%d/file.ts", i), Status: "M"}
	}
	got := scopeTestCommand("", "", "jest", diff)
	if got != "" {
		t.Errorf("expected fallback, got %q", got)
	}
}

// ── Fallback cases (DOD-07, DOD-14) ──────────────────────────────────────────

func TestScopeEmptyDiff(t *testing.T) {
	// DOD-07: empty diff → always fallback
	got := scopeTestCommand("", "", "go test -v ./...", nil)
	if got != "" {
		t.Errorf("expected empty for nil diff, got %q", got)
	}
	got = scopeTestCommand("", "", "go test -v ./...", []ChangedFile{})
	if got != "" {
		t.Errorf("expected empty for empty diff, got %q", got)
	}
}

func TestScopeUnrecognizedTestCmd(t *testing.T) {
	// DOD-14: unrecognized testCmd prefix → fallback
	for _, cmd := range []string{"cargo test", "mvn test -q", "flutter test", "make test", ""} {
		diff := []ChangedFile{{Path: "src/main.rs", Status: "M"}}
		got := scopeTestCommand("", "", cmd, diff)
		if got != "" {
			t.Errorf("testCmd=%q: expected fallback, got %q", cmd, got)
		}
	}
}

// ── xcodebuild heuristic (DOD-04) ────────────────────────────────────────────

func TestScopeXcodeHeuristicMatch(t *testing.T) {
	// DOD-04: Sources/FeatureAuth/ → -only-testing:FeatureAuthTests when dir exists
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "FeatureAuthTests")

	diff := []ChangedFile{
		{Path: "Sources/FeatureAuth/AuthViewModel.swift", Status: "M"},
	}
	got := scopeTestCommand(repoDir, "", "xcodebuild test -scheme App", diff)
	want := "xcodebuild test -scheme App -only-testing:FeatureAuthTests"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeXcodeHeuristicNoMatch(t *testing.T) {
	// DOD-04: heuristic fails when target dir does not exist → fallback ""
	repoDir := t.TempDir()
	// No FeatureAuthTests dir created

	diff := []ChangedFile{
		{Path: "Sources/FeatureAuth/AuthViewModel.swift", Status: "M"},
	}
	got := scopeTestCommand(repoDir, "", "xcodebuild test -scheme App", diff)
	if got != "" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestScopeXcodeNoSwiftFiles(t *testing.T) {
	// DOD-04: no .swift files → fallback
	repoDir := t.TempDir()
	diff := []ChangedFile{{Path: "README.md", Status: "M"}}
	got := scopeTestCommand(repoDir, "", "xcodebuild test -scheme App", diff)
	if got != "" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestScopeXcodeWithTestDir(t *testing.T) {
	// testDir scopes target search to repoDir/testDir; file outside testDir is skipped
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "apps/ios/FeatureAuthTests")

	diff := []ChangedFile{
		{Path: "apps/ios/Sources/FeatureAuth/Auth.swift", Status: "M"},
		{Path: "docs/README.md", Status: "M"}, // not .swift — skipped
	}
	got := scopeTestCommand(repoDir, "apps/ios", "xcodebuild test -scheme App", diff)
	want := "xcodebuild test -scheme App -only-testing:FeatureAuthTests"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScopeXcodeWithTestDirFileOutside(t *testing.T) {
	// Swift file outside testDir must be skipped; no targets found → fallback
	repoDir := t.TempDir()
	mustMkdir(t, repoDir, "apps/ios/FeatureAuthTests")

	diff := []ChangedFile{
		{Path: "other/Sources/FeatureAuth/Auth.swift", Status: "M"}, // outside apps/ios
	}
	got := scopeTestCommand(repoDir, "apps/ios", "xcodebuild test -scheme App", diff)
	if got != "" {
		t.Errorf("expected fallback for file outside testDir, got %q", got)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustMkdir(t *testing.T, repoDir, relPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repoDir, filepath.FromSlash(relPath)), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
