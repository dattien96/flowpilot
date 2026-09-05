# CA-745 — BUG-357: record per-run file_artifact OUTPUT instances on flow-child completion

# ---8<--- flowpilot:change-ledger
feature_key: artifacts
source_doc_id: BUG-357
change_type: bugfix
summary: writer completion now records per-run file_artifact OUTPUT instances (runArtifactStore, merged first in per-run artifacts API) so the panel surfaces actually-written bound files; Q-1 local-only, Supabase sync deferred
# --->8---

## Problem (E1 FAIL, CP-58 live 2026-09-05)

- After run-548341 (`plan_writer` wrote `Task-910-calc-core-lcm.md` via bound `plan_md` output), Desktop Artifacts panel showed "No local artifacts yet" / "No artifact runs yet", and `GET /client/workflow-runs/run-548341/artifacts` returned only the 2 generic finalizer/fake rows.
- Root cause (confirmed by research, no prior recording anywhere): bindings tell the writer WHERE to write and the gate verifies existence, but nothing materialized a per-run OUTPUT instance (no INSERT/append of `artifact_instances`/`artifact_runs`, no finalizer row for bound outputs). Task-307 built bindings + mirror seeding but missed the write-completion recording leg. Not a regression — never implemented.

## Decision (answers BUG-357 Q-1/Q-2)

- Q-1 → local per-run only: new in-memory `runArtifactStore` (same lifetime model as the finalizer store), merged FIRST in `handleListArtifacts` ahead of finalizer rows; fake catalog only when both absent. Supabase `artifact_runs` sync deferred as a separate cut-over (needs project/auth context; the per-run surface needs no sync).
- Q-2 → `fakeArtifacts` stub left untouched (pre-first-finalize fallback only).

## Change

- `runner/run_artifacts.go` (new): `runArtifactStore` (record upsert-by-ID, forRun copy), `buildFlowChildArtifactRecords` (pure; deterministic IDs `run:artifact:node:N`, Kind `file_artifact`, Name=base, Path/Preview=resolved rel path, NodeID), `snapshotFlowChildArtifactPathsLocked` (routing keys + plain+structured required OUTPUT paths, deduped), `recordFlowChildArtifacts` (existence filter via `flowgate.MissingRequiredFileArtifactOutputs` — gate-identical, workspace-escape-safe — then store).
- `runner/provider_event.go`: `Artifact` += `path` + `nodeId` (`omitempty`, backward-compatible).
- `runner/gate_hook.go`: `flowNodeForRun` split into locking wrapper + `flowNodeForRunLocked` (identical behavior; the Locked variant serves the settle hook which already holds `s.mu` — the wrapper would deadlock).
- `runner/interactive_service.go`: `InteractiveService.runArtifacts` field + constructor init; `settleFlowChildTurnCompletedLocked` snapshots bound paths synchronously then dispatches recording via goroutine (established `maybeScheduleHubStallCheck` pattern). No-op for runs without required OUTPUT bindings.
- `runner/interactive_handlers.go`: `handleListArtifacts` merges recorded-first + finalizer, fake fallback preserved.
- Only files actually present on disk are recorded (E2 no-reprompt regression guard: a writer that wrote the wrong path records nothing).

## Verification

- 6 new tests (`bug357_run_artifacts_test.go`) all PASS: builder fields/ID shapes/dedupe, store upsert+copy+nil-safety, TempDir only-existing-files (missing + `../escape` dropped), snapshot node resolution (required-only, unknown-label empty), handler merge order + counts.
- `go vet` clean; gofmt clean on all new/touched lines (4 flagged files are pre-existing churn in untouched regions — left alone per repo rule).
- Full `tui/...`, `flowgate`, `agentpack` suites green. Runner suite failure set == clean-tree baseline (proven via `git stash -u` symmetric runs: `TestFinalizerHookSurfacesArtifacts` pluralization + env-sensitive families fail identically without this change) — zero new failures, zero pre-existing test edits.
- 3-provider parity: shared server-side recording path + run-scoped (provider-agnostic) artifacts; no provider-specific branch added.
- Live-verify pending (operator): rerun task-harness → per-run API + panel show `plan_md` with the real resolved path → E1 PASS.
