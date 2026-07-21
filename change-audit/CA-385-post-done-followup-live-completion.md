# CA-385 — Post-"done" follow-up turn broadcasts live completion (no gate hang)

## Summary

Fixed BUG-305, found live while verifying CP-51 A10 (the second-turn-on-same-flow
scenario that BUG-302 unblocked). After a Review Loop finished, a follow-up chat
turn completed on the backend but the desktop UI stayed stuck (thinking row, Stop
button, history spinner) until the chat was reopened.

Root cause: `flowEngineDriven` persists across turns, so `emitLocked` deferred the
follow-up's `TurnCompleted` behind the post-turn flow gate; that gate
(`runFlowGateAtEpoch`) force-returns block for a "done" loop via `gateEpochStillValid`,
so the real `TurnCompleted` was never broadcast to live subscribers. The desktop's
turn stream (`sendTurn`) only ends on `TurnCompleted` for its turn id, so it hung.
A separate settle driver (allowing on `flowLoopDone`) still finalized the durable
record, so reopening the chat showed the correct "done" state.

Fix (3 tightly-scoped, same-condition changes): capture a per-turn flag
`turnStartedAfterLoopDone` at `startTurn` admission; in `emitLocked` skip the
deferral + route to the plain-chat completion branch when set; in `runTurn` skip
the gate evaluation for a root run when the loop was already done at turn start
(keeping the pass-path bookkeeping). Net effect: a follow-up on a finished flow
completes exactly like a normal chat turn.

## Cross-provider parity

Classification: **Case 1, provider-agnostic** — confirmed by reading and grep:
`emitLocked`, `gateEpochStillValid`, `runFlowGateAtEpoch`, and the `startTurn`
admission gate take no `providerKey` and never branch on one (every `provider`
reference in those paths is event/log metadata). The new tests use one
representative provider (Codex), matching the sibling BUG-302 test file.

## additive-tests-only compliance

Only a new test file was added
(`bug305_chat_followup_after_flow_done_stale_ui_test.go`); no existing test file
was modified.

## Verification

- git-stash: with the production fix stashed, `TestChatFollowUpAfterFlowDoneBroadcastsLiveCompletion`
  fails at the live-broadcast assertion ("TurnCompleted was never broadcast live");
  with the fix it passes. The test sets a non-empty `workspaceCwd` so the gate
  reaches the `gateEpochStillValid` block that reproduces the bug (an empty cwd
  makes the gate pass immediately and hides it).
- `TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState` proves the carve-out
  flag is false while the loop is running at admission (normal flow turns stay
  deferred + gated) and true only when it is already "done".
- Full-suite `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1`:
  failure set does not grow beyond the known pre-existing environment-only set,
  plus two pre-existing parallel-load flaky flow tests
  (`TestAdvanceHubDoneThroughEdgeDispatchesHubNotify`,
  `TestRun20332FlowHubHistoryParityForEveryProvider`) that pass in isolation and
  flake identically on the unfixed code (confirmed via git-stash).
- `go build ./...` passes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-305
change_type: bugfix
summary: A follow-up chat turn admitted onto an already-"done" flow loop (per BUG-302) no longer defers its TurnCompleted behind a post-turn gate that force-blocks a "done" loop; it now publishes and broadcasts completion live like plain chat, so the desktop turn stream ends instead of hanging (thinking row / Stop button / history spinner stuck until reopen).
# --->8---
