package flowgate

import (
	"strings"
	"testing"
)

// BUG-498 reachability pin: parseSuiteTestNames' scanner.Err arm is
// defense-in-depth — every production input passes through executeSuite's
// tailCapWriter (64KB retained) while the scanner cap is
// maxSuiteOutputBytes+8KiB (72KB). A suite emitting one giant line can never
// produce a retained line over the scan cap, so the truncation arm is
// unreachable via the real path. This test pins that property: the worst-case
// real input (one >4MB line tail-capped to 64KB) parses with err == nil, and
// a direct >cap call DOES return the typed error (the arm itself works).
func TestBug498_OracleScanErrArmUnreachableViaTailCap(t *testing.T) {
	// Simulate executeSuite's retained output: the tail of a 5MB line is a
	// single 64KB token — under the 72KB scan cap → parse completes.
	outCap := &tailCapWriter{max: maxSuiteOutputBytes}
	huge := strings.Repeat("n", 5*1024*1024) + "\n--- FAIL: TestGiant\n"
	_, _ = outCap.Write([]byte(huge))
	retained := outCap.String()

	if _, _, err := parseSuiteTestNames("go test ./...", retained); err != nil {
		t.Fatalf("tail-capped output must not trip the scan cap: %v", err)
	}

	// The arm itself works: a direct over-cap input returns the typed error.
	overCap := strings.Repeat("m", maxSuiteOutputBytes+16*1024)
	if _, _, err := parseSuiteTestNames("go test ./...", overCap); err == nil {
		t.Fatal("expected typed scan error for over-cap line")
	}
}
