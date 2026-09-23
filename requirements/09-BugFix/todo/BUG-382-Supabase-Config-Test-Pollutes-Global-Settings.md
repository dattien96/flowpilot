# BUG-382: Supabase config tests overwrite the real global user settings file

- Document ID: `BUG-382`
- Status: `done`
- Severity: high — a test run silently corrupts real user state and breaks `/client/projects`
- Area: `apps/local-runner/internal/runner/supabase_config.go`, `supabase_config_test.go`
- Related: BUG-377 (same class — test touching live state)

## Symptom

After running `go test ./internal/runner/`, the real file
`~/.flowpilot/settings/supabase-config.json` contained `demo-ref.supabase.co`
— fixture data that only exists in `supabase_config_test.go`. The live runner
then resolved `CatalogStoreFor` against the demo project, so
`GET /client/projects` returned `[]` and the desktop Navigator showed
"PROJECTS 0" while Settings (admin/Supabase direct) listed all 5 real projects.

## Root cause

`persistSupabaseWorkspaceConfig` unconditionally mirrors the config to
`globalSupabaseConfigPath()` (`os.UserHomeDir()/.flowpilot/settings/…`). The
tests stub the workspace (`t.TempDir()`) and secret store (memory) but never
isolated `HOME`, so every `SaveSupabaseWorkspaceConfig` call in tests wrote
fixture data over the user's real global config.

## Fix (test hygiene — same pattern as BUG-377, no assertions changed)

Added `t.Setenv("HOME"/"USERPROFILE", t.TempDir())` to the four test funcs
that exercise save/validate (`TestSupabaseWorkspaceConfigSaveLoadAndReset`,
`…ValidationAndBlankSecretSemantics`, `…SecretFailureDoesNotWriteConfig`,
`…PersistFailureRollsBackSecret`). `os.UserHomeDir` honours `$HOME` on unix
and `%USERPROFILE%` on Windows, so both paths are pinned.

## Verification

- `go test ./internal/runner/ -run 'TestSupabaseWorkspaceConfig|TestApplySupabaseMigrations'` — 5/5 pass.
- `stat` mtime of `~/.flowpilot/settings/supabase-config.json` unchanged across the run.
- Restored the real global config from the workspace copy (projectRef `ipgxvrxrhfhaeskfvctr`).

## Follow-up noted

Runner resolves the catalog once at startup; runners started while the config
was polluted keep serving the empty/demo catalog until restarted.
