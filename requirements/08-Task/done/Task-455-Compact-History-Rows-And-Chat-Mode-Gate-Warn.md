# Task-455 — Compact single-line history rows + chat-mode gate auto-warn

## Metadata

- Document ID: `Task-455`
- Title: `Compact history rows and chat-mode gate auto-warn`
- Phase: `task`
- Status: `done`
- Owner: `devin`
- Created: `2026-09-29`
- Last Updated: `2026-09-29`
- Tags: `desktop, navigator, ux, flow-gate, local-runner`

## AI Quick View

### Summary

- Navigator history rows render as ONE line: `[icon] [Cancelled] title… <relative time>`.
- Chat-mode runs (`normal_chat`, no flow engine) always gate in `warn`
  mode; `gate_mode` config only applies to flow-context runs.

### Current Ask

- Collapse the two-line history row (title + `type · datetime`) into a single
  line with CSS ellipsis and a compact relative timestamp (`1m`, `2h`, `3d`,
  `5mon`).
- Prefix `[Cancelled]` (and `[Failed]` for parity) on rows whose run ended
  cancelled/failed instead of the old meta label.
- Runner: `effectiveGateMode(dotFP, rs)` — return `"warn"` for chat-surface
  runs (mirroring the `shouldForceFlowYolo` split), `loadGateMode` otherwise;
  use it at the root gate and the gate-blind hook.

### Key Decisions

- `T-1` single line everywhere the row renders (active list, inactive-project
  peek, selection mode, pending skeleton).
- `T-2` chat-vs-flow split reuses `shouldForceFlowYolo(runKind, workflowID,
  flowEngineDriven)` — a chat run that actually launched a flow engine keeps
  the configured mode.
- `T-3` child/flow-internal gates (`runChildArtifactOutputGateAtEpoch`,
  `flow_validate_audit_dispatch`) are untouched — children only exist under
  flow contexts.

### Constraints

- Do not change runner gate evaluation/rules — only the mode resolution.
- Keep `project-history-*` token-based CSS (styles.tokens test).

## 4. Exact Change

- `T-1` `navigatorHistory.ts`: add `formatRelativeTime(iso): string`
  (`now`/`Nm`/`Nh`/`Nd`/`Nmon`/`Ny`).
- `T-2` `Navigator.tsx`: single-line row markup; drop `runTypeLabel`/`RUN_LABEL`/
  `RUN_TIME_FORMAT`; `[Cancelled]`/`[Failed]` tag before the title.
- `T-3` `styles.css`: `.project-history-item` → flex single-line; `.meta` →
  right-aligned time slot; new `.project-history-item-tag(--warn)`.
- `T-4` `engine_gate_config.go`: add `effectiveGateMode`; wire into
  `gate_hook.go` (enforce site + log) and `gate_blind_hook.go`.
- `T-5` `EngineSettings.tsx`: copy notes chat runs always warn.

## 7. Test Signatures

- `formatRelativeTime` unit cases (now/min/hour/day/month/year, invalid iso).
- `effectiveGateMode`: chat rs → warn under enforce config; workflow rs →
  enforce; nil rs → persisted mode.
- Existing navigatorHistory/store suites stay green (additive only).

## 9. Out of Scope

- Skipping gate evaluation entirely in chat mode (warn still evaluates).
- RemoteSyncPanel row styling.

## 11. Completion Notes

- result: shipped — Navigator rows are single-line (icon, `[Cancelled]`/`[Failed]`
  tag, ellipsis title, relative time); `effectiveGateMode` auto-warns chat-surface
  runs while flow-context runs keep the persisted gate_mode. CA-1048.
- follow-ups: none
- upstream docs updated: EngineSettings Flow Gate copy notes chat auto-warn.
