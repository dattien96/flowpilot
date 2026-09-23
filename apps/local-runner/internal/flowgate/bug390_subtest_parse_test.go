package flowgate

import "testing"

// BUG-390: indented go-test subtest lines ("    --- FAIL: TestX/sub") contain
// the marker but TrimPrefix did not strip leading whitespace, so the parser
// recorded the literal name "---". That junk name poisoned baselines and made
// isInBaseline("---", green) match every real subtest failure -> HasRegression
// -> tr.Tests.Failed suppressed -> reproduce gate reported "suite passed" on a
// RED suite (CP-50 run-169, CP-42 run-442 deadlocks).
func TestBug390_IndentedSubtestFailParsesToRealName(t *testing.T) {
	out := "=== RUN   TestBuggy\n" +
		"    --- FAIL: TestBuggy/sub_one (0.00s)\n" +
		"--- FAIL: TestBuggy (0.00s)\n" +
		"FAIL\n"
	passed, failed := parseSuiteTestNames("go test ./...", out)
	for _, n := range failed {
		if n == "---" {
			t.Fatalf("indented subtest line parsed to junk name %q: failed=%v", n, failed)
		}
	}
	foundSub, foundParent := false, false
	for _, n := range failed {
		if n == "TestBuggy/sub_one" {
			foundSub = true
		}
		if n == "TestBuggy" {
			foundParent = true
		}
	}
	if !foundSub || !foundParent {
		t.Fatalf("want both parent and subtest names; got %v", failed)
	}
	_ = passed
}

// The PASS side had the same defect — indented "--- PASS:" subtest lines were
// recorded as "---" in green_tests at baseline capture.
func TestBug390_IndentedSubtestPassParsesToRealName(t *testing.T) {
	out := "=== RUN   TestX\n    --- PASS: TestX/sub (0.00s)\n--- PASS: TestX (0.00s)\nPASS\n"
	passed, _ := parseSuiteTestNames("go test ./...", out)
	for _, n := range passed {
		if n == "---" {
			t.Fatalf("indented PASS subtest parsed to junk name: passed=%v", passed)
		}
	}
}

// A legacy baseline polluted with literal "---" entries (written before the
// parse fix) must not classify real failures as regressions.
func TestBug390_JunkBaselineEntryNeverMatches(t *testing.T) {
	if isInBaseline("TestNew", []string{"---"}) {
		t.Fatal("junk baseline entry must never match a real test name")
	}
	if isInBaseline("---", []string{"---"}) {
		t.Fatal("junk name must never be a baseline member")
	}
}
