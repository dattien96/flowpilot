# BUG-467: Drift zero_delta_progress never fires — runner bookkeeping counted as file delta

- status: done
- found: live run-49322 / turns 49626→50686 (C-23-2 drift-ladder drill on full bed)
- fixed_by: CA-964
- tests: internal/runner/bug467_drift_zerodelta_runtime_metadata_test.go

## Symptom (live)

On the full test bed (`/Users/tiendat/fp-beds/full`, where `.flowpilot/**`
is git-tracked), six consecutive heavy-token pure-explanation turns
(18k–99k tokens each, zero agent file writes) all logged
`[drift] ... score=0 signals=[] action=none`. The `zero_delta_progress`
signal (+20) could never fire, so the correction ladder
(inject_system_note → narrow_context → pause_for_human) was unreachable
no matter how many zero-progress turns stacked up.

Gate log showed why — every turn's turn-scoped diff contained only the
runner's own state files:

    diff=[{.flowpilot/chats/.../dispatch.ndjson M}
          {.flowpilot/chats/run-49322-turns.ndjson A}
          {.flowpilot/chats/sessions.ndjson M}]

The runner appends to these ndjson files during *every* turn (dispatch
records, session rows, per-run turn logs), so their worktree fingerprint
always differs from the turn-start snapshot → they survive the
turn-scoped filter → `TurnResult.ChangedPaths` non-empty →
`summary.FilesChanged` non-empty → `checkZeroDeltaProgress` returns
false forever.

## Root cause

`turnSummaryFromTurnResult` fed `tr.ChangedPaths`/`tr.WrittenPaths` into
`driftdetect.TurnSummary.FilesChanged` verbatim. BUG-288 #16/F-25
already established that `.flowpilot/**` is "runtime metadata, not
product code" (`flowgate.IsDocOrAuditFile`) — but that exclusion was
only applied to scope/contract checks, not to the drift summary's
progress evidence. Runner bookkeeping is not agent progress; counting
it made the "file delta" check meaningless.

## Fix

`turnSummaryFromTurnResult` now filters `.flowpilot/` paths from both
`WrittenPaths` and `ChangedPaths` via `stripRuntimeMetadataPaths`
(`gate_hook.go`). Only `.flowpilot/**` is excluded — `*.md`,
`requirements/`, `change-audit/` paths still count, since doc-writer
agents legitimately produce those as real deliverables.

No change to `changedPathsFromDiff` or the gate path: the raw
turn-scoped diff still feeds scope/contract/feature-key logic, where
seeing `.flowpilot/**` paths is correct (e.g. an agent forging
`frozen_contracts.ndjson` is a real scope violation, BUG-288 R13).

## Provider parity

Provider-agnostic: `ChangedPaths` comes from the shared
`observeTurnScopedDiff` path; the suppression hit identically on devin
turns. Any provider's zero-delta turn is now scored correctly.

## Live verification (post-fix)

Test-level ladder verified in
`TestBug467_DriftLadderReachesPauseOnMetadataOnlyDeltas`: 4 consecutive
metadata-only turns score 20→40→60→80 resolving
`none`→`inject_system_note`→`narrow_context`→`pause_for_human`.
Live re-verification on the bed tracked in CP-Full-Live-Test.md C-23-2.
