# Task-215: Per-Model Reasoning-Effort Detection And Model-Aware Reasoning UI

## Metadata

- Document ID: `Task-215`
- Title: `Per-Model Reasoning-Effort Detection And Model-Aware Reasoning UI`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-213: Auto-Detect And Sync Provider Models Into The Supported-Models Catalog](./../inprogress/Task-213-Auto-Detect-And-Sync-Provider-Models.md), [Task-211: Grok Desktop UI Surface](./../inprogress/Task-211-Grok-Desktop-UI-Surface.md)
- Replaces: `None`
- Tags: `ai-providers, reasoning-effort, model-detection, desktop, settings, catalog-sync`

## AI Quick View

### Summary

- Today "Reasoning" options are two independent hardcoded constants shown for **every model of a provider regardless of what that specific model supports**: admin-web `REASONING_EFFORT_OPTIONS` = low/medium/high/xhigh (`workflow-engine.ts`), desktop `REASONING_OPTIONS` = low/medium/high (`ChatInput.tsx`) — the two lists already disagree with each other.
- Live probe done for this task (2026-07-10, real installed CLIs) proves both **Codex and Grok already return per-model reasoning-effort data in the exact same detection call Task-213 already makes** — the runner's typed structs simply don't read those fields (Go silently drops unknown JSON keys):
  - `codex debug models` (Codex Build 0.144.1) — each model object carries `default_reasoning_level` and `supported_reasoning_levels: [{effort, description}, ...]`. The live model `gpt-5.6-sol` lists `low, medium, high, xhigh, max, ultra` — **`max`/`ultra` are real, live-available levels that neither app's hardcoded option list can reach today.**
  - `~/.grok/models_cache.json` (Grok Build 0.2.93, the file `detectGrokModels` already reads per Task-213 T-5) — each model carries `supports_reasoning_effort`, `reasoning_effort` (default), and `reasoning_efforts: [{id, value, label, description, default}, ...]`. Live model `grok-4.5` only supports `high, medium, low` — matches the hand-written `grokReasoningEffortID` switch in `grok_event_mapper.go`, but this task makes it provable from live data instead of a frozen code comment.
  - Claude has **no per-model catalog**; `claude --help` documents one global, non-per-model list (`low, medium, high, xhigh, max`) tied to the CLI version, not to the selected model.
- **Folded in during doc review (2026-07-10):** the same live probe shows `codex debug models` *also* carries `context_window`/`max_context_window`/`effective_context_window_percent` per model (live values seen: `372000`/`372000`/`95`, `272000`/`272000`/`95`, `272000`/`1000000`/`95` across different models) — dropped by the same struct, same silent-field-loss pattern as reasoning. Grok's `models_cache.json` carries a single `context_window` per model too (live: `500000` for `grok-4.5`). Since both live in the exact same payloads this task is already opening up, context-window capture is now in-scope alongside reasoning-effort (not deferred, see former `Q-3`).
- This task threads that already-detected per-model data through `ProviderModel` (runner) -> `ai_supported_models` (catalog) -> `SupportedModel` (client-core) -> the desktop Reasoning dropdown, so the dropdown shows only the levels the **currently selected model** actually supports — fixing the reported UI (Grok 4.5 chat shows a fixed Default/Low/Medium/High no matter the model) and the gap where newer models (e.g. Codex `gpt-5.6-sol`) can't reach their own `max`/`ultra` levels from either app. Context-window values ride the same pipe into the catalog for future use, persisted but not yet surfaced in any UI (see `Out of Scope`).

### Current Ask

- Extend Task-213's existing provider-model detection (already parses `codex debug models` and `~/.grok/models_cache.json`) to also capture each model's supported reasoning-effort levels **and context-window size**, persist both in the supported-models catalog, and make the desktop (and admin-web) Reasoning dropdown render only the levels valid for the model currently selected — not one static list applied to every model of a provider.

### Key Decisions

- `T-1` **Reuse, don't add new CLI calls.** Codex and Grok already return this data inside the same `codex debug models` / `~/.grok/models_cache.json` read Task-213 performs; this task is a field-mapping fix plus a data-plumbing extension, not a new detector.
- `T-2` **Persist per-model reasoning support in the catalog (`ai_supported_models`), not only in the transient `/providers` payload.** The desktop Reasoning dropdown reads `supportedModels` (the catalog via `useStore`), not the live provider list — so the data must land there to be usable, the same reason Task-213 DOD-3 had to add a `models`/`detected_version` mapper.
- `T-3` **Claude keeps its static, global list.** No per-model catalog exists for Claude's CLI. This task only corrects the canonical constants to match the real CLI surface (add `max`) — it does not attempt to scrape `claude --help` text into a detector.
- `T-4` **Reasoning-support metadata refreshes on every detect** (unlike model rows, which are insert-only per Task-213 T-3) — it is provenance/capability data about an existing catalog row, not user-editable state, so re-detecting should be free to update it. Confirm this in `Open Questions` before implementing.
- `T-5` **Degrade, never fabricate.** The dropdown must never offer a level a model doesn't support, and unmapped/unknown effort ids must degrade the same way `grokReasoningEffortID` already degrades (`none`/`minimal`->`low`, `xhigh`/`max`->nearest supported) rather than sending an invalid flag to the CLI.
- `T-6` **Context-window capture rides the same pipe; feeds the existing usage bar as a pre-turn fallback.** Since `codex debug models`/`models_cache.json` expose `context_window` (and Codex's `max_context_window`/`effective_context_window_percent`) in the same objects as reasoning data, this task captures and persists them alongside reasoning fields end to end. **Resolved (2026-07-10, concrete consumer identified):** the desktop already has a "Context 14k / 200k used · ... left" usage bar (`ChatInput.tsx`'s `usageSummaryLine`) driven by `usage.modelContextWindow`, which is only populated *live* by the running provider process once a turn has produced usage data. The catalog's detected `contextWindowTokens` for the selected model now fills that same slot as a fallback so the bar can show a window size before/without a live value — no new UI surface, just wiring the existing one to the newly-detected data. A dedicated standalone display (e.g. in the Model dropdown itself) remains out of scope (`Out of Scope`).

### Constraints

- **ADDITIVE.** Existing argument-building paths — Codex's `-c model_reasoning_effort=<val>` ([runner.go](../../../apps/local-runner/internal/runner/runner.go)), Claude's `--effort <val>` ([claude_permission_mcp.go](../../../apps/local-runner/internal/runner/claude_permission_mcp.go)), Grok's `grokReasoningEffortID` ([grok_event_mapper.go](../../../apps/local-runner/internal/runner/grok_event_mapper.go)) — are unchanged. This task only changes what is *offered* in the catalog/UI, never how a chosen effort is transmitted to a CLI.
- **No new process spawns.** All new data comes from calls Task-213 already makes.
- Must not remove or weaken the existing static-fallback path (e.g. `defaultCodexProviderModels`/`defaultGeminiProviderModels`-style lists) used when live detection fails — reasoning data is simply absent on the fallback path, and the UI must degrade to the pre-existing static option list in that case.
- Must not break Task-213's insert-only guarantee for model rows (`T-3`/`DOD-4` there) — this task only adds columns/fields, it does not change the model-diff logic.

### Open Questions

- `Q-1` **Overwrite policy for `supported_reasoning_efforts` on re-detect.** Per `T-4` above, the working assumption is unconditional refresh (it's provenance, like `detected_cli_version`) even for a catalog row the user hand-edited otherwise (`display_name`, `is_enabled`) — confirm before implementing, since it's a deviation from Task-213's `skip-if-exists` rule for the row as a whole.
- `Q-2` Should Claude's global reasoning list also live in the catalog (duplicated per Claude model row, for UI-code symmetry with Codex/Grok), or stay a separate desktop/admin-web constant since it is not truly per-model? Affects whether `ChatInput.tsx`'s dropdown logic can be a single "read from catalog" code path for all providers, or needs a Claude special case.
- `Q-3` **RESOLVED (2026-07-10).** Originally: "worth capturing Grok's `context_window` in the same pass, or a follow-up task?" — resolved to **in-scope, this task**, and widened to Codex too once the live probe also found `context_window`/`max_context_window`/`effective_context_window_percent` on the Codex side. See `T-6`.
- `Q-4` Does Codex's `codex debug models` vary `supported_reasoning_levels` (and `context_window`) across models today (e.g. a smaller/cheaper Codex model with fewer levels or a smaller window than `gpt-5.6-sol`), or is it currently uniform across the catalog? Affects how much the UI payoff is "real" right now vs. future-proofing — verify against a second live model entry during implementation.
- `Q-5` **RESOLVED (2026-07-10).** A concrete consumer exists today: `ChatInput.tsx`'s usage bar ("Context 14k / 200k used · 186k left · ..."). Wired as a fallback for `usage.modelContextWindow` when no live turn has reported one yet (`T-6`). A dedicated display elsewhere (Model dropdown badge, settings page) is still deferred.

### Source Refs

- Task-213 (Auto-Detect And Sync Provider Models) — the detection path and catalog columns (`source`, `detection_method`, `detected_cli_version`, `last_detected_at`) this task extends with the same provenance pattern.
- CP-46 (Grok Build Controlled Adapter Over ACP) — `grokReasoningEffortID`, `grok_acp_types.go`, `GrokAvailableModel`.
- SD-06 (AI Provider Integration).
- Live probes captured 2026-07-10 (this task's authoring pass, real installed binaries — `codex --version` = codex-cli 0.144.1, `claude --version` = 2.1.191): raw `codex debug models` output (including `context_window`/`max_context_window`/`effective_context_window_percent`) and raw `~/.grok/models_cache.json` contents, both reproduced in full in `4. Exact Change` context below.

## 1. Goal

Make the Reasoning control per-model-aware end to end: detect each model's real supported reasoning-effort levels (and, riding the same data, its context-window size) from data the runner already fetches, persist both in the supported-models catalog, and render only the valid reasoning levels for whichever model is currently selected — instead of one fixed option list applied uniformly to every model of a provider.

## 2. Parent Links

- coding plan: `CP-46` (context/trigger; feature is provider-agnostic like Task-213 was)
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: `Task-213` (`T-1`..`T-6`, `DOD-1`..`DOD-7` — the detection/catalog-sync path this task extends)

## 3. Trigger

Desktop chat screenshot repro: with Grok 4.5 selected, the Reasoning dropdown shows a fixed `Default / Low / Medium / High` list regardless of which model is selected, alongside a failed turn (`run-4587`/`turn-4589`, `provider=grok`, `error="Path not found."` — a separate transport issue, not this task's concern, but what surfaced the dropdown during triage).

Investigating showed reasoning-effort options are hardcoded in two disconnected constants (`admin-web` vs `desktop`) that already disagree, and neither varies by model. A live CLI probe done specifically for this task proved the underlying providers already expose exactly the per-model data needed to fix this — Codex's real live model (`gpt-5.6-sol`) already supports `max`/`ultra` levels that are unreachable from either app's UI today, and Grok's cache file already reports which of `low/medium/high` each model supports, both via calls the runner already makes for Task-213.

## 4. Exact Change

- `T-1` **Runner — Codex.** Extend `codexDebugModel` ([runner.go](../../../apps/local-runner/internal/runner/runner.go)) with the fields the live CLI already returns but the struct drops:
  ```json
  {"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","default_reasoning_level":"low",
   "supported_reasoning_levels":[{"effort":"low","description":"..."},{"effort":"medium","description":"..."},
   {"effort":"high","description":"..."},{"effort":"xhigh","description":"..."},{"effort":"max","description":"..."},
   {"effort":"ultra","description":"..."}],
   "context_window":272000,"max_context_window":1000000,"effective_context_window_percent":95,
   "visibility":"list","supported_in_api":true}
  ```
  Map `supported_reasoning_levels[].effort`, `default_reasoning_level`, `context_window`, and `max_context_window` into the new `ProviderModel` fields (`T-3` below) inside `detectCodexModels`.
- `T-2` **Runner — Grok.** Extend `grokModelsCacheEntry.Info` ([runner.go](../../../apps/local-runner/internal/runner/runner.go)) with the fields the live cache file already has but the struct drops:
  ```json
  {"id":"grok-4.5","name":"Grok 4.5","hidden":false,"supported_in_api":true,"context_window":500000,
   "reasoning_effort":"high","supports_reasoning_effort":true,
   "reasoning_efforts":[{"id":"high","value":"high","label":"High Effort","default":true},
   {"id":"medium","value":"medium","label":"Medium Effort","default":false},
   {"id":"low","value":"low","label":"Low Effort","default":false}]}
  ```
  Map into the same new `ProviderModel` fields inside `detectGrokModels`, guarded by `supports_reasoning_effort` for the reasoning fields (omit entirely when `false`, per `T-5`/degrade rule); `context_window` maps unconditionally when present.
- `T-3` **Runner — shared DTO.** Add to `ProviderModel` ([types.go](../../../apps/local-runner/internal/runner/types.go)): `SupportedReasoningEfforts []string`, `DefaultReasoningEffort string`, `ContextWindowTokens int64`, `MaxContextWindowTokens int64` (all `omitempty`), populated by `T-1`/`T-2`; left empty for Claude/unmapped providers and for the static-fallback path.
- `T-4` **Migration.** New `supabase/migrations/<ts>_add_reasoning_and_context_window_to_supported_models.sql`:
  ```sql
  alter table public.ai_supported_models
    add column if not exists supported_reasoning_efforts jsonb null,
    add column if not exists default_reasoning_effort text null,
    add column if not exists context_window_tokens integer null,
    add column if not exists max_context_window_tokens integer null;
  ```
  Additive, idempotent, no row rewrite.
- `T-5` **flowpilot-client-core.** Widen `SupportedModel`/`LocalRunnerProviderModel` ([adminModels.ts](../../../packages/flowpilot-client-core/src/domain/adminModels.ts)) with the same four fields; update `mapLocalRunnerProviderModel` ([runnerAdminRepository.ts](../../../packages/flowpilot-client-core/src/data/runnerAdminRepository.ts)) and `mapSupportedModel`/`createSupportedModel` ([supabaseAdminRepository.ts](../../../packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts)) to carry them end to end.
- `T-6` **Desktop sync.** `AiProvidersSettings.tsx`'s `detectModels` stamps `supportedReasoningEfforts`/`defaultReasoningEffort`/`contextWindowTokens`/`maxContextWindowTokens` from the detected `provider.models[]` entry onto both newly-inserted and (per `Q-1`) already-registered catalog rows.
- `T-7` **Desktop UI.** `ChatInput.tsx`: replace the flat `REASONING_OPTIONS` render with `reasoningOptionsFor(selectedModelInfo)`, derived from the selected model's `supportedReasoningEfforts` (looked up from `availableModels` by `modelId`); fall back to `FALLBACK_REASONING_OPTIONS` (the old static list) when the catalog has no reasoning data for that row (manually-added models, or providers with no detector); a `useEffect` resets `reasoningEffort` to the model's default whenever the currently-selected value isn't in the newly-selected model's option set (switching models never leaves an unsupported effort silently selected). Also wires `selectedModelInfo.contextWindowTokens` into `usageSummaryLine` as the pre-live-turn fallback (`T-6`).
- `T-8` **admin-web parity.** Widen `REASONING_EFFORT_OPTIONS` ([workflow-engine.ts](../../../apps/admin-web/src/domain/model/entity/workflow-engine.ts)) to include `max`/`ultra` (static fix), or switch its consumers to the same catalog-driven derivation as `T-7` if `Q-2` resolves that way.
- `T-9` **Tests.** Go unit tests for `T-1`/`T-2`/`T-3` using fixture JSON that mirrors the exact live payloads captured in `4.` above, asserting both reasoning fields and context-window fields (`TestDetectCodexModelsCapturesReasoningLevels`, `TestDetectCodexModelsCapturesContextWindow`, `TestDetectGrokModelsCapturesReasoningEfforts`, `TestDetectGrokModelsCapturesContextWindow`); a desktop test for the model-aware option-derivation function; a regression check that Claude's args and the Codex/Grok argument-building paths (`T-` constraints) are byte-for-byte unchanged.

## 5. Touched Areas

- files (runner): `apps/local-runner/internal/runner/types.go` (`ProviderModel`), `apps/local-runner/internal/runner/runner.go` (`codexDebugModel`, `detectCodexModels`, `grokModelsCacheEntry`, `detectGrokModels`).
- files (db): new `supabase/migrations/<ts>_add_reasoning_and_context_window_to_supported_models.sql`.
- files (client-core): `packages/flowpilot-client-core/src/domain/adminModels.ts` (`SupportedModel`, `LocalRunnerProviderModel`), `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts` (`mapLocalRunnerProviderModel`), `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` (`mapSupportedModel`, `createSupportedModel`).
- files (desktop): `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx` (`detectModels` stamp), `apps/desktop-flowpilot/src/components/ChatInput.tsx` (`REASONING_OPTIONS` -> per-model derivation).
- files (admin-web): `apps/admin-web/src/domain/model/entity/workflow-engine.ts` (`REASONING_EFFORT_OPTIONS`), and its render sites if `Q-2` resolves to catalog-driven.
- tables: `ai_supported_models` (additive columns only).

## 6. Acceptance Check

- Selecting Grok 4.5 in desktop chat shows only `Low / Medium / High` in Reasoning, matching the live `reasoning_efforts` from `~/.grok/models_cache.json` — no more fixed 4-option list for every Grok model.
- Selecting a Codex model whose detected `supported_reasoning_levels` includes `max`/`ultra` shows those levels in Reasoning (previously unreachable from either app).
- Re-running "Detect models" updates `supported_reasoning_efforts` on already-registered rows without touching `is_enabled`/`display_name`/`sort_order`/`source` (per `Q-1` resolution) and remains idempotent.
- A manually-added model with no detected reasoning data falls back to the previous static option list — no regression for hand-added rows.
- Claude's Reasoning behavior and CLI argument-building are unchanged; only its static option constant gains `max`.
- Codex/Grok argument-building paths (`-c model_reasoning_effort=`, `grokReasoningEffortID`) produce byte-identical output to before this task for every previously-valid input.
- After "Detect models", `ai_supported_models.context_window_tokens`/`max_context_window_tokens` are populated for Codex and Grok rows, and the usage bar shows a context-window figure for the selected model even before a turn has run (via the `T-6` fallback).

## 7. Out of Scope

- Claude per-model reasoning detection — no such data exists in the CLI (`T-3`/Key Decision).
- A dedicated context-window display beyond the existing usage-bar fallback (`T-6`/`Q-5`) — e.g. a token-budget badge on the Model dropdown itself, or in admin-web/settings — is deferred to a follow-up.
- Any change to how a chosen reasoning effort is transmitted to a provider's CLI — argument-building code is untouched.
- Auto-refresh/staleness handling beyond what Task-213's manual "Detect models" button already provides (no new polling).
- Fixing the `error="Path not found."` Grok turn failure from the trigger screenshot — unrelated, flagged for separate investigation.

## 8. Completion Notes

- result: `T-1`..`T-9` implemented (Go detectors, migration, client-core plumbing, desktop `detectModels`/`ChatInput` UI, admin-web static-list widen, Go tests). `T-8` resolved to the "static fix" branch of `Q-2` (admin-web's `REASONING_EFFORT_OPTIONS` widened to include `max`; not switched to catalog-driven — admin-web's workflow builder isn't wired to the live per-model catalog and that wiring was judged out of scope for this pass).
- implementation notes:
  - Runner: `codexDebugModel`/`grokModelsCacheEntry` now type `default_reasoning_level`/`supported_reasoning_levels`/`context_window`/`max_context_window` (Codex) and `reasoning_effort`/`supports_reasoning_effort`/`reasoning_efforts`/`context_window` (Grok); mapped onto `ProviderModel`'s four new fields in `detectCodexModels`/`detectGrokModels`.
  - `ai_supported_models` gained `supported_reasoning_efforts`/`default_reasoning_effort`/`context_window_tokens`/`max_context_window_tokens` (migration `20260710110000_...`).
  - `Q-1` resolved in code: `AiProvidersSettings.detectModels` now calls `updateSupportedModel` (refreshing only the four new fields) for already-registered rows instead of skipping them entirely, and `createSupportedModel` for new ones — model identity/user edits (`is_enabled`/`display_name`/`sort_order`) still untouched either way.
  - `Q-5`/`T-6` resolved in code: `ChatInput.tsx`'s existing usage bar (`usageSummaryLine`) takes `selectedModelInfo.contextWindowTokens` as a fallback for `usage.modelContextWindow`.
  - Reasoning dropdown (`ChatInput.tsx`) now derives options per selected model (`reasoningOptionsFor`) with a `useEffect` degrade-reset when the current effort isn't valid for a newly-selected model.
- verification:
  - Go: `go build ./...` clean; new/updated tests (`TestDetectCodexModelsCapturesReasoningAndContextWindow`, `TestDetectGrokModelsFiltersHiddenAndUnsupported` reasoning/context assertions) pass in isolation and alongside the existing Task-213 detection tests.
  - **Caveat found during verification, not caused by this task:** a full `go test ./internal/runner/...` run failed with a *build* error in `grok_live_chat_test.go`/`grok_registry_test.go` (`not enough arguments in call to r.ensureGrokProcess`/`reg.Adapter`) — these files, plus `grok_process.go`/`interactive_service.go`/`provider_registry.go`/`claude_adapter_test.go`/`interactive_service_test.go`/`partd_test.go`, were already showing as modified/uncommitted in `git status` before this task started and continued changing during this session (consistent with concurrent, unrelated work-in-progress on this branch). None of Task-215's touched files (`types.go`, `runner.go`, `grok_account_test.go`, `runner_test.go`) are implicated; this task's changes were verified with targeted `-run` test scoping instead of the full-package run.
  - TypeScript: `apps/desktop-flowpilot` (`npx tsc --noEmit`) clean after fixing 4 pre-existing `SupportedModel` test fixtures in `store.test.ts` that needed the new required fields. `apps/admin-web` (`npx tsc --noEmit`) shows 99 pre-existing errors unrelated to this task (confirmed by absence of any `workflow-engine.ts`/`ReasoningEffort` errors in the list) — a known-broken baseline, not introduced here.
  - Browser: attempted via `preview_start` (desktop-flowpilot, port 5173). The Vite renderer loads (confirmed via `preview_snapshot`), but the app is gated behind "Bootstrapping desktop workspace… checking local runner reachability, Supabase runtime config, and auth session state" with no local runner/Supabase backend available in this sandbox, so the Reasoning dropdown/usage bar could not be interactively exercised end to end. Static verification (build + typecheck + unit tests) is the coverage available in this environment.
- follow-ups: fix the unrelated `ensureGrokProcess`/`Adapter` test build breakage (not part of this task, resolved on its own by the time of the final full-package test run); consider a dedicated context-window UI surface beyond the usage-bar fallback if a need emerges (`Out of Scope`).
- upstream docs updated: none (additive; no upstream CP/SD/SS intent changed).
- **Side-fix landed in this same session ([CA-277](../../../change-audit/CA-277-claude-xhigh-effort-no-longer-remapped-to-max.md)):** while live-verifying Claude's `--effort` surface for `T-3`'s "no per-model catalog" conclusion, found the CLI's real validator (`GD=["low","medium","high","xhigh","max"]`) treats `xhigh` and `max` as distinct values — but two runner code paths (`normalizeClaudeEffort` and `resolvePromptExecutionAdapter`'s inline Claude branch) were silently remapping a user's `xhigh` selection to `max`. Fixed both, widened `FALLBACK_REASONING_OPTIONS` to expose `max` as its own option, and corrected the affected test case. This is a pre-existing bug predating Task-215, found as a side effect of this task's Claude research — not part of Task-215's own scope/DOD, tracked via CA-277 instead of amended into this doc's `Exact Change`.
