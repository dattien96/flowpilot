# Task 013: Dynamic Way To Add Supported Model

## Problem

Currently, adding a supported model requires code and SQL migration changes.

Current static sources:
- Supabase SQL `CHECK` constraint on `step_definitions.model`
- Frontend constant `STEP_MODEL_OPTIONS`
- Runtime validation that treats the constant list as the source of truth

This means the current provider settings page can only display detected models from the local runner. It cannot persist a new supported model for FlowPilot itself.

## Expected Outcome

Add a UI form on each provider card in `Settings > AI Providers` so an admin can manage supported models dynamically.

Expected behavior:
- Each provider has an `Add model` form.
- Each provider has a `Check and import missing models` action.
- Admin can enter at least:
  - model id
  - display name
  - provider
- Admin can import newly detected models without typing them manually when detection is available.
- Manual entry remains available as the fallback path when detection is incomplete or unavailable.
- Saved models are persisted in database data, not hard-coded in migration SQL.
- Saved models appear immediately in:
  - project settings model dropdown
  - workflow create/detail model dropdown
  - workflow step create/detail model dropdown
  - workflow start/runtime validation
- Provider remains derived from model selection. Users still do not edit provider independently in workflow/project forms.

## Implementation Status As Of 2026-05-29

Already implemented:
- `ai_supported_models` migration exists, including:
  - seed rows
  - `source`
  - `detection_method`
  - `detected_cli_version`
  - `last_detected_at`
- `Settings > AI Providers` now shows:
  - runner-detected models
  - FlowPilot supported models
  - manual `Add model`
  - `Check and import missing models`
  - enable/disable
  - delete
- Supported model CRUD is wired through `workflowEngineGateway`.
- Import flow is working.
- Runtime validation in `workflow-start-runtime.ts` now checks `ai_supported_models` first.
- Provider derivation in runtime/gateway paths now checks `ai_supported_models` when prefix-only derivation is not enough.

Detection status:
- `codex`: implemented with `codex debug models`
- `gemini`: implemented with installed Gemini CLI bundle parsing
- `claude`: not implemented dynamically yet; current runner inventory still falls back to static Claude entries

Important implementation detail:
- `/api/local-runner/providers/import` is detection-only.
- Actual inserts into `ai_supported_models` happen from the authenticated browser-side mutation.
- This was done intentionally to avoid the RLS failure seen when the API route attempted to write with a non-user-authenticated Supabase client.

Still not complete:
- Several admin-web forms still use `STEP_MODEL_OPTIONS` as a fallback/default source.
- Some mapper/default logic still falls back to static model constants.

## Clarified Product Expectation

There are two different concepts and they must stay separate:

1. `Runner detected models`
- Read-only inventory reported by the local runner from provider-local detection.
- Host-machine state.
- Useful for visibility and availability.
- May come from different detection strategies per provider, not necessarily a first-class CLI `list models` command.

2. `FlowPilot supported models`
- Admin-managed registry used by app forms and workflow validation.
- Project data / application configuration.
- This is what must become dynamic.

The new UI should manage `FlowPilot supported models`, not only display runner-detected models.
Runner detection can help prefill or bulk-import models, but it must not silently replace registry ownership.

## Updated Product Direction

Use a hybrid management flow:

1. Manual registry editing stays supported
- Admin can always add a model manually.
- This is the fallback when detection is unavailable, incomplete, or suspicious.

2. Provider-local detection assists import
- Each provider card exposes a `Check and import missing models` button.
- FlowPilot asks the local runner to detect the models available for that provider using local CLI/package inspection.
- The app compares detected models with the current `ai_supported_models` rows for that provider.
- Only missing models are inserted into the registry.

3. Detection remains advisory, not authoritative
- Detection is used to propose/import models.
- The database registry remains the source of truth for forms and runtime validation.
- Imported models should be clearly marked as detected/imported so admins can review or disable them later.

## Recommended Approach

Use a database-backed model registry as the new source of truth for supported models.

Recommended phase-1 rule:
- Replace static model lists with registry reads in the app.
- Remove the rigid SQL list constraint that requires migrations for every new model.
- Keep provider derivation from model id prefix or registry mapping.

Recommended phase-2 hardening:
- After registry-backed reads are stable, add stronger database validation using the registry table instead of hard-coded SQL lists.

## Data Design

Create a new table, for example `ai_supported_models`.

Suggested fields:
- `id uuid primary key`
- `provider_key text not null`
- `model_id text not null unique`
- `display_name text not null`
- `is_enabled boolean not null default true`
- `sort_order integer not null default 0`
- `source text not null default 'manual'`
- `detection_method text null`
- `detected_cli_version text null`
- `last_detected_at timestamptz null`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

Suggested constraints:
- `provider_key` limited to current supported providers: `codex`, `claude`, `gemini`
- unique on `model_id`
- optional unique on `(provider_key, display_name)` if wanted for UX cleanliness

Seed migration:
- backfill the current static model list into `ai_supported_models`
- mark those rows with `source = 'seed'`

Suggested `source` values:
- `seed`
- `manual`
- `detected`

Suggested `detection_method` values:
- `codex_debug_models`
- `gemini_bundle_registry`
- `claude_binary_registry`

Actual current usage:
- `codex_debug_models` is in use
- `gemini_bundle_registry` is in use
- `claude_binary_registry` exists as planned metadata, but current Claude inventory is still static fallback data

## UI / UX Plan

Update `apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx`.

Per provider card:
- Keep the existing install/auth/refresh controls.
- Keep the read-only runner model chips section.
- Add a second section: `Supported models`.
- Show persisted supported models for that provider.
- Add a `Check and import missing models` button near the supported-model controls.
- Add inline form or modal with:
  - model id input
  - display name input
  - save button
- Optional near-term actions:
  - disable model
  - delete model

Recommended UX states:
- prevent duplicate `model_id`
- trim whitespace
- disable save while pending
- surface validation error inline
- refresh the provider card list after mutation success
- after import, show summary such as `3 new models imported, 5 already registered`
- show detected models that are already registered as `already imported`
- show detected models that were imported through automation with a `detected` badge
- if detection fails, keep the manual form available and surface the failure inline without blocking the page

Recommended provider-card interaction:

1. `Refresh inventory`
- Existing action.
- Refreshes install/auth/provider readiness and runner-side detected inventory.

2. `Check and import missing models`
- New action.
- Calls an app route that asks the runner for provider-specific detected models.
- The route returns normalized models plus detection metadata.
- The browser mutation compares them against the DB registry.
- The browser mutation inserts only missing rows.
- The UI shows:
  - detected count
  - imported count
  - skipped count
  - optional warnings

3. `Add model`
- Manual fallback.
- Used when detection misses a valid model or when an admin wants to pre-register a model before the runner can detect it.

## App Architecture Changes

### 1. New registry contract

Add a dedicated domain model and gateway methods for supported models.

Suggested operations:
- `listSupportedModels()`
- `createSupportedModel(input)`
- `updateSupportedModel(input)` if edit is needed now
- `deleteSupportedModel(modelId)` or `disableSupportedModel(modelId)`
- `importDetectedSupportedModels(providerKey)` or equivalent app-level mutation

Suggested import result:
- `providerKey`
- `detectedModels`
- `importedModels`
- `skippedModels`
- `warnings`

Actual current shape:
- detection route returns:
  - `providerKey`
  - `detectedAt`
  - `detectedCliVersion`
  - `detectionMethod`
  - `models`
  - `warnings`
- browser mutation computes:
  - `importedCount`
  - `skippedCount`
  - summary text rendered on the provider card

### 2. Route loader composition

`AI Providers` page loader should fetch both:
- local runner provider inventory
- supported model registry from Supabase

Then merge by `provider_key` in the UI.

### 3. Runner detection contract

Add a dedicated local-runner capability for provider model detection.

Suggested operations:
- `detectProviderModels(providerKey)`
- or `syncProviderModels(providerKey)` if the runner performs detection and returns normalized models

Recommended response shape:
- `providerKey`
- `detectedAt`
- `cliVersion`
- `detectionMethod`
- `models: Array<{ modelId, displayName, source }>`
- `warnings: string[]`

Provider-specific detection strategy:
- `codex`
  - use `codex debug models`
  - parse JSON
  - include only user-selectable models (`visibility = list`)
- `gemini`
  - inspect the installed Gemini CLI bundle for its local model registry
  - include stable auto aliases exposed by the CLI (`auto-gemini-3`, `auto-gemini-2.5`)
  - exclude internal `customtools` variants
- `claude`
  - planned: inspect the installed Claude Code binary/package for embedded canonical model ids
  - current runner code does not implement this dynamic path yet
  - current runner inventory still falls back to static Claude entries

Important rule:
- Detection is provider-specific and may use CLI/package introspection rather than a uniform `list models` command.
- The runner must normalize output into one common shape before returning it to admin-web.

### 4. Replace static dropdown sources

Current workflow/project/step forms read from `STEP_MODEL_OPTIONS`.

Replace that with a shared registry-backed query/hook so these pages always use the same dynamic list:
- `projects/$projectId/settings.tsx`
- `workflows/create.tsx`
- `workflows/$workflowId.tsx`
- `workflow-steps/create.tsx`
- `workflow-steps/$stepType.tsx`

### 5. Runtime validation update

Current runtime validation still assumes a hard-coded supported model list.

Update validation in:
- `workflow-start-runtime.ts`
- `start-run-http-handler.ts`
- Supabase mappers/gateways where model coercion still falls back to `STEP_MODEL_OPTIONS`

New rule:
- validate against the registry-backed supported model list
- continue normalizing legacy aliases if needed

## Database Migration Plan

### Migration A
- create `ai_supported_models`
- seed current models

### Migration B
- add optional detection metadata columns:
  - `detection_method`
  - `detected_cli_version`
  - `last_detected_at`

### Migration C
- stop relying on hard-coded `step_definitions_supported_model_check`
- either remove it or replace it with registry-aware validation later

### Migration D
- optional follow-up: add stronger FK/trigger-based validation for:
  - `projects.default_model`
  - `workflows.model_override`
  - `workflow_steps.model_override`
  - `workflow_runs.model`
  - `step_definitions.model`

Recommendation:
- do not block phase 1 on cross-table FK hardening
- first move the app and admin flow to the registry source of truth

## Implementation Steps

### Phase 1: Registry foundation
- Add `ai_supported_models` migration and seed current models.
- Add domain types and Supabase gateway CRUD methods.
- Add tests for mapping and persistence behavior.

### Phase 2: Manual management UI
- Extend `AI Providers` route loader to fetch registry data.
- Add supported-model list UI per provider.
- Add create mutation from provider card form.
- Add optimistic refresh or targeted reload after save.

### Phase 3: Runner-side detection
- Add local-runner endpoint/use case for provider-specific model detection.
- Implement `codex` detection through `codex debug models`.
- Implement `gemini` detection through installed bundle registry parsing.
- Implement `claude` detection through installed binary/package registry parsing.
- Normalize all outputs into one shared response contract.
- Add targeted tests for each detection strategy.

### Phase 4: Import missing models flow
- Add `Check and import missing models` mutation on each provider card.
- Compare detected models vs `ai_supported_models`.
- Insert only missing models with `source = 'detected'`.
- Persist detection metadata when available.
- Return a summary to the UI.

### Phase 5: Replace static app model sources
- Remove `STEP_MODEL_OPTIONS` as the source of truth for forms.
- Load model options from registry for project/workflow/step forms.
- Keep `gpt-5.4` fallback only if it still exists in registry seed data.

### Phase 6: Runtime and validation alignment
- Replace hard-coded supported-model checks with registry-based checks.
- Keep alias normalization for backward compatibility.
- Ensure provider derivation still works for saved and legacy model ids.

### Phase 7: Cleanup
- Remove obsolete static lists where no longer needed.
- Update spec/design/coding-plan docs that still imply SQL-only model expansion.
- Document the provider-specific detection confidence and fallback behavior.

## Current Delivery Status

Completed:
- Phase 1: Registry foundation
- Phase 2: Manual management UI
- Phase 4: Import missing models flow

Completed with scope adjustment:
- Phase 3: Runner-side detection
- `codex` is done
- `gemini` is done
- `claude` is not done yet and still uses static fallback inventory

Partially completed:
- Phase 5: Replace static app model sources
- Phase 6: Runtime and validation alignment

Not completed:
- Phase 7: Cleanup

## Acceptance Criteria

- Admin can add a supported model from the `AI Providers` page under a specific provider.
- Admin can click `Check and import missing models` for a provider.
- Missing detected models are inserted into `ai_supported_models` without duplicating existing rows.
- Detection failure does not block manual model entry.
- Newly added model is persisted without adding a new SQL migration for that model.
- Newly added model appears in all model selectors in admin-web.
- Workflow/project/step save flows accept the new model.
- Workflow runtime validation accepts the new model.
- Existing seeded models continue to work.
- Existing legacy alias normalization continues to work.
- Provider install/auth/detection behavior remains unchanged.

Acceptance status today:
- `Done`: manual add model
- `Done`: per-provider import action
- `Done`: deduplicated insert into `ai_supported_models`
- `Done`: detection failure does not block manual fallback
- `Done`: runtime accepts DB-registered models
- `Done`: Codex detected-model import
- `Done`: Gemini detected-model import
- `Partial`: all selectors use dynamic registry data
- `Partial`: Claude detected-model import

## Risks

- If only the UI is added and runtime validation stays static, new models will save but fail at execution time.
- If only SQL is changed and the dropdowns stay static, admins still cannot actually use the new models.
- If runner-detected models and registry models are mixed into one concept, the UX will become confusing.
- `codex` detection is relatively stable, but `gemini` and `claude` detection may depend on installed package/binary internals rather than a stable public CLI contract.
- If provider package internals change in a future CLI release, detection may degrade and manual entry must remain the fallback.
- Imported models may include preview, deprecated, or internal-looking ids unless normalization/filtering rules are explicit.

## Out Of Scope

- Silent background auto-sync from runner-detected models into the supported-model registry without admin action
- Provider-specific advanced metadata such as reasoning support, tier gating, or pricing
- Reworking provider derivation away from the current model-to-provider mapping strategy

## Definition Of Done

- migration added
- admin UI added on each provider card
- per-provider `Check and import missing models` flow added
- manual add-model fallback kept in place
- supported model CRUD wired to Supabase
- detected-model import wired from local runner to Supabase registry
- workflow/project/step selectors use dynamic registry data
- runtime validation uses same registry data
- targeted tests updated
- task docs/spec notes updated where static model list assumptions remain

Current DoD status:
- `[x]` migration added
- `[x]` admin UI added on each provider card
- `[x]` per-provider `Check and import missing models` flow added
- `[x]` manual add-model fallback kept in place
- `[x]` supported model CRUD wired to Supabase
- `[x]` detected-model import wired from local runner to Supabase registry
- `[ ]` workflow/project/step selectors use dynamic registry data everywhere
- `[~]` runtime validation uses same registry data, with static fallback still present in some paths
- `[x]` targeted tests updated for implemented runner detection paths
- `[x]` task docs/spec notes updated where static model list assumptions remain
