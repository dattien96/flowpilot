# CA-061: Fix Claude Limit Error Classification

## Scope

- Corrected Claude controlled-mode runtime error classification when the selected account is usage-limited.
- Kept the fix in the local-runner Claude path; no Codex, Gemini, Supabase, or desktop UI contracts changed.

## Completed

- Added `claudeUsageLimitError` to read Claude local auth metadata and detect disabled extra-usage reasons such as `out_of_credits`.
- Wired the usage-limit preflight into `ProviderRegistryFor(r)` before constructing the live Claude adapter.
- Added Claude result-message normalization so quota-shaped result frames do not surface as misleading login errors.
- Added focused runner tests for account metadata preflight and result-frame normalization.
- Updated `07-Claude-Adapter-Plan.md` and recorded `BUG-051`.

## Verification

- `go test ./internal/runner -run "Test(MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata|ClaudeRegistryGating)$"` in `apps/local-runner`
- `npm run typecheck` in `apps/desktop-flowpilot`

## Residual Notes

- Claude local auth metadata does not expose a precise reset timestamp, so the current message tells the user to switch account or wait for reset without showing the reset time.
- GitNexus MCP tooling was not available in this thread, so impact and scope checks were done with local search and targeted tests.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-051
change_type: fix
summary: Fix Claude Limit Error Classification
# --->8---
