# BUG-352: DB-carry mang stale model của node non-delegate làm sập cả flow (422)

## Metadata

- Document ID: `BUG-352`
- Title: `DB-carry mang stale model của node non-delegate làm sập cả flow (422)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [Task-320](../../08-Task/inprogress/Task-320-Per-Node-Model-Tiering-For-Harness-Delegate-Nodes.md)
- Child Documents: `none`
- Related Documents: [CA-616](../../../change-audit/CA-616-run135037-hubless-planner-fail-waiting.md)
- Replaces: `none`
- Tags: `agent-flow-engine, task-320-regression, validation, mirror`

## AI Quick View

### Summary

- Symptom (live 2026-09-04): start `/flow task-harness` fail ngay `Send turn failed: runner API error 422 (invalid_flow_definition)`: node `context` (`context.produce`) declares model `gpt-5.4` nhưng behavior không consume model.
- Root cause (regression Task-320): row `step_definitions` của node `context` còn sót `model='gpt-5.4'` từ seed cũ — trước Task-320 giá trị này bị runtime ignore lặng lẽ. T-6 carry nó lên `node.Model` cho MỌI behavior, rồi T-3 fail-closed reject → sập cả flow load.
- Fix: `recordFromWorkflowRow` chỉ carry `Model` khi behavior canonical là `agent.delegate`; stale model trên behavior khác giữ nguyên ignored như pre-Task-320 (đúng cả runtime `resolveFlowNodeModel` vốn return `""` cho non-delegate).
- Không đụng DB: row stale `gpt-5.4` cứ nằm đó, vô hại sau fix.

### Current Ask

- Start lại `/flow task-harness` sau rebuild+restart runner → qua validation, vào S2.

## Fix (2026-09-04, done)

- `internal/runner/supabase_workflow_flow_store.go`: gate `defn.Model` carry bằng `NormalizeBehaviorID(node.Behavior) == "agent.delegate"`.
- Test mới `TestRecordFromWorkflowRowDropsNonDelegateStaleModel` (row `context.produce` + `gpt-5.4` → `node.Model == ""` + `ValidateFlowDefinition` nil); `TestRecordFromWorkflowRowCarriesNodeModel` (delegate) vẫn green.
- Verification: `go test ./internal/runner/ -run 'TestResolveFlowNodeModel|TestProviderKeyFromModel|TestRecordFromWorkflowRow'` PASS (14/14).
- Không sửa pre-existing tests.
