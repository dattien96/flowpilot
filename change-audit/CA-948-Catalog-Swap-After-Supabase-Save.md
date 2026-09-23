# CA-948 — BUG-384: re-resolve runner catalog after Supabase config save/reset

## Summary

`/client/projects` served the catalog captured at runner boot forever. Saving a
service-role key therefore had no effect until restart. `InteractiveService`
gains an atomic `catalogOverride` + `SetCatalogStore`; the `PUT`/`DELETE`
`/supabase-config` handlers re-resolve `CatalogStoreFor(instance)` after a
successful mutation. Failed saves never touch the live catalog.

## Files

- `internal/runner/interactive_service.go` — `catalogOverride`, `currentCatalog`, `SetCatalogStore`
- `internal/runner/interactive_handlers.go` — 8 `s.catalog` reads → `s.currentCatalog()`
- `internal/runner/scaffold_handler.go` — `lookupProject` reads via getter
- `internal/runner/flow_executor.go` — model-resolution read via getter (lock no longer needed around an atomic load)
- `internal/cli/root.go` — swap wired after successful PUT + DELETE
- `internal/runner/catalog_reload_test.go` — BUG-384 red→green coverage

## Verified

- `go test -count=1 -race -run TestCatalogSwap ./internal/runner/` — 2/2 pass
- Broader `-run 'Scaffold|Catalog|ResolveConfiguredModel|StepModel|StartRun|Launch'` — 3 failures, all byte-identical on baseline `f28169f2` (pre-existing/env)
- `go build ./...` clean

## Provider parity

Provider-agnostic: catalog store seam only; no adapter/session/gate path touched.
