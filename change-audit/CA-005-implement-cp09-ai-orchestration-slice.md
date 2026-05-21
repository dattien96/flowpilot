# CA-005 Implement CP-09 AI Orchestration Slice

## Scope

Implemented the first production-facing CP-09 slice in `apps/admin-web` and updated the CP-09 requirements doc with a concrete manual verification guide.

This batch covered:

- prompt template management UI
- AI runs monitoring UI
- dedicated AI orchestration gateway/usecase layer
- demo and Supabase data access wiring
- route-level tests
- CP-09 verification checklist

## Completed

- Added `AiOrchestrationGateway` plus `AiPromptTemplate`, `AiRun`, and `AiRunSummary` entities.
- Added use cases for prompt-template listing/saving and AI-run listing/summary retrieval.
- Added `InMemoryAiOrchestrationGateway` for demo mode with seeded CP-09-shaped data.
- Added `SupabaseAiOrchestrationGateway` for `ai_prompt_templates` and `ai_runs`.
- Wired both `apps/admin-web/src/data/repository/browser-factory.ts` and `apps/admin-web/src/data/repository/factory.ts` to expose the new gateway.
- Replaced the placeholder `/settings/prompt-templates` route with a working management page:
  - create draft template
  - save template
  - edit provider/model/scope/version/status
  - edit input/output schema JSON
- Replaced the placeholder `/ai-runs` route with a working monitoring page:
  - list runs
  - show lineage ids
  - show payloads, token usage, cost, and errors
  - filter by status, project, model, and date range
- Added route content tests for prompt templates and AI runs.
- Added the CP-09 manual verification guide section to the requirements doc.

## Verification

Executed successfully:

- `npm test` in `C:\working\flowpilot\apps\admin-web`
- `npm run build` in `C:\working\flowpilot\apps\admin-web`

Verified outcomes:

- 33 test files passed
- 99 tests passed
- production build completed successfully

## Residual Notes

- The actual document-page "Generate from ..." mutations and Edge Function calls are still pending.
- The implementation intentionally avoided widening the shared workflow-engine gateway because `createGatewayBundle` and related factory surfaces have CRITICAL blast radius.
- Router generation still reports pre-existing warnings for route-adjacent test files without `Route` exports.
- Existing `mcp-servers` tests still emit React `act(...)` warnings unrelated to this CP-09 slice.
