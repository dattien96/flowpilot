---
id: CA-514
feature_key: cli-tui
title: TUI stays interactive when runner is dead or catalog load hangs
date: 2026-08-14
status: COMPLETE
---

## Problem

Operators saw the TUI “completely frozen” **after recent cli-tui work**
(CA-502 open orch attach, CA-512 mode restore, CA-513 live agent hydrate poll)
when the runner was down or catalogs stalled.

Root regression chain:

1. `/open` flow always starts orch SSE (CA-502)
2. CA-513 polled steps **and** `ListAgentRuns` every ~1.6s while
   `shouldPollStepsRuntime()` — and that predicate treated `orchStream != nil`
   as “live”, so **completed** opens kept polling forever
3. Dead runner + `http.Client{Timeout:0}` + no in-flight guard → stacked cmds
4. `sessionLoading` blocked almost all keys until catalog returned (unbounded)

Symptom timeline matched “only started hanging recently” (after F2/open/live-agent work).

## Second pass (post-operator repro: “TUI hang lại”)

After the first pass the TUI still hung in the reported scenario: **runner alive,
account APIs answer, `GET /client/projects` times out** → catalog empty → no
`project_id` → user presses Enter → `cmdStartRun` re-fetched the catalog with
`context.Background()` → hang in `ConnRunning`/“thinking…”.

Root causes found:

1. `runnerUnreachableErr` classified **`context deadline exceeded`** as
   “runner dead” → catalog timeout was reported as “runner not responding” and
   **no retry was scheduled** (`cmdLoadProjectsCatalog` never ran).
2. `cmdStartRun` fallback `ListProjects(ctx)` used an unbounded context.
3. `processInput` started a run with no bound project (no guard).
4. `sessionLoadTimeoutMsg` set `sessionDefaultsLoaded=true`, so a late real
   `SessionDefaultsMsg` lost first-load handling (no persist / restore).
5. Transport `ResponseHeaderTimeout: 12s` cut the 30s/45s catalog budgets.
6. `/login` reloaded the session without the early keys-unlock + safety timeout.

## Fix

First pass:

- Bound session provider path (8s); **projects catalog 30s** (Supabase slow ≠ runner down)
- **Early keys unlock** at 1.5s (`sessionKeysUnlockMsg`); hard fail only at 45s if defaults never arrive
- Catalog timeout → background `cmdLoadProjectsCatalog` retry (45s), bind project when ready
- HTTP dial 3s + response-header 12s on TUI client transport (SSE still Timeout=0)
- Do **not** hard-block keyboard while loading — only non-slash *send* blocked
- Clear in-flight flags; pause auto-poll after 3 dial/timeouts
- Auto-poll only while live / non-terminal handle
- Do **not** treat orch-only (late listener on completed open) as poll-live
- Do **not** report "runner not responding" when only `/client/projects` times out

Second pass:

- **Split classifications**: `runnerDialDeadErr` (dial/reset = runner process
  down) vs broad `runnerUnreachableErr` (poll-streak accounting). Catalog paths
  use the strict check, so `context deadline exceeded` on `/client/projects` is
  **retried**, never reported as “runner offline”.
- `sessionLoadTimeoutMsg` **no longer marks `sessionDefaultsLoaded`** — a late
  real `SessionDefaultsMsg` still counts as first load (persists provider/model,
  restores flow via `tryApplyPendingFlowRestore`).
- **`processInput` refuses fast when `sessionDefaultsLoaded` and no project is
  bound** (records the draft line, no `ConnRunning`, no `cmdStartRun`) instead
  of re-fetching the catalog with an unbounded context. `bindProjectIfPossible`
  binds from the known catalog first.
- **`cmdStartRun` catalog fallback bounded** to 5s (returns `ErrMsg` on empty).
- `cmdLoadSkills` bounded to 5s; `cmdLogin` bounded to 15s.
- **`/login` schedules the same early-keys-unlock + 45s safety timeout** as cold start.
- Transport `ResponseHeaderTimeout` raised to 60s so it cannot cut the 30s/45s
  catalog budgets (per-call ctx deadlines do the real capping).

## Provider impact

Case 1 (agnostic). TUI HTTP client timeouts + catalog classification only; no
provider adapters, no SSE/stream path changes. The three providers share the
same TUI client (`internal/tui/client`) and `processInput` guard, so the hang
fix is identical for Claude / Codex / Grok flows.

## Tests

`tui_runner_offline_nofreeze_test.go` (first pass)
`tui_no_freeze_catalog_test.go` (second-pass matrix: catalog-slow vs runner-dead
classification, no-project refuse, catalog bind, bounded start-run fallback,
`runnerDialDeadErr` matrix, `/login` reload unlock)
Restored `TestSlashYolo_PersistsChatPreference`, `TestNew_YoloFlagOverridesPrefs`,
`TestPersist_FlowModeKeepsChatYoloPreference` in `tui_mode_flow_prefs_test.go`.
Legacy suite untouched (one WIP no-freeze test updated to the new timeout contract).

## Third pass (post-review fixes)

- **No false “ready”**: `SessionDefaultsMsg` no longer shows `statusMsg="ready"`
  when the catalog produced no project — status reflects the real state
  (`catalog unavailable`, `loading catalog…`, `no project match`, `no project`).
  The first-load “Ready — type /…” onboarding chat line is unchanged (legacy
  contract: shown once).
- **Help always surfaces**: `formatMissingProjectHelp` (the 4-step fix) now
  shows whenever project is nil and a path is configured — previously it was
  skipped when `CatalogErr != ""`, so the screenshot repro (catalog timeout +
  empty list) showed no guidance until the user pressed Enter.
- **`"i/o timeout"` no longer classified as runner-dead**: only TCP-level
  dial/reset failures (`connection refused`, `connectex`, `dial tcp`,
  `connection reset`, `no such host`, `network is unreachable`) mean the runner
  process is down. Bare `"i/o timeout"` is catalog-slow (retried); it still
  counts toward the broad `runnerUnreachableErr` poll-pause streak.
- **`processInput` refusal marks the state**: when no project_id is bound the
  refusal also sets `statusMsg="chat disabled — no project_id"` so the operator
  sees why Enter is ignored.

## Fourth pass (catalog belongs to the banner phase)

- **`cmdLoadSessionDefaults` starts `ListProjects` in parallel** with the
  account/provider path (separate client in a goroutine, 30s budget). Previously
  it ran after the 8s fast path, so the catalog only started after the banner
  was long gone.
- **The FlowPilot banner stays up until `SessionDefaultsMsg`** decides the
  catalog — removed the 1.5s `sessionKeysUnlockMsg` schedule from `ConnectedMsg`
  and `LoginResultMsg`. `sessionKeysUnlockMsg` is now a defensive no-op; typing
  is never hard-locked, non-slash *send* stays blocked (`processInput`), and the
  45s `sessionLoadTimeoutMsg` safety net remains. Banner text
  (“loading session · project · providers — chat locked”) now matches reality.
- Result: catalog load overlaps the banner phase end-to-end; chat unlocks only
  after the project decision (bound → chat on, no project → disabled with the
  4-step help + real status, never a false “ready”).

## Residual / out of scope

- Fix makes the TUI never hang, but chat still **needs** `project_id`: a dead
  Supabase catalog means “chat cannot start” until `/login` / runner env is fixed.
- Rebuild the Windows TUI from this tree — the screenshot binary predates the
  source and shows stale messages (“runner not responding”) that no longer exist.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Unblock TUI keys during load; timeout polls; pause auto-poll when runner offline; classify catalog-slow vs runner-dead; refuse no-project chat fast; load project catalog in parallel during the FlowPilot banner phase
# --->8---
