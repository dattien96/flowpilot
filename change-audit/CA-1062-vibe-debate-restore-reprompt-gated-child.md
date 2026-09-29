# CA-1062 — vibe owner-debate restore reprompts the gated child

## Symptom (live run-136 / run-9597)

A sprint node child completed, its post-turn gate routed into
`vibe-owner-debate`, the debate resolved — and then nothing resumed the
interrupted sprint chain. `restoreVibeFlowAfterDebate` put the parked
topology back, but the gate had diverted the child's completion *before*
`tryAdvanceFlowFromNode` could fire its done-edge, so no join reinvoke was
ever armed. The loop sat `running` with nothing in flight until
`hub_stalled` parked it; every Continue re-entered remediation instead of
resuming the sprint, leaving `coder`/`validate`/`audit` steps PENDING
forever (audit stayed `blocked_validation_failed` even though code existed
in the workspace).

## Root cause

Restore swapped the graph back but armed no continuation for the specific
interrupted node. A generic hub reinvoke is also wrong-positioned:
`activeHubNodeID` is empty post-debate, so a hub `done` would resolve
against the flow's `hub.inline` node (synthesis), skipping the gated node
entirely.

## Fix

- `stashVibeFlowForDebate` now also records the gated child run id in the
  durable `vibe_parked_gated_run_ids` field (plumbed through
  `ProviderSessionState`, the ndjson session record, `sessionStateOf`,
  resume rehydrate, and the pending-run residue clear). Recorded even when
  the topology is already parked (second gate fire mid-debate).
- `startVibeOwnerDebate` takes the gated run id; both resolvers
  (`applyVibeGateResolver`, `applyVibeDriftOnlyResolver`) pass it.
- `restoreVibeFlowAfterDebate` arms the standard gate-reprompt intent
  (`pendingGateReprompt*` + gen bump) on each recorded gated child with the
  debate verdict rows carried inline (the child session never saw the
  debate — it ran on the parent hub), persists both snapshots, then
  dispatches via the existing `startTurnClearingIntent` durable-intent path.
  The child's re-completion re-runs the post-turn gate and fires the
  interrupted node's done-edge through the normal
  `advanceOrNotifyHub`/`tryAdvanceFlowFromNode` completion path — bounded by
  the existing reprompt/escalation caps.
- Fallback preserved: when no gated child was recorded (drift-only debate on
  the hub, or the child row is closed/cancelled/gone), restore still arms
  `pendingHubReinvoke` with the resume prompt so the loop cannot stall.

## Tests

- `TestVibeDebateRestoreRepromptsGatedChild` (new): gated child gets the
  reprompt intent with the verdict inline; hub reinvoke is not armed on top.
- `TestVibeDebateRestoreArmsHubContinuation` (new): no gated child → hub
  reinvoke fallback arms.
- `TestVibeDebateRestoreNoParkedGraphNoArm` (new): no parked graph → no-op.
- CA-792 restore/precedence tests still green (signature updated only).

## Verification

- `go test -count=1 -run 'VibeDebate|CA792|VibeGatePrecedence' ./internal/runner/` — ok.
- Wider runner suite: 4 failures, all verified pre-existing/environment
  (`TestBug425*`/`TestBug514*` reproduce on clean HEAD worktree;
  `TestStartSession*` need codex/gemini binaries absent from PATH).
- Durability: the armed reprompt rides the same `pendingGateReprompt*`
  fields already drained by `reconstructPendingChildSessions`/idle-flush,
  so a restart between restore and dispatch still delivers it.
