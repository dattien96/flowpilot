# BUG-233: Flow Step Timeline Shows Stale "All Done" While A Reviewer Still Runs; Blocked Card Shows Internal Diagnostic Instead Of Reviewer Findings

## Metadata

- Document ID: `BUG-233`
- Title: `Flow Step Timeline Shows Stale "All Done" While A Reviewer Still Runs; Blocked Card Shows Internal Diagnostic Instead Of Reviewer Findings`
- Phase: `bugfix`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [BUG-231: Escalate/Awaiting-User Flow State Is Not Actionable](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md), [CA-226: Hub Synthesis Fallback Escalation](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, step-timeline, desktop, flow-control, escalate`

## AI Quick View

### Summary

- Surfaced by the same live test session that shipped BUG-231's Continue/Stop card. Two separate, pre-existing issues — neither caused by BUG-231's own code:
  1. **Step timeline staleness**: right after a Continue-driven new round starts, the step timeline briefly (and sometimes not-so-briefly) shows every step as done while a freshly spawned reviewer child agent is still visibly `running` in the agent panel.
  2. **Blocked-card content mismatch**: the new BUG-231 `FlowAwaitingUserCard` sometimes shows an internal diagnostic string ("Hub synthesis turn completed without calling submit_review_outcome. Final message: …") instead of anything resembling the reviewers' actual findings — confusing to a user who is being asked to make a call.

### Current Ask

- Investigate (done, this doc) and fix. Not yet implemented — split out from BUG-231 per explicit user direction (fix BUG-231's own regressions immediately; document these pre-existing issues separately).

### Key Decisions

- `D-1` For bug #4 (content mismatch), the fix direction is: when the loop blocks because the hub skipped `submit_review_outcome` (the CA-226 fallback path), the card should show a **concise, consolidated summary of the reviewers' actual findings** — not the raw internal diagnostic sentence — so the user can act on real review content instead of engine plumbing text.
- `D-2` For bug #1 (timeline staleness), no fix design has been decided yet — see Root Cause and Open Questions below for the two candidate directions.

### Open Questions

- `Q-1` Bug #1's exact trigger step (which refresh ordering) needs a live-log repro to confirm before choosing between "make one channel authoritative" vs "sequence the two refreshes" (see Fix Strategy options below).
- `Q-2` Bug #4's "reviewers' findings" source: cohort results are joined into `pendingAgentContext`/the auto-reinvoke note (see CA-226) but are not currently retained anywhere as a structured, re-displayable value once the hub's own turn (successful or fallback) has consumed them. Need to decide whether to capture the joined cohort text into `AgentLoopState`/`GateReason` explicitly at join time, or re-derive it from the reviewer child transcripts on demand when rendering the card.

### Source Refs

- `apps/local-runner/internal/runner/flow_step_runtime.go:186-205` — `markFlowRunComplete` settles ALL steps to `DONE` + run status `EngineDone`; never inspects whether any child agent (e.g. a newly spawned reviewer for the next round) is still running.
- `apps/desktop-flowpilot/src/state/store.ts` — `refreshWorkflowStepRuntime` and `refreshAgentRuns` (`agent_graph_updated`) are two independent, unordered refresh channels; nothing currently sequences a step-runtime refresh to wait for an in-flight agent-graph refresh (or vice versa) after a new round starts.
- `apps/local-runner/internal/runner/interactive_service.go:2519-2522` — the CA-226 fallback that escalates with `Summary: "Hub synthesis turn completed without calling submit_review_outcome. Final message: " + fin.FinalMessage`, which becomes the card's `gateReason` verbatim.
- `apps/desktop-flowpilot/src/components/FlowAwaitingUserCard.tsx:28-32` — `detail` falls back to `loopState.gateReason` unconditionally, with no distinction between "genuine hub reasoning" and "CA-226 diagnostic fallback text".

## 1. Issue Summary

Two independent, pre-existing defects in the flow-engine review loop, surfaced by live testing of BUG-231's new awaiting-user UI:

1. The step timeline can show a stale "all steps done" snapshot immediately after a new review round starts (post-Continue or mid-loop), while a reviewer child agent is still actually running — reading as a false completion.
2. The awaiting-user card can display an internal engine diagnostic sentence instead of anything resembling the reviewers' findings, when the loop blocks via the CA-226 no-outcome-call fallback path rather than a genuine `submit_review_outcome(blocked)` call.

## 2. Parent Links

Surfaced during live validation of [BUG-231](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md); the diagnostic text bug #4 originates in [CA-226](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md)'s fallback escalation.

## 3. Environment and Reproduction

- Built-in "Review Loop" workflow, flow-engine-driven run, 2 reviewers (mixed verdicts: one approve, one changes-requested).
- Bug #1: reproduced once during live testing right after pressing Continue on a blocked loop, which spawned a fresh reviewer cohort for another round — the step-timeline panel briefly showed every step (including the still-running reviewer's) as done.
- Bug #4: reproduced once during live testing where the hub's synthesis turn completed without calling `submit_review_outcome`, triggering the CA-226 fallback; the awaiting-user card showed the fallback's diagnostic `Summary` text.
- Neither has an automated repro yet (both were observed via manual UI interaction, not captured in a test).

## 4. Expected vs Actual

- Bug #1 — Expected: the step timeline reflects the true in-flight state (a running reviewer shows as running) at all times, including immediately after a new round starts. Actual: a stale "all done" snapshot can render transiently (or longer) while a reviewer is genuinely still running.
- Bug #4 — Expected: when asked to make a call on a blocked loop, the user sees content that helps them decide (what the reviewers actually found). Actual: the user can see an internal engine sentence about the hub's tool-calling behavior, which carries no decision-relevant information.

## 5. Impact

- Bug #1: confusing/misleading status display; could cause a user to believe a run finished and navigate away while a reviewer is still consuming provider budget.
- Bug #4: undermines the very purpose of BUG-231's Continue/Stop card — the user is asked to decide but shown no basis for the decision.

## 6. Root Cause

- **Bug #1**: `refreshWorkflowStepRuntime` (drives the step-timeline panel) and `refreshAgentRuns`/the `agent_graph_updated` stream event (drives the agent panel's running/done badges) are two independently-triggered, unordered fetch/update paths in the desktop store. There is no guarantee that a step-runtime refresh reflects the same point-in-time state as the latest agent-graph snapshot; a step-runtime refresh that lands right after a round transitions can read a snapshot where the new round's steps haven't been re-seeded to `PENDING`/`RUNNING` yet, while the previous round's steps are already `DONE`. Compounding factor: `markFlowRunComplete` (the "done" settlement path) unconditionally marks every step `DONE` and never inspects live child-agent status — not itself the trigger here (this run didn't reach `done`), but confirms the general pattern that step-runtime settlement and live agent status are not cross-checked anywhere in the codebase.
- **Bug #4**: `applyFlowControl`'s CA-226 fallback constructs `Summary` as a hard-coded diagnostic sentence with the hub's raw final message appended, and this `Summary` is what ends up in `AgentLoopState.GateReason`. `FlowAwaitingUserCard` treats `gateReason` as generically displayable, with no differentiation between a genuine hub-authored blocked reason (e.g. from `submit_review_outcome(status="blocked", reason="...")`) and this fallback's engine-internal wording. There is also no structured retention of "what did the reviewers actually say" once their cohort results have been joined and consumed by the hub's turn — the joined text lives transiently in `pendingAgentContext`/the auto-reinvoke note, not in anything the card can read back out.

## 7. Fix Strategy

Not yet implemented. Candidate directions to evaluate before coding:

- Bug #1 — either (a) have the step-runtime refresh explicitly wait on / be triggered by the same event that reseeds the new round's step rows (making one channel authoritative for round transitions), or (b) have the desktop reconcile the two snapshots before rendering (if the agent panel shows a child `running` for a node the step-timeline shows `DONE`, prefer `running`).
- Bug #4 — capture the joined cohort/reviewer text into a structured field at cohort-join time (e.g. a `LastReviewSummary` on `AgentLoopState` or similar) so the fallback (and the genuine escalate path) can populate the card with an actual concise findings summary instead of (or in addition to) the raw diagnostic sentence; keep the diagnostic text for logs/telemetry only, not user-facing `GateReason`.

## 8. Validation

- Not yet executed — no fix implemented.

## 9. Regression Guard

- Bug #1: add a test that starts a second review round (post-Continue) with a reviewer cohort still in-flight and asserts the step-runtime snapshot for that reviewer's node is not `DONE` while the child agent's run status is `running`.
- Bug #4: add a test that drives the CA-226 no-outcome fallback path and asserts the resulting `GateReason`/card content is the reviewers' findings summary, not the internal diagnostic sentence.

## 10. Follow-Up Document Updates

- None yet — pending fix design and implementation.

## 11. Definition of Done

- `[ ]` **DOD-1 — Bug #1 fix.** Step timeline no longer shows a stale "done" snapshot for a node whose child agent is still actually running, verified across at least one live multi-round Continue flow.
- `[ ]` **DOD-2 — Bug #1 regression test.** Automated Go and/or desktop test reproducing the stale-snapshot window and asserting it no longer occurs.
- `[ ]` **DOD-3 — Bug #4 fix.** Blocked/awaiting-user card shows a concise, reviewer-findings-derived summary when blocked via the CA-226 no-outcome fallback, not the raw internal diagnostic sentence.
- `[ ]` **DOD-4 — Bug #4 regression test.** Automated test driving the CA-226 fallback path and asserting the card-facing content is findings-based.
- `[ ]` **DOD-5 — Full regression green.** `go build`/`go vet`/`go test ./internal/runner/...` and desktop `typecheck`/`build`/unit tests at the established baseline, no new failures.
- `[ ]` **DOD-DOC — Docs.** This doc moved to `done/` with a Validation section; a `change-audit/CA-*` note written for the shipped fix.

## 12. Code Change Plan (per DoD item)

Not yet drafted — pending resolution of `Q-1` and `Q-2` above (which fix direction to take for each bug). To be filled in once a live-log repro for bug #1 and a decision on the findings-capture mechanism for bug #4 are in hand.
