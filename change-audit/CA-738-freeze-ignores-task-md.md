# CA-738 — contract.freeze ignores plan Task md (run-201704)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: the freeze planner-mutation guard now ignores doc/audit surfaces (requirements/**, change-audit/**, *.md via flowgate.IsDocOrAuditFile), so task-harness's plan_writer Task md (requirements/08-Task/todo/Task-*.md, written between flow start and freeze) no longer false-blocks preflight_contract_freeze with "planner changed 1 file(s)"; a planner-written *.go still blocks
# --->8---

## Problem

- Live run-201704 (`/flow task-harness`, S6, `D:\working\gate-sandbox`): CA-737 worked — `plan_synthesis` DONE, freeze dispatch started — but the freeze step parked WAITING_USER_APPROVAL with `Contract freeze blocked: planner changed 1 file(s) (requirements/08-Task/todo/Task-951-gcd-provider-agnostic-euclidean.md); the contract planner must be read-only`.
- Root cause: the guard diffs the flow-start fingerprint against freeze time. task-harness freezes AFTER plan_writer, so the plan phase's own Task md lands in the window and reads as a planner mutation. The check was designed for rag-harness (planner → freeze directly). `isFlowPlannerExcludedPath` covered `.flowpilot`/`.gitnexus`/skillpack (CA-640/645) but not doc surfaces — while the gate scope path already treats them as docs (`IsDocOrAuditFile`).
- (`Turn failed: interrupted by user` in the screenshot is the operator interrupting after the card, not a separate bug.)

## Changes

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`isFlowPlannerExcludedPath`): also exclude `flowgate.IsDocOrAuditFile` paths. Applies to both baseline capture and diff (same defense-in-depth shape as CA-640). A planner-written `*.go` still blocks (CA-426 intact).

## Tests added (new file only)

- `run201704_freeze_ignores_task_md_test.go`:
  - `TestRun201704_FreezeIgnoresTaskMdWrittenAfterFlowStart` (Claude/Codex/Grok): Task md written after baseline → freeze succeeds, coder spawns, loop not blocked.
  - `TestRun201704_FreezeStillCatchesPlannerCodeMutation` (matrix): `.go` written after start → still escalates, no writer spawn.
  - `TestRun201704_WorktreeMutatedSincePathsIgnoresDocs` / `...StillCatchesCode`: pure-diff units.
  - `TestRun201704_BaselineFingerprintExcludesTaskMd`: baseline skips Task md, still captures `real.go`.

## Verification

- `go test ./internal/runner/ -run 'TestRun201704|TestRun201295|TestRun200816|TestRun199617|TestBug353|TestRun198699|TestCA623|TestRunContractFreeze|TestFreeze|TestRun2047|TestRun243681|TestRun151954|TestWorktreeMutatedSincePaths|TestBaselineWorktreeFingerprint|TestApplyFlowControl|TestRun5296|TestFlowSettle|TestBug327|TestRagHarness|TestRun147126' -count=1` PASS; `go test ./internal/flowgate/` PASS. Old tests untouched (R1).
- One run hit the known intermittent Windows TempDir cleanup file-lock race on the new repro test; passes on re-run (same pre-existing environmental flake as `TestRunContractFreezeNodePersistsBeforeCoderSpawn`).
- Provider-agnostic (R2): guard takes no providerKey; repro + near-miss run the Claude/Codex/Grok matrix.
- Will not undo: CA-737 (successor dispatch + fail-closed escalate), CA-640/645 (runtime/scaffold exclusions), CA-426 (genuine planner code mutation still blocks).
- `go vet ./internal/runner/ ./internal/flowgate/` clean; runner binary builds.