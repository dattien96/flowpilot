---
id: CA-501
feature_key: chat-history
title: Filter shared-session pollution when flow hub has no assistants yet
date: 2026-08-14
status: COMPLETE
---

## Change

run-98153 /open mixed foreign chats + child reviews into the hub transcript.
`preferFlowHubTurnLogTranscript` only skips provider history when the hub turn
log already has assistant frames (run-24377). Mid-flow hubs (prompt only) fall
through to `chat_history.jsonl` (run-12613). With a shared Grok session,
`overlayRawTurnPrompts` can map the durable hub prompt onto a foreign user slot
and keep the following assistant.

Fix (provider-agnostic, Grok + Claude/Codex seed paths):
- When a flow hub falls through (`shouldFilterFlowHubProviderHistory`), run
  `filterFlowHubUnassistedProviderHistory` **before** overlay/prepend.
- Assistant-only provider dumps kept (run-12613).
- Interleaved foreign/child users dropped; only frames matching durable user
  prompts (and assistants until the next user) remain.
- Prefer turn-log path with assistants unchanged (run-24377).

Does not undo CA-500 cohort join, CA-24377 prefer gate, or run-12613 fall-through.

## Provider impact

Case 1 (agnostic). Filter takes no `providerKey`. Wired into Grok and
Claude/Codex seed after load. New tests + old 12613/24377/prefer gates.

## Tests

New `run98153_hub_unassisted_shared_session_test.go` only. Old suite untouched.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-51
change_type: bugfix
summary: Flow hub without assistants still drops shared-session sibling/foreign transcript on resume
# --->8---
