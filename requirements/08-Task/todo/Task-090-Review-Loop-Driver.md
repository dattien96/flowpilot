# Task-090: Review Loop Driver

## Metadata

- Document ID: `Task-090`
- Title: `Review Loop Driver (Verdict → Round/Cap/Restart)`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SS-15: Agent Review Loop](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md)
- Child Documents: `None`
- Related Documents: [Task-089: Submit Review Outcome Tool](./Task-089-Submit-Review-Outcome-Tool.md)
- Replaces: `None`
- Tags: `multi-agent, review-loop, orchestrator, state-machine, go`

## AI Quick View

### Summary

- Implement `submitReviewOutcome` in the interactive service: apply the verdict, advance the round, restart the coder with consolidated feedback, enforce the cap, and move to `blocked` (ask-user) when the cap is hit with open issues.
- Extend `AgentLoopState` with `openIssues`, `mode`, `coderRunId`, `extendCount`; add the `review-outcome` and `agent-loop/extend-cap` HTTP routes; enforce the bounded-extend ceiling.

### Current Ask

- Build the deterministic loop state machine driven solely by `submit_review_outcome`, with bounded termination.

### Key Decisions

- `T-1` `changes_requested` under cap restarts the coder via the existing restart path; at cap it sets `blocked` and does NOT restart (SD-18 `D-6`).
- `T-2` Extend is bounded: `+2` per extend, `extendCount ≤ 2`, ceiling = initial cap + 4 (SD-18 `D-9`).

### Constraints

- Run GitNexus impact analysis before editing `interactive_service.go` loop code and `agent_orchestrator.go`; warn on HIGH/CRITICAL.
- Reuse the existing coder restart path (`interactive_service.go:899-927`, `scheduleChildTurn`, `takeQueuedFeedbackPrompt`); do not mutate provider transcripts directly.
- Loop must always terminate (cap or stop). No Supabase migration.

### Open Questions

- None (defaults locked in SD-18 `D-6`/`D-9`).

### Source Refs

- CP-36 `P-2`; SD-18 `D-6`, `D-9`, §5 (state transitions), §7; SS-15 `AC-4`, `AC-5`, `AC-6`, `BR-4`, `BR-5`, `BR-10`.
- Anchors: `agent_orchestrator.go` (`AgentLoopState`, `transition`, `advanceRound`, `mutateLoop`, `addBus`); `interactive_service.go:343-384` (loop control), `:899-927` (restart path), `:1174` (bridge); `interactive_handlers.go:41-48,860`.

## 1. Goal

A verdict submitted by the orchestrator deterministically drives the loop: clean → done; issues under cap → coder re-runs with the merged feedback and a new round begins; issues at cap → `blocked` awaiting a bounded user decision.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) `P-2`
- tech design: [SD-18](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md) `D-6`, `D-9`, §7
- system spec: [SS-15](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md) `AC-4`/`AC-5`/`AC-6`
- specific upstream ids: CP-36 `P-2`

## 3. Trigger

Task-089 delivers the verdict signal but nothing acts on it. The loop needs the state machine that turns a verdict into the next round, a clean finish, or a bounded user gate.

## 4. Exact Change

- `T-1` Extend `AgentLoopState` (Go `provider_event.go` + TS `contract.ts:123`) with `openIssues`, `mode` (`keyword`|`explicit`), `coderRunId`, `extendCount`.
- `T-2` Implement `submitReviewOutcome(parentRunID, in)` in `interactive_service.go`: record a `review-outcome` bus message; set `mode="explicit"`; apply `roundCapOverride`; branch on status:
  - `approved` → `transition("approved")`, `openIssues=0`, append a parent handoff note, clear `autoOrchestrate`, `nextAction="done"`.
  - `changes_requested` → `openIssues=len(issues)`; `advanceRound`; if round `≥ cap` → `blocked` + gate reason, `nextAction="awaiting_user"`; else restart `coderRunId` with the consolidated feedback prompt (issues numbered by file + `feedback`), `nextAction="looping"`.
  - `blocked` → `Status="blocked"`, `nextAction="awaiting_user"`.
- `T-3` Add routes (`interactive_handlers.go`): `POST .../review-outcome` → `handleSubmitReviewOutcome`; `POST .../agent-loop/extend-cap` → `handleExtendRoundCap`.
- `T-4` Enforce bounded extend in `handleExtendRoundCap`/`submitReviewOutcome`: reject when `extendCount ≥ 2`; else `cap += 2`, `extendCount++`, resume if was `blocked`.
- `T-5` Emit an updated graph snapshot on every transition; add `[review-loop]` logs.

## 5. Touched Areas

- files: `agent_orchestrator.go`, `provider_event.go`, `interactive_service.go`, `interactive_handlers.go`, `interactive_resume.go` (persist/restore `mode`/`round`/`openIssues`/`extendCount`), `contract.ts`.
- modules: local-runner orchestration + interactive service.
- routes: `POST /client/workflow-runs/{runId}/review-outcome`, `POST /client/workflow-runs/{runId}/agent-loop/extend-cap`.
- tables: none.

## 6. Acceptance Check

- Unit: approved→done; changes_requested under cap → round++ and coder restart scheduled; changes_requested at cap → `blocked`, no restart; blocked→awaiting_user; extend from `blocked` resumes and is rejected after 2 extends.
- Contract: both new routes round-trip; `AgentLoopState` new fields serialize.
- Manual: a forced `changes_requested` re-runs the coder with the merged issue list; cap-hit shows `blocked`.

## 7. Out of Scope

- The consolidated multi-reviewer note (Task-091); auto-reinvoke (Task-092); the legacy-mode gate (Task-095); UI (Task-094).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
