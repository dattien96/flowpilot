---
id: CA-519
feature_key: cli-tui
title: /open continue sends stepId; child-focus composer locked read-only
date: 2026-08-15
status: COMPLETE
---

## Problem

Two TUI regressions reported after CA-516/517/518 (screenshot run-193749):

1. **Continue after `/open` of a completed flow failed with
   `stepId is required` (400).** Root cause: `openTurnStream` derived the turn's
   `stepId` from only `m.stepID` → `runHandle.StepID`, then stopped. The runner
   only mints a synthetic `"chat-<runId>"` step on the handle for **normal chat**
   runs (`resumeRun`, interactive_handlers.go:901 — `if rs.runKind == "chat"`);
   workflow/flow runs intentionally leave `handle.StepID` empty so the client
   drives the step. So after `/open <completed-flow>` the TUI POSTed
   `/client/workflow-runs/{id}/turns` with an **empty stepId**, and
   `startTurn` rejects that with 400 `"stepId is required"`
   (interactive_service.go:7207). This is the runner's existing T-7 contract —
   the regression is the TUI failing to send a stepId, which Desktop never does
   (`store.sendPrompt` always sends `activeStepId`/`launchTargetId`).

2. **Chat composer stayed editable while a sub-agent transcript was focused.**
   `canSend()` already returned false for `viewingChild()`, but the `chat` input
   line still rendered with an editable caret, and printable keys still
   inserted text (only Enter-send was rejected by `processInput`). Desktop locks
   the child view: composer read-only, placeholder "Child transcript is
   read-only."

## Fix (TUI-only)

**A. `resolveTurnStepID()` (helpers.go)** — mirrors Desktop `store.sendPrompt`:

- `m.stepID` (persisted after open/start)
- `runHandle.StepID` (synthetic `chat-<runId>` for normal chat)
- `launch.StepID` (direct step-mode arm)
- `launch.WorkflowID` (catalog workflow open — the workflow id, same as Desktop's
  `launchTargetId` for workflow mode)
- `"chat-"+runID` last resort for a chat run with no handle step

`openTurnStream` (turn_stream.go) now uses the resolver; `ChatOpenedMsg`
(app.go, after `applyOpenedRunFlowChrome`) persists the resolved step id into
`m.stepID` so the fallback is stable for every subsequent turn. A resumed
workflow run therefore POSTs `stepId=<workflowId>` instead of empty — no 400.

**B. Child-focus composer lock (CA-519)**

- `renderInputLine`: while `viewingChild()`, render a read-only banner
  ("Child transcript is read-only — chat continues on main") instead of an
  editable `chat` box.
- `handleKey`: new `allowsKeyWhileViewingChild(msg)` gate (twin of the existing
  `allowsKeyWhileLoading`) — only navigation, F2/F3/F4, `[back]`/Esc, Tab agent
  cycle, Ctrl+V/copy, and slash commands pass. Printable chat text and
  Enter-to-send are dropped.

## Provider impact

Provider-agnostic. The step-id resolver and the composer lock branch on run
kind and focus state only — no provider adapter or provider-key dispatch is
involved (`resolveTurnStepID`/`allowsKeyWhileViewingChild`/`renderInputLine`
never read `providerKey`). All new tests parameterize Claude / Codex / Grok so a
future per-provider branch trips the guard immediately (cross-provider-parity
Case 1 + parameterized guard).

## Tests

`tui_child_composer_lock_stepid_test.go` (additive, new file):

- `resolveTurnStepID` fallback chain: handle step wins; workflow open →
  workflow id; step-mode step id beats workflow; chat run → synthetic
  `chat-<runId>`.
- `ChatOpenedMsg` workflow open persists resolved step id for
  claude/codex/grok; normal-chat open keeps the synthetic step id.
- `openTurnStream` actually POSTs `stepId=<workflowId>` (never empty) for a
  resumed workflow, via httptest, for claude/codex/grok.
- `canSend` blocks child focus for claude/codex/grok.
- `renderInputLine` shows the read-only banner in child view and restores the
  editable composer on main.
- `handleKey` drops chat text and Enter in child view, still allows `/` and Tab.

Legacy suite untouched and green (`go test ./internal/tui/... ./internal/cli/...`,
build + vet clean; focused re-run of agent-focus / input / open tests).

## Out of scope / residual

- Runner `startTurn` step validation is unchanged (still requires a non-empty
  `stepId`; workflow id is accepted the same way Desktop sends it).
- `allowsKeyWhileLoading` remains unused (pre-existing); the new child-view
  twin is the live consumer.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Continue after /open of a workflow/flow run now sends a resolved stepId (workflow id / synthetic chat step) instead of an empty one that startTurn rejects with 400; the child-focus composer is locked read-only with only navigation and slash commands allowed
# --->8---