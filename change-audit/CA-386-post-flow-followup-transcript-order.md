# CA-386 — Post-flow follow-up transcript order preserved on restart

## Summary

Fixed BUG-306, found live on run-18371 (Workflow mode) while verifying CP-51 A10.
After a flow finished and follow-up turns were sent, restarting the server
produced a scrambled restored transcript: follow-up prompts floated to the top
and the original "fix bug 1+1 != 2" prompt sank to the bottom.

Root cause (two compounding issues in the shared `overlayRawTurnPrompts`
reconstruction): (1) the first post-flow follow-up is sent to the provider
wrapped in `composeAgentContextBlock`'s `"[FlowPilot system note — sub-agents …]"`
prefix (BUG-122), so the persisted user turn looked like a pure system prompt and
the overlay skipped it; (2) the overlay matched raw prompts to historical slots
front-to-back, but a flow hub's turn-1 is suppressed (no provider frame, BUG-300),
so the positional match was off by one. Together they overlaid prompt-1 onto the
last follow-up's slot and prepended the follow-ups.

Fix (all local to the overlay; `isSystemPrompt` untouched): add
`stripAgentContextBlock` + `isOverlayableUserTurn` so a wrapped genuine user turn
is overlay-able while a wrapped join note stays internal (BUG-300/run1264), and
align the overlay from the end via `promptIndex = len(rawPrompts) − overlayableSlots`
so suppressed leading prompts fall through to `prependMissingPromptOnlyEvents`.
Shared `agentContextBlockOpen/Close` constants prevent the strip logic from
drifting from the wrapper text.

## Cross-provider parity

Classification: **Case 1/2, shared with per-provider entry.** `overlayRawTurnPrompts`
is the shared overlay for Claude and Codex; Grok's `overlayRawGrokTurnPrompts`
delegates directly to it, so one fix covers all three. Verified by the existing
cross-provider suites `TestRun20332FlowHubHistoryParityForEveryProvider` and
`TestRun2334…AnchorsSidecarsByDurableTurnID` (both parameterized over
Codex/Claude/Grok) passing with the fix.

## additive-tests-only compliance

Only a new test file was added
(`bug306_post_flow_followup_transcript_order_test.go`); no existing test file was
modified. `isSystemPrompt`/`isFlowEnginePrompt` were deliberately left unchanged
so BUG-300 and run1264 (which keep an agent-context-wrapped join note internal)
stay green.

## Verification

- git-stash: with the production fix stashed, BOTH integration tests (Claude
  `TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestart` and Grok `…Grok`) fail
  with `prompt order[0] = "hello …"` — the scramble reproduced on the shared
  Claude/Codex seed path AND Grok's separate seedGrokTranscriptFromDisk; with the
  fix both pass. `TestIsOverlayableUserTurn` unit-pins the discriminator so the
  BUG-300 collision (wrapped join note) cannot be reintroduced.
- Prior transcript-order battery all green with the fix (35 targeted tests):
  `TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne` (BUG-300),
  `TestRun1264Restore*` (Claude + Grok), `TestSeedTranscriptUsesRawPromptFromTurnLog`
  (BUG-083), `TestSeedTranscriptFromDiskMovesSidecarPrefix…`, run2334/run20332
  cross-provider. Every existing scenario resolves to offset 0 with inert wrapper
  logic — byte-identical behavior.
- Full-suite `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1`:
  failure set does not grow beyond the known environment-only set plus known
  parallel-load flakes.
- `go build ./...` and `gofmt` clean.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-306
change_type: bugfix
summary: Restart transcript reconstruction keeps post-flow follow-up prompts in order (original prompt first) by treating an agent-context-note-wrapped user turn as overlay-able and offsetting the overlay for a suppressed hub turn-1, instead of skipping the wrapped slot and mis-aligning positionally so the first prompt sank to the bottom.
# --->8---
