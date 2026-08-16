---
id: CA-528
feature_key: cli-tui
title: Retry agent hydrate so step [open] shows on first open; keep [open] on long step rows
date: 2026-08-17
status: COMPLETE
---

## Problem

After `/open` of a flow run, the F2 step sidebar can be missing the `[open]`
sub-agent chip on the **first** open while it appears after reopening (reported:
"vẫn code đó, tôi mở lại thì nó show nút Open rồi"). This was not truncation —
`[open]` only renders when `childRunForStep` maps a step to `m.agentRuns`, and
the hydrate that fills `agentRuns` raced the steps render:

1. `applyOpenedRunFlowChrome` **clears** `agentRuns` on open (`history.go`).
2. Steps-runtime and `cmdHydrateAgentRuns` (`GET …/agents`) run in parallel.
   Steps usually paint first → step rows visible but no `[open]` (correct
   contract from CA-513).
3. **In-flight swallow:** `stepsSuggestChildAgentOpen` fires when the steps
   transition, but if the open hydrate is still in flight,
   `cmdHydrateAgentRuns` returns `nil` — the only "child should be here" signal
   is lost.
4. **Last-writer-wins:** a faster `agent_graph_updated` already mapped children
   via `applyAgentGraph`, then the list hydrate lands **main-only** (synthetic
   main row) and **overwrites** `m.agentRuns`, erasing the `[open]` chips.
5. **No retry on terminal flow:** a completed flow stops polling
   (`shouldPollStepsRuntime` false), so nothing re-hydrates — `[open]` stays
   missing until the runner is warm enough on a later open.

Separately, long step names/statuses: `renderRightSidebar` truncated step rows
with `truncateVisual(line, w-2)`, which cuts from the **end** — the trailing
`[open]`/`[back]` chip was the first thing removed, and `openRunIDFromPanelLine`
matched by full name so a truncated name lost the click mapping too.

## Fix (TUI-only)

**A. Merge instead of clobber (`adoptAgentRuns`)**

- `agentRunsHydratedMsg` and `applyAgentGraph` share `afterAgentRunsAdopted()`
  (clamp focus index, expand F2, re-settle flow chrome).
- `adoptAgentRuns` (list hydrate path) never drops known children: an empty
  list is ignored, and a main-only list that would erase existing children is
  ignored. The live graph still replaces authoritatively.

**B. Bounded retry ladder (`cmdHydrateAgentRunsIfNeeded`)**

- `stepsNeedChildOpenChip()` = an agent-bearing step is in
  RUNNING/WAITING_USER_APPROVAL/DONE/FAILED but no child run is mapped.
- After a hydrate lands (empty, main-only, or soft error) or a steps refresh
  still shows a step without its chip, re-arm a `tea.Tick(400ms)` hydrate,
  capped at `agentHydrateRetries = 3`.
- Reset on `ChatOpenedMsg` / `RunStartedMsg` / any hydrate or graph that maps a
  child. Unreachable-runner errors do **not** re-arm (CA-514 no-flood kept).

**C. Long step rows keep their chip (`truncateStepLine`)**

- New `truncateStepLine(line, width)` reserves the trailing `[open]`/`[back]`
  suffix and squeezes the styled name/status prefix into the remaining width
  (with `…`), instead of end-truncating the chip away.
- Used by both `renderRightSidebar` (line `w-2`) and the narrow overlay
  (`maxInner-2`, replacing `lipgloss.MaxWidth`).
- `openRunIDFromPanelLine` now matches via `stepRowMatchesName`, which accepts a
  meaningful name-prefix + `…` so a truncated `[open]` row stays clickable.

## Provider impact

Provider-agnostic (Case 1): all touched logic reads step/agent/layout state,
never `providerKey`. The main-only-clobber and long-step-sidebar tests
parameterize Claude / Codex / Grok.

## Tests

New additive file `app/tui_open_chip_hydrate_retry_test.go`:

- `TestAdoptAgentRuns_MainOnlyListKeepsChildren` — children survive a main-only
  list hydrate, `[open]` remains × claude/codex/grok.
- `TestHydrateRetry_RearmsWhenStepsMissingChip` — main-only hydrate returns a
  retry tick; the tick fires a fresh hydrate without double-counting.
- `TestHydrateRetry_CapsAtLimit` — no re-arm past `agentHydrateRetryLimit`.
- `TestHydrateRetry_ResetsOnChildSuccess` — child hydrate resets the counter and
  stops the ladder; `[open]` appears.
- `TestHydrateRetry_UnreachableErrorDoesNotRetry` / `_SoftErrorRearms` —
  dead runner no re-arm (CA-514), soft error yes.
- `TestTruncateStepLine_KeepsOpenChip` — `[open]`/`[back]` survive, width ≤ limit.
- `TestStepRowMatchesName_TruncatedRow` / `TestOpenRunIDFromPanelLine_TruncatedRowStillMaps`
  — prefix+ellipsis matching maps a squeezed row to the child run.
- `TestRightSidebar_LongStepKeepsOpenChip` — long name on a wide sidebar keeps a
  hittable `[open]`, all lines within `sideWidth` × 3 providers.
- `TestOverlay_LongStepKeepsOpenChip` — narrow overlay keeps the chip too.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/... ./internal/cli/...` clean.
- `gofmt` clean on all changed files.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green, incl. CA-513 `tui_live_agent_open_test.go` and CA-524
  `tui_f2_right_sidebar_test.go`).
- `go test ./internal/tui/app -race -count=1` clean.

## Out of scope / residual

- `/agents` API not changed; no runner-side change.
- Retry ladder caps at 3; a genuinely unmapped child (name mismatch) stops
  retrying after the cap until the next steps transition / graph event.
- 8-step cap in the sidebar unchanged.