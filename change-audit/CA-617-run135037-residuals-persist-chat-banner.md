# CA-617: Residuals of CA-616 — persist, single-child, late note + banner

## What

CA-616 shipped. Residual review found 3 non-blocking gaps: (1) Continue could spawn a 2nd planner when `stepID==""`, (2) `lastFailedDelegateNodeID` only RAM → restart loses Continue, (3) chat `(no detail)` when note arrives late, banner `delegate_failed` without 400 text.

## Why

1. `reinvokeMatchingFlowChild` did `if runID=="" || stepID=="" return false` after mutating child to `Running`. A pre-adapter fail with `stepID==""` left child `Running` then returned `false` → resume spawned a 2nd child (same label). Test fell through to spawn fallback (`run-6`).
2. `lastFailedDelegateNodeID`/`lastEscalatedInlineNodeID` not in `ProviderSessionState` / `ndjsonSessionRecord` → `reconstructRun` lost it.
3. `formatStepChatNotices` only `RUNNING→FAILED`. First poll already `FAILED` or `FAILED "" → FAILED "gpt-5.4"` never emitted. `showBlockedBanner` only `blocked:`.

## Fix

- **P1 single-child:** `flow_executor.go:1358` fill `stepID = lastTurnStepID || "retry-"+id`; guard `if runID==""` (not `stepID==""`). Resume fallback spawns only when no reusable child with label exists. Will not create `run-6` when `run-3` reusable (`reinvoke` flips same `RunID`).
- **P2 persist:** `workflow_store.go` `ProviderSessionState.LastFailedDelegateNodeID/LastEscalatedInlineNodeID` + `interactive_service.go` `sessionStateOf` copy + `interactive_resume.go` restore + `local_file_session_store.go` record + both `r→s` and `s→ProviderSessionState` converters. Fallback in `resumeFlowWithFeedback`: if RAM empty && `prevBlockReason==delegate_failed` && `hub-less` → `LoadRunSteps` infer `preflight_contract_plan` `FAILED` (+ note fallback), reload `nodes/edges` if empty.
- **P3 chat+ banner:** `step_runtime.go` `formatStepChatNotices` adds late-note loop: first poll `FAILED+note` and `FAILED "" → FAILED "note"` each emit `formatStepChatLine`; keeps `RUNNING→FAILED` as 1 line. `showBlockedBanner` appends `GateReason` (truncated 120) when `delegate_failed`.
- Provider-agnostic: no `ProviderKey` branch; matrix `claude/codex/grok` in new tests.

Will not undo: CA-616 park/inherit; CA-355 hub reinvoke; CA-230/239/241 resolve; `step_chat_notice_test.go` unchanged.

## Tests (additive)

- `run135037_delegate_fail_restart_fallback_test.go` (runner, table 3 provider via codex mutate): `TestRun135037_DelegateFailPersistsAcrossReconstruct` (session + reconstruct + fallback `FAILED` step → `RUNNING`), `TestRun135037_ContinueWithEmptyStepIDStillReusesSameChild` (empty `stepID/lastTurnStepID` → same `RunID`, not `Failed`, step `RUNNING`, no 2nd planner).
- `run135037_tui_failed_notice_late_note_test.go` (tui/app, 3 provider): first-poll `FAILED+note`, late `FAILED "" → FAILED note`, `RUNNING→FAILED` still 1 line, banner `delegate_failed` + `gpt-5.4`.
- Old: `go vet`, `go test ./internal/runner -run TestRun135037|TestNonCohort|TestHubStall|TestPark`, `go test ./internal/tui/app -run TestRun135037|TestFormatStepChatNotices|TestBlockedBar` pass.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: persist delegate fail + single-child Continue + late note/banner (CA-616 residuals)
# --->8---
