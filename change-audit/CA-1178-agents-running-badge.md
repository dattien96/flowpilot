# CA-1178 — "N agents running" badge must count only executing children

## Evidence

Live run-183756 (PrivateVault Task-038 vibe sprint): the desktop header showed
"5 agents running" while the agent graph held exactly one executing agent —
three children were parked at provider approval prompts (`waiting_approval`)
or question cards (`waiting_question`) that had no UI surface and could never
be answered, and two more were zombie `running` records (BUG-1177 settle
loss). The badge made a stalled run look healthy and hid the real state.

## Root cause

`runningAgentCount` in both `RunStatus.tsx` and `AgentsPanel.tsx` counted
`status === "running" | "waiting_approval" | "waiting_question"` as running.
`waiting_*` statuses are parked surfaces — the child holds a seat but no
provider turn is in flight. Counting them inflated the badge and, for
provider-side approvals with no card, presented permanently-blocked children
as live work.

## Fix

Extracted `isExecutingAgentRun(run)` (AgentsPanel.tsx): true only for
`status === "running"` AND `agentStatus !== "waiting_user_approval"` (a stale
summary can carry `status=running` with a parked `agentStatus`). Both badge
sites filter on it.

Deliberately unchanged: `hasBlockingChild` (AgentsPanel) keeps its wider
predicate — a parked wait=true child still holds the blocking seat and may
resume on approval, so spawn gating must still see it (BUG-133 contract).

## Tests

`AgentsPanel.test.ts` — `isExecutingAgentRun counts only genuinely executing
runs` covers running / waiting_approval / waiting_question /
waiting_user_approval / stale (running + parked agentStatus) / terminal.

## Deferred findings from the same live-turn sweep (documented, no code change)

- `premature_hub_done_advance_missing_cohort`: verified covered — the
  missing-verdict gate defers and (post BUG-1176) escalates zombie members
  for redrive; the audit-defer (`hasRunningSprintStep`) converges once the
  redriven member settles.
- `dispatch_deadend_running_no_turn` at the sprint boundary: the observed
  wedge was the missing-verdict defer, not a lost node activation — the
  boundary chain (`takeNext` → `mutateLoop` → `startTakenVibeSprint`) already
  verifies a new non-cancelled child spawn and rolls the index back on
  failure.
- `agent_truncated_output_no_verdict`: reprompt machinery exists
  (`verdictRepromptCount` gated reprompt); observed once live, machinery
  fired correctly.
- `agent-loop/continue` HTTP hang shape: observed once — handler blocked on a
  non-`s.mu` path while sibling GETs still served; not reproducible
  post-mortem with the runner down. Watcher records the hang signature
  (`hang_no_progress`) for the next occurrence.
