# CA-696 — TUI chat switch surface slice 1 (Task-315): /provider + /model routing, in-place adopt, envelope collapse

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: TUI routes /provider <key> and /model <foreign> on a live chat through the runner switch endpoint; ChatSwitchedMsg adoption keeps the transcript, resets per-run stream state, and attaches the new leg's orchestration stream; handoff seed envelope collapses to a divider in addMessage; client SwitchChatProvider/GetChatTimeline bindings
# --->8---

## What changed

- `tui/client/client.go`: `HandoffPromptPrefix` const (parity-tested), `ChatSwitchInput/ChatSwitchHandoffStats/ChatSwitchResponse/ChatTimelineResponse` DTO mirrors, `SwitchChatProvider` (typed refusals surface as errors), `GetChatTimeline`.
- `tui/app/chat_switch.go` (new): `ChatSwitchedMsg`, `routeProviderSwitch` routing rule (chat run + ChatID + foreign provider + not in-flight → switch; everything else falls back to the legacy path — same-provider stays in-place per CP-59 P-7), `cmdSwitchChatProvider` (single in-flight guard, CS-05), `applyChatSwitched` (adopt: transcript kept, handle/provider/model swapped, stream state reset, failure keeps the source leg with one error line).
- `app.go`: `/provider <key>` mid-chat routes via `routeProviderSwitch` before the legacy block (flag-off/typed-failure falls back to the exact legacy text); `/model <foreign-id>` cross-provider branch routes before mutating `m.model` (BUG-330 class killed at the TUI entry points handled in this slice); `ChatSwitchedMsg` case adopts + attaches `cmdStartOrchestrationStream` (seed turn streams live through the existing orch machinery); `addMessage` collapses the handoff seed envelope into a one-line system divider (never a raw user bubble).
- `model.go`: `chatSwitchInFlight`, `chatSwitchQueuedPosture` fields.

## Prior claims honored

- CA-679/CA-686/CA-689c — the `/model` in-place paths (same-provider, no-run, workflow) are untouched; routing only intercepts the cross-provider-on-live-chat case.
- CA-693/694/695 (Task-313/314) — DTO mirrors match the runner contracts verbatim.

## R1 evidence

- 5 new TUI tests green (routing rules incl. in-flight drop + same-provider in-place; adopt keeps transcript byte-identical on success and appends one error line on failure; seed collapse; client round-trip; prefix parity).
- Full TUI suite after the slice: **7 failures — identical to the rebased clean-tree baseline**.

## Honest gaps (Task-315 slice 2)

- Posture Tab / `/mode` cross-provider routing (bare-model pin derivation + persist) — entry points /provider + /model only in this slice.
- Detached-chat reattach predicate (startRun with chatId+switchFromRunID) — surface wired runner-side, TUI predicate pending.
- Restore-by-chat timeline fetch (`/open` via GetChatTimeline), seed-stats divider formatting (currently a static one-liner from addMessage), Tab queued-posture application.

## Falsifiable expectations locked

1. Success adoption never adds a client divider and never clears the transcript.
2. A refused/failed switch leaves the chat on the source leg with exactly one error line.
3. `/model`/`/provider` on a workflow run or without a run behave byte-identically to the legacy paths.
