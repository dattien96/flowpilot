# BUG-167: Claude Supported Model Ids Missing "claude-" Prefix

## Metadata

- Document ID: `BUG-167`
- Title: `Claude Supported Model Ids Missing "claude-" Prefix`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-166-Claude-Models-Missing-From-Supported-Models-Registry.md`
- Child Documents: `none`
- Related Documents: `apps/local-runner/internal/runner/provider_registry.go`, `apps/admin-web/src/domain/model/entity/workflow-engine.ts`
- Replaces: `requirements/09-BugFix/done/BUG-166-Claude-Models-Missing-From-Supported-Models-Registry.md`
- Tags: `supabase, migration, ai-supported-models, provider-derivation`

## AI Quick View

### Summary

- Corrects `BUG-166`'s diagnosis. A user-provided full export of `ai_supported_models` showed the three Claude rows have `model_id = 'haiku' | 'sonnet' | 'opus'` — **not** `'claude-haiku'` / `'claude-sonnet'` / `'claude-opus'` as every other consumer expects.
- Every other place a Claude model id is used, uses the prefixed form: `step_definitions.model` literally stores `'claude-haiku'` (confirmed live); `STEP_MODEL_OPTIONS` (`apps/admin-web/src/domain/model/entity/workflow-engine.ts`) lists `'claude-haiku'`/`'claude-sonnet'`/`'claude-opus'`; and — most consequentially — `providerKeyFromModel` (`apps/local-runner/internal/runner/provider_registry.go:172-183`) derives the Claude provider via `strings.HasPrefix(m, "claude-")`. A bare `'haiku'` matches none of that function's prefix cases at all.
- Practical effect beyond the Step Definitions dropdown mismatch this was originally reported for: **any UI path that lets a user pick a model straight from `ai_supported_models.model_id` and hands that value to provider-derivation logic would silently fail to resolve Claude** for `'haiku'`/`'sonnet'`/`'opus'` and fall through to whatever the fallback default provider is — a real, if narrow, correctness risk, not just a display bug.
- `BUG-166`'s migration (which would have *inserted* `'claude-haiku'` as a brand-new row alongside the existing `'haiku'` row, leaving both present) was removed before ever being applied anywhere, and replaced with an in-place id correction here.

### Current Ask

- Correct the three Claude rows' `model_id` to the canonical `'claude-<name>'` form the rest of the system already expects.

### Key Decisions

- `F-1` New migration renames `'haiku'` → `'claude-haiku'`, `'sonnet'` → `'claude-sonnet'`, `'opus'` → `'claude-opus'` in place (`update ... set model_id = 'claude-' || model_id`), preserving each row's `id`, `source`, `sort_order`, and timestamps — a rename, not a replace.
- `F-2` Also handles the case where `BUG-166`'s (now-removed) migration already ran somewhere and inserted a duplicate `'claude-haiku'` row before this fix landed: a `delete` step runs first, dropping the legacy bare-id row whenever a prefixed duplicate already exists, so the rename step never hits a unique-constraint conflict regardless of which migrations actually executed on a given deployment.
- `F-3` Did not touch the unrelated `'claude-fake'` row (`source = 'manual'`) — that's the user's own manually-added test entry, not part of this seed-naming drift.
- `F-4` Did not audit Gemini/Codex rows for the same class of drift — this export confirms all of them already use the correct prefixed form (`gemini-*`, `gpt-*`).

### Constraints

- The migration must be safe to run whether or not `BUG-166`'s (removed) migration ever executed against a given database — handled via the delete-duplicate-first, then-rename approach in `F-2`.

### Open Questions

- None.

### Source Refs

- `supabase/migrations/20260702160000_fix_claude_supported_model_id_prefix.sql`
- `apps/local-runner/internal/runner/provider_registry.go:172-183` (`providerKeyFromModel` — the prefix-match logic this fix aligns the registry with)
- `apps/admin-web/src/domain/model/entity/workflow-engine.ts` (`STEP_MODEL_OPTIONS` — already uses the prefixed form)
- User-provided `ai_supported_models_rows.csv` export — the evidence that overturned `BUG-166`'s original diagnosis.

## 1. Issue Summary

`BUG-166` incorrectly diagnosed the Step Definitions Model-dropdown mismatch as a missing `claude-haiku` row. A full table export showed the row exists, just under the wrong (unprefixed) id, which is a more consequential bug than originally scoped — it also breaks provider derivation for any code path that trusts the registry's id directly.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: Supabase `ai_supported_models` table, any deployment seeded from `20260529153000_add_ai_supported_models.sql`'s original run (or however the current `haiku`/`sonnet`/`opus` rows were created).
- reproduction steps: `select model_id from ai_supported_models where provider_key = 'claude';` returns `haiku`, `sonnet`, `opus` — not the `claude-`-prefixed forms every consumer expects.
- frequency: deterministic wherever these three rows exist in their current (bare-id) form.

## 4. Expected vs Actual

- expected: Claude rows use `model_id = 'claude-haiku' | 'claude-sonnet' | 'claude-opus'`, matching `step_definitions.model`, `STEP_MODEL_OPTIONS`, and `providerKeyFromModel`'s prefix expectation.
- actual: Claude rows use bare `model_id = 'haiku' | 'sonnet' | 'opus'`.

## 5. Impact

- users affected: anyone whose registry has these bare-id rows (confirmed on the reporting user's deployment).
- workflows affected: the Step Definitions Model dropdown (originally reported); potentially any other UI path that resolves provider directly from an `ai_supported_models.model_id` value.
- severity: medium — beyond the originally-reported display glitch, this is a latent provider-misresolution risk for any code path trusting the registry id as-is.

## 6. Root Cause

- confirmed cause: the three Claude rows in `ai_supported_models` were created (seed or otherwise) with bare ids (`haiku`/`sonnet`/`opus`) instead of the `claude-`-prefixed canonical form every other part of the system (the `step_definitions` catalog, the static `STEP_MODEL_OPTIONS` list, and the Go provider-derivation logic) already uses.
- evidence: user-provided full-table CSV export; `provider_registry.go:172-183`'s prefix-only matching logic; `workflow-engine.ts`'s `STEP_MODEL_OPTIONS` list.

## 7. Fix Strategy

- `F-1`..`F-4` as described in Key Decisions.

## 8. Validation

- `V-1` Reviewed the migration for idempotency/order-independence: the delete-duplicate-first step means the rename step can never conflict, regardless of whether `BUG-166`'s removed migration ran on a given database before this fix landed.
- `V-2` Not executed: re-running `select model_id from ai_supported_models where provider_key = 'claude'` after applying this migration to confirm all three now show the prefixed form — no Supabase/backend access in this environment. The user should re-run their own export/query after applying to confirm.

## 9. Regression Guard

- tests: none — pure data migration; no application code changed (the code already correctly expects the prefixed form, which is exactly why the bare ids were wrong).
- alerts: none.
- audit checks: recorded in `change-audit/CA-204-fix-claude-supported-model-id-prefix.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `BUG-166` itself was left unedited (only its metadata/status updated) as an accurate record of the original, later-corrected investigation.
