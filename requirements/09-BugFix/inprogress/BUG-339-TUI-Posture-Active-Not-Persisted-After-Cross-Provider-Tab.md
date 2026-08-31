# BUG-339 — TUI posture Active not persisted after cross-provider Tab

## Metadata

- Document ID: `BUG-339`
- Title: `TUI posture Active not persisted after cross-provider Tab`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `codex`
- Reviewers: `codex`
- Created: `2026-08-31`
- Last Updated: `2026-08-31`
- Parent Documents: `CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat`, `SD-26-Chat-Continuity-Ssot`, `CP-56-Posture-Scan-Plan-Code`
- Child Documents: ``
- Related Documents: `CA-564-chat-posture-restart-restore`, `CA-697-tui-posture-tab-routing-task315-slice2`, `CA-685-posture-non-mode`
- Replaces: ``
- Tags: `chat-ui, cli-tui, posture, regression, CP-59`

## AI Quick View

### Summary

- Cross-provider posture Tab (e.g. `code` opencode → `plan` grok) switches legs via `switchChatProvider` and queues the posture for re-apply on the new leg, but never persists the new `active` to the runner's `chat-posture.json` SSOT.
- Quit immediately after the switch and reopen TUI restores the old `active` (e.g. `code`) via `SessionDefaultsMsg` restore, so the mode looks lost. Same-provider Tab and `/mode` in-place paths already PUT `active`; only the queued cross-provider path was missing.
- Detached Tab with already-pinned provider had the same hole (local apply without PUT), and the 409 `chat_no_active_leg` race also lost the new active.

### Current Ask

- Persist the new `active` posture to the runner on every successful cross-provider Tab (live and detached, including the 409 race), so restart restores the same `scan/plan/code/non` the user last saw. Full profile (reasoning/yolo) must land on the new leg as well.

### Key Decisions

- `V-1` After a successful `ChatSwitchedMsg`, the queued posture's `Active` is written to `chatPostureCfg` and `chatPostureDirty` is raised; the `Update` handler batches `cmdSaveChatPosture` with `cmdStartOrchestrationStream` (no extra round-trip).
- `V-2` Detached Tab (already-pinned provider) now applies the full profile locally (provider/model/reasoning/yolo) and PUTs `active` immediately, so a quit before the next prompt still survives. The bare-model detached branch already PUT; this makes the already-pinned detached branch match it.
- `V-3` The `409 chat_no_active_leg` race (switch endpoint returns detached) also persists the queued posture's `Active` and its reasoning/yolo before the defer notice, and the `Update` handler sends the save even on the error path.

### Constraints

- Additive tests only; no edits to the legacy suite without asking. Provider-agnostic: the fix is `chatId`/`active` JSON only, no `providerKey` branch — verified by a parameterized test over `grok/codex/claude/opencode`.
- No change to the same-provider in-place path (already correct) or to `non` semantics (CA-685).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/tui/app/chat_switch.go:158` `routePostureSwitch`, `applyChatSwitched`, `chat_switch_tui_test.go:136`
- `apps/local-runner/internal/tui/app/app.go:1318` `ChatSwitchedMsg` handler, `chat_posture.go:121` queued apply
- `CA-564` restore, `CA-697` cross-provider routing, `CA-685` non mode
- Repro: `code` opencode → Tab `plan` grok → quit → reopen → `Mode: code` instead of `plan`

## 1. Issue Summary

After `CA-697` made posture Tab cross-provider via the switch endpoint, a user who Tabs from `code` (opencode) to `plan` (grok) sees the UI switch to `plan` and `grok` correctly, but quitting TUI and reopening restores `code`. The runner's `chat-posture.json` still holds `active: code`. The same loss occurs for a detached Tab (restored chat with no active leg) and for the `409 chat_no_active_leg` race where the switch fails and defers to reattach.

## 2. Parent Links

- impacted coding plan: `CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat` P-6/Q-2 (posture Tab routing)
- impacted tech design: `SD-26-Chat-Continuity-Ssot` (posture SSOT), `CP-56` posture persistence
- impacted system spec: `SS-05-Workflow-Ai-Provider` chat posture contract

## 3. Environment and Reproduction

- environment: `cp59-chat-ssot` at `CA-699` + `CA-697`, runner `chat-posture.json` SSOT, TUI `tui-session.json` also persists `provider/model` but not `active`
- reproduction steps:
  1. Start TUI `just chat-dev` on any project, `chatPosture` is `code` (opencode).
  2. Tab to `plan` whose pin is `grok/grok-4.5` (cross-provider, already-pinned provider).
  3. Observe `Mode: plan` and `provider: grok` locally; `chatPostureCfg.Active` is still `code` in memory, `chatPostureDirty` is false.
  4. Quit TUI, reopen `just chat-dev` — `SessionDefaultsMsg` firstLoad does `GET /client/chat-posture` and restores `active: code`, so the UI shows `Mode: code` again.
- frequency: 100% for cross-provider Tab with already-pinned provider; same-provider Tab and `/mode` in-place are unaffected.

## 4. Expected vs Actual

- expected: After the switch succeeds, `GET /client/chat-posture` returns `active: plan`; restart restores `Mode: plan` with the `plan` profile's pins (provider/model/reasoning/yolo). Detached Tab and the 409 race also persist `active`.
- actual: `GET` still returns `active: code`; restart restores `code`; detached Tab also loses `active` if quit before the next prompt.

## 5. Impact

- users affected: anyone using posture Tab to switch providers (BUG-330 fix) and then restarting TUI — the mode appears to not stick.
- workflows affected: chat persistence across restarts (CA-564 contract).
- severity: medium — no data loss, but breaks the "mode persists" promise.

## 6. Root Cause

- hypothesis: queued posture path never PUT `active`.
- confirmed cause: `routePostureSwitch` for a cross-provider pin with an existing provider sets `chatSwitchQueuedPosture = name` and returns only the switch cmd (no `Active`/`dirty`). `applyChatSwitched` then calls `applyChatPostureProfile` (which sets `chatPosture` and provider/model in RAM) but never sets `chatPostureCfg.Active` or `chatPostureDirty`. The `ChatSwitchedMsg` handler only starts the orchestration stream, never a save. The detached already-pinned branch had the same missing `Active`/PUT, and the 409 error branch cleared `chatDetached` without persisting the queued posture.
- evidence: `chat_switch.go:220` `chatSwitchQueuedPosture = name` without `Active`; `applyChatSwitched:272` `applyChatPostureProfile` without `Active`; `app.go:1318` no save on success; `chat_switch.go:192` detached already-pinned returns `detachedNotice` without save.

## 7. Fix Strategy

- `F-1` `chat_switch.go` detached already-pinned branch: apply the full profile locally (provider/model/reasoning/yolo via `posturePinProvider`/`setPostureProvider`, `clampReasoningForCurrentModel`, Grok YOLO sync) and PUT `active` immediately via `tea.Batch(detachedNotice, cmdSaveChatPosture)`, mirroring the bare-model detached branch which already PUT.
- `F-2` `chat_switch.go` `applyChatSwitched` queued branch: after `applyChatPostureProfile`, set `chatPostureCfg.Active = queued` and `chatPostureDirty = true` (CA-685 semantics: the pin's full profile lands on the new leg, and the dirty flag survives).
- `F-3` `app.go` `ChatSwitchedMsg` handler: batch `cmdSaveChatPosture` with `cmdStartOrchestrationStream` when `dirty` (success path) and also send the save on the `409` error path where `applyChatSwitched` set `dirty`.
- `F-4` `chat_switch.go` `409` branch: when `chatSwitchQueuedPosture` is set, persist its `Active` and its profile's `reasoningEffort`/`yolo` before the defer notice and clear the queue, so a quit before reattach still sees the new `active`.

## 8. Validation

- `V-1` New TUI tests `tui_posture_persist_cross_provider_test.go`: cross-provider Tab → switch success persists `Active=plan` and the `plan` profile's pins; detached Tab already-pinned persists `Active` and profile; same-provider Tab still persists; 409 race persists `Active`; provider-agnostic table over `grok/codex/claude/opencode`.
- `V-2` Existing suite: `go test ./internal/tui/app -count=1` 7.4s PASS (no legacy edits).
- `V-3` Manual: Tab `code` opencode → `plan` grok → quit → reopen → `Mode: plan` and `provider: grok` (and `GET /client/chat-posture` shows `active: plan`).

## 9. Regression Guard

- tests: `tui_posture_persist_cross_provider_test.go` (5 tests, parameterized) + existing `chat_switch_tui_test.go`, `chat_posture_restore_test.go`, `ca685_posture_non_mode_test.go` remain green.
- alerts: `chatPostureDirty` must be false after the save's `chatPostureMsg`; `Active` must equal `chatPosture` after any posture switch (live, detached, or 409).
- audit checks: provider-agnostic (no `providerKey` branch in the fix), additive only.

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-59` P-6/Q-2 note that cross-provider posture Tab now PUTs `active` (already implied by CA-564, now enforced).
- notes left unchanged on purpose: `CA-685` `non` semantics, `CA-564` restore-without-PUT for `SessionDefaultsMsg`.
