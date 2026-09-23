# BUG-400: Turn-level flowRef bypasses FlowAllowedForWorkingMode — vibe-sprint ran in a dev-mode run

## Metadata

- Document ID: `BUG-400`
- Title: `handleStartTurn skips working-mode gate for vibe-family flowRefs — vibe-sprint launched and completed under workingMode:"dev"`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-41-RAG-Harness-Flow-Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)
- Feature Keys: `vibe-mode`, `agent-flow-engine`

## AI Quick View

### Summary

- `enforceWorkingModeStart` (`working_mode_http.go:30-59`) forbids `vibe-sprint` for dev user/system starts (`workingmode.go:188-196`), but only on the run-create path.
- `handleStartTurn` skips `validateChatOrchestrationSelection` for all vibe-family ids via `SkipChatOrchestrationCheck` (`workingmode.go:142-145`; `interactive_handlers.go:428`) and applies **no** working-mode check before `startResolvedFlow` (`interactive_handlers.go:428-470`, `interactive_service.go:8996-9015`).
- Observed live: `flowRef:"vibe-sprint"` in a `workingMode:"dev"` run launched and **completed** the full sprint chain (runs 11597, 13679, 15641 — incl. audit DONE with `flow_audit_draft status:"ready"`).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** The working-mode flow allowlist is enforced at run creation but not at the turn-level `flowRef` path — a dev-mode run can start a vibe/system flow by naming it on a turn.
- **Expected:** `FlowAllowedForWorkingMode` (or equivalent) applies to turn-level `flowRef` starts; `vibe-sprint` under `dev` returns the same `working_mode_flow_forbidden`-class rejection as the create path.
- **Actual:** The skip-orchestration-check fast path admits harness + vibe ids straight to `startResolvedFlow` with no mode gate; the hidden-flow list is enforced on this path only incidentally via the orchestration-option check.
- **Impact:** Mode isolation between dev and vibe surfaces is porous — a vibe-only/system flow can be driven from a dev run, bypassing the client/mode restrictions CP-60 verified on the create path (L-60-1 probes: dev→vibe flows → 400 `working_mode_flow_forbidden`).

## Reproduction

1. Create a run with `workingMode:"dev"`.
2. `POST /client/workflow-runs/{run}/turns {"flowRef":"vibe-sprint", …}`.
3. Observe the sprint launch and run to completion (plan → freeze → context → tdd → coder → validate → synthesis → audit).
- runIds: `run-11597`, `run-13679`, `run-15641` (all completed vibe-sprint chains under dev-mode runs, CP-41 session).

## Root cause

- `apps/local-runner/internal/runner/interactive_handlers.go:428` — `if !workingmode.SkipChatOrchestrationCheck(body.FlowRef)` diverts vibe/harness ids past `validateChatOrchestrationSelection`; no `FlowAllowedForWorkingMode` call exists on the turn path before `startResolvedFlow` (`interactive_service.go:8996-9015`).
- `apps/local-runner/internal/runner/working_mode_http.go:30-59` / `workingmode.go:142-145,151-196` — the mode gate (`FlowAllowedForWorkingMode`) is wired to run-create/`enforceWorkingModeStart` only; `SkipChatOrchestrationCheck` returns true for the same family of ids and is used as a full bypass.

## Evidence

- `~/fp-beds/lt-evidence/cp41/RESULT.md` (BUG-LIVE-3; launch-path probes table)
- `~/fp-beds/lt-evidence/cp41/probe-*.json`/`probe-*.headers` — literal launch rejections vs accepted turn-level `vibe-sprint`
- `~/fp-beds/lt-evidence/cp41/l41-1/` — run-13679/run-15641 vibe-sprint audit paths (flow_audit_draft `status:"ready"`, featureKey `calc-format`)
- `~/fp-beds/lt-evidence/cp41/l41-5/` — run-11597 blocked_missing_feature_key draft via same path

## Severity

- `low` — mode-boundary enforcement gap; observed flows executed correctly (no corruption), but the dev↔vibe isolation contract is unenforced on the turn surface.

## Completion Notes (implemented 2026-09-23, CA-920)

- `handleStartTurn` applies `FlowAllowedForWorkingMode` to resolved `flowRef` before dispatch — `vibe-sprint` under `dev` rejected; allowed dev harness flows pass. Run-creation enforcement unchanged.
- Unit: `TestBug400_TurnFlowRefRespectsWorkingMode`, `TestBug400_TurnFlowRefHarnessAllowedUnderDev` — green.
- (Merged from duplicate report `BUG-400-TurnFlowRef-Bypasses-Working-Mode.md` during BUG-444 doc reconciliation.)
