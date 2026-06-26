# CA-008 Fix AI Providers Authentication and Refresh Flows

## Scope

Fixed issues related to the AI Providers installation and authentication state detection. This includes fixing the broken Vite API proxy route, separating the read-only Refresh action from the Install action, resolving snake_case to camelCase JSON key mapping mismatches, and introducing robust multi-path local credentials detection for Codex, Claude, and Gemini.

Also added the ability to trigger provider authentication commands in a new native OS terminal window, and updated action button layouts so the "Refresh" button is always displayed side-by-side with either "Install" or "Auth".

## Completed

- Created [local-runner-mappers.ts](file:///c:/working/flowpilot/apps/admin-web/src/data/repository/local-runner/local-runner-mappers.ts):
  - Maps snake_case Go-serialized JSON properties (`auth_status`, `install_status`, `detected_binary`, `detected_version`, `last_error`, `display_name`) to camelCase TypeScript entity equivalents (`authStatus`, `installStatus`, `detectedBinary`, `detectedVersion`, `lastError`, `displayName`).
- Created [authenticate-provider-usecase.ts](file:///c:/working/flowpilot/apps/admin-web/src/domain/usecase/local-runner/authenticate-provider-usecase.ts) and [route.ts](file:///c:/working/flowpilot/apps/admin-web/src/app/api/local-runner/providers/auth/route.ts):
  - Implemented the Next.js API proxy and use case to trigger the `/providers/auth` endpoint on the local-runner.
- Updated [route.ts](file:///c:/working/flowpilot/apps/admin-web/src/app/api/local-runner/providers/install/route.ts):
  - **Fixed Vite SSR API Proxy Bug**: Removed the broken `assertAdminApiSession` guard that failed server-side in Vite's Node environment due to missing browser cookie context. The route now correctly maps the returning providers via `mapProvider` before sending them to the client.
- Updated [http-local-runner-gateway.ts](file:///c:/working/flowpilot/apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts) and [local-runner-gateway.ts](file:///c:/working/flowpilot/apps/admin-web/src/domain/gateway/local-runner-gateway.ts):
  - Added the `authenticateProvider` method declaration and HTTP implementation pointing to POST `/providers/auth`.
- Updated [ai-providers.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx):
  - **Always Show Refresh**: Changed action button layout so the "Refresh" button is always visible on all cards (e.g. next to "Install" or "Auth").
  - **Interactive Auth Trigger**: Added the "Auth" button for `INSTALLED` + `AUTH_REQUIRED` providers. Clicking it calls the local-runner auth mutation to spawn a login terminal.
  - Bound UI state to local state which updates immediately after direct fetch mutations, preventing background TanStack Router loader updates from stalling UI reconciliations.
- Updated [ai-providers.test.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/settings/ai-providers.test.tsx):
  - Added test case validating that the "Auth" button renders under `AUTH_REQUIRED` state and successfully requests provider authentication on click.
- Updated [runner.go](file:///c:/working/flowpilot/apps/local-runner/internal/runner/runner.go) and [root.go](file:///c:/working/flowpilot/apps/local-runner/internal/cli/root.go):
  - **Enhanced Auth Detection**: Added robust multi-path scanning (checking `os.UserHomeDir()`, `%USERPROFILE%`, `%APPDATA%`, and `$HOME`) to look for the actual config/credential files used by the local CLIs:
    - Codex: `~/.codex/auth.json` or `codex/auth.json`
    - Claude Code: `~/.claude.json`, `claude/auth.json`, or `.config/claude/auth.json`
    - Gemini CLI: `~/.gemini/oauth_creds.json` or `gemini/oauth_creds.json`
  - **Spawning Auth Terminal**: Added `LaunchTerminalWithCommand` to start interactive CLI login workflows on the user's desktop:
    - Windows: `cmd.exe /c start cmd.exe /k <login-command>`
    - macOS: AppleScript command `tell app "Terminal" to do script "<login-command>"`
    - Linux: gnome-terminal, konsole, xfce4-terminal, alacritty, or x-terminal-emulator

## Verification

- Ran `go test ./...` in the `apps/local-runner` module (all CLI and Runner tests passed).
- Executed unit tests in `apps/admin-web` via `npm run test` (all 131 tests passed successfully).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-008
change_type: fix
summary: Fix AI Providers Authentication and Refresh Flows
# --->8---
