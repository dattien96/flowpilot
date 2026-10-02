# CA-1128 — BUG-575: reopening a vibe/flow chat from History drops back to Chat Mode

## Why

Live run-100368: clicking the run's row in HISTORY opened the chat in
normal-chat mode — no step sidebar, no orchestration board — even though the
run was an armed flow run. `openHistoryRun` mapped `runKind === "chat"`
straight to `chatMode = "normal_chat"`, ignoring `flowArm === "started"`,
which is the real signal that a run entered flow-driven mode (a vibe/forward
launch keeps `runKind: "chat"` on the row).

## What changed

`apps/desktop-flowpilot/src/state/store.ts` (`openHistoryRun`):

- `flowArmStarted = (handle.flowArm ?? historyItem?.flowArm) === "started"`
  is computed first; a row counts as workflow-driven when
  `runKind !== "chat"` OR `flowArmStarted`. With no history row, the run
  handle's own `runKind`/`flowArm` decides. Rows with no flow arm still open
  as Chat Mode.

## Tests

- `src/state/store.bug575-history-flow-mode.test.ts` — three cases:
  `runKind=chat + flowArm=started` restores Flow Mode; handle-only
  `flowArm=started` restores Flow Mode; no `flowArm` keeps Chat Mode.
