package flowgate

import "testing"

// BUG-573 (live run-100368, owner_2 finding): IsTestFile had no C/C++ shapes,
// so `vault_core_test.cpp` was classified as a production file — the
// scaffold bounded-stub gate froze it read-only and the reproduce lock never
// saw it as a test. Same conventions as the other ecosystems: _test/_tests
// suffix, Test/Tests suffix (gtest), test_ prefix.
func TestIsTestFileCpp(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"tests/vault_core_test.cpp", true},
		{"tests/audit_log_test.cc", true},
		{"test/ring_buffer_tests.cxx", true},
		{"tests/VaultCoreTest.cpp", true},
		{"tests/VaultCoreTests.cpp", true},
		{"tests/test_audit_drain.cpp", true},
		{"core/vault/VaultCore.cpp", false},
		{"core/vault/TestHelper.cpp", false},
		{"core/vault/contest.cpp", false},
		{"core/vault/latest.cpp", false},
	}
	for _, c := range cases {
		if got := IsTestFile(c.path); got != c.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// BUG-574 (live run-100368, owner_2 finding): parseSuiteTestNames only knew
// go test/npm/pytest. A red gtest suite run through ctest or a bash harness
// produced zero parsed names → oracle.Failed stayed empty → with a
// red-at-capture baseline the gate saw no failure signal at all
// (gate_hook's failedTests only fills from oracle.Failed when the baseline
// was red) — a red suite read green.
func TestParseSuiteTestNamesGtest(t *testing.T) {
	out := `[==========] Running 3 tests from 1 test suite.
[----------] Global test environment set-up.
[ RUN      ] VaultCoreTest.Sanitize
[       OK ] VaultCoreTest.Sanitize (0 ms)
[ RUN      ] VaultCoreTest.Zeroize
[  FAILED  ] VaultCoreTest.Zeroize (1 ms)
[ RUN      ] VaultCoreTest.Drain
[       OK ] VaultCoreTest.Drain (0 ms)
[  PASSED  ] 2 tests.
[  FAILED  ] 1 test, listed below:
`
	for _, cmd := range []string{"ctest --output-on-failure", "./vault_tests", "bash ./tests/run_tests.sh"} {
		passed, failed, err := parseSuiteTestNames(cmd, out)
		if err != nil {
			t.Fatalf("cmd=%q parse err: %v", cmd, err)
		}
		if len(passed) == 0 {
			t.Fatalf("cmd=%q: no passed names parsed from gtest output", cmd)
		}
		foundFail := false
		for _, f := range failed {
			if f == "VaultCoreTest.Zeroize" {
				foundFail = true
			}
		}
		if !foundFail {
			t.Fatalf("cmd=%q: gtest failure not parsed, failed=%v", cmd, failed)
		}
	}
}

// ctest's own per-test summary lines must parse too.
func TestParseSuiteTestNamesCtest(t *testing.T) {
	out := `    Start 1: vault_core_test
1/3 Test #1: vault_core_test ................   Passed    0.01 sec
    Start 2: audit_log_test
2/3 Test #2: audit_log_test .................***Failed    0.02 sec
    Start 3: fd_guard_test
3/3 Test #3: fd_guard_test ..................   Passed    0.01 sec
`
	passed, failed, _ := parseSuiteTestNames("ctest --output-on-failure", out)
	if len(passed) != 2 {
		t.Fatalf("ctest passed = %v, want 2 entries", passed)
	}
	found := false
	for _, f := range failed {
		if f == "audit_log_test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ctest failure not parsed, failed=%v", failed)
	}
}

// TAP is the common shape for ad-hoc bash harnesses.
func TestParseSuiteTestNamesTap(t *testing.T) {
	out := `TAP version 13
ok 1 sanitize clears buffer
not ok 2 zeroize wipes memory
ok 3 drain emits records
`
	passed, failed, _ := parseSuiteTestNames("bash ./tests/run_tests.sh", out)
	if len(passed) != 2 {
		t.Fatalf("TAP passed = %v, want 2 entries", passed)
	}
	found := false
	for _, f := range failed {
		if f == "zeroize wipes memory" || f == "2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("TAP failure not parsed, failed=%v", failed)
	}
}
