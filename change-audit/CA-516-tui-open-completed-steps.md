---
id: CA-516
feature_key: cli-tui
title: /open of a completed workflow must still render the F2 step timeline
date: 2026-08-15
status: COMPLETE
---

## Problem

`/open <run>` of a **completed** flow-mode run showed the flow chrome (mode
border `[flow]`, `Opened flow grok-flow · run-189839`, transcript) but the F2
top-right panel rendered **no step rows** — no `Steps N:` header and no
`[open]` chips, so a step sub-agent could not be reopened from the panel.

Root cause chain:

1. `applyOpenedRunFlowChrome` clears `flowSteps` on every open (history.go:71),
   expecting a refill after open.
2. `ChatOpenedMsg` only scheduled the one-shot `cmdRefreshStepsRuntime()` when
   `shouldPollStepsRuntime()` was true (app.go:611).
3. For a completed open the handle/snapshot status is terminal, so
   `shouldPollStepsRuntime()` returns false (CA-508/CA-514: completed + idle +
   orch-listener must not auto-poll every 1.6s) → the one-shot **never fired**.
4. `flowSteps` stayed `nil` → `flowStepsPanelLines()` returned nothing.
5. `cmdHydrateAgentRuns` did run (`kind=="flow"`), but without step rows there
   was no `[open]` to show.

The gate on `ChatOpenedMsg` was the regression: `cmdRefreshStepsRuntime` is a
**one-shot** and is explicitly safe for completed opens (per its doc comment and
the CA-514 test `TestShouldPollStepsRuntime_IdleTerminalOpenDoesNotAutoPoll`).

## Fix

`ChatOpenedMsg` (app.go): schedule `cmdRefreshStepsRuntime()` unconditionally
for `kind=="flow" && runHandle != nil` — the same guard already used for
`cmdHydrateAgentRuns`. The cursor auto-poll cadence is untouched and still gated
by `shouldPollStepsRuntime()`, so completed opens do a single fetch and stop
(no CA-508 `[stop]` regression, no CA-514 dead-runner flood).

## Provider impact

Agnostic. No provider adapter or SSE path touched; the open/refresh logic
branches on run kind / mode only. Tested explicitly with Claude, Codex, and
Grok provider keys.

## Tests

`tui_open_completed_steps_test.go` (additive, new file):

- Completed workflow open arms the one-shot steps refresh for each of
  claude/codex/grok while `shouldPollStepsRuntime()` stays false.
- Completed open + orch listener shows no `[stop]` (CA-508 preserved) while
  `stepsPollInFlight` is true.
- Full CA-516 path: open completed → `StepsRuntimeMsg` → `agentRunsHydratedMsg`
  → F2 lists `Steps 2:` with `[open]` rows and no `[stop]`.
- Running workflow open still arms refresh and keeps auto-poll (CA-502
  preserved).

Legacy suite untouched and green (`go test ./internal/tui/... ./internal/cli/...`).

## Residual / out of scope

- Runner `RunSnapshot` still carries no runKind/workflowId/flowRef (TUI-only
  scope per operator); the F2 restore relies on the resume handle + history
  meta, which already carry the flow fields (CA-502).
- Transcript replay on `/open` of old flows is a separate, unconfirmed issue;
  this change is scoped to the F2 step timeline only.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Fire one-shot cmdRefreshStepsRuntime on every /open of a flow run (incl. completed) so the F2 step timeline renders with [open]; keep cursor auto-poll gated by shouldPollStepsRuntime (CA-508/514)
# --->8---