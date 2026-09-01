# CA-713 — Opencode aborts the turn after a denied permission: no-reply notice (BUG-341 live verification)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: live B4 verification + opencode.db ground truth prove the model never writes an answer after a denied permission — emit an honest no-reply notice instead of a blank turn (421135/424302/430742/433929/437116/440305)
# --->8---

## Live verification of CA-712 (2026-09-01 23:09, runner API, opencode 1.18.25)

Re-ran the exact BUG-341 scenario headlessly (`POST /client/workflow-runs` → `POST .../turns`,
prompt `test B4 in-place, toi la Nam`, `chatPosture=scan`, model muse-spark): the wire reproduced
the failure shape exactly — bash permission rejected (scan auto-deny) → `end_turn` → zero
`agent_message_chunk`. CA-712's recovery fired correctly (`session/load` issued, replay consumed,
user_message_chunk of the current prompt present) **but the replay contains no assistant text part
either** — `WARN turn_completed with EMPTY finalMessage permissionDenied=true`.

## Ground truth from opencode's own store (`~/.local/share/opencode/opencode.db`)

Both the live session (`ses_fa244db73ffeYrMgTZtQoxvv1a`) and the morning failing sessions
(`ses_fa2aae269ffe1O2u73ZceT98kz` = run-440305) have the identical structure:

- 1 user message, then assistant **steps** every one ending `step-finish reason="tool-calls"`.
- The LAST step contains reasoning parts + the denied tool call with `state.status="error"`
  ("The user rejected permission…") — **and no `type:"text"` part anywhere**.

Conclusion: opencode **aborts the agent loop at the denied tool call and never gives the model
another round**, so no answer text exists — not streamed, not stored, not replayed. The 92–118
`outputTokens` are reasoning + tool-call args. This matches the upstream report of opencode
stopping after rejection (Zed #48540) and retroactively explains why CA-706..711 (wait longer) and
CA-712 (replay recovery) could never find text: **there is no payload.**

## What changed

- `opencode_adapter.go` `recoverOpencodeEmptyAnswer`: when the replay recovery also returns empty,
  emit an honest terminal notice instead of leaving `FinalMessage` blank:
  `[no reply text] opencode aborted the turn right after a tool permission was denied — … Switch
  posture (plan/code) or approve the tool to continue.` (one delta + the FinalMessage, so TUI and
  Desktop both render it). A log line `[opencode] emitted no-reply notice …` marks it.
- The CA-712 recovery stays as layer 1: if a future opencode build lets the model answer after a
  denial (or streams post-result), the answer is picked up unchanged.

## R1 evidence

- New additive test `TestOpencodeNoReplyNoticeWhenRecoveryEmpty` — denied turn, `session/load`
  returns no text → `FinalMessage` starts with `[no reply text]`; red-before proven by mutation
  (notice disabled → FAIL `FinalMessage = ""`).
- All CA-712 tests still PASS (`TestCollectOpencodeReplayAnswer*`,
  `TestOpencodeReplayRecoveryAfterDeniedPermission` — recovery path unchanged,
  `TestOpencodeReplayRecoverySkippedWithoutDenial` — no notice without denial).
- Full `TestOpencode` suite PASS (73s; one `TestOpencodeMcpReadyGateWithholdsPromptWhenNeverReady`
  flake under load — passes 3/3 isolated and 3/3 with the TestOpencodeMcp group, no interaction
  with the changed path: no denial in that test).

## Honest gaps

- The notice text is provider-facing English, rendered as assistant text in TUI/Desktop; if
  product wants localized or card-style UX, the terminal carries `FinalMessage` for either.
- If opencode later streams a real answer after denials, the notice becomes dead code on that
  path (recovery layer wins first) — harmless.
- Root fix remains upstream: opencode should feed the tool error back to the model (one more
  round) instead of aborting; tracked via Zed #48540 / opencode PR #15614 class.

## Prior CA not undone

- CA-712 replay recovery (layer 1) and the denied-wait shortening remain; this CA adds layer 2
  (notice). CA-705..711 contracts unchanged.
