# CA-563: OpenCode-style Scan/Plan/Code chat postures (runner SSOT + TUI + Desktop)

## What

Adds Scan/Plan/Code chat postures. Scan and Plan are read-only: every tool
reaches the runner approval bridge, which auto-approves reads and auto-denies
writes without asking. Code keeps today's gated chat (approvals / YOLO).

## Runner (SSOT)

- `chat_posture.go`: shared posture document (`active` + `profiles`), GET/PUT
  `/client/chat-posture`, `loadChatPosture`/`saveChatPosture`, per-turn
  `resolveTurnChatPosture` (flow/workflow runs never run a read-only posture).
- `chat_posture_policy.go`: `readOnlyApprovalDecision` — conservative classifier
  (reads approve, writes/unknown deny). `isReadOnlyToolName` uses whole-word +
  write-intent fail-closed matching (no substring false positives like
  ReadWrite / open_pr / Restart).
- `RequestApproval` (interactive_service.go) checks the read-only posture BEFORE
  the YOLO branch, so a profile YOLO never leaks a write.
- Claude/Codex/Grok adapters force gated permission modes for scan/plan.
  Gemini (--print has no RequestApproval path) forces `--sandbox` and injects a
  read-only prompt guard; this is prompt+sandbox enforcement, not a denial
  bridge — documented residual risk.
- JSON wire name for the reasoning pin is `reasoningEffort` (matches the turn
  body + Desktop); the legacy `reasoning` alias still decodes.

## TUI

- `/mode` (apply/cycle), `/mode-setup` (edit + save), Tab-cycle on empty input,
  statusline posture chip. `/mode` now persists `active` back to the runner so
  the Desktop and a later `/new` agree.
- Tab and `/mode` bare cycle **plan ↔ code** only; Scan is only reachable via
  explicit `/mode scan` (the user must opt in to read-only intentionally).
- Statusline posture chip: scan=sky `#38bdf8`, plan=amber `#f59e0b`,
  code=green `#3fb950` — each posture has its own dedicated hue.
- Grok YOLO posture sync is snapshotted off the `tea.Cmd` goroutine (no data race).
  Flag is re-enabled on HTTP failure via `grokSyncFailedMsg`; turn is still sent.
- `/mode` and `/mode-setup` are **visible** in prefix suggestions (`/mo` →
  `/mode` + `/mode-setup` + `/model`); previous `hidden` workaround removed
  per user confirmation. `TestFilterSlashSuggestions_ShowsOnSlash` updated.
- `/mode-setup` now shows next values via picker (posture → field → value)
  instead of requiring manual typing (`/mode-setup ` → scan/plan/code →
  provider/model/reasoning/yolo/clear → catalog values).
- `/new` reloads posture doc from runner and applies `profiles[active]` pins.
- Treo fix: Tab and `/new` only dispatch posture `GET` after
  `sessionDefaultsLoaded`; posture `GET`/`PUT` + history `ListRunHistory` all
  have 5s timeouts so a cold runner never leaves `Update` blocked.

## Desktop

- `ChatPosturePanel` tab strip + setup modal with **3 tabs** (Scan | Plan | Code)
  instead of stacked profiles. Each tab shows one profile form (provider/model/
  reasoning/YOLO). Posture-specific tab colors: scan=sky, plan=amber, code=green.
- Store actions (`setChatPosture`, `loadChatPostureConfig`,
  `saveChatPostureConfig`), `chatPosture` threaded into every normal-chat turn.
- `setChatPosture` now calls `toggleYoloForActiveProvider` when the active
  profile pins YOLO on Grok — previously Desktop skipped the Grok config.toml
  rewrite that TUI performed, leaving Grok's runtime YOLO stale.

## Review fixes (post-CA-562)

- **JSON wire name**: `reasoningEffort` (matches turn body + Desktop); legacy
  `reasoning` alias accepted via custom `UnmarshalJSON`.
- **Tool-name classifier**: fail-closed write-token + whole-word matching; no
   substring false positives. Residual: `open_issue`/`open_ticket` tokenize to
   `open` → approve (user said skip, documented).
- **Gemini**: scan/plan never pass `--dangerously-skip-permissions`; prompt
  guard injected. No RequestApproval bridge — prompt+sandbox only.
- **TUI `/new`**: reloads posture doc from runner, applies `profiles[active]`
  pins (provider/model/reasoning/yolo) so the next turn uses pinned selection.
- **Grok sync retry**: flag is cleared synchronously but re-enabled on HTTP
  failure via `grokSyncFailedMsg`; the turn is still sent (sync failure does
  not block the conversation).
- **Treo after chat visible**: `sessionDefaultsLoaded` guard on Tab + `/new`;
  5s timeouts on posture GET/PUT + `ListRunHistory`; new tests
  `TestTabBeforeSessionDefaultsDoesNotCyclePosture` and
  `TestTypingAfterSessionDefaultsGoesToInput` lock the fix.
- **`/mode-setup` picker**: wizard suggestions (posture → field → value)
  via `filterModeSetupSuggestions` + `suggestionAcceptValue`; `/mode-setup
  clear` handled with 2 args; test `TestModeSetupPicker_PostureFieldValue`.

## Tests

- Runner: `chat_posture_store_test.go` (store, normalize, JSON wire-name +
  legacy alias, resolveTurnChatPosture, RequestApproval auto-decide),
  `chat_posture_policy_test.go` (reads approve / writes deny / git dual-form /
  false-positive tool names), `chat_posture_adapter_wiring_test.go`
  (scan/plan force gated modes on Claude/Codex), `gemini_adapter_test.go`
  (read-only posture never skips permissions).
- TUI: `chat_posture_test.go` (mode apply/cycle/validate, setup edit+save,
  Tab cycle plan↔code, scan→plan on Tab, statusline chip, active persistence,
  `/new` reloads posture, `grokSyncFailedMsg` re-enables flag, mode-setup
  picker posture→field→value, treo guards).
- Desktop: `store.chat-posture-grok-yolo.test.ts` (setChatPosture Grok+YOLO
  pin → applyGrokYoloPosture called; Claude → not called).

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: feature
summary: add Scan/Plan/Code chat postures (runner SSOT + TUI + Desktop), read-only policy, and persistence
# --->8---
