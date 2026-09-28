# CA-1048 — Compact single-line history rows + chat-mode gate auto-warn (Task-455)

## Summary

Two UX complaints bundled in one task:

1. Navigator history rows took two lines per chat (title + `type · long
   datetime`) — noisy and hard to scan. They are now a single line:
   `[icon] [Cancelled] title-with-ellipsis… <relative time>`.
2. The flow gate ran in whatever `gate_mode` the project had saved — for
   plain chat-mode turns that meant enforce could reprompt/block a casual
   chat. Chat-surface runs now always gate as `warn` (violations surface
   inline, never interrupt); the persisted mode governs flow-context runs
   only.

## Changes

- `apps/desktop-flowpilot/src/components/navigatorHistory.ts`
  - `formatRelativeTime(iso, nowMs?)` — `now`/`Nm`/`Nh`/`Nd`/`Nmon`/`Ny`.
  - `historyStatusTag(status)` — `Cancelled`/`Failed` tags; live states stay
    icon-only.
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - All four row variants (active list, inactive-project peek, selection
    mode, pending skeleton) collapse to one flex line; title ellipsizes,
    cancelled/failed rows get an inline `[tag]` prefix, right slot carries
    the relative time (or `Syncing…`/`Starting…`).
  - `runTypeLabel`/`RUN_LABEL`/`RUN_TIME_FORMAT` removed; row tooltip now
    shows the full prompt so truncation stays readable.
- `apps/desktop-flowpilot/src/styles.css`
  - `.project-history-item` → single-line flex; `.meta` pins right;
    `.project-history-item-tag(--warn)` added; `-top` kept for
    RemoteSyncPanel rows (now the truncating flex child).
- `apps/desktop-flowpilot/src/components/settings/EngineSettings.tsx`
  - Flow Gate copy notes chat-mode turns always warn.
- `apps/local-runner/internal/runner/engine_gate_config.go`
  - `effectiveGateMode(dotFP, rs)` — returns `"warn"` for chat-surface root
    runs (`runKind:"chat"`, no `workflowID`, no live `flowEngineDriven`,
    `parentRunID` empty — i.e. `shouldForceFlowYolo` false); otherwise the
    persisted `gate_mode`. Spawned children and flow-driven chat runs keep
    the configured mode.
- `gate_hook.go` (enforce site + log) and `gate_blind_hook.go` now resolve
  through `effectiveGateMode`, so a blind-block also warns instead of
  interrupting a chat turn.
- Child-gate (`runChildArtifactOutputGateAtEpoch`) and flow-internal
  `flow_validate_audit_dispatch` call sites untouched — children exist only
  under flow contexts.

## Tests (RED→GREEN)

- `task455_chat_gate_warn_test.go` (new): chat run under enforce config →
  `warn`; workflow/empty-kind/workflowID-pinned/flowEngineDriven/spawned
  child/nil → persisted mode; `gateBlindBlocksTurn` on a chat run warns,
  never blocks.
- `navigatorHistory.test.ts` +4 cases: relative-time buckets, future-clamp,
  invalid input, status tags.

## Verification

- `go test ./internal/runner -run 'Task455'` — PASS; gate/blind/vibe/enforce
  subset (52s) — PASS, no regression.
- Desktop `tsc -p tsconfig.phase1-tests.json` clean;
  `navigatorHistory.test.js` 16/16, `RemoteSyncPanel.render` + tokens suites
  unchanged.
- Full phase-1 desktop suite: same 13 pre-existing baseline failures as the
  CA-1047 run (Node-26 `localStorage`, stale-state flakes).

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: Task-455
change_type: feature
summary: single-line Navigator history rows (ellipsis title, [Cancelled]/[Failed] tag, compact relative time) and chat-mode runs auto-gate as warn — enforce/warn config governs flow-context runs only
# --->8---
