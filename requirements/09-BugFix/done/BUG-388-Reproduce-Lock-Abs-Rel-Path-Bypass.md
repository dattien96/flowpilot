# BUG-388: Reproduce lock no-op — absolute ReadOnlyPaths vs relative enforcement candidate

## Metadata

- Document ID: `BUG-388`
- Title: `Reproduce lock unenforceable — stored absolute paths never match relativized write candidates`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-64-Test-Steps](../../07-Coding-Plan/done/CP-64-Test-Steps.md)
- Feature Keys: `reproduce-first-gate`, `change-contract`

## AI Quick View

### Summary

- The reproduce gate locked the RED test file and the **coder then overwrote it twice** — no `reproduce_test_locked` deny ever fired.
- `changecontract.LockReproduceTestPaths` stores `WrittenPaths` verbatim → `ReadOnlyPaths` holds **absolute** paths; enforcement `decideReproduceTestLock` builds its candidate via `normalizeReproduceLockCandidate`, which converts the write path to **workspace-relative**; `IsReadOnlyLockedPath` compares with `normalizeScopePath`, which only cleans separators and never relativizes → the two sides never match → YOLO auto-approves the write.
- The unit test `TestCoderNodeHasTestFileAsReadOnly` uses relative written paths, so the live abs-vs-rel asymmetry is uncovered.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** After `reproduce_test` locks the test file (contract v2 `read_only_paths`), the coder's `fs/write_text_file` to the exact locked absolute path succeeds — twice, on independent runs.
- **Expected:** `IsReadOnlyLockedPath` matches the write candidate and the approval layer denies with `reproduce_test_locked`.
- **Actual:** Zero denies. run-4501: lock 05:09:06 (runner.log:6611) → coder overwrite 05:09:45 (runner.log:6921), rewriting the fabricated RED assertion into always-passing checks. run-6008: lock 05:28:25 (runner.log:10717) → coder overwrite 05:28:49 (runner.log:10882), reintroducing a syntax error.
- **Impact:** The CP-64 "locked read-only for coder" guarantee is void for any provider emitting absolute write paths (confirmed opencode; absolute paths also stored in run-3914/run-5026). The evidence test the lock exists to protect can be mutated by the very agent it restrains — compounding BUG-389 (fabricated RED) since the coder can "fix" by weakening the locked test.

## Reproduction

1. Run `bug-harness` on a provider whose write calls carry absolute paths (opencode observed) through RED → lock.
2. In the coder child, call `fs/write_text_file` on the locked file's absolute path.
3. Observe: write approved/auto-approved; no `reproduce_test_locked` deny in runner.log.
- runIds: `run-4501` (coder `run-4687`), `run-6008` (coder `run-6184`); absolute `read_only_paths` also recorded in `run-3914`, `run-5026`.

## Root cause

- `apps/local-runner/internal/changecontract/frozen_scope.go:634` — `IsReadOnlyLockedPath` compares `normalizeScopePath` of stored vs candidate; `normalizeScopePath` (`frozen_scope.go:278`) only cleans separators — it never relativizes.
- `apps/local-runner/internal/runner/reproduce_gate.go:159-180` — `normalizeReproduceLockCandidate` converts the approval's absolute write path to workspace-relative; used at `reproduce_gate.go:254` inside `decideReproduceTestLock`.
- `LockReproduceTestPaths` stores `WrittenPaths` verbatim (absolute) into `ReadOnlyPaths` (`frozen_contracts-final.ndjson`, every v2 record) → relative candidate `calc_..._test.go` ≠ stored `/Users/.../calc_..._test.go` → deny never fires.
- Test gap: `TestCoderNodeHasTestFileAsReadOnly` passes because its fixture uses **relative** written paths.

## Evidence

- `~/fp-beds/lt-evidence/cp64/RESULT.md` (Bugs table, BUG-LIVE-CP64-02)
- `~/fp-beds/lt-evidence/cp64/BUG-LIVE-CP64-02-lock-bypass-abs-vs-rel-path.md` — full field report (rated critical there)
- `~/fp-beds/lt-evidence/cp64/runner.log` — lock at :6611/:10717, overwrites at :6921/:10882
- `~/fp-beds/lt-evidence/cp64/frozen_contracts-final.ndjson` — absolute `read_only_paths` in v2 records
- `~/fp-beds/lt-evidence/cp64/run4501/`, `~/fp-beds/lt-evidence/cp64/run6008/` — per-run artifacts

## Severity

- `high` (assigned; field report rated critical) — the reproduce-first lock does not lock anything for absolute-path providers; evidence files are writable by the coder.

## Completion Notes (implemented 2026-09-23, CA-919b)

- ReadOnlyPaths now stored workspace-relative (LockReproduceTestPaths/LockScaffoldArtifactsForStep); IsReadOnlyLockedPathUnder tolerates legacy absolute records; deny bridge + drift filter updated. Test: bug388_reproduce_lock_abspath_test.go.
