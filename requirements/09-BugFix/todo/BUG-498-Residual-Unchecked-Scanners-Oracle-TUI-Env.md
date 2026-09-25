# BUG-498 — residual unchecked scanner.Err() sites: oracle test output, TUI SSE stream, runner env file

## Status
todo — discovered in deep-review round 2 scanner sweep

## Severity
Low-medium — diagnostics honesty; truncation surfaces are narrow or self-healing

## Sites

1. **`internal/flowgate/oracle.go` ~:290** — `parseSuiteTestNames` scans
   combined suite output with a raised cap (`maxSuiteOutputBytes+8KiB`) but
   never checks `scanner.Err()`. A line beyond that cap stops the parse
   mid-stream: per-test `failed` names after the truncation point are
   silently absent from the regression diff (the suite verdict itself
   still comes from the process exit code). Surface the parse error to the
   caller so the name-level diff is honestly marked partial.

2. **`internal/tui/client/client.go` ~:1663** — `openStream` SSE parser
   exits the loop on scanner error with no signal; the channel just
   closes. Consumers reconnect with `afterSeq` so event loss self-heals,
   but the fault is invisible — log the scan error before returning so
   diagnostics distinguish "stream ended" from "stream failed".

3. ~~`internal/runner/runner.go` env-file loader~~ — **false positive**:
   `loadWorkspaceEnvFile` already returns `scanner.Err()` at the end of the
   loop; the sweep window cut off before the check. No change needed.

## Fix (implemented)

- `parseSuiteTestNames` returns `(passed, failed []string, err error)`;
  caller logs and treats name lists as partial.
- `openStream` goroutine logs `scanner.Err()` before closing.

## Tests

- oracle: input with a line beyond the cap → non-nil parse error
  (`bug390_subtest_parse_test.go` call sites adapted mechanically).
- TUI stream error path is log-only by design (seq-resume self-heals);
  covered by inspection — no injectable seam without refactoring the
  goroutine.
