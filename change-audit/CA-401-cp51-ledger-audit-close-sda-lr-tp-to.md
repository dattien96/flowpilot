# CA-401: Close CP-51 §10.1 ledger gaps SDa/LR/TP/TO with new tests

## Summary

Follow-up to the CP-51 §10.1 acceptance-ledger audit (same day): after the
first audit pass flipped 38 stale `☐` rows to `✅` on existing test evidence,
4 rows (`SDa`, `LR`, `TP`, `TO`) had only partial/related coverage, not a test
for the exact stated sub-case. Per operator instruction, wrote the 4 missing
tests instead of leaving them open.

## What was added

- `TestSettle_StopMidSettle_BookkeepingCompletes_ReleaseSuppressed`
  (`dispatch_settle_barrier_test.go`) — ledger `SDa`. Distinct from the
  existing `TestSettleSubBarriers_B8aToB8e_PartialThenResume`, which only
  simulates a crash; this one requests a real Stop before terminal commit and
  proves `SettleDriver.DriveSettle` still reaches `SettleFinalized`, runs
  exactly one `finalizer` effect, and the `dependents_release` effect payload
  durably records the stop-generation suppression.
- `TestRecoveryScanner_StaleSnapshotNeverActs_FreshReadDrivesCorrectDecision`
  (`dispatch_recovery_test.go`) — ledger `LR`. Proves `ClaimRecovery`'s
  revision CAS rejects a snapshot that has gone stale (a cancel request landed
  after it was taken), so a stale reconcile attempt has zero effect and never
  wrongly redispatches; a fresh snapshot of the same record correctly drives
  the pre-send-cancel path instead.
- `TestTerminalCommit_OutcomeFidelityMatrix` (`dispatch_record_test.go`) —
  ledger `TP`. Table-driven across {completed, failed, cancelled} ×
  {stopped, not-stopped}, including the row's explicit completed-before-cancel
  case. The existing single-case test
  (`TestTerminalCommit_DerivesStopOutcomeFromDurableAuthority`) only covered
  one of these 6 combinations.
- `TestIntentClear_NoTOCTOUResurrectionUnderConcurrentAccess`
  (`dispatch_record_test.go`) — ledger `TO`. 20 iterations of a reader
  goroutine hammering `IsIntentCleared` (2000 reads/iteration) concurrently
  against a `CommitReceiptAndClearIntent` writer, asserting the observed
  cleared flag never flips from true back to false.

## additive-tests-only compliance

All 4 are new test functions appended to existing files; zero existing test
functions or assertions were touched.

## Verification

- `go build ./...`, `go vet ./internal/runner/`: clean.
- All 4 new tests pass in isolation and together.
- Mutation check on `TP`: temporarily forced `deriveStopOutcomeLocked` (the
  function the row's "completed-before-cancel" guarantee depends on) to
  always return `""`, confirmed `TestTerminalCommit_OutcomeFidelityMatrix`
  fails on exactly the 3 stopped sub-cases (not the 3 non-stopped ones,
  confirming precise fault isolation), then reverted the mutation cleanly
  (`git diff` empty afterward).
- Regression sweep (`TestDispatch*`, `TestCrashMatrix*`, `TestRecoveryScanner*`,
  `TestSettle*`, `TestSync*`, `TestChatSession*`, `TestBug*`, etc.): only the
  same pre-existing, already-documented failure
  (`TestRestoreChatRunFromDriveMissingActiveAccountHome`) — no new failures.

## Follow-up

CP-51's §10.1 ledger now has `SDa`/`LR`/`TP`/`TO` at `✅`. Remaining open rows:
`GR` (no C toolchain in this dev environment to run `-race`), `NR` (18
failures found elsewhere in `internal/runner`, entirely outside CP-51/dispatch
scope — separate investigation, not blocking this ledger), and `CE-GEM`
(intentionally out of scope — Gemini V2 dispatch is not supported, operator
confirmed 2026-07-22, not a pending task).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: test
summary: Added 4 tests (SDa/LR/TP/TO) closing the remaining CP-51 §10.1 ledger gaps that had only partial existing coverage, each mutation-verified.
# --->8---
