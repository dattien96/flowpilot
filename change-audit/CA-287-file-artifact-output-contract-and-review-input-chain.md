# CA-287 — File Artifact Output Contract And Review Input Chain

Implements Task-223: `file_artifact.v1` is a path-designated deliverable with a write contract on OUTPUT bindings and a read contract on INPUT bindings (coder → review chain), enforced by prompt injection + flow-gate `r-artifact-output`.

## Files Changed

- `apps/local-runner/internal/runner/artifact_type_registry.go` — path helpers, `composeFlowNodeAgentPrompt`, required-output / input-read sections
- `apps/local-runner/internal/runner/flow_executor.go` — inject on entry, inline-chain, and auto-advance review spawns
- `apps/local-runner/internal/runner/interactive_service.go` — continue re-entry re-applies compose
- `apps/local-runner/internal/runner/gate_hook.go` — resolve required OUTPUT paths from flow node label
- `apps/local-runner/internal/flowgate/rules.go`, `evaluate.go`, `enforce.go` — `r-artifact-output` + filesystem check
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` — OUTPUT/INPUT copy
- Tests under `flowgate` and `runner` packages
- `requirements/08-Task/done/Task-223-...md`

## Scope

Phase-1 MVP only. Does not add DMS, snapshots, or mtime-strict "must rewrite" proof.

## Residual Notes

- Pre-existing file at the designated path satisfies existence (Phase 2 may require WrittenPaths/mtime).
- Review inject covers tryAdvanceFlowFromNode + continue re-entry + entry spawn paths.
- Codex NEEDS_FIX follow-up (same CA): child-only `runChildArtifactOutputGate` (BUG-152 full gate still root-only); `MergeDefaultRules` for old `flow-rules.json`; symlink-safe existence check; unlock before compose on continue re-entry.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-223
change_type: feature
summary: file_artifact OUTPUT write contract + gate + INPUT inject for review chain
# --->8---
