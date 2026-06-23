# CA-118: Fix Codex Rollout Migration Rename On Windows

## Scope

- Windows file-handle lifetime in the BUG-124 rollout migration
- Cross-platform mode-preservation assertion in the migration test

## Completed

- Replaced `defer src.Close()` with an idempotent `closeSrc()` helper in `migrateCodexReservedSpawnAgentTool`.
- Closed the source handle after `io.Copy` drains it and before `os.Rename`, so Windows can replace the original file (no more `Access is denied`).
- Kept `defer closeSrc()` so every early-return path still releases the handle.
- Changed `TestCodexAdapterResumeMigratesLegacySpawnAgentTool` to assert the migrated file's mode equals the original file's OS-reported mode instead of a hardcoded `0640`.

## GitNexus Impact

- GitNexus index reported stale; impact assessed by local inspection.
- `migrateCodexReservedSpawnAgentTool`: change is internal to the function (handle lifetime only); the migrated content, temp-file strategy, and Unix behavior are unchanged.
- No HIGH or CRITICAL impact: the only caller is the Codex resume preparation path.

## Verification

- `TestCodexAdapterResumeMigratesLegacySpawnAgentTool` — pass on Windows (previously `rename … Access is denied`).
- `TestCodexAdapter*`, `TestMapCodexNotification` group — pass.
- No-regression: full `go test ./internal/runner -count=1` failure set is identical with and without this change, except the migration test now passes. The 14 remaining failures are pre-existing environment-dependent tests (real-`codex`-binary resume, google-drive proxy path, provider-home skill merges) unrelated to this change.
- `go build ./internal/runner/...` — pass.

## Residual Notes

- File modes on Windows are reported as `0666`; the migration preserves whatever mode the OS reports, which is the correct cross-platform behavior.
- Pre-existing environment-dependent test failures remain out of scope and are not introduced by this change.
