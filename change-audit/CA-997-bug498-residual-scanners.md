# CA-997 — BUG-498: residual unchecked scanners (oracle, TUI stream)

## What changed

- `internal/flowgate/oracle.go`: `parseSuiteTestNames` returns
  `scanner.Err()`; `executeSuite` logs an incomplete name parse so a
  truncated suite output is not presented as a complete per-test diff.
- `internal/tui/client/client.go`: `openStream` logs `scanner.Err()`
  before closing — stream faults are distinguishable from clean ends in
  diagnostics (delivery self-heals via the `afterSeq` reconnect).
- `internal/runner/runner.go` env loader — reviewed: already returns
  `scanner.Err()`; sweep false positive, no change.

## Tests

`bug390_subtest_parse_test.go` call sites adapted to the new signature;
parse-error arm exercised via oversized-line input.
