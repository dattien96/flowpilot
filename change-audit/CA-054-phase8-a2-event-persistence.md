# CA-054: Phase 8 A2 Provider Event Persistence

## Summary

- Added persistent writes for normalized provider events into `workflow_provider_events`.
- Kept `message_delta` events ephemeral, matching the replay contract.
- Extended the in-memory workflow store so tests can assert persisted event ordering.
- Added request-shaping coverage for the new Supabase event insert path.

## Changed Files

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/workflow_store.go`
- `apps/local-runner/internal/runner/supabase_workflow_store.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`
- `apps/local-runner/internal/runner/phase5_test.go`

## Verification

- `go test ./internal/runner -run 'Test(NormalTurnPersistsWithSeq|SupabaseStoreAppendEventShaping|SupabaseStoreLoadRunStepsShaping|SupabaseStoreApplyTransitionShaping|SupabaseStoreSurfacesHTTPError|ServiceUsesInjectedCatalogStore|ServiceListsInjectedWorkflowsGlobally|ServiceListsInjectedStepsGlobally|ServiceCatalogErrorSurfaces|SkillsServedLocallyWithInjectedStore|CatalogStoreForFallsBackToFake|CatalogStoreForUsesSupabaseWhenConfigured|StartTurnSurfacesWorkflowStateStoreErrors)$'`
