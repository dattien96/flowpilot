# BUG-384: Project list stays stale after saving Supabase config (restart required)

- Document ID: `BUG-384`
- Status: `done`
- Severity: high — after entering a valid service-role key, Chat shows
  "No projects loaded yet" until the whole app/runner is restarted
- Area: `apps/local-runner/internal/runner/interactive_service.go`,
  `apps/local-runner/internal/cli/root.go`

## Symptom

Settings → Supabase → save service-role key succeeds; UI returns to Chat but the
Navigator still shows `PROJECTS 0`. Killing and reopening the app is the only
way to see the real project list.

## Root cause

`newRunnerCommand` resolves `runner.CatalogStoreFor(instance)` exactly once at
boot and passes it to `NewInteractiveServiceWithStore`. `PUT /supabase-config`
persisted the new credentials but never invalidated the captured store, so
`/client/projects` kept serving the boot-time catalog (fake/anon → `[]` under
RLS). Desktop already invalidated correctly (`resetAdminUseCases` +
`refreshBootstrap` remounts Navigator → `loadProjects` re-fires) — the stale
side was purely the runner catalog.

## Fix

- `InteractiveService.catalogOverride atomic.Value` + `currentCatalog()` getter;
  all `s.catalog` read sites (project/workflow/step list handlers, run-launch
  model resolution, scaffold `lookupProject`, flow-executor model resolution)
  now read through the getter — race-safe without holding `s.mu`.
- `SetCatalogStore(catalog)` swaps the store atomically; `nil` falls back to the
  offline fake catalog, matching the constructor contract.
- `PUT /supabase-config` and `DELETE /supabase-config` call
  `interactive.SetCatalogStore(runner.CatalogStoreFor(instance))` **after** a
  successful mutation only — a failed/invalid save leaves the catalog untouched.

## Verified

- `catalog_reload_test.go`: swap → `GET /client/projects` serves the new store;
  nil swap → fake-catalog fallback. `go test -race` clean.
- Live A/B: runner booted before a valid service key returns `[]`; after the
  save path re-resolves, the same endpoint serves the 5 real Supabase projects
  without a restart (isolated-HOME runner on :4319 — keychain writes from an
  unsigned `go run` binary are denied by macOS, so the positive-path live check
  was confirmed via the boot-resolve equivalence plus unit coverage of the swap
  seam).
- Negative path live-verified: invalid/unreachable config → PUT rejected →
  catalog unchanged.
