# BUG-245: Chat Mode Review Loop Runs Never Marked Flow-Engine-Driven

## Metadata

- Document ID: `BUG-245`
- Title: `Chat Mode Review Loop Runs Never Marked Flow-Engine-Driven`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [Task-177: Review Loop Pack And Chat Picker](../../08-Task/todo/Task-177-Review-Loop-Pack-And-Chat-Picker.md), [BUG-174: Flow-Mode Workflow-Picker Run Does Not Orchestrate](../done/BUG-174-Flow-Mode-Workflow-Picker-Run-Does-Not-Orchestrate-No-Agent-Spawn-Steps-Bulk-Completed.md)
- Child Documents: `none`
- Related Documents: [BUG-234: Loop-Back Lifecycle Runaway Advance And Cohort Node Settlement](../done/BUG-234-Loop-Back-Lifecycle-Runaway-Advance-And-Cohort-Node-Settlement.md). Code comments also reference a "BUG-226" no-tool-call escalation safety net (`interactive_service.go`), but no `requirements/09-BugFix/BUG-226-*` document exists in this checkout — a pre-existing doc-tracking gap (same class as Task-177's dangling `CA-152/156/158/160` references), not something this document's fix touches or resolves.
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, chat-mode, flow-engine-driven, regression`

## AI Quick View

### Summary

- Chat Mode's "Bug" sub-mode picker sends an explicit `flowRef` on the first turn (e.g. `flowpilot-core-flow-pack/review-loop`), which `startTurn` correctly resolves and spawns via `startResolvedFlow` — the exact same function Flow Mode uses. Agents visibly spawn; the loop appears to run.
- What silently never happened: the run's `flowEngineDriven` flag was never set for this path. `handleStartTurn` only called `markFlowEngineDriven` inside its `resolveWorkflowFlowRef` branch — the Flow-Mode workflow-picker bridge (BUG-174) — which only fires when the request carries no `flowRef` at all. Chat Mode always sends `flowRef` directly, so that branch (and the flag) was always skipped.
- Every `isFlowEngineDriven`-gated behavior was therefore silently disabled for every Chat-Mode-triggered Review Loop run: the legacy bulk step planner kept running alongside the executor (the exact double-planner bug BUG-174 fixed for Flow Mode, regressed for Chat Mode), BUG-226's no-tool-call escalation safety net never fired, and BUG-234's per-node cohort step settlement was skipped — while the flow's actual agent spawning/orchestration worked correctly.
- Not caught by the existing test suite: every "chat mode + flowRef" test in the repo calls `svc.startTurn(...)` directly, bypassing `handleStartTurn` (the real HTTP handler a live desktop turn hits) — so none of them exercised the code path where the flag was actually being dropped.
- Fix: set `flowEngineDriven` inline inside `startTurn` itself, at the single point that decides to call `startResolvedFlow`, rather than in the caller (`handleStartTurn`). Both the Flow-Mode workflow-picker path and the Chat Mode explicit-`flowRef` path set `in.FlowRef` before reaching `startTurn`, so one fix point now covers both by construction, and any future caller of `startTurn` with a `FlowRef` is covered automatically.

### Current Ask

- Owner priority: "Review Loop build-in phải work trong CHAT MODE" (Flow Mode was already owner-tested; Chat Mode was not). Investigation surfaced this as the concrete gap once the owner separately decided to retire the model-driven `agent-review-loop` skill trigger (a local-only, gitignored file, never distributed with the product — see Constraints), leaving the "Bug" sub-mode picker's `flowRef` path as the sole supported way to trigger Review Loop from Chat Mode.

### Key Decisions

- `D-1` Fix at the `startTurn` call site, not by duplicating `handleStartTurn`'s branch logic for the chat path. `handleStartTurn`'s own `markFlowEngineDriven` call (in its `resolveWorkflowFlowRef` branch) is now redundant and removed — `startTurn` is the single source of truth for this flag going forward.
- `D-2` Do not call the existing `markFlowEngineDriven` helper method from inside `startTurn`: the helper re-acquires `s.mu`, which `startTurn` already holds across this entire code region — calling it there would deadlock. The flag is set directly on the already-locked `rs` instead (`rs.flowEngineDriven = true`), which is exactly what the helper does internally.
- `D-3` Added a genuine HTTP-level regression test (`TestChatModeHandleStartTurnMarksRunFlowEngineDriven`) that drives the real `POST /client/workflow-runs/{runId}/turns` route with the exact `subMode`/`flowRef` payload the desktop sends, rather than calling `startTurn` directly like every pre-existing "chat + flowRef" test — closing the coverage gap that let this regression through unnoticed.

### Constraints

- Out of scope: the `agent-review-loop` skill (a model-driven, markdown-instruction-based alternate trigger for Review Loop via a plain prompt) was separately retired by owner decision in this same work cycle — it lived only as a local, `.gitignore`d file (`.claude/skills/`, `.agents/skills/`, `.codex/skills/` are all excluded per `.gitignore` line 44-46) and was never committed to this repository or distributed via any code-level scaffolding mechanism (searched; none found). Its removal produced no git diff and is not otherwise tracked in this document.
- Scoped to the `flowEngineDriven` flag itself; no change to `startResolvedFlow`, cohort/join logic, or the flow definition resolver.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` `startTurn` — `rs.flowEngineDriven = true` now set inline alongside `flowStartOnly = true`.
- `apps/local-runner/internal/runner/interactive_handlers.go` `handleStartTurn` — redundant `markFlowEngineDriven` call removed from the `resolveWorkflowFlowRef` branch; stale `turnBody.SubMode`/`FlowRef` doc comment ("not wired yet") corrected.
- `apps/local-runner/internal/runner/chat_builtin_orchestration.go` `validateChatOrchestrationSelection` — stale doc comment corrected to point at `startTurn`.
- `apps/local-runner/internal/runner/flow_executor_test.go` `TestChatModeHandleStartTurnMarksRunFlowEngineDriven` — new HTTP-level regression test.

## 1. Issue Summary

A Review Loop run started from Chat Mode's "Bug" sub-mode picker (explicit `flowRef` on the first turn) spawned agents correctly but was never flagged `flowEngineDriven`, silently disabling the legacy-planner suppression, the no-tool-call escalation safety net, and per-node step settlement for that run — while the identical launch through Flow Mode's workflow picker was flagged correctly.

## 2. Parent Links

- Surfaced while scoping the owner's stated Priority 1 ("Review Loop must work in Chat Mode"; Flow Mode already owner-tested) — Task-177 (chat picker wiring) and BUG-174 (the Flow-Mode bridge whose `markFlowEngineDriven` call this bug's fix generalizes).

## 3. Environment and Reproduction

- environment: Chat Mode, "Bug" sub-mode, "Review Loop" selected from the built-in orchestration picker.
- reproduction (before fix): `POST /client/workflow-runs` (chat mode) then `POST .../turns` with `subMode: "bug"`, `flowRef: "flowpilot-core-flow-pack/review-loop"` — the coder entry node spawns, but `svc.isFlowEngineDriven(runID)` reads `false` for the run's entire lifetime.
- frequency: deterministic, every Chat-Mode-triggered flow launch.

## 4. Expected vs Actual

- expected: a Chat-Mode-triggered Review Loop run is flagged `flowEngineDriven`, exactly like a Flow-Mode workflow-picker launch, so the flow executor owns its step timeline and safety nets.
- actual: the flag stayed `false` for the run's entire lifetime; agents still spawned via `startResolvedFlow`, but every `isFlowEngineDriven`-gated behavior was inactive.

## 5. Impact

- users affected: any Chat Mode "Bug" sub-mode Review Loop run.
- severity: medium — the flow still executes and produces a result in the common case, but silently regresses BUG-174 (legacy bulk step planner double-counts step completion), disables BUG-226's stall-prevention escalation (a prose-only hub turn can silently stall with no user-visible recovery), and disables BUG-234's cohort step settlement (multi-reviewer round timeline can misreport).

## 6. Root Cause

- confirmed cause: `handleStartTurn` (`interactive_handlers.go`) called `s.markFlowEngineDriven` only inside the block that resolves a Flow-Mode workflow-picker's `workflowID` into a `flowRef` (`if strings.TrimSpace(body.FlowRef) == ""`). Chat Mode always sends `body.FlowRef` non-empty from the client, so that whole block — and the flag-setting call inside it — was always skipped for the chat path. `startResolvedFlow` itself only reads `isFlowEngineDriven`; nothing else in the spawn path ever set it.
- confirmed via direct code inspection (`markFlowEngineDriven` had exactly one production call site) and by reproducing the gap with a new HTTP-level test that failed before the fix and passed after.

## 7. Fix Strategy

- `F-1` Set `rs.flowEngineDriven = true` directly inside `startTurn`'s existing `if flowRef := strings.TrimSpace(in.FlowRef); flowRef != ""` block (the same block that launches `go s.startResolvedFlow(...)`), since `s.mu` is already held there and both the Flow-Mode and Chat-Mode paths converge on this one call site via `in.FlowRef`.
- `F-2` Remove the now-redundant `markFlowEngineDriven` call from `handleStartTurn`'s `resolveWorkflowFlowRef` branch.
- `F-3` Correct two stale doc comments (`interactive_handlers.go`'s `turnBody` struct, `chat_builtin_orchestration.go`'s `validateChatOrchestrationSelection`) that claimed flow resolution/execution was "not wired yet" — it was wired; only the flag was missing.

## 8. Validation

- `V-1` `go build ./...` — clean.
- `V-2` `TestChatModeHandleStartTurnMarksRunFlowEngineDriven` (new) — confirmed to FAIL without the fix (temporarily reverted `rs.flowEngineDriven = true` and re-ran) and PASS with it restored.
- `V-3` Full `go test ./internal/runner/...` — 1093 passed (was 1092 before this test was added), 15 pre-existing environment failures only (missing real Codex CLI, Windows-specific home-dir/path assertions, unrelated skills-merge tests), identical set before and after this change — zero regressions.

## 9. Regression Guard

- tests: `TestChatModeHandleStartTurnMarksRunFlowEngineDriven` (new, HTTP-level, drives the real `handleStartTurn` route rather than calling `startTurn` directly like every pre-existing chat-flowRef test).

## 10. Follow-Up Document Updates

- none — Task-177's own completion notes already describe the chat picker wiring as implemented; this bug was a gap in that wiring's bookkeeping, not a contract change.
