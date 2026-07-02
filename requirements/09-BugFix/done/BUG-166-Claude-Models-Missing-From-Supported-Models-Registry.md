# BUG-166: Claude Models Missing From Supported-Models Registry

## Metadata

- Document ID: `BUG-166`
- Title: `Claude Models Missing From Supported-Models Registry`
- Phase: `bugfix`
- Status: `superseded`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-162-Coder-Reviewer-Steps-Must-Default-To-Claude-Haiku.md`
- Child Documents: `requirements/09-BugFix/done/BUG-167-Claude-Supported-Model-Ids-Missing-Prefix.md`
- Related Documents: `supabase/migrations/20260529153000_add_ai_supported_models.sql`
- Replaces: `none`
- Tags: `supabase, migration, ai-supported-models, settings`

> **Superseded by `BUG-167`**: a follow-up live query (`select * from ai_supported_models`) showed the diagnosis in this document was wrong — `claude-haiku` was never *missing*; the registry had it under the bare id `"haiku"` instead of the canonical `"claude-haiku"`. This document's migration (which would have inserted a second, duplicate `"claude-haiku"` row alongside the existing `"haiku"` one) was removed before ever being applied anywhere and replaced by `BUG-167`'s in-place id correction. Left in place, unedited otherwise, as an accurate record of what was investigated and concluded at the time — see `BUG-167` for the corrected root cause and fix.

## AI Quick View

### Summary

- User-reported "Issue 1": the desktop `Settings > Workflows > Step Definitions` detail editor showed "Gemini 3.5 Flash (Medium)" in the Model dropdown for the "Flow: Coder" catalog row, even though that row's actual `step_definitions.model` (confirmed via the Supabase table editor and the list card's plain-text display) is `claude-haiku`.
- A live query (`select ... from ai_supported_models where model_id = 'claude-haiku'`) returned **no rows** — `claude-haiku` is entirely absent from the registry table, even though the original seed migration (`20260529153000_add_ai_supported_models.sql`) includes it.
- Not a code bug: `stepDraft.model` correctly holds `"claude-haiku"` throughout (confirmed by tracing `mapStepDefinition` → `refresh()` → `setStepDraft`), but the Model `<select>` has no `<option value="claude-haiku">` to match it against (`modelOptions` is built from `ai_supported_models`, filtered to enabled rows) — a controlled `<select>` with no matching option renders the browser's default (the first option), even though the bound value is still correct underneath.

### Current Ask

- Get `claude-haiku` (and, defensively, `claude-sonnet`/`claude-opus`, sharing the same root cause) back into `ai_supported_models` so the dropdown has a matching option.

### Key Decisions

- `F-1` New migration re-inserts the three Claude seed rows with `on conflict (model_id) do nothing` — never overwrites a row an admin may have already edited via the registry's own edit UI (Task-013/Task-054).
- `F-2` No application code change — the desktop UI's data flow (`mapStepDefinition`, `stepDraft`, the Model `<select>`) was already correct; the bug was purely a missing registry row.

### Constraints

- Scoped to the three Claude models only; did not re-audit or re-seed Gemini/Codex rows, since the user's screenshots show those provider tabs and models rendering correctly (only Claude was affected).

### Open Questions

- Whether the original seed migration genuinely never ran on this deployment, or the row was deleted later, is unknown and not resolvable from this environment — the fix (re-insert, idempotently) is correct either way.

### Source Refs

- `supabase/migrations/20260702150000_reseed_missing_claude_supported_models.sql`
- `supabase/migrations/20260529153000_add_ai_supported_models.sql` (original seed, confirmed to include `claude-haiku` — so this is data drift, not a missing feature)
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (`buildModelOptions`, the Model `<select>` — confirmed correct, not modified)

## 1. Issue Summary

The Step Definitions detail editor's Model dropdown displayed the wrong (first-in-list) model for any step type whose actual configured model was `claude-haiku`, because that model id had no matching `<option>` in the registry-backed dropdown.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, `Settings > Workflows > Step Definitions`, a live Supabase instance where `claude-haiku` is missing from `ai_supported_models`.
- reproduction steps: open the "Flow: Coder" (or any `claude-haiku`-configured) step definition's detail editor; the Model field shows a different model than the row's actual `model` column.
- frequency: deterministic on any deployment where `claude-haiku` is missing from the registry.

## 4. Expected vs Actual

- expected: Model dropdown shows `claude-haiku` / "Claude Haiku" selected, matching the underlying data.
- actual: Model dropdown shows the first option in the (Claude-less) list instead.

## 5. Impact

- users affected: anyone on a deployment missing Claude rows from `ai_supported_models`.
- workflows affected: `Settings > Workflows > Step Definitions` display only — the underlying `step_definitions.model` value was never actually wrong or corrupted, only the dropdown's visual selection.
- severity: low — cosmetic/confusing, not a data-correctness bug.

## 6. Root Cause

- confirmed cause: `ai_supported_models` is missing `claude-haiku` (confirmed via direct query returning zero rows) on this deployment, despite the seed migration defining it — a data-drift issue, not an application bug.
- evidence: live query result (zero rows for `model_id = 'claude-haiku'`); code trace confirming `stepDraft.model`/`mapStepDefinition` correctly carry the real DB value throughout, with only the `<select>`'s option list affected.

## 7. Fix Strategy

- `F-1`/`F-2` as described in Key Decisions.

## 8. Validation

- `V-1` Reviewed the new migration for safety: purely additive (`on conflict do nothing`), cannot clobber an existing row.
- `V-2` Not executed: re-querying `ai_supported_models` after applying the migration to confirm `claude-haiku` now exists and the Step Definitions editor renders it correctly — no Supabase/backend access in this environment. The user should re-run the same `select ... where model_id = 'claude-haiku'` query after applying this migration to confirm it now returns a row.

## 9. Regression Guard

- tests: none — pure data migration.
- alerts: none.
- audit checks: recorded in `change-audit/CA-203-reseed-missing-claude-supported-models.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: none.
