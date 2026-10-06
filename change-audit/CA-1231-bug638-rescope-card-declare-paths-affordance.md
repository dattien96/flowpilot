# CA-1231 — desktop: BUG-638 awaiting-user card offers a declare-paths amend field on non-drift parks

- **Area**: apps/desktop-flowpilot/src/components (flowAwaitingUserDrift.ts,
  FlowAwaitingUserCard.tsx)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-044), 2026-10-06 —
  a RESCOPE debate verdict rendered "DECISION REQUESTED
  (rescope/contract amend): declare CMakeLists.txt in scope" with only
  `Stop` / `Retry — run again with old scope`. The escalation's one
  asked-for action was unreachable: `awaitingUserDriftState` derives
  `isDrift` solely from the drift marker string, so the Allow button
  (`isDrift`-gated) never rendered for prose verdicts.
- **Fix**: `awaitingUserDriftState` now returns `canAmend` — true for
  every parked decision except owner-actioned parks (member_stalled
  keeps Retry/Skip; vibe_sprint_boundary keeps Continue-to-next-task).
  The card renders an "Amend scope…" disclosure → free-form
  comma-separated path input + Amend button calling `amendFlow(paths)`,
  shown only when `canAmend && !isDrift && !isVibeLock` (drift parks keep
  the marker-driven Allow button; vibe_lock is pre-contract). The server
  validates each path and reports unamendable entries, so offering the
  field on a generic escalate fails honestly, never silently.
- **Stacking note**: requires CA-1230 (BUG-637) — the amend endpoint now
  reaches the run's active contracts while parked inside a nested
  sub-flow; before that fix this button would 404 mid-debate.
- **Tests (additive)**: `flowAwaitingUserDrift.test.ts` —
  `canAmend exposes the declare-paths affordance on amendable parks`
  asserts: RESCOPE prose → canAmend=true/isDrift=false; drift marker →
  canAmend=true; member_stalled/vibe_sprint_boundary → canAmend=false.
- **Verification**: `tsc --noEmit` clean; phase1 node:test suite for the
  file green (7/7). Provider-agnostic — presentation-layer change only.
- **Worktree**: n/a.

## Review hardening (post-review pass)

- **Reviewer M-2**: the Amend submit swallowed rejections — a 404
  `no_frozen_contract` or 422 `amend_failed` left the field open with no
  feedback and an unhandled rejection. The card now keeps the input open
  and renders the server error under it (`role=alert`). The
  `unamendablePaths` response field is decoded on `AgentGraphSnapshot`
  for completeness.
