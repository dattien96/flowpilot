# BUG-411: `vibe-owner-debate` parks `WAITING_USER_APPROVAL`/`hub_stalled`; accepted gate decision produces zero flow events — dead option

## Metadata

- Document ID: `BUG-411`
- Title: `Owner-debate escalate park: POST /gate-decision keep-test-fix-code returns {"status":"accepted"} but never lifts the park — no API surface resumes the sprint`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-67-Test-Steps](../../07-Coding-Plan/todo/CP-67-Test-Steps.md), [CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Feature Keys: `vibe-mode`, `owner-debate`, `gate-decision`

## AI Quick View

### Summary

- CP67-4, two independent repros (run-21689 clipx, run-25555 pairx): after `debate_synthesis` submits `changes_requested` → `flow_control_continue` → `debate_trigger` parks `WAITING_USER_APPROVAL` (`flow_parked_awaiting_user`); watchdog marks `blocked / hub_stalled` (~2m). Submitting `keep-test-fix-code` via `POST /gate-decision` returns `{"status":"accepted"}` (logged `[gate-decision] runID=…`) but produces **zero** subsequent flow events — the park is never lifted.
- run-21689 detail: operator answered twice (`agent-loop/continue` → re-parked; `gate-decision {"option":"custom"}` → accepted, still wedged). A single scope-drift escalate permanently wedges the run inside the owner-debate resolver even after the drifted file was removed.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** the gate-decision API accepts remediation options on a debate-parked run (`{"status":"accepted"}`, audit log line emitted) yet nothing happens — no reprompt dispatch, no sprint resume, no further flow events; the loop stays `blocked/hub_stalled` with `debate_trigger` `WAITING_USER_APPROVAL` indefinitely.
- **Expected:** `keep-test-fix-code` (keep the test, fix the code) should lift the park and re-drive the sprint remediation — or the option should be refused if it cannot act.
- **Actual:** dead option — accepted-but-inert gate decisions; the only observed exits are abandoning the run or the BUG-404-style false-done path after restart.
- **Impact:** vibe owner-debate escalation is a one-way wedge: operators are offered remediation options that cannot fire; the sprint never retries `tdd` even when the underlying violation was resolved (drifted file removed). Two live repros in one session.

## Reproduction

1. Vibe sprint run whose scaffold/coder child gets gate-blocked (e.g. scope drift — run-21689: `strayx/evil.go` outside frozen contract, later removed) → `vibe-owner-debate` overlay launches (`debate_trigger → owner_1/owner_2 → debate_synthesis`).
2. Debate resolves `changes_requested` → `debate_trigger` parks `WAITING_USER_APPROVAL`; watchdog marks `blocked/hub_stalled`.
3. `POST /client/workflow-runs/{childRunId}/gate-decision {"option":"keep-test-fix-code"}` → `{"status":"accepted"}` + `[gate-decision]` log → zero follow-on events.
4. `agent-loop/continue` re-runs `debate_trigger` → re-parks; `gate-decision {"option":"custom"}` → accepted, still wedged.

## Root cause

- Suspected: the `debate_trigger` `WAITING_USER_APPROVAL` park is lifted only through the debate-specific continue path; the generic `gate-decision` handler records acceptance against the child run (`[gate-decision] runID="run-25724"`) but does not feed the option back into the debate/sprint state machine — no edge is dispatched and `parkFlowForAwaitingUser`'s cleared reprompt fields (cf. BUG-403 mechanism, `interactive_service.go` ~L2643-2649) leave nothing to resume.

## Evidence

- `~/fp-beds/lt-evidence/cp67/l67b-run21689-debate-deadlock.txt` — full timeline (08:12:54 gate block → 08:19:44 WAITING_USER_APPROVAL → accepted gate-decision → still wedged).
- `~/fp-beds/lt-evidence/cp67/l67b-run21689-agentgraph-stalled.json`, `l67b-run21689-flowdiag.ndjson`, `l67b-run21689-agentgraph-escalated.*`.
- `~/fp-beds/lt-evidence/cp67/l67b-run25555-snapshot.json`, `l67b-run25555-sse.ndjson` — second repro.
- `runner.log` — `[vibe-gate] start vibe-owner-debate hub=run-25555 child=run-25724 drift=0`; `[gate-decision] runID="run-25724" option="keep-test-fix-code"` with no follow-on.
- `~/fp-beds/lt-evidence/cp67/RESULT.md` (BUG-LIVE-4).

## Severity

`medium` — recoverable only by abandoning the run; user-facing remediation option is a dead button; every owner-debate escalate can permanently wedge the sprint.

## Completion Notes (implemented 2026-09-23, CA-921b)

- Root cause: `SubmitGateDecision` consumed `pendingGateBlock` then dispatched via `go startTurn` — a parked (blocked) loop made `startTurn` reject `flow_awaiting_user`, the goroutine swallowed the error, and the API returned accepted-and-dead (debate remediation options did nothing).
- Fix: when the target run's own loop — or its parent's — is blocked awaiting a decision, the option routes into `resumeFlowWithFeedback` so the parked hub re-drives with the remediation in its reinvoke note; dispatch rejection restores `pendingGateBlock` and returns the real error.
- Files: `internal/runner/gate_hook.go`.
- Tests: `TestBug411_GateDecisionOnParkedChildRoutesToParentResume`, `TestBug411_GateDecisionOnBlockedHubResumesFlow`, `TestBug411_GateDecisionSurfacesStartTurnReject`. Baseline-red verified.
