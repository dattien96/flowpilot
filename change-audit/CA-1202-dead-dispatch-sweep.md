# CA-1202 — Wedge sweep re-drives dead-dispatched RUNNING steps (live run-183756)

## Evidence
run-183756 ledger fingerprints `node-running-no-turn-dispatch|coder`
(critical) and `hub-no-turn-after-gate-resolve|synthesis` (x2): a flow step
stamped RUNNING whose dispatch never materialized — coder after a batch
reset had >5min with zero ACP traffic and no turn row; the synthesis hub
reinvoke was consumed (flags cleared) with no turn either. Every existing
heal early-returned: the armed-settle class needs `pendingFlowGateSettle`,
the orphan-wait class needs a waiting_* run status, and the debate trigger
wedge only fires inside a mounted overlay. The durable ledger claimed
running; nothing executed.

## Root cause
`RUNNING` is stamped at activation but the dispatch itself is asynchronous —
a lost spawn/schedule leaves the row RUNNING forever with no driver. No sweep
class scanned for "aged RUNNING step + no live work + no driver".

## Fix
`sweepWedgedFlowWork` gains a fifth class: for each live flowEngineDriven
parent run whose run-level drivers are all quiet (no turnInFlight, armed
settle, pending/in-flight reinvoke, mounted-debate mount, or live gate
eval), `maybeRedriveDeadDispatchedSteps` scans active topology nodes. A step
qualifies only when it is RUNNING, its StartedAt stamp is older than
`flowStepDeadDispatchBound` (45s — mirrors vibeDebateTriggerWedgeBound), no
non-terminal child carries the label (waiting_user_approval legs count — a
carded park owns its own resume), and no persisted Completed leg exists
(that shape is the BUG-1197 settle gap, not a dead dispatch).

Re-drive by behavior:
- `hub.inline` → `maybeAutoReinvokeHubWithPrompt` (single-flight, re-arms
  pending when blocked) — covers the synthesis hub-no-turn shape.
- delegate nodes (`flowNodeAgentName` non-empty) →
  `reinvokeMatchingFlowChild` first, then a fresh `spawnChildRun` labelled
  node.ID — same contract as the failed-delegate Continue respawn; the
  BUG-1194 cohort rebind inside spawnChildRun covers open cohort seats.
- other behaviors (contract.freeze, command.validate, …) → diag-logged only;
  inline nodes execute synchronously so their RUNNING leak is a different
  failure class.

## Tests
- `ca1202_dead_dispatch_sweep_test.go`: aged RUNNING delegate respawns a
  fresh leg; aged RUNNING hub node re-arms/reinvokes; fresh RUNNING stamp,
  waiting_user_approval leg, and persisted-Completed leg all stay untouched.
- Wedge/stall/debate/settle/cohort/live-039 regression suite green.
