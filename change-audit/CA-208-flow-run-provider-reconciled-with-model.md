# CA-208: Flow Run Provider Reconciled With Resolved Model

## Summary

Fixed `BUG-171`: a Flow Mode run was stamped `codex` + `claude-haiku` (the desktop sends `providerKey: selectedProvider` with no model for flow launches, and the server-side Step>Flow>Project>default resolution landed on the Claude-Haiku coder/reviewer default from BUG-162). The turn then failed with "the 'claude-haiku' model is not supported when using Codex", and the step-timeline sidebar showed a wrong `CODEX` tag. `createRun` now treats the resolved model's implied provider as authoritative.

## What Changed

- `apps/local-runner/internal/runner/interactive_handlers.go`: in `createRun`, a resolved model with a known provider prefix (`claude-*`/`gpt-*`/`gemini-*`) now sets the provider, overriding a conflicting explicit/inherited `in.ProviderKey`; the explicit provider (or registry default) is used only when the model implies no known provider. Centralized here so both the hub launch and spawned children (both route through `createRun`) are fixed, and the step-timeline provider tag — read off `rs.providerKey` — is corrected too.
- `apps/local-runner/internal/runner/workflow_model_resolution_test.go`: added `TestCreateRunReconcilesProviderToResolvedModel` plus a `registryWithClaudeAvailable` helper (Claude marked Available so the reconciled provider is selectable in-test).
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`: the `main`-card provider badge was hardcoded to `CODEX`; it now renders `mainProvider`/`mainModel` from `workflowStepRuntimeMeta.provider`/`.model` (the run's real provider in Flow Mode), falling back to the selected provider/model for normal chat and to `codex` only when nothing is known yet — so a Claude-/Gemini-driven hub reads the correct badge.

## Verification

- `TestCreateRunReconcilesProviderToResolvedModel` — passes (codex launch + claude-haiku model → run stamped claude + claude-haiku).
- `TestCreateRunNeverInjectsResolvedModelForChat` and the four `TestCreateRun*` model-tier tests — still pass (no-op for chat and gpt-* workflows).
- Change-relevant groups (`TestCreateRun* / TestListAgentRunSummaries* / TestSpawnAgent* / TestResumeRun*`) — 23 passed, 0 failed.
- Full runner `go test ./...` — remaining failures are all pre-existing and environment-specific (missing `codex`/`agy`/`go` binaries, OS path quoting), confirmed identical on a clean `git stash` baseline.
- `npm run typecheck` in `apps/desktop-flowpilot` — clean (covers the Agents-panel badge change).
- Not verified live (no runner + provider account in this environment) — flagged in `BUG-171` (`V-6`).

## Notes

- Desktop still sends `providerKey: selectedProvider` for flow launches: harmless now the server reconciles, and a sensible fallback for a resolved model with no known prefix.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-171
change_type: bugfix
summary: Make a flow run's resolved model authoritative over the launched provider so a Claude model no longer runs on Codex, fixing the mismatch failure and the wrong provider tag
# --->8---
