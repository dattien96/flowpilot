# CA-711 — Opencode activity budget keeps wait alive (BUG-341 run-437116)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: keep wait-for-text budget alive on session activity (usage/tool) so long generations past the initial 8s cap are not cut off (run-437116)
# --->8---

## What changed

- `opencode_adapter.go` `drainOpencodeNotificationsBlocking`:
  - text grows → `resetOpencodeTimer(150ms)` (quiet, return fast) — unchanged from CA-708.
  - no text yet + any mapped session event (`usage_update`, `tool_call_update`, …) → `resetOpencodeTimer(15s)` — **new**. CA-708 deliberately did NOT reset on activity, which let the fixed 8s initial cap fire mid-generation.
  - text already present + non-text event → `resetOpencodeTimer(150ms)` (quiet mode) — new.
- Added `resetOpencodeTimer` (safe stop+drain+reset) and `opencodeNotificationKindAndContent` (diagnostics).
- Added DEBUG log when `agent_message_chunk` yields no text (raw content snippet) — so if the answer shape is ever unmapped, the next failure shows the exact wire bytes.

## Live evidence (run-437116, fixed binary, 2026-09-01)

TUI event stream for `test B4 in-place, toi la Nam` (opencode/muse-spark, scan):
- `evt-437127` token_usage_updated 14:07:00.450 (session/prompt result; drain 8s starts)
- `evt-437128` token_usage_updated 14:07:08.456 (model still generating, no text chunks)
- `evt-437129` turn_completed 14:07:08.462 — EMPTY finalMessage (8s cap fired; CA-708 no-reset-on-activity let it)

The model generated ~8s emitting only `usage_update` frames, then the answer text. The 8s cap was a razor-thin race; a 15s activity re-arm now survives it. Turn-2 `session/load` replay is what surfaced the answer later (the long-standing "second prompt shows first reply" symptom).

## R1 evidence

- New additive test `opencode_activity_budget_test.go` `TestOpencodeActivityKeepsBudgetPastInitialCap` (2s initial, usage at 0.1s + 2.2s, text at 2.5s):
  - **Red on CA-708** (checked out 0f32c5ef): `got "" want answer after long generation` FAIL.
  - **Green after fix**: PASS (2.65s), bridge sees the delta.
- All prior opencode drain/mapper tests still green (no edits): `LateChunk*`, `NoWaitWhenTextAlreadyPresent`, `UsageUpdate/ToolUpdate/ThoughtChunkDoesNotStarveText`, `TextContentArray/OutputText`, `ArrayThenTextViaBlockingDrain`.
- Full suites: `go test ./internal/runner -run TestOpencode -count=1` PASS (72s), `go test ./internal/tui/app -count=1` 7.2s PASS.
- Desktop parity: `HttpWsRunnerClient.sendTurn` (ts:423) uses the same `providerTurnId` filter + close-on-`turn_completed`; runner now emits `turn_completed` with `FinalMessage` → both surfaces paint the tool-heavy reply. No Desktop change.

## Honest gaps

- 15s activity budget bounds a textless-but-active turn at ~15s after the last event; a pathological runaway emitting events forever would keep the wait alive until the RPC ctx deadline (turn timeout) — acceptable.
- 150ms quiet after first text can truncate a multi-part answer with >150ms inter-chunk gaps (partial text shown, not blank); turn-2 replay may complete it.
- TUI hydrate-on-empty-terminal still not implemented (cannot recover chunks dropped by `unregisterSession`).

## Prior CA not undone

- `CA-708` wait-on-text-growth, `CA-709` array/output_text mapper, `CA-707` 8s/150ms conditional wait, `CA-706` blocking drain, `CA-705` TUI `turnLive`/C2 — all remain. This extends CA-708's "no reset on non-text" to "reset to 15s on activity" (the correct middle ground proven by run-437116).
