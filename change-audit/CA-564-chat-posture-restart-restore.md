# CA-564: Chat posture restart restore (TUI + Desktop)

## What

Fixes restart bug: switching mode (scan/plan/code) then closing and reopening TUI or Desktop showed `code` again — the runner's persisted `active` was not restored.

## Root cause

Runner already persisted `active` via `PUT /client/chat-posture` (SSOT `chat-posture.json`).
Both surfaces only **wrote** on user switch, never **read back on boot**:

- **TUI**: `chatPosture == ""` defaults to `code` display. `SessionDefaultsMsg` (firstLoad) never `GET`ed the posture doc, so `*` chip and profile pins stayed on `code`. Manual `/mode`, `Tab`, `/new` required to apply.
- **Desktop**: `loadChatPostureConfig` only ran in `ChatPosturePanel.useEffect([])` — panel is `rightSidebarVisible && chatMode normal_chat` gated, so hidden sidebar = never mounted. Even when mounted, the single `GET` happened **before** the runner finished the `go run` compile window (`loadProjects` retries only `listProjects` 10×). `loadChatPostureConfig` also only set `chatPosture`/`chatPostureConfig`, never the pinned `selectedProvider`/`selectedModel`/`reasoningEffort`/`yoloMode`.

`/new` path was correctly `apply:` + `dirty PUT` and stays untouched (old test `TestNewChatReloadsPostureProfile` locks it).

Provider-agnostic: `GET/PUT` is shared JSON, no per-adapter branch — one fix covers Claude/Codex/Grok. Verified via existing adapter tests + new parity restore tests.

## Fix

### TUI (`apps/local-runner/internal/tui/app/`)

- `chat_posture.go`: new pending `restore` + `restoreChatPostureProfile` — same pinned-field application as `applyChatPostureProfile` but **no** `dirty` (no `PUT`) and no "Mode: …" banner. `chatPostureCmdFromPending` now handles `restore` → sets `chatPosture`/`chatPostureCfg.Active` from `cfg.Active` (falls back to `code`).
- `app.go` `SessionDefaultsMsg`: on `firstLoad`, sets `chatPosturePending = "restore"` and `cmds += cmdLoadChatPosture()` (5s timeout). This runs after catalog resolve, mirroring `tryApplyPendingFlowRestore` — the only place that already ran on first load.

### Desktop (`apps/desktop-flowpilot/src/state/store.ts`)

- `loadChatPostureConfig()`: now also applies `config.active`'s profile pins to `selectedProvider`/`selectedModel`/`reasoningEffort`/`yoloMode` and Grok YOLO sync (mirrors `setChatPosture` but without `PUT`). Added `ChatPostureProfile` import, removed `ReasoningEffort` cast.
- `loadProjects()` boot: after `listProviderAccounts`, added `withRetry(getChatPosture)` block with the same profile-apply logic, so the `GET` survives the runner boot compile window and fires even when the right sidebar is hidden. `ChatPosturePanel.useEffect` is kept as secondary single-shot fetch.

## Tests (additive only)

- TUI `chat_posture_restore_test.go`:
  - `TestSessionDefaultsRestoresRunnerActive` — runner `active:plan` with `claude/opus` pin → restore sets `chatPosture=plan`, `provider=claude`, `model=opus`, `dirty=false`, `cfg.Active=plan`.
  - `TestSessionDefaultsRestoreHandlesScanWithYolo` — `active:scan` Grok `yolo:true` → `yolo=true`, `provider=grok`, no dirty.
  - `TestSlashModeApplyStillPutsActive` — user switch `/mode plan` still marks `dirty` and `PUT` body contains `"active":"plan"` (locks old save path).
- Desktop `store.chat-posture-restore.test.ts` (`npx tsx --test`):
  - `loadChatPostureConfig restores runner active and pinned profile` — plan → provider/model/reasoning/yolo.
  - `restores scan with Grok YOLO and triggers sync` — `applyGrokYoloPosture` called once.
  - `keeps code when runner says code`.
- Old suite stays green: `go test ./internal/tui/app` (all), `go test ./internal/runner -run ChatPosture` pass; `store.chat-posture-grok-yolo.test.ts` pass; `npx tsc --noEmit` only shows pre-existing `store.run75035` errors.

## Out of scope / residual

- `/new` still uses in-memory `activePosture()` + `GET` + `apply` + `PUT` — not changed.
- Gemini read-only remains prompt+`--sandbox` (no RequestApproval bridge) — unrelated.
- `open_issue` tokenization residual from CA-563 stays as documented skip.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: bugfix
summary: restore runner-persisted active posture and profile pins on TUI/Desktop restart (restore pending without PUT, boot retry for Desktop)
# --->8---
