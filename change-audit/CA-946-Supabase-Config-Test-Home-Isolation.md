# CA-946 — BUG-382: supabase config tests pin HOME so global settings survive

## Summary

`persistSupabaseWorkspaceConfig` mirrors config to
`~/.flowpilot/settings/supabase-config.json`. Four tests in
`supabase_config_test.go` exercised Save without isolating HOME — each run
overwrote the user's real global config with `demo-ref` fixture data, which
made the live runner's `/client/projects` serve an empty/demo catalog while
Settings showed the real registry.

Fix: `t.Setenv("HOME"/"USERPROFILE", t.TempDir())` in the four save-exercising
tests (hygiene isolation; no assertions weakened — same class as BUG-377).
Real global config restored from the workspace copy.

## Verified

- 5/5 supabase config tests pass.
- Global file mtime unchanged across test run.
- Post-fix `/client/projects` serves real registry after runner restart.
