# BUG-177: Sub-Agent Approvals Not Surfaced On The Main/Hub View

## Metadata

- Document ID: `BUG-177`
- Title: `Sub-Agent Approvals Not Surfaced On The Main/Hub View`
- Phase: `bugfix`
- Status: `cancelled`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`

> **REVERTED (user decision).** Live testing showed the hub-stream mirror flooded the main view with unresolved sub-agent approvals ("2/3/14 approvals required" piling up) and contributed to the main run appearing to hang. Per user direction the mirror is reverted: sub-agent approvals stay in the agent (child) view — the user focuses that agent to approve, or runs YOLO for a hands-off UX. Both code halves (runner `RequestApproval` mirror, desktop `timelineReducer` dedup) were removed; see `change-audit/CA-216-revert-subagent-approval-mirror.md`. The root-cause analysis below is retained for history.
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-157-Multiple-Concurrent-Approvals.md`, `requirements/09-BugFix/done/BUG-176-Cohort-Reviewer-Can-Terminate-Flow-Before-Join-Barrier.md`, `requirements/09-BugFix/done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md`
- Replaces: `none`
- Tags: `agent-flow-engine, runner, ui, desktop, flow-mode, approvals, review-loop`

## AI Quick View

### Summary

- In Flow Mode with YOLO off, a single coder sub-agent's approval showed on the main/hub view and could be approved there, but when the two-reviewer cohort each requested approval at once, neither surfaced on main — leaving the user with nothing to approve.
- Root cause is two-sided: (runner) a sub-agent's `permission_required` was emitted only on that child's own event stream, never forwarded to the hub; (desktop) `pendingApprovals` is fed only by the *currently focused* run's stream. So a child's approval surfaced only if that child happened to be focused. One coder is usually focused (works); two simultaneous reviewers cannot both be focused (neither surfaces).
- Fix: the runner mirrors a child's approval onto the hub run's stream (which the desktop's always-on orchestration stream already ingests via `applyEvent`), and the desktop dedups approvals by `approvalId` so a concurrently-focused child plus the mirror never double-surface.

### Current Ask

- Every sub-agent approval must be visible and actionable from the main/hub view, including multiple concurrent ones from a cohort.

### Key Decisions

- `V-1` A child's `permission_required` is mirrored onto its parent/hub run's event stream (`turnBridge.RequestApproval`); approvals resolve globally by `approvalId`, so approving from the hub drives the child's own record.
- `V-2` The desktop `permission_required` reducer is idempotent by `approvalId`, so the mirror + a focused-child stream can't create duplicate cards / `pendingApprovals` entries.

### Constraints

- Runner change is additive (an extra emit on the parent stream); the child's own stream and approval-resolution path are unchanged. Desktop change is a guard that also hardens replay-from-seq-0.
- The desktop's always-on orchestration stream already routes non-graph/bus events through `applyEvent` (store.ts:2003), so no new subscription was needed — only the mirror + dedup.

### Open Questions

- `user_question_required` (`ask_user`) from a sub-agent has the identical single-stream gap and is NOT addressed here (scoped to the reported approvals case). The same mirror+dedup pattern would extend to it.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go:1695-1726` (`turnBridge.RequestApproval` — emits on the child only, pre-fix)
- `apps/local-runner/internal/runner/interactive_service.go:1598-1605` (`emitLocked` broadcasts to `rs.subs` only; no parent fan-out for `permission_required`)
- `apps/desktop-flowpilot/src/state/store.ts:1976-2006` (`consumeOrchestrationStream` — applies non-graph/bus events via `applyEvent`)
- `apps/desktop-flowpilot/src/state/store.ts:547-599` (`focusAgentRun` — only a focused child's stream is consumed by `consumeAgentStream`)
- `apps/desktop-flowpilot/src/state/timelineReducer.ts:281` (`permission_required` reducer → `pendingApprovals`)

## 1. Issue Summary

Sub-agent approval requests only reached the main/hub approval UI when that specific sub-agent was the focused run. A single coder was usually focused, so its approval worked; two reviewers requesting approval simultaneously could not both be focused, so neither (or at most one) appeared on the main view and the user had nothing to click.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local runner; a Flow-Mode review-loop with a two-reviewer cohort, YOLO off.
- reproduction steps:
  1. Run a review-loop until the reviewer cohort spawns.
  2. Have both reviewers hit a gated (approval-required) operation at once.
  3. Observe the main/hub chat shows no approval card, so the flow stalls with nothing to approve — while a single coder approval earlier did surface.
- frequency: deterministic for ≥2 concurrent sub-agent approvals; a single focused sub-agent's approval surfaced.

## 4. Expected vs Actual

- expected: all sub-agent approvals appear on the main/hub view and can be approved there.
- actual: only the focused sub-agent's approval surfaced; concurrent cohort approvals did not.

## 5. Impact

- users affected: anyone running multi-agent flows with approvals (Review Loop and similar), YOLO off.
- workflows affected: the flow stalls — the user cannot approve the gated operations the reviewers need.
- severity: high — a hard stall with no visible action on the primary view.

## 6. Root Cause

- runner: `turnBridge.RequestApproval` (`interactive_service.go:1695-1726`) emits `permission_required` via `emitLocked(b.rs, …)` on the child's own run; `emitLocked` broadcasts only to that run's subscribers (`:1598-1605`). Only an `agent_graph_updated` snapshot is forwarded to the parent (child shown as `waiting_approval`); the approval request itself never reaches the hub stream. Cohort logic aggregates only *terminal* results, not approvals.
- desktop: `pendingApprovals` is populated by `applyEvent`/`applyTimelineEvent`, driven by `consumeAgentStream` for the focused `runId` only (`store.ts:1947-1974`); the hub's orchestration stream applies non-graph/bus events via `applyEvent` but only sees the hub run's own stream. A child's stream is consumed only when the user manually focuses it (`focusAgentRun`, `store.ts:547-599`) — and only one run can be focused at a time.

## 7. Fix Strategy

- `F-1` (runner) In `turnBridge.RequestApproval`, when `b.rs.parentRunID != ""`, also emit the `permission_required` onto the parent/hub run's stream (same `approvalId`/details). The hub's always-on orchestration stream on the desktop already ingests it via `applyEvent`. Resolution stays global by `approvalId` (`s.approvals`), so approving from the hub drives the child's own record (`interactive_service.go`).
- `F-2` (desktop) Make the `permission_required` timeline reducer idempotent by `approvalId`: skip if an approval with that id is already pending or already in the timeline, so the mirror + a concurrently-focused child stream don't double-surface (`timelineReducer.ts`).

## 8. Validation

- `V-1` **(done)** `TestChildApprovalMirroredToHubStream` (`flow_step_runtime_test.go`): a spawned child's approval appears on the hub run's own event stream with the same `approvalId`. Passes.
- `V-2` **(done)** No runner regression: `go test ./internal/runner/ -run 'Approval|Flow|Workflow|Orchestrat|Cohort|Coder|Reviewer|Spawn|Child|Progress|StepRuntime|Advance|Resolve|Hub|Review|Loop'` → 389 passed; the only failures are the pre-existing codex-resume tests that need a real codex binary (confirmed environment-specific, unrelated).
- `V-3` **(done)** `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-4` **(not executed)** The desktop unit suite (`vitest`) cannot run in this environment (pre-existing `require is not defined in ES module scope` config error that prevents the test files from loading at all — reproduces on unrelated suites too). The dedup guard is covered by typecheck + reasoning; a live multi-reviewer approval on the main view should be confirmed by the user.

## 9. Regression Guard

- tests: `TestChildApprovalMirroredToHubStream` (runner) guards the mirror; existing approval-gate tests (`TestSpawnChildRunWaitTrueWaitsThroughApprovalGate`) and BUG-157 multi-approval behavior guard the surrounding paths.
- alerts: none.
- audit checks: recorded in `change-audit/CA-214-mirror-subagent-approvals-to-hub-stream.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the identical single-stream gap for `user_question_required` (sub-agent `ask_user`) is intentionally left for a follow-up (see Open Questions) — same mirror+dedup pattern applies.
