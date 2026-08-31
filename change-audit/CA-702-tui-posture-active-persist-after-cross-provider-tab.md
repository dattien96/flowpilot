# CA-702 — TUI posture Active persist after cross-provider Tab (BUG-339)

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-339
change_type: bugfix
summary: persist posture Active to runner on cross-provider Tab (live, detached, and 409 race) so restart restores the same scan/plan/code/non
# --->8---

## What changed

- `chat_switch.go` `routePostureSwitch` detached already-pinned branch: now applies the full profile (provider/model/reasoning/yolo + Grok YOLO sync) locally and PUTs `active` immediately via `tea.Batch(detachedNotice, cmdSaveChatPosture)`, matching the bare-model detached branch which already PUT.
- `chat_switch.go` `applyChatSwitched` queued branch: after `applyChatPostureProfile` set `chatPostureCfg.Active = queued` and `chatPostureDirty = true` (CA-685 semantics).
- `app.go` `ChatSwitchedMsg` handler: batches `cmdSaveChatPosture` with `cmdStartOrchestrationStream` when `dirty` (success) and also sends the save on the `409 chat_no_active_leg` error path where `applyChatSwitched` set `dirty`.
- `chat_switch.go` `409` branch: when a queued posture exists, persist its `Active` and its profile's `reasoningEffort`/`yolo` before the defer notice and clear the queue.

## R1 evidence

- New tests `tui_posture_persist_cross_provider_test.go` (5 tests): cross-provider Tab → switch success persists `Active=plan` and profile pins; detached Tab already-pinned persists `Active` and profile; same-provider Tab still persists; 409 race persists `Active`; provider-agnostic table over `grok/codex/claude/opencode`.
- Full suite: `go test ./internal/tui/app -count=1` 7.4s PASS (no legacy edits).

## Honest gaps

- None for posture persist; Desktop Chat vs Workflow last-surface persist and Desktop detached parity are covered in `CA-703`.

## Prior CA not undone

- `CA-564` restore-without-PUT for `SessionDefaultsMsg` remains; this CA only PUTs on user-initiated posture switches (live, detached, or 409).
- `CA-685` `non` semantics and `CA-697` cross-provider routing remain the source of truth.
