# CA-053: Phase 8 A1 Interactive Orchestrator Bridge

## Summary

- `InteractiveService` now owns a `WorkflowStore` and `WorkflowOrchestrator` instead of leaving the live interactive path fully outside the workflow orchestration boundary.
- New runs seed the in-memory workflow store with the requested step when the fake store is active.
- Turn start now marks the active step `RUNNING`, and a clean turn completion advances the seeded run through `WorkflowOrchestrator.Progress`.
- A typed `workflow_state_unavailable` error now surfaces when the workflow-state bridge cannot record turn start.

## Changed Files

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`
- `apps/local-runner/internal/runner/phase8_a1_test.go`
- `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`

## Verification

- `go test ./internal/runner -run 'Test(NormalTurnPersistsWithSeq|ServiceUsesInjectedCatalogStore|ServiceListsInjectedWorkflowsGlobally|ServiceListsInjectedStepsGlobally|ServiceCatalogErrorSurfaces|SkillsServedLocallyWithInjectedStore|CatalogStoreForFallsBackToFake|CatalogStoreForUsesSupabaseWhenConfigured|StartTurnSurfacesWorkflowStateStoreErrors)$'`

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-053
change_type: feature
summary: Phase 8 A1 Interactive Orchestrator Bridge
# --->8---
