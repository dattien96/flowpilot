# BUG-404: Runner restart during negotiation loses stashed sprint state → false-done, remediation dropped

## Metadata

- Document ID: `BUG-404`
- Title: `Restart during owner-debate negotiation loses vibeParkedNodes/pendingBatchSignatureByStep → run ends done with reprompt remediation dropped, work never delivered`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-67-Test-Steps](../../07-Coding-Plan/todo/CP-67-Test-Steps.md), [CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Feature Keys: `vibe-mode`, `owner-debate`, `flow-resume`, `negotiation`

## AI Quick View

### Summary

- CP67-5 (run-25555, child run-25724): runner restarted while the vibe-owner-debate was parked `WAITING_USER_APPROVAL` mid-negotiation. Post-restart every run → `run_not_found` (no auto/lazy resume); the child was **never** reconstructed. After explicit `POST /resume`, the parent offered "Resume from owner_2?" → `ok` → `debate_synthesis` re-invoked → submitted `done` → `flow_run_complete_done` — while the pending `changes_requested` reprompt remediation was silently dropped and `pairx/` was never delivered. False-done on restart.
- Root cause: the stashed sprint topology (`vibeParkedNodes`/`vibeParkedEdges`/`vibeParkedAcceptance`/`vibeParkedFlowRef`, `interactive_service.go:202-205`) and `pendingBatchSignatureByStep` (`interactive_service.go:481-486`) are **in-memory only** — `interactive_resume.go` restores neither (zero references).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** after a runner restart during a parked owner-debate negotiation: (1) `GET /client/workflow-runs/{id}` → `run_not_found` for parent AND child until explicit `POST /resume` (no boot/lazy resume); (2) child run-25724 (scaffold, `waiting_user_approval`) is never reconstructed — `run_not_found` persists after the parent's resume; (3) resumed parent re-ran `debate_synthesis`, submitted `done` (`{"status":"done","round":1,"nextAction":"done"}` evt seq 65) → `flow_run_complete_done`; the sprint's `tdd` node was never re-dispatched — `pairx/` absent from the worktree, run marked `done`.
- **Expected:** restart during negotiation restores the stashed sprint topology + pending negotiation batches; the debate `done` verdict resumes the sprint and re-drives the pending remediation (reprompt coder); children are reconstructed or explicitly reconciled.
- **Actual:** `debate_synthesis` `done` found `len(vibeParkedNodes)==0` → `restoreVibeFlowAfterDebate` returned false → sprint flow permanently lost → the pending `changes_requested` remediation was dropped and the run reported `done` with the feature undelivered.
- **Impact:** false-done on restart — the exact BUG-401 settle path, but triggered by losing in-memory negotiation state. Any crash/restart during a debate or signature negotiation silently abandons the sprint and emits a successful terminal state.

## Reproduction

1. Run a vibe sprint whose coder child ends gated → `vibe-owner-debate` overlay parks `WAITING_USER_APPROVAL` mid-negotiation (run-25555 shape: scaffold rejection → owners verdict `changes_requested`).
2. SIGKILL/restart the runner while parked.
3. `GET /client/workflow-runs/run-25555` → `run_not_found`; `POST /resume` the parent; answer the "Resume from owner_2?" offer with `ok`.
4. Observe `debate_synthesis` re-invoke → `done` → `flow_run_complete_done` with no `tdd` re-dispatch and no remediation delivered; child run-25724 remains `run_not_found` forever.

## Root cause

- `vibeParkedNodes`, `vibeParkedEdges`, `vibeParkedAcceptance`, `vibeParkedFlowRef` (`apps/local-runner/internal/runner/interactive_service.go:202-205`) and `pendingBatchSignatureByStep` (`interactive_service.go:481-486`) live only on the resident `interactiveRun` — nothing persists them, and `interactive_resume.go` never repopulates them (zero references).
- On `debate_synthesis` done, `restoreVibeFlowAfterDebate` (`vibe_cp.go:666`) early-returns false on `len(rs.vibeParkedNodes)==0` → the `done` verdict falls through to whole-run settle (BUG-401 shape) → `flow_run_complete_done` with the sprint abandoned.
- Secondary gap: boot recovery/lazy resume does not surface non-terminal runs (`run_not_found` until explicit resume) and never reconstructs parked children.

## Evidence

- `~/fp-beds/lt-evidence/cp67/l67d-run25555-flowlog-tail.txt` — `flow_control_done`, `flow_run_complete_done` at 08:41:59 with no tdd re-dispatch.
- `~/fp-beds/lt-evidence/cp67/l67d-run25555-pre-restart-*.json` vs `l67d-run25555-post-restart-*` (status/events/graph/steps/timeline) — pre-restart negotiation park vs post-restart done.
- `~/fp-beds/lt-evidence/cp67/l67d-run25724-post-restart-status.json` — child stays `run_not_found` after parent resume.
- `run-25555-turns.ndjson` — resumed synthesis: "synthesis already complete from the prior turn… Calling the control tool to close this step".
- `~/fp-beds/lt-evidence/cp67/RESULT.md` (BUG-LIVE-5).

## Severity

`high` — restart turns a recoverable negotiation park into silent false-done; pending remediation and undelivered work are dropped with a `done` verdict.

## Completion Notes (implemented 2026-09-23, CA-921b)

- Root cause: the debate-parked sprint topology (`vibeParkedNodes`/`Edges`/`Acceptance`/`FlowRef`) and buffered coder batch signatures lived only in RAM — a restart mid-negotiation lost them and the resumed flow false-done'd.
- Fix: fields added to `ProviderSessionState` and round-tripped through `sessionStateOf`/reconstruct restore.
- Files: `internal/runner/interactive_service.go`, `interactive_resume.go`, `sessions.go`, `workflow_store.go`, `local_file_session_store.go`.
- Tests: `TestBug404_ParkedSprintStateRoundTripsSession`. Baseline compile-red (new fields).
