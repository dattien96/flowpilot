---
id: CA-916
title: Devin ACP provider fixes (permission correlation, file_changed, history, model, process, replay) + durable reprompt rearm
type: BugFix
feature: ai-providers
date: 2026-09-22
status: done
---

## Context

~120 live CP tests surfaced a cluster of Devin ACP defects (BUG-374..379,
433..436) plus one provider-agnostic durability defect (BUG-377). All Devin
fixes were validated against real wire captures under
`~/fp-beds/lt-evidence/` (cp37/cp46/cp70).

## Change

- `internal/runner/devin_event_mapper.go`
  - `devinToolCallIndex` + `devinPendingToolCall`: per-session toolCallId →
    {title, kind, toolName, rawInput, paths} cache populated from `tool_call`
    start frames (BUG-374/375/434). Devin's `tool_call_update` and
    `session/request_permission` frames carry only `toolCallId` (+ editableCommand
    meta); identity lives only on the start frame.
  - `devinCorrelateToolNotification`: enriches bare `tool_call_update` frames
    with cached title/kind/locations/rawInput/`_meta.cognition.ai/toolName`;
    drops cache entry on terminal status. Mirrors `grokCorrelateToolNotification`.
  - `devinToolCallStatusTerminal` (BUG-436): `in_progress`/`pending` updates no
    longer emit `tool_completed` — only terminal statuses map, unknown statuses
    keep legacy emit (safer than dropping a completion).
  - `devinToolNameFromMeta`: prefers `_meta.cognition.ai/toolName` (concrete MCP
    name, e.g. `mcp__flowpilot__submit_review_outcome`) over
    `inferenceToolName` (can be generic `mcp_call_tool`).
- `internal/runner/devin_adapter.go`
  - `toolCalls` map + `toolCallIndexFor` (per-session, shared by turn goroutine
    and dispatcher inbound goroutine); cleaned up on turn end.
  - `devinApprovalDetailsFromRequest(params, idx)` (BUG-374/434): builds
    ApprovalDetails from correlated `tool_call` metadata; falls back to parsing
    the MCP tool name out of permission option labels
    ("allow calling X on the flowpilot MCP server") when no correlated frame
    exists; reads `cognition.ai/editableCommand` as canonical editable command
    surface; Reason carries the real tool NAME so verdict/ask_user/read-only
    matchers key correctly.
  - Applied-model tracking (BUG-379/433): `appliedModel` map,
    `setAppliedModel`/`appliedModelFor`, `devinConfigOptionCurrentValue` reads
    `configOptions[].currentValue` from session/new|load|set_config_option
    results; `recordAppliedSessionModel` re-upserts the provider-session record
    with the model Devin ACTUALLY applied. Rejected or coerced
    set_config_option calls now emit a user-facing `[model]`/`[mode]` message
    delta instead of failing silently while metadata claims the requested id.
  - `applyDevinSessionConfig` signature gains `bridge` for the warnings above.
- `internal/runner/interactive_service.go`
  - `shouldInjectFeatureHistory` now includes `ProviderKeyDevin` (BUG-376) —
    Devin turns were silently excluded from feature-history prompt context.
  - Child-run teardown calls `CloseDevinProcessesForChildRun(rs.id)` alongside
    the existing Opencode teardown (BUG-378) — Devin ACP child processes no
    longer outlive their child run.
  - Post-turn-gate blocked path: when `pendingFlowGateSettle` was never armed
    (provider emitted no `file_changed`, or a non-code violation), a queued
    durable reprompt is now persisted + dispatched through the same
    `scheduleRootGateRepromptOrPark`/`startTurnClearingIntent` path as the
    armed branch (BUG-377) instead of being dropped with only a single-shot
    tail flush.
- `internal/runner/interactive_resume.go`
  - `durableIntentRearmProbe` (15s) + `rearmDurableIntentIdleCheck` (BUG-377):
    when `claimDurableIntentLocked` fails in `flushDurableTurnIntents` or
    `startTurnClearingIntent`, a bounded one-shot `notifyTurnIdle` is armed for
    min(claimLeaseRemaining, probe). A wedged delivery goroutine can no longer
    strand a durable reprompt until restart — the chain re-checks each probe
    tick and reclaims once the 30-min lease expires or the holder releases.
  - `transcriptAssistantInsertIndex` (BUG-435): a `message_completed`
    recovered from the turn log now lands BEFORE the turn's terminal event —
    previously it inserted after `turn_completed`, producing inverted order
    and a duplicate terminal on tail close.
  - `terminalBroadcastTurns` per-run set + broadcast guards (BUG-438,
    live-found while verifying this CA): for a chat run, `deferGateCompleted`
    is false so the live `turn_completed` already reached subscribers — the
    armed-settle pass branches then materialized and broadcast a second
    identical terminal. Live emitLocked now records the broadcast turnID and
    both post-gate materialization sites (live pass + resumePendingFlowGate)
    skip re-broadcasting it. RAM-only by design — after a restart no live
    broadcast happened in-process, so the resume materialization still emits.

## Tests

- `bug_devin_toolcall_correlation_test.go` — tool_call index remember/lookup/
  forget, update enrichment, permission-request correlation + options fallback
  + editableCommand reconstruction (BUG-374/375/434/436).
- `bug376_devin_feature_history_test.go` — `shouldInjectFeatureHistory`
  includes Devin (BUG-376).
- `bug378_devin_child_process_teardown_test.go` — child-run terminal path
  closes Devin processes (BUG-378).
- `bug379_devin_model_fallback_test.go` — rejected/coerced set_config_option
  surfaces warning + records applied model (BUG-379/433).
- `bug435_replay_terminal_ordering_test.go` — assistant insert lands before
  terminal; no duplicate terminal on replay (BUG-435).
- `bug377_reprompt_strand_test.go` — wedged same-gen claim reclaimed via
  probe; unarmed-settle reprompt still dispatches; expired lease reclaimed on
  next flush (BUG-377).
- `bug438_terminal_dup_test.go` — chat run with file_changed + gate pass emits
  exactly one `turn_completed` to a live subscriber (BUG-438).

## Provider parity

- Devin-specific files (`devin_*`) are provider-isolated by construction.
- `shouldInjectFeatureHistory` change is additive-only (Devin added to an
  existing allow-list already covering codex/claude/gemini/grok).
- BUG-377 rearm + unarmed-settle dispatch is provider-agnostic durable-intent
  plumbing — no `providerKey` branches; benefits all providers identically.
- Child teardown callsite mirrors the existing Opencode pattern.

## Regression

- Focused: `go test -count=1 -run 'TestBug3|TestDevin|TestRun23820|TestBug288|
  Reprompt|DurableIntent|GateSettle|TestRehydrat' ./internal/runner/` → ok 28.7s.
- Full `./internal/runner/` suite: 30 failures — all reproduced on clean HEAD
  (`d191004f` baseline worktree, 21 verified byte-identical) or documented
  pre-existing in BUG-427; 3 TempDir-cleanup flakes pass in isolation on both
  trees (gitnexus auto-index race, pre-existing).

## Live evidence

- cp37 run-1 (Devin `swe-2-max`): queued `pending_gate_reprompt_*` stranded
  with `status: completed` and no dispatch record — reproduced by
  `TestBug377_UnarmedSettleRepromptStillDispatches`.
- cp46/cp70 wire captures: `session/request_permission` `toolCall` shape
  (toolCallId + editableCommand only), `tool_call_update` in_progress frames,
  configOptions currentValue shapes — pinned in the new tests.
- lt-verify-devin run-1 (this tree's binary, real `devin acp`,
  `devin/swe-2-max`): post-turn gate reprompt on a chat run DISPATCHED as
  turn-42 and settled `accepted` (the cp37 wedge is gone); 3 `file_changed`
  events emitted (BUG-375 fixed); `tool_started`/`tool_completed` balanced
  5/5 despite `in_progress` frames (BUG-436 fixed). The same run exposed the
  armed-settle duplicate terminal → filed + fixed as BUG-438.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-374
change_type: bugfix
summary: Devin ACP provider fixes (tool-call correlation, permission/file_changed/model/replay) plus durable reprompt rearm; feature migrated provider-runtime -> ai-providers (BUG-442)
# --->8---
