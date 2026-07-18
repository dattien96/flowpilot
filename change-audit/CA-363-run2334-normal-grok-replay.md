# CA-363 — run-2334 normal Grok restart replay

## Summary

Fixed normal-chat replay for `run-2334` after a runner restart:

- Grok transcript prompts are now replaced with the durable raw user prompts
  before replay, so provider-injected MCP / `ask_user` reinforcement and
  system gate reprompts do not render as user messages or create duplicates.
- The raw durable turn id is retained on replay events. Resolved approval and
  question sidecar records are inserted directly after the prompt of their
  owning normal-chat turn instead of accumulating at the end of the timeline
  when the provider transcript has no timestamps.
- The same durable turn-id overlay now applies to Codex and Claude replay, so
  all supported normal-chat providers preserve card placement under the same
  no-timestamp restart condition.

## Verification

- Added `TestRun2334NormalGrokRestartReplayUsesRawPromptsAndTurnAnchors` in a
  new test file. It covers composed Grok prompts, an internal gate reprompt,
  and durable approval/question sidecars without provider timestamps.
- Existing Grok restart and flow internal-prompt replay guards remain green.
- Added Codex and Claude restart regressions with timestamp-free transcript
  fixtures, proving the shared replay path restores raw prompts and anchors
  approval/question cards in their owning turns.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Restore normal chat prompts and anchor durable approval/question cards within their originating turn after restart for Grok, Codex, and Claude.
# --->8---
