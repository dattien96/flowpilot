# CA-211: Flow-Mode Workflow-Picker Executor Engagement And Step-Timeline Wiring

## Summary

Fixed `BUG-174`: a "review-loop" run started from the Flow-Mode workflow picker did not orchestrate — the hub agent did all the work inline, no coder/reviewer sub-agents spawned, the step timeline stayed stuck on step 1, and at the end all steps flipped to DONE at once, out of causal order. Root cause was two disjoint subsystems: the workflow-picker launch used the legacy CP-19 catalog-step path (which sends no `flowRef`, so the CP-42 flow executor never engaged) and the executor was never wired to the step-runtime timeline. This bridges the workflow-picker launch to the executor and drives the timeline from the executor's real node lifecycle.

## What Changed

- `apps/local-runner/internal/runner/flow_step_runtime.go` (new): `flowEngineDriven` gating helpers (`isFlowEngineDriven`/`markFlowEngineDriven`), `reseedFlowStepRuntime` (rebuild the step list one-per-flow-node), `setFlowStepStatus` (transition a node's step by id), `markFlowRunComplete` (terminal hub node DONE + run DONE), and `hubInlineNodeID`/`flowEntryNodeID` helpers.
- `apps/local-runner/internal/runner/flow_executor.go`: added `resolveWorkflowFlowRef` (resolves a run's `workflowID` to a flow's canonical `flowRef` via the flow definition store's UUID-accepting `GetByRef`); `startResolvedFlow`/`startInlineEntryChain` now reseed steps from flow nodes and mark the entry node RUNNING; `tryAdvanceFlowFromNode` now marks the completed node DONE and its forward targets RUNNING — all gated on `flowEngineDriven`.
- `apps/local-runner/internal/runner/interactive_handlers.go`: `handleStartTurn` adopts the auto-resolved workflow `flowRef` and marks the run flow-engine-driven when no explicit `flowRef` was sent.
- `apps/local-runner/internal/runner/interactive_service.go`: added the `flowEngineDriven` run field; skip the bulk `orchestrator.Progress` planner for flow-engine-driven runs; the review-cohort join marks reviewers DONE + the inline hub node RUNNING; `applyFlowControl` marks the run complete on "done" and resets/re-runs the entry node on "continue".
- `apps/local-runner/internal/runner/flow_step_runtime_test.go` (new): unit tests for reseeding, node transitions, coder→reviewer advancement, the Progress gate (with a non-flow control), and the `workflowID→flowRef` bridge.

All new behavior is gated behind `flowEngineDriven`, set only for the workflow-picker auto-resolved path, so the explicit chat/flowRef (bugfix-tab) path and plain workflows keep their exact prior behavior.

## Verification

- New tests in `flow_step_runtime_test.go` pass; full flow/workflow/orchestration/step-runtime subset (`go test -run 'Flow|Workflow|Orchestrat|Cohort|Coder|Reviewer|Progress|StepRuntime|Advance|Resolve'`) → 284 passed. `go build ./internal/runner/` clean.
- The ~15 unrelated failures in the full package run were confirmed pre-existing/environment-specific by stashing this change and re-running the same subset on a clean tree (identical failures: codex-resume needs a real binary; Windows path/home tests; `TestSkillsMerge*WithPrecedence`/`TestStartInteractiveAuthLaunchesFromWorkspace`, already flagged in BUG-169/170).
- Not verified live end-to-end (a real cohort spawn/synthesis against a provider) — no runner + provider account available; flagged in `BUG-174` (`V-4`).

## Notes

- GitNexus MCP tools were unavailable this session for the mandated pre-edit impact analysis on the HIGH-blast-radius symbols (`startTurn`, `orchestrator.Progress`, `startResolvedFlow`, `tryAdvanceFlowFromNode`); proceeded with careful manual inspection, keeping every change additive and gated.
- Sibling cosmetic fix `BUG-173` (expanded step-timeline numbering) and `BUG-172` (black-screen error boundary) are separate, already-landed items from the same user report.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-174
change_type: bugfix
summary: Engage the flow executor for Flow-Mode workflow-picker launches of built-in flows and drive the step timeline node-by-node, gated behind flowEngineDriven, replacing the inline hub turn + bulk step completion
# --->8---
