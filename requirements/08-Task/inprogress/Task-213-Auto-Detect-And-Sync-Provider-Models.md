# Task-213: Auto-Detect And Sync Provider Models Into The Supported-Models Catalog

## Metadata

- Document ID: `Task-213`
- Title: `Auto-Detect And Sync Provider Models Into The Supported-Models Catalog`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Child Documents: `None`
- Related Documents: [Task-211: Grok Desktop UI Surface](./Task-211-Grok-Desktop-UI-Surface.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](./Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Replaces: `None`
- Tags: `ai-providers, supported-models, model-detection, desktop, settings, catalog-sync`

## AI Quick View

### Summary

- Today, to make a model selectable in FlowPilot (workflow/step/chat pickers), someone must hand-add a row to `ai_supported_models` via the desktop **Settings → AI Providers → Add Supported Model** form. Providers rotate models frequently, so this manual step goes stale.
- FlowPilot calls Codex/Claude/Grok through their **official CLIs**, and every one of those CLIs already exposes the exact model list it will accept. The runner **already detects** that list per provider (`detectProvider` → `resolveProviderModels`) and publishes it on `GET /providers` as `provider.models[]` (each carrying a `source`), alongside `provider.detectedVersion`.
- The `ai_supported_models` table **already has the columns** for provenance (`source`, `detection_method`, `detected_cli_version`, `last_detected_at`), the repository (`SupabaseAdminRepository.createSupportedModel`) already writes them, and **the sibling `apps/admin-web` app already ships this feature** (an "import missing models" action that diffs detected-vs-registered and upserts the new ones as `source:"detected"`). The **desktop app (`apps/desktop-flowpilot`) is the only surface missing it.**
- This task adds a **"Detect models" button** per provider (and/or a global one) to the desktop `AiProvidersSettings` page that, on one click, reads the runner-detected model list, inserts any models not already in the catalog (`source:"detected"` + detection provenance), and refreshes the list — porting the proven `admin-web` UX. It also (a) makes **Grok detection real** in the runner (`detectGrokModels`, currently Grok falls back to a static list), and (b) adds the **DB migration** so `grok` is an allowed `provider_key` (the table's `CHECK` constraint currently rejects it — a latent blocker for the Grok "Add model" option added in Task-211).

### Current Ask

- Give a desktop user a one-click way to pull the current official model list from each installed provider CLI into `ai_supported_models`, so newly-released models become selectable without hand-typing — reusing the runner detection and the Supabase catalog that already exist, and extending detection to Grok.

### Key Decisions

- `T-1` **Reuse, don't rebuild.** Runner detection (`resolveProviderModels`), the `GET /providers` payload (`provider.models[]` + `provider.detectedVersion`), the `ai_supported_models` provenance columns, and `SupportedModelRepository.{listSupportedModels,createSupportedModel}` all already exist. This task is a desktop-UI + one runner-detector + one migration; it does **not** invent a new persistence layer.
- `T-2` **Manual, button-triggered — not automatic-on-startup** (per the request: "bấm 1 cái là new model được input"). No background polling; the user decides when to sync. (Auto-on-launch is an explicit Open Question, not in scope here.)
- `T-3` **Diff by `model_id`, insert-only, never clobber user state.** A sync inserts only models whose `model_id` is not already registered for that provider; it must NOT overwrite an existing row's `is_enabled`, `display_name`, `sort_order`, or `source` (so a user who disabled a model, renamed it, or hand-added it keeps their edits). This mirrors `admin-web`'s `importMissingModels` (skip-if-exists) exactly.
- `T-4` Detected rows are stamped `source:"detected"`, `detection_method:<per-provider>`, `detected_cli_version:<provider.detectedVersion>`, `last_detected_at:<now>` — the same shape `admin-web` writes and the same columns `mapSupportedModel` already reads.
- `T-5` **Grok detection source of truth** = `detectGrokModels` in the runner. Preferred source (proven live during this pass): parse `~/.grok/models_cache.json` (structured JSON with `id`/`name`/`context_window`/`hidden`/`supported_in_api`/`reasoning_efforts`), filtering out `hidden==true`/`supported_in_api==false`. Fallbacks: the `grok models` subcommand (human-readable text, like `agy models` — note this is on the REAL xAI binary; the npm `@vibe-kit/grok-cli` shim does not expose it), or the ACP `initialize.modelState.availableModels` the adapter already parses (`GrokAvailableModel`, `grok_acp_types.go`). Detection method label: `grok_models_cache` (or `grok_models` if the subcommand path is chosen).
- `T-6` **DB migration is mandatory and comes first.** `ai_supported_models.provider_key` has `CHECK (provider_key in ('codex','claude','gemini'))`. Add `'grok'` (and align any TS union that mirrors it). Without this, both this task's Grok sync AND the Task-211 "Add model → grok" option fail at insert time with a constraint violation.
- `T-7` **Provenance UX.** Detected rows render a "detected" badge (admin-web already does this via `model.source === "detected"`); the desktop list should do the same so users can tell auto-synced models from hand-added ones.

### Constraints

- **ADDITIVE / no regression to the existing manual flow.** The current "Add Supported Model" form and the Codex/Claude/Gemini detection must keep working unchanged; this is a new button + a new Grok detector + a widened `CHECK`/union, not a rewrite.
- Do not change `SupabaseAdminRepository` CRUD semantics; if an upsert-by-`model_id` helper is needed it is additive (a new method), leaving `createSupportedModel` as-is.
- The desktop must reach detection only through the existing `RunnerClient`/admin-use-cases boundary (`listLocalProviders`, `createSupportedModel`, `listSupportedModels`) — no new bespoke HTTP call from a component.
- Grok detection must run with the same ambient-MCP-scan-disabled discipline as the adapter if it spawns the CLI (`GROK_CLAUDE_MCPS_ENABLED=false`/`GROK_CURSOR_MCPS_ENABLED=false`, CP-46 R-1) — but the `models_cache.json` path reads a file and spawns nothing, which is why it is preferred.
- The migration must be idempotent (`drop constraint if exists` → re-add) and must not drop/rewrite existing rows.

### Open Questions

- `Q-1` **Where does the diff+upsert run?** **Resolved: (A).** Desktop-only: `AiProvidersSettings.tsx`'s `detectModels` diffs the already-loaded `provider.models[]` (now actually mapped into `LocalRunnerProvider`, which required fixing `runnerAdminRepository.ts` — see `DOD-3`) against `listSupportedModels()` by `modelId`, and calls `createSupportedModel` per missing model. No new runner endpoint (`/providers/import`) was added.
- `Q-2` **Auto-refresh on launch / staleness indicator?** Should the page also show "N new models available since last sync" or auto-detect on open, or stay purely manual? (Out of scope for the button itself; flagged for a follow-up.)
- `Q-3` **Prune / mark-stale?** When a provider stops offering a previously-detected model, do we leave the row (safe, may 400 at turn time) or flag it? This task is insert-only (`T-3`); pruning is deferred.
- `Q-4` **Grok detection when unauthenticated / offline.** `grok models` prints "You are not authenticated" but still lists the cached default; `models_cache.json` persists the last good fetch. Decide the freshness/΅staleness tolerance for the cache-file path.

### Source Refs

- Table + constraint + seed: `supabase/migrations/20260529153000_add_ai_supported_models.sql` (the `CHECK (provider_key in ('codex','claude','gemini'))` and the `source`/`detection_method`/`detected_cli_version`/`last_detected_at` columns).
- Persistence: `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` (`mapSupportedModel`, `createSupportedModel`, `listSupportedModels`); interface `packages/flowpilot-client-core/src/domain/adminRepositories.ts` (`SupportedModelRepository`); `adminModels.ts` (`SupportedModel`).
- Runner detection: `apps/local-runner/internal/runner/runner.go` (`detectProvider`, `resolveProviderModels`, `detectCodexModels` via `codex debug models`, `detectGeminiModels` via `agy models`); `types.go` (`Provider.Models []ProviderModel`, `ProviderModel{ID,DisplayName,Available,Source}`, `Provider.DetectedVersion`); endpoint `apps/local-runner/internal/cli/root.go` `GET /providers`.
- Desktop target: `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx` (manual `addModel`, `loadLocalProviders`, `providers`/`models` state).
- Reference implementation to port: `apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx` (`importMissingModels` mutation — detect → diff-by-modelId → `createSupportedModel({source:"detected", detectionMethod, detectedCliVersion, lastDetectedAt})`; "detected" badge via `model.source === "detected"`).
- Grok detection sources (verified live, Grok Build 0.2.93): `~/.grok/models_cache.json`; `grok models` subcommand; ACP `initialize.modelState.availableModels` → `GrokAvailableModel` in `apps/local-runner/internal/runner/grok_acp_types.go`.

## 1. Goal

Let a desktop user pull the current official model catalog from each installed provider CLI into `ai_supported_models` with one click, so freshly-released models are selectable without manual data entry — reusing the runner's existing detection and the existing Supabase catalog, extending real detection to Grok, and unblocking Grok at the DB layer.

## 2. Parent Links

- coding plan: `CP-46` (trigger/context — Grok exposed the gap; the feature itself is provider-agnostic and could be promoted to its own CP if desired)
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: `CP-46 P-8` (Grok model detection), Task-211 (the Grok "Add model" option this unblocks)

## 3. Trigger

Providers rotate models often; the manual "Add Supported Model" form goes stale. The runner already detects the live model list and `admin-web` already turns that into a catalog sync — the desktop just lacks the button. Adding Grok made the gap concrete (its models are static placeholders and the DB `CHECK` rejects `grok`).

## 4. Exact Change

- `T-1` **Migration** (`supabase/migrations/<new>_allow_grok_supported_models.sql`): `alter table public.ai_supported_models drop constraint if exists ...provider_key_check` then re-add `CHECK (provider_key in ('codex','claude','gemini','grok'))`. Idempotent; no row changes. Mirror the widened union anywhere TS hard-codes the three (`admin-web`'s `createSupportedModel` input type, and the desktop `AiProvidersSettings` `<select>` cast already includes `grok` from Task-211 — verify consistency).
- `T-2` **Runner `detectGrokModels`** (`runner.go`): add a `grok` branch to `resolveProviderModels` (currently only `codex`/`gemini` are special-cased). Parse `~/.grok/models_cache.json` (preferred), filtering `hidden`/`!supported_in_api`; map to `[]ProviderModel{ID,DisplayName,Source:"grok_models_cache"}`. Fall back to `grok models` / static defaults on read failure. Keep the FLOWPILOT_GROK_BIN override + ambient-scan-disabled discipline if the subcommand path is used.
- `T-3` **Desktop "Detect models" action** (`AiProvidersSettings.tsx`): add a per-provider (and optionally a global) button that:
  - reads detected models from the already-loaded `provider.models[]` (from `listLocalProviders`), or a re-fetch;
  - diffs by `model_id` against the current `listSupportedModels()` for that provider (skip existing — `T-3` decision);
  - inserts each missing model via `createSupportedModel({providerKey, modelId, displayName, isEnabled:true, sortOrder:max+n, source:"detected", detectionMethod:<provider method>, detectedCliVersion:provider.detectedVersion, lastDetectedAt:now})`;
  - shows an inline summary ("X new imported, Y already registered") and refreshes the Supported Models list. Port `admin-web`'s `importMissingModels` shape.
- `T-4` **"detected" badge** in the desktop Supported Models list (`model.source === "detected"`), matching admin-web.
- `T-5` (If Open Question `Q-1` resolves to B) add `POST /providers/import` to `cli/root.go` returning `{providerKey, detectedAt, detectedCliVersion, detectionMethod, models[], warnings[]}` and a matching `RunnerClient`/repository method; otherwise skip.
- `T-6` Tests: runner `detectGrokModels` unit test (fixture `models_cache.json`); desktop diff/skip-existing logic test; a migration-applies check; a regression test that manual Add + Codex/Claude/Gemini detection are unchanged.

## 5. Touched Areas

- files (db): new `supabase/migrations/<ts>_allow_grok_supported_models.sql`.
- files (runner): `apps/local-runner/internal/runner/runner.go` (`resolveProviderModels` grok branch + `detectGrokModels`); optionally `apps/local-runner/internal/cli/root.go` (`/providers/import`, only if `Q-1`=B).
- files (desktop): `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx` (detect button + summary + badge); possibly `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts` + `domain/adminRepositories.ts` (only if `Q-1`=B needs a new method); reuse `createSupportedModel`/`listSupportedModels`.
- modules: provider inventory/detection; supported-models catalog; desktop AI-providers settings.
- routes: reuse `GET /providers`; new `POST /providers/import` only under `Q-1`=B.
- tables: `ai_supported_models` (constraint widen only; no column/row rewrite).

## 6. Acceptance Check

- On the desktop AI Providers page, clicking "Detect models" for an installed provider inserts any newly-offered models into the catalog as `source:"detected"` and they become selectable in the model pickers, without touching already-registered rows.
- Re-clicking is a no-op for models already present (idempotent; reports "0 new / N already registered").
- Grok detection returns the real current model list (from `models_cache.json`), not the static placeholder; and a `grok` row inserts successfully (migration applied).
- Codex/Claude/Gemini manual-add and existing detection behavior are unchanged.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Migration adds `grok` to the `ai_supported_models.provider_key` CHECK constraint, idempotently, with no row changes; a `grok` model inserts successfully. `supabase/migrations/20260710100000_allow_grok_supported_models.sql`, following the exact drop-if-exists/re-add pattern already used for every other CHECK in this repo (e.g. `cp06_artifact_storage.sql`, `cp29_step_mcp_access_mode.sql`). **Not executed against a live Supabase instance** in this pass (no local DB available here) — syntactically verified by pattern-matching the repo's own convention, not by running it.
- [x] `DOD-2` Runner `detectGrokModels` returns the live model list from `~/.grok/models_cache.json` (hidden/unsupported filtered), wired into `resolveProviderModels`; `GET /providers` reports real Grok models with a `source`. (`runner.go`; `TestDetectGrokModelsFiltersHiddenAndUnsupported`, `TestDetectGrokModelsMissingCacheReturnsError`, `TestResolveProviderModelsGrokUsesDetectedCache`, `TestResolveProviderModelsGrokFallsBackToStaticListOnDetectFailure`.) Resolved `Q-1` in favor of desktop-only diffing (Option A) — no new `/providers/import` endpoint was added.
- [x] `DOD-3` Desktop "Detect models" button diffs detected-vs-registered by `model_id` and inserts only missing models as `source:"detected"` with detection provenance; shows an import summary; refreshes the list. (`AiProvidersSettings.tsx detectModels`.) Required widening `LocalRunnerProvider`/adding a `mapLocalRunnerProvider` mapper in `runnerAdminRepository.ts` — the runner's `/providers` response already carried `models`/`detected_version`, but the desktop repository was silently dropping them (no transform existed; `apps/admin-web` has its own separate mapper that already did this).
- [x] `DOD-4` Sync never overwrites an existing row's `is_enabled`/`display_name`/`sort_order`/`source` (skip-if-exists); re-running is idempotent. Enforced by construction (`registeredModelIds` computed before the insert loop, checked per detected model before any `createSupportedModel` call) — not covered by an automated test (see `DOD-7` gap).
- [x] `DOD-5` Detected models render a "detected" badge distinct from manual rows. Reuses the pre-existing `.settings-badge` CSS class (no new styles needed).
- [x] `DOD-6` Manual "Add Supported Model" + Codex/Claude/Gemini detection are unchanged (regression). `addModel`/`detectCodexModels`/`detectGeminiModels` untouched; full `go test ./...` before and after this task's Go changes shows the identical 15 pre-existing, unrelated failures.
- [ ] `DOD-7` Tests: `detectGrokModels` unit (cache fixture) — **done**. Desktop diff/skip logic test — **not done** (no JS/TS test runner is installed in this environment — `node_modules` is absent repo-wide, confirmed while working Task-211 too — so `detectModels`'s skip-if-exists logic was verified by code review only, not an automated test). Migration-applies check — **not done** (no local Supabase instance available to run it against).

## 7. Out of Scope

- Auto-sync on launch / staleness indicators (`Q-2`).
- Pruning or flagging models a provider has retired (`Q-3`) — this task is insert-only.
- Per-model rich metadata (context window, reasoning-effort menus) beyond `model_id`/`display_name` in the catalog; the runtime already reads richer data elsewhere (e.g. Grok `initialize`).
- Porting any admin-web-only UI chrome beyond the detect/import action + badge.

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
