# CA-730 — DB model carry delegate-only, stale non-delegate ignored (BUG-352)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-352
change_type: bugfix
summary: recordFromWorkflowRow only carries step model onto agent.delegate nodes so a legacy model on context.produce and other behaviors stays ignored instead of tripping pack validation and failing flow load with 422
# --->8---

## Problem

- Task-320 regression (live 2026-09-04): starting `/flow task-harness`
  failed immediately with `422 invalid_flow_definition` — the mirrored
  `context` node (`context.produce`) carried a stale `gpt-5.4` from its step
  row onto `node.Model`, tripping the delegate-only fail-closed rule.
- Pre-Task-320 the stale value was silently ignored at runtime.

## Changes

- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`:
  `recordFromWorkflowRow` gates the `defn.Model` carry on
  `NormalizeBehaviorID(node.Behavior) == "agent.delegate"`.
- `apps/local-runner/internal/runner/node_model_resolution_test.go` (new):
  `TestRecordFromWorkflowRowDropsNonDelegateStaleModel`.
- `requirements/09-BugFix/todo/BUG-352-*.md`: status done.
- No DB change needed: the stale `gpt-5.4` row stays, now harmless.

## Verification

- `go test ./internal/runner/ -run
  'TestResolveFlowNodeModel|TestProviderKeyFromModel|TestRecordFromWorkflowRow'
  -count=1`: 14/14 PASS, zero pre-existing edits.
- Live-verify pending: rebuild + restart runner, `/flow task-harness`
  passes validation into S2.
