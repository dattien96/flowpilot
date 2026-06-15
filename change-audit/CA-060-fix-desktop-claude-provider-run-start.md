# CA-060: Fix Desktop Claude Provider Run Start

## Scope

- Fixed the desktop direct-chat failure path when Claude is selected as the provider.
- Updated the live local-runner provider registry so Claude is selectable without `FLOWPILOT_CLAUDE_ADAPTER`.
- Kept `DefaultProviderRegistry()` placeholder behavior intact for demo and non-live test paths.

## Completed

- `apps/local-runner/internal/runner/provider_registry.go` now registers Claude as available in `ProviderRegistryFor(r)` and keeps Gemini as a placeholder.
- `apps/local-runner/internal/runner/claude_process.go` no longer carries the obsolete Claude adapter env-gate helper.
- `apps/local-runner/internal/runner/claude_adapter_test.go` now verifies default-placeholder versus live-runner-available Claude registry behavior.
- `apps/desktop-flowpilot/src/state/store.ts` now formats run-start/send-turn failures for the timeline so a rejected `startRun` does not leave an empty chat view.
- `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md` now documents that Claude is available in the live runner registry without the old feature flag.
- `requirements/09-BugFix/done/BUG-050-Desktop-Claude-Provider-Blank-Chat.md` records the bug, root cause, fix, and verification.

## Verification

- `go test ./internal/runner -run TestClaudeRegistryGating` in `apps/local-runner`
- `npm run typecheck` in `apps/desktop-flowpilot`

## Residual Notes

- A full live desktop Claude turn still depends on a valid Claude account or `ANTHROPIC_API_KEY` and should be smoke-tested in the user's authenticated environment.
- GitNexus MCP tooling was not available in this thread, so impact and affected-scope checks were performed with local search, targeted tests, and diff inspection.
