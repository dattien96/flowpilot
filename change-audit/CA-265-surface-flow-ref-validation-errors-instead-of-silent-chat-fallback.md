# CA-265: Surface Flow-Ref Validation Errors Instead Of Silent Chat Fallback

## Scope

Fixed BUG-270, found while live re-testing BUG-269's own fix: `resolveWorkflowFlowRef` treated a real flow definition failing CP-44/CP-45 validation the same as a workflowID that simply isn't a flow — both silently fell through to a normal chat turn, with no error surfaced anywhere. This made BUG-269's fail-fast fix invisible end to end through the actual "Run Flow" UI path.

## Changes

- `flow_definition_resolver.go`: new `ErrFlowDefinitionInvalid` wraps the 6 validation-failure return sites in `ResolveFlowRef`/`ResolveBuiltin` (agentpack/context-source/artifact-binding checks, both stored and mirrored branches).
- `flow_executor.go`: `resolveWorkflowFlowRef` now does `errors.As` on the resolver's error — a plain error still bails silently and unchanged; an `*ErrFlowDefinitionInvalid` additionally stashes onto the run before the same `("", false)` return. New `takePendingFlowRefInvalidErr` one-shot read-and-clear helper.
- `interactive_service.go`: `interactiveRun` gained `pendingFlowRefInvalidErr`.
- `interactive_handlers.go`: `handleStartTurn` checks the stashed error right after `resolveWorkflowFlowRef` returns false and, if set, writes `422 invalid_flow_definition` instead of falling through to `startTurn`.
- `flow_executor_test.go`: +1 test proving the stashed error is set, names the bad source id, and clears on read.

## Verification

- `go build ./...`, `go vet ./...` — clean.
- `go test ./internal/runner/ -run 'TestResolveWorkflowFlowRefSurfacesArtifactBindingValidationError|TestCustomUserOwnedFlowResolvesSpawnsEntryAndAdvancesEdge'` — both pass.
- `go test ./internal/...` — no new failures beyond the pre-existing, already-confirmed-unrelated environment-dependent flakes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-270
change_type: bugfix
summary: surface a flow definition's real validation failure to the user (422) instead of silently falling back to a normal chat turn on any resolveWorkflowFlowRef error
# --->8---
