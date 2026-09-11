# Task-327: Vibe missing-SS auto-resume from ingest_reader

## Metadata

- Document ID: `Task-327`
- Title: `Vibe missing-SS auto-resume — clear ss_lock park and restart ingest_reader`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md), [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [CA-793](../../../change-audit/CA-793-Vibe-Checkpoint-File-Exist.md), [CA-770](../../../change-audit/CA-770-Turn2-R3.md)
- Replaces: `None`
- Tags: `vibe-mode, resume, checkpoint, ss_lock, O-6`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- Close CP-60 Test-Steps **O-6** / live **R-SS-D** residual: when SS artifacts are deleted while the run is parked on `ss_lock`, do not leave a zombie lock card and do not let Continue advance to `cp_writer`.
- On **reconstruct** and on **Continue/lock**: if workspace has a cwd and SS is missing → clear `vibe_awaiting_lock` park + auto `startResolvedFlowFromNode(..., ingest_reader)`.
- Extends CA-793 demote (checkpoint empty) with the missing resume wiring; does not undo CA-770 restore when SS still exists.

### Current Ask

- Landed in CA-827. Live re-prove R-SS-D spawn on next TUI binary; optional follow-up for missing CP/Task auto-spawn.

### Key Decisions

- `T-1` Empty `workspaceCwd` = **unknown**, not missing — preserve CA-770 reconstruct fixtures that set lock paths without a sandbox.
- `T-2` Recovery start node is **`ingest_reader`** (R-SS-D spawn column), flow `vibe-ingest` (pack-prefixed).
- `T-3` Continue on missing SS must **not** stamp approved / advance to `cp_writer`; same recover helper as reconstruct.

### Constraints

- `feature_key: vibe-mode`. Additive tests only; no edits to CA-770 / CA-793 tests without operator approval.
- Provider-agnostic (no `providerKey` branch). Claude/Codex/Grok share reconstruct + lock resume.
- Do not auto-resume when SS files still exist (R-SS-K / CA-770 must stay green).
- Out of scope: CP-lock missing-CP recover; Task-layer demote auto-spawn (separate residual unless free).

### Open Questions

- None — live evidence from gate-sandbox R-SS-D (run-225468) is enough.

### Source Refs

- CP-60-Test-Steps §11 R-SS-D, O-6; CA-793 Replay note; SS-18 lock-before-sprint.

## 1. Goal

Operator deletes `requirements/05-System-Specs/SS-*.md` while parked on **SS Preview & Lock**, reopens the same run (or presses Continue): durable checkpoint stays/empties correctly **and** the flow clears the lock park and restarts SS generation from `ingest_reader` instead of showing a stale lock card or advancing to `cp_writer`.

## 2. Parent Links

- coding plan: `CP-60` live verification residual O-6; CA-793 follow-up resume wiring
- tech design: `SD-24` vibe ingest / lock
- system spec: `SS-18` SS lock before coding
- specific upstream ids: CP-60-Test-Steps R-SS-D, O-6; CA-793

## 3. Trigger

Live R-SS-D on `D:/working/gate-sandbox` (run-225468): SS deleted, reopen restored `ss_lock` WAITING card; `vibe_checkpoint_node` empty; Continue would wrongly try to lock. Operator asked why Continue does not detect missing SS and re-run gen SS — that is O-6.

## 4. Exact Change

- `T-1` Helper: detect missing SS for `ss_lock` park when `workspaceCwd` is set (`collectVibeArtifactsForNode` / `collectVibeSSFiles` + exist-gate).
- `T-2` `maybeRecoverMissingVibeSSLock(parentRunID)`: clear awaiting-lock fields + loop block; `startResolvedFlowFromNode(ctx, parent, pack:vibe-ingest, prompt, "ingest_reader")`.
- `T-3` Call after reconstruct (run registered) and at start of `resumeVibeLock` before stamp/advance.
- `T-4` Additive tests: reconstruct missing-SS recovers; reconstruct with SS present keeps park; Continue missing-SS recovers (no cp_writer advance); empty-cwd still preserves park (CA-770 contract).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/vibe_checkpoint.go` (or small sibling), `interactive_resume.go`, `vibe_lock.go`, new `task327_*_test.go`, `change-audit/CA-827-*.md`, `requirements/08-Task/done/Task-327-*.md`, `CP-60-Test-Steps.md`
- modules: `runner` vibe checkpoint / lock / reconstruct
- routes: none
- tables: none

## 6. Acceptance Check

- New tests green; `TestVibeSession_ReconstructAwaitingLockAndIdempotent` and `TestCA793_*` untouched and green.
- Missing SS + awaiting `ss_lock` → park cleared + ingest restarted from `ingest_reader` on reconstruct and on Continue.
- SS present → park restored (R-SS-K / CA-770).
- CP-60-Test-Steps: R-SS-D spawn column / O-6 updated for this shape.

## 7. Out of Scope

- Auto-resume for missing CP/Task at other layers (may share helper later).
- Desktop-only chrome; Admin Web.
- Editing legacy CA-770/CA-793 tests.

## 8. Completion Notes

- result: `done` — `maybeRecoverMissingVibeSSLock` on reconstruct + `resumeVibeLock`; `TestTask327_*` green; old CA-770/CA-793/BUG-365 untouched green
- follow-ups: optional same pattern for `cp_lock` / task demote auto-spawn; live R-SS-D re-tick after rebuild
- upstream docs updated: CP-60-Test-Steps O-6 / R-SS-D / F1; CA-827
