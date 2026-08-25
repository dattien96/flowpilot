# CA-634: run-144900 — pre-existing skill drift false park + hub-ful Continue fakes synthesis

<!-- flowpilot:change-ledger -->
```yaml
schema_version: 1
change_id: CA-634
feature_key: change-contract
parent_change_id: CA-627
date: 2026-08-25
author: main
summary: Subtract unchanged pre-existing dirt from frozen writer gate and retry writer on Continue even when hub.inline exists (run-144900)
intent: Fix false scope-drift park on leftover untracked .agents/.claude/.grok skill dirs and 5-minute fake synthesis hang on hub-ful Continue
declared_paths:
  - apps/local-runner/internal/runner/gate_hook.go
  - apps/local-runner/internal/runner/flow_validate_audit_dispatch.go
  - apps/local-runner/internal/runner/interactive_service.go
  - apps/local-runner/internal/runner/run144900_preexisting_skill_drift_and_hubful_continue_test.go
  - change-audit/CA-634-run144900-preexisting-skill-drift-and-hubful-continue.md
```
<!-- /flowpilot:change-ledger -->

## Context & Problem (run-144900, rag-harness grok-4.5)

1. `?? .agents/skills/flow-mode-orchestrator/SKILL.md` existed untracked since 10:28 before the flow started (similar copies under `.claude/skills` and `.grok/skills`). Frozen contract declared `format.go` + `format_test.go`. Tester `test_signatures` wrote only `format_test.go` and was correctly within scope.
2. Frozen writer gate compared `ObserveGitDiffSince(BaseSHA)` verbatim. The leftover appeared as `writtenAgainstFrozen` and survived the exemption filter (not a runner ledger, not a CA note, not a bookkeeping path). Result: `flow scope drift: wrote outside the frozen contract's declared paths: .agents/skills/flow-mode-orchestrator/SKILL.md` → `WAITING_USER_APPROVAL`. No implement child spawned, no gate `[settle]` on the child turn.
3. User clicked **[Continue]**. `resumeFlowWithFeedback` retried the writer only when `hubInline == ""`. Live rag-harness declares `synthesis` = `hub.inline`, so both `escalatedNodeID` and `failedDelegateNodeID` paths were skipped and fell through to `maybeAutoReinvokeHubWithNote`. Hub was reinvoked as a synthesizer (5 min “review tools” turn, `Thinking 4m55s`) and eventually `changes_requested / coder looping` — a fake review hang.
4. TUI still accepted clicks (not a CA-631/633 input freeze); the hang was the hub turn, not the composer.
5. Root causes: (a) freeze `BaselineWorktree` capture used `uncommittedChangedPaths` which excludes `*.md` and caps at 20, so the leftover was never in the baseline; (b) frozen gate did not subtract pre-existing dirt via `turnStartWorktree`; (c) Continue routing was tested only on hub-less `ragHarnessNodes()` (no synthesis) in CA-627.

## Changes Made

1. **F-1: baseline capture (`flow_validate_audit_dispatch.go:baselineWorktreeFingerprint`)**  
   Captures via `flowgate.ObserveGitDiff` (full dirty snapshot, includes `.md`, no cap) instead of `uncommittedChangedPaths`. A leftover `SKILL.md` present at freeze time is now in `FrozenContractRecord.BaselineWorktree`.

2. **F-2: frozen writer gate (`gate_hook.go:runChildArtifactOutputGateAtEpoch`)**  
   After code-only filtering, subtracts unchanged pre-existing files: if `rs.turnStartWorktree[p]` or `rec.BaselineWorktree[p]` equals current `worktreeFileFingerprint(cwd, p)`, the path is dropped. Only new or mutated files can drift. Does **not** exempt whole `.agents/**` (CA-427) — a fresh or modified skill still blocks.

3. **F-3: Continue routing (`interactive_service.go:resumeFlowWithFeedback`)**  
   Writer escalate (`agent.code` / `agent.delegate`) now always maps `escalatedNodeID` → `failedDelegateNodeID` and retries via `reinvokeMatchingFlowChild` even when `hubInline != ""`. Inline behaviors (`contract.freeze`, `command.validate`, etc.) still `tryAdvanceFlowThroughInline`. Hub `submit_review_outcome` path only reached for genuine hub parks.

## Validation

- `go vet ./internal/runner ./internal/changecontract ./internal/agentpack` clean.
- New file `run144900_preexisting_skill_drift_and_hubful_continue_test.go` — 8 tests, all green per provider matrix (grok/codex/claude):
  `TestRun144900_PreexistingSkillDoesNotCauseScopeDrift`, `TestRun144900_PreexistingMultipleSkillCopiesDoNotDrift`, `TestRun144900_TrueDriftViaExtraFileStillBlocks`, `TestRun144900_MutatedLeftoverStillBlocks`, `TestRun144900_TurnStartWorktreeAloneHidesPreExistingDirt`, `TestRun144900_ContinueOnHubFulFlowReinvokesWriterNotHub`, `TestRun144900_ContinueOnHubFulFlowReinvokesImplementNotHub` (each 3 subtests).
- Related old patterns green: `TestBUG327_*`, `TestBaselineWorktreeFingerprint*`, `TestRunContractFreeze*`, `TestFlowCoder*`, `TestRAGHarness*` — `go test -count=1 -run "TestBUG327_|TestBaselineWorktreeFingerprint|TestRunContractFreeze|TestFlowCoder|TestRAGHarness"` pass 19s.
- Provider parity: Case 1 agnostic — changed functions (`runChildArtifactOutputGateAtEpoch` frozen branch, `baselineWorktreeFingerprint`, `resumeFlowWithFeedback` writer path) do not branch on `providerKey` (grep `providerKey` on changed lines: 0 hits). Tests are provider-parameterized anyway.

## Will not undo

CA-427 (no wholesale `.flowpilot/**` or `*.md` exemption), CA-616 hub-less retry, CA-627 ledger/CA exact-path exemption + stamp, CA-632/633 TUI fixes, CA-629 live back-edge.

## Residual

Leftover `.agents/.claude/.grok` skill dirs can remain on gate-sandbox; gate now hides them when unchanged. A future write that genuinely touches an undeclared `.agents/**` path still drifts.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-327
change_type: bugfix
summary: Subtract unchanged pre-existing dirt from frozen writer gate and retry writer on Continue even when hub.inline exists (run-144900)
# --->8---
