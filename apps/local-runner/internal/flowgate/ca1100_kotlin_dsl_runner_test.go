package flowgate

import (
	"os"
	"path/filepath"
	"testing"
)

// CA-1100: Kotlin-DSL Gradle roots were invisible to runner detection.
// detectRunnerInDir only statted "build.gradle", so an Android project using
// build.gradle.kts/settings.gradle.kts missed the root fast path entirely and
// fell through to the nested walk — where any foreign runner marker (e.g. a
// scratch go.mod harness) outranks Gradle (go=0, gradle=3). Observed live in
// PrivateVault run-3362: the baseline pinned `go test` for a Kotlin codebase.
func TestCA1100DetectKotlinDslGradleRunner(t *testing.T) {
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("build.gradle.kts at root detects gradlew", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "settings.gradle.kts"), "")
		write(t, filepath.Join(dir, "build.gradle.kts"), "")
		write(t, filepath.Join(dir, "gradlew"), "#!/bin/sh\n")
		runner := DetectTestRunner(dir)
		if runner.Cmd != "./gradlew test" {
			t.Fatalf("got %q, want \"./gradlew test\"", runner.Cmd)
		}
		if runner.Dir != "" {
			t.Fatalf("Dir = %q, want empty (root)", runner.Dir)
		}
	})

	t.Run("settings.gradle.kts alone still detects gradlew", func(t *testing.T) {
		// Multi-module roots where only settings.gradle.kts + gradlew exist at
		// root and build files live in modules.
		dir := t.TempDir()
		write(t, filepath.Join(dir, "settings.gradle.kts"), "")
		write(t, filepath.Join(dir, "gradlew"), "#!/bin/sh\n")
		runner := DetectTestRunner(dir)
		if runner.Cmd != "./gradlew test" {
			t.Fatalf("got %q, want \"./gradlew test\"", runner.Cmd)
		}
	})

	t.Run("nested go.mod does not outrank kotlin-dsl root", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "settings.gradle.kts"), "")
		write(t, filepath.Join(dir, "build.gradle.kts"), "")
		write(t, filepath.Join(dir, "gradlew"), "#!/bin/sh\n")
		write(t, filepath.Join(dir, "tools", "harness", "go.mod"), "module example.com/x\n")
		runner := DetectTestRunner(dir)
		if runner.Cmd != "./gradlew test" || runner.Dir != "" {
			t.Fatalf("kotlin-dsl root must beat nested go.mod, got cmd=%q dir=%q", runner.Cmd, runner.Dir)
		}
	})
}
