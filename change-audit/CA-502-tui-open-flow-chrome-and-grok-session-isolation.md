---
id: CA-502
feature_key: cli-tui
title: /open restores flow chrome; Grok refuses sibling session steal
date: 2026-08-14
status: COMPLETE
---

## Change

### TUI `/open` flow chrome (cli-tui)

Opening run-98153 left ModeChat, no Steps panel. `ChatOpenedMsg` now calls
`applyOpenedRunFlowChrome` from resume handle (+ history list fallback):
workflow → ModeFlow + LaunchArm + `cmdRefreshStepsRuntime`. Plain chat clears
stale flow arm/steps.

### Resume handle fields (runner DTO, additive)

`RunHandle` gains optional `runKind`, `workflowId`, `flowRef` on resume so TUI
does not depend only on the history picker cache.

### Grok session isolation (runner)

run-98153 dual reviewers shared one ACP `provider_session_id`. Fixes:

- `grokRunSessionIndex` reverse map: refuse `remember` if another run owns the id
- `ensureSession`: force `session/new` when load target is owned by another run
- `isForeignProviderSessionID`: durable (non-`thread-*`) ids for Grok too
- `refreshResumeHandleLocked` (Grok): skip promote when id is foreign sibling

Does not undo CA-501 seed filter, CA-500 cohort heal, run-536 no cwd-steal.

## Provider impact

TUI chrome: agnostic. Grok isolation: Case 3 (Grok adapter/index only);
Claude/Codex already had foreign checks for rollouts.

## Tests

New `tui_open_flow_chrome_test.go`, `run98153_grok_session_isolation_test.go`.
Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: /open restores flow mode+steps; Grok blocks sibling ACP session steal
# --->8---
