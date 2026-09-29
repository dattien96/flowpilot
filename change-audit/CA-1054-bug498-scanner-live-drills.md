# CA-1054 — BUG-498 scanner-error live-style drills + runbook closure

## What changed

Test-only addition plus runbook evidence — no production code touched.

- `internal/tui/client/bug498_stream_scan_err_test.go` (new):
  `TestBug498_TUIStreamOverCapLineLogsScanError` runs the TUI SSE client
  against a real `httptest.Server` emitting a line over the 8MB scanner cap
  and asserts `scanner.Err()` is logged before the stream closes — the live
  arm of BUG-498 that a unit-only mock could not prove (the fault is in the
  wire bytes, not the parser's input objects).
- `internal/flowgate/bug498_oracle_reachability_test.go` (new):
  `TestBug498_OracleScanErrArmUnreachableViaTailCap` documents and pins that
  the oracle `>4MB suite-name` scanner arm is unreachable through the current
  production path — gate output is tail-capped (64KB) below the scanner's
  max token size, so `scanner.Err()` is defense-in-depth, not dead code to
  remove.
- `requirements/07-Coding-Plan/todo/CP-Full-Live-Test.md` §R appended:
  2026-09-29 closure campaign results for the remaining fault legs
  (BUG-479/482/494/498/493/489-PhaseC/538, A-58-6) — each classified as
  live-verified, split-verdict, structurally unreachable, or env-blocked —
  plus per-item recommendations for the open design observations.

## Verification

```
go test -count=1 ./internal/tui/client/ -run TestBug498   # PASS
go test -count=1 ./internal/flowgate/ -run TestBug498      # PASS
```

Live evidence (runner :4317, ws /tmp/fp-live): BUG-538 parked-successor arm
verified organically — `run-400` spawned `blockedStart=false`, dispatch
refused by re-blocked loop, parked on durable `pending_resume_gen=1`,
flushed exactly once (`turn-406`) on `agent-loop/resume`; restart arm via
seeded `run-334` flushed once (`turn-361`). BUG-494 stub bins
(`FLOWPILOT_*_BIN` emitting >8MB lines) → `/compat/deep` reports real
`bufio.Scanner: token too long` on both probes. BUG-489 mid-handler write
fault injectable via `chmod 555 .flowpilot/chats` → honest
`persistProviderSession: durable write failed … permission denied`.
