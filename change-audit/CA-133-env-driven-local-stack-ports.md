# CA-133: Env-Driven Local Stack Ports

## Scope

- Local development stack launch configuration in `Justfile` and `scripts/supervisor.js`.
- Desktop and admin-web runner/admin URL resolution.
- Google Drive OAuth callback/referrer defaults and runner-side callback validation.

## Completed

- Added `.env` loading to `Justfile` and introduced env-driven ports for admin web, runner, and desktop dev server while keeping the existing `3002`/`4317` defaults.
- Updated `just dev`/`just dev-no-desktop`/`just desktop-dev` to run directly in the current worktree with `.env.dev`, so any feature/task branch can launch its own dev stack locally.
- Added a `scripts/start-production-worktree.js` launcher so `just production` first prepares the dedicated production worktree, syncs its checkout to the current repo's local `main`, refreshes the copied `.flowpilot` state, and then runs the stack there with `.env`, while still using the current checkout's `supervisor.js` via `--root-dir`.
- Added `scripts/self-worktree.js` plus a `just production-worktree` recipe to create or reuse a linked production worktree under `.linked-worktrees/flowpilot-main`, seed its `.env`, link shared dependency directories, refresh the copied `.flowpilot` state, and support pinning that path via `FLOWPILOT_PRODUCTION_WORKTREE`.
- Kept `just self-worktree` as a generic helper for creating other linked worktrees when needed.
- Added tracked `.env.example` and `.env.dev.example` templates while ignoring both real `.env` and `.env.dev` files.
- Propagated the resolved runner/admin origins from `scripts/supervisor.js` into admin-web, desktop, and runner child processes.
- Made the runner child process default `GOCACHE` to a tmp-backed path when the environment does not provide one.
- Forced the desktop Vite dev server launched by the supervisor to bind `127.0.0.1` instead of an IPv6 loopback default.
- Made desktop direct runner calls, chat transport, and "Open Admin Web" use env-derived origins.
- Made admin-web server/browser runner URL helpers derive from `FLOWPILOT_RUNNER_URL`, `VITE_LOCAL_RUNNER_URL`, or `FLOWPILOT_RUNNER_PORT`.
- Made Google Drive setup and runner validation derive the OAuth callback and Picker referrers from the configured runner origin, so a `4318` dev runner can coexist with a `4317` main runner when Google Console allows both.

## Verification

- `go test ./internal/runner -run 'TestGoogleDrive(RuntimeRedirectURIUsesRunnerPortEnv|WorkspaceConfigSaveAndLoad|ValidateGoogleDriveWorkspaceConfig)' -count=1` passed.
- `go test ./internal/cli ./internal/runner -run 'TestGoogleDrive(RuntimeRedirectURIUsesRunnerPortEnv|WorkspaceConfigSaveAndLoad|ValidateGoogleDriveWorkspaceConfig)|^$' -count=1` passed.
- `npm run typecheck` in `apps/desktop-flowpilot` passed.
- `npm test -- src/lib/env/browser-env.test.ts src/lib/env/app-env.test.ts` in `apps/admin-web` passed.
- `node --test .phase1-tests/tests/phase1/desktopRunnerMode.test.js` passed.
- `node --check scripts/start-production-worktree.js` passed.
- `node --check scripts/self-worktree.js` passed.
- `node scripts/start-production-worktree.js --dry-run --restart-existing` prepares the linked production worktree, syncs it to local `main`, and prints the `scripts/supervisor.js --root-dir <main-worktree> --env-file .env` command.
- `node scripts/self-worktree.js --dry-run` prints the expected `../flowpilot-dev` creation plan and `.env.dev` seed step.
- Verified the linked `.linked-worktrees/flowpilot-main` worktree is on `main`, has working symlinks for root/admin-web/desktop `node_modules`, and both app-local `node_modules/.bin/vite` executables resolve there.
- `git diff --check` passed.

## Residual Notes

- Full `tsconfig.phase1-tests.json` compile still fails on pre-existing fake test fixtures missing `applySupabaseMigrations` and `pickDirectory`; the new `config.ts` CommonJS issue found during verification was fixed.
- GitNexus MCP tools were unavailable in this session, so impact/detect-changes checks were approximated with local symbol and diff searches.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: Task-160
change_type: feature
summary: Make local stack ports and runner origins env-file driven for parallel main/dev instances
# --->8---
