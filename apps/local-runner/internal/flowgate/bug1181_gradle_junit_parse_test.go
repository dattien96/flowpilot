package flowgate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BUG-1181 (live run-204891 sprint): a Gradle/Kotlin unit-test suite printed
// `VaultIoTest > encrypts data FAILED`-shape lines that parseSuiteTestNames
// did not recognise — Failed stayed empty, so an expected-red scaffold suite
// surfaced as an unnamed "regression" instead of named red tests. The live
// workaround patched the project's harness to re-emit TAP; this fix makes the
// parser understand Gradle console lines and JUnit XML result files directly.

func TestBug1181_GradleConsoleFailureParsed(t *testing.T) {
	out := "" +
		"> Task :app:testDebugUnitTest\n" +
		"\n" +
		"VaultIoTest > encrypts plaintext round trip PASSED\n" +
		"VaultIoTest > rejects short passphrase FAILED\n" +
		"    java.lang.AssertionError at VaultIoTest.kt:88\n" +
		"com.vault.KeyStoreTest > rotates key FAILED\n" +
		"VaultIoTest > skips when disabled SKIPPED\n" +
		"\n" +
		"42 tests completed, 2 failed\n" +
		"FAILURE: Build failed with an exception.\n"
	passed, failed, err := parseSuiteTestNames("./gradlew :app:testDebugUnitTest", out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(failed) != 2 {
		t.Fatalf("expected 2 gradle failures, got %v", failed)
	}
	joined := strings.Join(failed, "|")
	if !strings.Contains(joined, "rejects short passphrase") || !strings.Contains(joined, "rotates key") {
		t.Fatalf("gradle failure names lost: %v", failed)
	}
	if len(passed) != 1 || !strings.Contains(passed[0], "encrypts plaintext") {
		t.Fatalf("gradle PASSED line not captured: %v", passed)
	}
}

func TestBug1181_JUnitXMLFallbackNamesFailures(t *testing.T) {
	// Console output carried no per-test names (Gradle default verbosity), but
	// the suite dropped JUnit XML under build/test-results — the fallback must
	// name the failing testcase.
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "app", "build", "test-results", "testDebugUnitTest")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.vault.VaultIoTest" tests="3" failures="1">
  <testcase name="encrypts plaintext" classname="com.vault.VaultIoTest" time="0.012"/>
  <testcase name="rejects short passphrase" classname="com.vault.VaultIoTest" time="0.004">
    <failure message="expected true">java.lang.AssertionError</failure>
  </testcase>
  <testcase name="skips when disabled" classname="com.vault.VaultIoTest" time="0.001">
    <skipped/>
  </testcase>
</testsuite>`
	if err := os.WriteFile(filepath.Join(resultsDir, "TEST-com.vault.VaultIoTest.xml"), []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	passed, failed := junitXMLTestNames(dir, time.Now().Add(-time.Minute))
	if len(failed) != 1 || failed[0] != "com.vault.VaultIoTest.rejects short passphrase" {
		t.Fatalf("junit failure name wrong: %v", failed)
	}
	if len(passed) != 1 || passed[0] != "com.vault.VaultIoTest.encrypts plaintext" {
		t.Fatalf("junit pass names wrong: %v", passed)
	}
}

func TestBug1181_JUnitXMLSkipsStaleResults(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "build", "test-results", "test")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(resultsDir, "TEST-old.xml")
	if err := os.WriteFile(stale, []byte(`<testsuite><testcase name="old" classname="X"><failure/></testcase></testsuite>`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	passed, failed := junitXMLTestNames(dir, time.Now().Add(-time.Minute))
	if len(passed)+len(failed) != 0 {
		t.Fatalf("stale XML must not feed names — got passed=%v failed=%v", passed, failed)
	}
}

func TestBug1181_ExecuteSuiteUsesXMLWhenConsoleSilent(t *testing.T) {
	// End-to-end: a suite command that fails while writing JUnit XML, with no
	// console test names, must still produce named failures.
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, "build", "test-results", "test")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	xml := `<testsuite name="T"><testcase name="boom" classname="T"><failure/></testcase></testsuite>`
	if err := os.WriteFile(filepath.Join(resultsDir, "TEST-T.xml"), []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	suitePassed, passed, failed, envErr, _ := executeSuite(context.Background(), dir, "false", "")
	if envErr != "" {
		t.Fatalf("env error: %s", envErr)
	}
	if suitePassed {
		t.Fatal("false must fail")
	}
	if len(failed) != 1 || failed[0] != "T.boom" {
		t.Fatalf("XML fallback missing: passed=%v failed=%v", passed, failed)
	}
}
