# CA-175: Validate Stored Flow Definitions at Resolve Time (BUG-NOTE-CP42 #28)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: a flow definition loaded from Supabase (a mirrored built-in or a user-owned flow) never went through the same validation the embedded pack gets for free at load time.

## The bug

`agentpack.LoadFlowFS` runs `validateFlowDefinition` on every embedded flow file, rejecting an unknown `behavior_id` at pack-load time. `SupabaseWorkflowFlowStore.recordFromWorkflowRow` copies whatever string sits in the `behavior_id` column straight into `FlowNode.Behavior` with no validation at all. The executor then silently skips any node it doesn't recognize (`entryDelegateNodes`/`forwardDoneTargets` just never match it) instead of erroring — so a workflow with a typo'd or free-text `behavior_id` (the Settings UI lets a user type this field directly) would resolve successfully and then spawn nothing, the exact "silent no-op" failure mode BUG#9/#15 also describe for other gaps in this same area.

## Fix

Exported `agentpack.validateFlowDefinition` as `agentpack.ValidateFlowDefinition` (its one production caller, `LoadFlowFS`, updated to match; its two existing tests too). `FlowDefinitionResolver.ResolveFlowRef` and `ResolveBuiltin` (`flow_definition_resolver.go`) now call it on every record returned by the store — both the `GetByRef` (user-owned flow lookup) and `GetByPackFlow` (built-in mirror lookup) branches — before returning it to the caller. A garbage `behavior_id` now surfaces as a clear resolution error instead of a silent no-op. The embedded-pack fallback path is unaffected: it's already validated once at `LoadFlowFS` time, and running the same check again there is harmless (idempotent, no side effects).

## Verification

- New test `TestFlowDefinitionResolverRejectsStoredDefinitionWithUnknownBehavior` (`flow_definition_resolver_test.go`): stores a fake record with `Behavior: "totally.not.a.real.behavior"` and asserts `ResolveFlowRef` returns an error instead of the invalid record.
- Full `internal/agentpack` (12 tests) and `internal/runner` (1006 passed, 15 pre-existing/environmental failures unchanged) suites pass.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: export agentpack.ValidateFlowDefinition and call it on every store-resolved flow definition (mirrored or user-owned) so an unknown behavior_id fails resolution loudly instead of the executor silently skipping the node
# --->8---
