# Task-059: Desktop Check Version Tested Baseline Config

## Metadata

- Document ID: `Task-059`
- Title: `Desktop Check Version Tested Baseline Config`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [05: Codex App-Server Migration Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md), [Task-056: Fix Codex Ask-User Live Flow](./Task-056-Fix-Codex-Ask-User-Live-Flow.md)
- Child Documents: `none`
- Related Documents: [BUG-070: Codex MCP Permission Approval Request Unsupported](../../09-BugFix/done/BUG-070-Codex-MCP-Permission-Approval-Request-Unsupported.md), [BUG-073: Claude Cannot Use FlowPilot MCP Servers](../../09-BugFix/done/BUG-073-Claude-Cannot-Use-FlowPilot-MCP-Servers-Google-Drive-And-AskUser-Yolo-On.md)
- Replaces: `none`
- Tags: `desktop, settings, compatibility, codex, claude, runner, config`

## AI Quick View

### Summary

- The Check Version page used hardcoded tested versions (`2.1.179` for Claude and `0.140.0` for Codex), which made baseline refreshes a code change instead of a workspace setting.
- This task adds `.flowpilot/settings/compat-config.json` as the workspace-scoped source of truth for the latest tested versions, with fallback to the existing built-in defaults when the file does not exist.
- The desktop Settings page now includes editable inputs for the tested Claude/Codex versions and saves them through the runner, creating the JSON file on first save.
- `scripts/quicktest.ps1` now reads the same JSON file first, so the UI and terminal-side compatibility checks share one baseline.

### Current Ask

- Done. The tested-version baseline is now configurable from Settings, persisted to `.flowpilot/settings/compat-config.json`, and used by the compat checks with fallback defaults.

### Key Decisions

- `T-1` Keep `CompatTestedClaudeVersion` and `CompatTestedCodexVersion` in code as fallback defaults only.
- `T-2` Store user-updated tested versions at `.flowpilot/settings/compat-config.json`, matching the existing workspace-config pattern used by other desktop settings.
- `T-3` Expose a dedicated runner config endpoint (`/compat-config`) instead of overloading `/compat`.
- `T-4` Keep `scripts/quicktest.ps1` aligned with the same workspace config so the script and UI cannot drift.

### Constraints

- Do not change the compatibility policy itself; only change where the tested baseline comes from.
- If the config file is missing or incomplete, fallback must remain `2.1.179` for Claude and `0.140.0` for Codex.
- The feature must remain workspace-scoped, not global-machine scoped.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/compat.go`
- `apps/local-runner/internal/cli/root.go`
- `apps/desktop-flowpilot/src/components/settings/CheckVersionSettings.tsx`
- `packages/flowpilot-client-core/src/domain/runner.ts`
- `packages/flowpilot-client-core/src/data/runnerRepository.ts`
- `scripts/quicktest.ps1`

## 1. Goal

Make the latest tested Claude/Codex versions configurable from the desktop Check Version page and persist them into a workspace JSON file so version-baseline refreshes do not require editing Go constants.

## 2. Parent Links

- coding plan: [05: Codex App-Server Migration Detail](../../10-Refactor/New-System/05-Codex-AppServer-Migration-Detail.md)
- tech design: `none`
- system spec: `none`
- specific upstream ids: `Task-056`, `BUG-070`, `BUG-073`

## 3. Trigger

The current Check Version flow correctly identifies version drift, but the tested versions were hardcoded in the runner. The user wanted the latest tested versions to live in `.flowpilot/settings`, be editable from the Settings UI, and fallback to the current built-in pair when the file is not present.

## 4. Exact Change

- `T-1` Add runner-side compat config loading/saving at `.flowpilot/settings/compat-config.json`, with fallback defaults from `CompatTestedClaudeVersion` and `CompatTestedCodexVersion`.
- `T-2` Update compat info / fast checks / deep checks to use the workspace compat config values instead of always using the hardcoded pair.
- `T-3` Add `GET/PUT /compat-config` to the local runner HTTP surface.
- `T-4` Extend `@flowpilot/client-core` with load/save compat config contracts and use cases.
- `T-5` Add a `Latest Tested Versions` editor to the desktop Check Version page that saves the config file and refreshes the displayed tested-version badges.
- `T-6` Update `scripts/quicktest.ps1` to read `.flowpilot/settings/compat-config.json` first, then fall back to the built-in defaults when the file is absent or invalid.
- `T-7` Add focused runner tests for fallback behavior and file creation.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/compat.go`
  - `apps/local-runner/internal/runner/compat_config_test.go`
  - `apps/local-runner/internal/cli/root.go`
  - `packages/flowpilot-client-core/src/domain/runner.ts`
  - `packages/flowpilot-client-core/src/data/runnerRepository.ts`
  - `apps/desktop-flowpilot/src/clientCore.ts`
  - `apps/desktop-flowpilot/src/components/settings/CheckVersionSettings.tsx`
  - `scripts/quicktest.ps1`
- modules:
  - local-runner compat checks
  - desktop settings compatibility panel
  - client-core runner repository
- routes:
  - `GET /compat`
  - `POST /compat`
  - `POST /compat/deep`
  - `GET /compat-config`
  - `PUT /compat-config`
- tables:
  - `none`

## 6. Acceptance Check

- If `.flowpilot/settings/compat-config.json` does not exist, the page still shows `2.1.179` for Claude and `0.140.0` for Codex as the tested baseline.
- Saving new tested versions from the Settings page creates `.flowpilot/settings/compat-config.json` if needed.
- After save, the tested-version badges and compat checks use the saved values.
- `scripts/quicktest.ps1` uses the same saved tested-version baseline when the file exists.
- Desktop TypeScript compiles cleanly.
- Focused runner compat-config tests pass.

## 7. Out of Scope

- Auto-promoting the tested baseline after deep checks pass.
- Rewriting the broader compatibility policy or quicktest result semantics.
- Migrating existing provider/account config files into a shared config subsystem.

## 8. Completion Notes

- result: Implemented. The tested baseline is now workspace-configurable, persists to `.flowpilot/settings/compat-config.json`, and is editable from the Check Version page.
- follow-ups:
  - optionally add a one-click `Use Installed Versions As Tested Baseline` action if the team wants faster baseline updates after successful deep checks
  - optionally show the exact saved config path in the UI
- upstream docs updated: `Task-059`
