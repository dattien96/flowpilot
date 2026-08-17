---
id: CA-512
feature_key: cli-tui
title: Persist TUI mode and selected flow across restarts
date: 2026-08-14
status: COMPLETE
---

## Change

Extend `tui-session.json` (same prefs path as provider/model/reasoning) so
`flowpilot chat` restores:

1. **Mode** — `chat` | `flow` | `step`
2. **Selected flow** when mode is flow/step — `flowRef` (builtin) and/or
   `workflowId` (catalog) plus human `flowLabel`
3. **Yolo** — chat-mode toggle only (`yolo: true|false`). Flow mode stays
   auto-on via `effectiveYolo()`; disk stores `m.yolo` (chat preference),
   never the flow auto-on value. `--yolo` flag wins over prefs.

### Behavior

| Action | Persist |
|--------|---------|
| `/flow <ref>` | mode=flow + flow identity (+ keep chat yolo pref) |
| `/chat` | mode=chat, clear flow fields |
| `/yolo` | chat only; toggle + persist yolo |
| `/provider` `/model` `/reasoning` `/new` | keep current mode+flow+yolo |
| `New()` | stash flow prefs; **stay chat** until project binds |
| Project bound | then restore ModeFlow + arm (tryApplyPendingFlowRestore) |
| Silent/list `FlowListMsg` | re-resolve arm against catalog (label/SubMode) |

`/open` run chrome does **not** rewrite prefs (session-only chrome; prefs
owned by `/flow` and `/chat`).

Chat-mode save normalizes away stale flow fields so restart cannot re-arm a
cleared flow.

Package tests set `FLOWPILOT_TUI_SKIP_MODE_RESTORE=1` so a shared session
file cannot force ModeFlow after `/flow` tests; mode-restore coverage uses an
isolated session file.

### Provider impact

Case 1 (agnostic). TUI session prefs only; no provider adapters.

### Files

- `internal/tui/prefs/prefs.go` — Session fields + Load empty-check
- `internal/tui/app/helpers.go` — persist/restore/refine helpers
- `internal/tui/app/app.go` — New restore; /flow /chat persist; FlowListMsg refine
- Tests: `prefs_test.go` additive; `tui_mode_flow_prefs_test.go` new;
  `statusline_test.go` TestMain skip-mode-restore for suite hermeticity

### Prior claims kept

CA-511 quiet chrome; CA-502 open flow chrome; CA-503 name hydrate. Prefs
restore does not override an active run arm (refine skips re-resolve when
`runHandle != nil`).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: task
summary: Persist mode/flow/yolo; defer flow-mode restore until project catalog binds (cold-start hang fix)
# --->8---
