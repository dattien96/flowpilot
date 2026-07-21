# CA-366 - run-1264 agent replay causal order

## Summary

Fixed the replay ordering defect visible in `run-1264`: a consolidated-result
message carried the run-created fallback timestamp even though its durable
sequence was after the three agent-spawn events. The timeline therefore placed
the agent cards below the result.

`orderHistoryReplayEvents` now treats stream sequence as a causal constraint.
When an event persisted after a known agent spawn has an earlier timestamp, its
replay time is clamped to that spawn. The normal durable-timestamp ordering and
sequence tie-breaker remain unchanged. This is shared renderer behavior; it
does not branch on provider.

Added `durable-replay-contracts`, a focused coverage skill for restart/resume
and timeline changes. It requires a durable transcript, event-order,
interactive-state, agent-lifecycle, terminal-state, and Codex/Claude/Grok
matrix. `additive-tests-only` remains the guard against changing legacy tests;
this new contract specifies what new coverage a replay change must add.

## Verification

- `npm --prefix apps/desktop-flowpilot run build` passed, including TypeScript
  compilation of the new three-provider replay-order regression test.
- `go test ./internal/runner -run "TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider" -count=1 -v` passed for Codex, Claude, and Grok.
- `npm --prefix apps/desktop-flowpilot run test:phase1` remains blocked before
  test execution by legacy fixture/interface drift in `tests/phase1`:
  `adminLogic`, `desktopSupabaseAuthRepository`, `navigatorCatalog`,
  `settingsHelpers`, and `workflowFlowEngineAttrs`. Those tests and fixtures
  were not edited.

## Provider Matrix

| Provider | Shared replay ordering contract | Restart transcript regression |
|---|---|---|
| Codex | covered by provider-neutral sorter test | passed |
| Claude | covered by provider-neutral sorter test | passed |
| Grok | covered by provider-neutral sorter test | passed |

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-51
change_type: bugfix
summary: Preserve causal agent-card placement when replay timestamps fall back to run creation time.
# --->8---
