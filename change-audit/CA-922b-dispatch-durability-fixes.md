---
id: CA-922b
title: Dispatch/durability fixes — leg-field reconstruction, terminal-commit seq ordering, repair-resolution idempotency, abandon terminalization, builtin-flow mirror fallback (BUG-405, 406, 407, 408, 409)
type: BugFix
feature: agent-flow-engine
date: 2026-09-23
status: done
---

## Context

Live-verification wave found five defects in the dispatch/durability layer:

- Reconstructed chat legs lost `legState`/`legClosedReason`/`switchFromRunID`
  (timeline lied, switch-provider 409'd, and `omitempty` rewrote durable rows
  empty — progressive data loss).
- Every terminal commit wrote a duplicate `seq` and burned a gap in
  `dispatch.ndjson` — the durable log's ordering invariant was broken 11/11.
- `repair-resolution` replay with the same `resolutionId` returned HTTP 502
  instead of the recorded outcome (contract row RR violated).
- `repair-resolution abandon` never terminalized the stranded dispatch record,
  so the boot scanner re-opened a fresh repair at rev=1 forever.
- A configured-but-dead Supabase mirror silently disabled every built-in flow
  (`flowRef` fell back to a plain ungated chat turn; live bed contamination
  run-4 wrote production code outside any contract/gate).

## Changes

### BUG-405 — leg fields restored on reconstruct

`internal/runner/interactive_resume.go` (`reconstructRunInternal`): restore
`legState`/`legClosedReason`/`switchFromRunID` from `ProviderSessionState` —
same class as the BUG-330 `chatID`/`legSeq` restore right above it. Persisted
rows stop regressing because the resident run now carries the values
`persistSessionSnapshot` writes back.
Test: `TestBug405_ReconstructRestoresLegFields`.

### BUG-406 — terminal-commit seq ordering

`internal/runner/dispatch_store_memory.go` (`commitTerminal`): append the
`commit_terminal` audit BEFORE `commitLine`, matching every other mutation
path — `appendAuditLocked` is the seq allocator, so the record line must take
the post-increment value. Previously the line reused the pre-increment seq
(duplicate) and the audit's increment never landed on a durable line (gap).
Test: `TestBug406_TerminalCommitSeqUniqueAndContiguous` (parses the real
`dispatch.ndjson` from a `NewLocalDispatchStore` and asserts strict
uniqueness + contiguity across three terminal commits).

### BUG-407 — repair-resolution idempotent replay

`internal/runner/dispatch_store_memory.go` (`BeginRepairResolution`): when the
repair is already `resolved` under the same `resolutionID`, return the new
typed `*RepairResolutionReplay{Revision, Outcome}` instead of
`ErrRepairNotOpen` — the record carries `ResolutionID`/`ResolvedAction`
durably, so replay idempotency survives restart. A different `resolutionID`
on a closed repair is still a real conflict.
`internal/runner/dispatch_record.go`: new `RepairResolutionReplay` error type.
`internal/runner/dispatch_operator.go`: handler maps the replay to HTTP 200
with the recorded outcome; `ErrRepairNotOpen` now maps to 409
`dispatch_conflict` instead of falling through to 502.
Tests: `TestBug407_RepairResolutionReplayReturnsRecordedOutcome`,
`TestBug407_RepairNotOpenMapsToConflict`.

### BUG-408 — abandon terminalizes stranded records; repair rev continuity

`internal/runner/dispatch_store_memory.go`:
- `CommitRepairResolution`: on `RepairResolvedAbandon`, every non-terminal
  `DispatchRecord` for the run transitions `terminal_cancelled`
  (`abandoned,resolved_by=operator`) through the legal edge table, with
  attach revoke + claim clear + intent clear + audit + durable line per
  record. The boot scanner skips terminal records, so the
  `repair_required`/`cancel_required` attention leak stops recurring.
- `OpenRepair`: a genuinely new repair episode on a run with a resolved
  repair continues the revision sequence (`existing.RepairRevision+1`)
  instead of overwriting at rev=1 — the durable log no longer reads
  `resolved rev=3 → open rev=1`.
Tests: `TestBug408_RepairAbandonTerminalizesStrandedRecords`,
`TestBug408_ScannerDoesNotResurrectResolvedRepair` (arms the run stop so the
scanner would classify `cancel_required`, then asserts no re-open).

### BUG-409 — embedded-pack fallback on mirror store error

`internal/runner/flow_definition_resolver.go`:
- `ResolveBuiltin`: `GetByPackFlow` error now logs and falls through to the
  embedded pack (the function's own documented contract: "or is unavailable").
- `ResolveFlowRef`: `GetByRef` error is remembered (`storeErr`) and the
  built-in path is still attempted; for refs that don't resolve built-in,
  the original store error is re-surfaced — fail closed, never silent.
Tests: `TestBug409_ResolveBuiltinFallsBackOnStoreError`,
`TestBug409_ResolveBareRefFallsBackOnStoreError`,
`TestBug409_NonBuiltinRefStillErrorsOnDeadStore` (converse: non-builtin ref
still errors).

## Provider parity

All touched seams are provider-agnostic: session reconstruction field copy,
durable dispatch store ordering/repair/operator surfaces, flow-definition
resolution. No adapter or event-stream code. The live BUG-409 repro was
config-level (dead Supabase mirror) and verified on the local runner binary —
`vibe-cp-ingest` resolved from the embedded pack and spawned `cp_reader`.

## Verification

- Red-first: 9 tests in `internal/runner/bugg_cluster_dispatch_test.go` all
  failed by assertion before the production changes (7 assertion-reds + the
  typed error introduced as a declaration-only stub first).
- Focused: `go test -count=1 -run 'TestBug40[5-9]' ./internal/runner/` — 9/9
  green.
- Regression: dispatch/recovery/resume/resolver-focused suite — the only
  failures are `TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted` and
  `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil`, identical on the
  clean baseline worktree (machine-global config/env deps, BUG-427 class).
- Live: BUG-409 verified during the BUG-399 live session — writing `{}` to the
  workspace supabase config was only needed because the store errored; with
  the fix the embedded pack resolves even with a dead mirror.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-409
change_type: bugfix
summary: Cluster G dispatch/durability fixes — leg-field restore, terminal-commit seq ordering, repair-resolution replay idempotency, abandon terminalization, embedded-pack fallback on dead mirror
# --->8---
