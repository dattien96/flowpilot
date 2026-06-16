# Task-054: AI Provider Supported Models Edit And Delete

## Metadata

- Document ID: `Task-054`
- Title: `AI Provider Supported Models Edit And Delete`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-013: Dynamic Way To Add Supported Model](./Task-013-Dynamic-Way-To-Add-Support-Model.md)
- Child Documents: `none`
- Related Documents: [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](../09-BugFix/done/BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md)
- Replaces: `none`
- Tags: `desktop, admin-web, settings, ai-providers, supported-models`

## AI Quick View

### Summary

- Task-013 added create + toggle + delete (admin-web) and create + toggle (desktop) for supported models, but left no edit path for `modelId` or `displayName` on either surface.
- The desktop `AiProvidersSettings` had no delete button at all.
- `supabaseAdminRepository.updateSupportedModel` accepted a `modelId` patch but never wrote `model_id` to the Supabase payload — model ID changes were silently discarded.
- Both surfaces now support inline edit (Display Name + Model ID) and delete, backed by a corrected repository method.

### Current Ask

- Done. Inline edit and delete are live on both the desktop settings panel and the admin-web AI Providers page.

### Key Decisions

- `T-1` Inline edit replaces the row in-place (no modal): two inputs pre-filled with current values, Save/Cancel controls. Switching a row to edit mode does not affect other rows.
- `T-2` Save updates local state directly from the `updateSupportedModel` return value — no full `refresh()` round-trip — so the list reflects the change even when the local runner is offline.
- `T-3` Delete confirms via `window.confirm` before calling `deleteSupportedModel`; the row is removed optimistically in admin-web (filter from local state) and via `refresh()` in desktop.
- `T-4` The `model_id` column was missing from the Supabase `UPDATE` payload in `supabaseAdminRepository`; the fix adds it so model ID edits now persist in the DB.

### Constraints

- Do not change the provider list, toggle, add-model, or import flows introduced in Task-013.
- Desktop component uses existing `settings-*` CSS classes; no new stylesheet additions.

### Open Questions

- None for this slice.

### Source Refs

- User request on `2026-06-16`: need edit/delete UI for supported models on the AI Providers settings tab in the desktop app.
- Root-cause discovery: `supabaseAdminRepository.ts` line 706–709 mapped only `displayName`, `isEnabled`, `sortOrder`; `modelId` was not mapped.

## 1. Goal

Users can edit the Display Name and Model ID of any supported model, and delete models they no longer need, from both the desktop settings panel and the admin-web AI Providers page — without code or migration changes.

## 2. Parent Links

- coding plan: [Task-013: Dynamic Way To Add Supported Model](./Task-013-Dynamic-Way-To-Add-Support-Model.md)
- tech design: `none` (UI delta on top of existing CRUD gateway)
- system spec: `none`
- specific upstream ids: `none`

## 3. Trigger

Task-013 shipped create + toggle for both surfaces and delete for admin-web only. The desktop had no delete and neither surface had edit. Additionally, users discovered that renaming a model ID silently failed — the repository bug blocked any future edit feature from working.

## 4. Exact Change

- `T-1` **`packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`** — add `if (patch.modelId !== undefined) payload.model_id = patch.modelId` to `updateSupportedModel` so model ID changes persist in Supabase.
- `T-2` **`apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`** — add `editingModelId` + `editDraft` state; add `saveEditModel` (calls `updateSupportedModel`, patches local `models` state from return value); add `deleteModel` (confirms, calls `deleteSupportedModel`, then `refresh()`); each model row gains **Edit** and **Delete** buttons; clicking Edit expands inline inputs pre-filled with current values.
- `T-3` **`apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx`** — import `Pencil`, `Check`, `X` from lucide-react; add `editSupportedModel` mutation (patches `models` state from return value on success); add `editingModelId` + `editDraft` state; model rows gain a pencil icon that switches to inline inputs + ✓/✗ controls; delete button gap changed from `gap-2` to `gap-1` to accommodate the new icon.

## 5. Touched Areas

- files:
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`
  - `apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx`
- modules: `flowpilot-client-core` (data layer), desktop settings, admin-web settings
- routes: `/_authenticated/settings/ai-providers` (admin-web)
- tables: `ai_supported_models` (Supabase — `model_id` column now writable via the update path)

## 6. Acceptance Check

- Edit a model's Display Name → row updates immediately in the list without page reload.
- Edit a model's Model ID → row updates immediately in the list; the new ID is persisted in the DB (was the bug).
- Click Delete → confirm dialog appears; on confirm, model is removed from the list.
- Cancel edit → row reverts to read-only view with original values unchanged.
- Save button is disabled when either field is empty.
- `npm run typecheck --prefix apps/desktop-flowpilot` passes.
- `npx tsc --noEmit` in `apps/admin-web` shows no new errors in `ai-providers.tsx`.

## 7. Out of Scope

- Reordering models (drag-and-drop sort order).
- Editing `providerKey` of an existing model (create + delete is the intended path for re-assigning).
- Bulk edit or bulk delete.
- Admin-web delete confirmation UI (uses native `window.confirm`, same as desktop).

## 8. Completion Notes

- result: All three files updated and typechecked clean. Repository bug (`model_id` not written to Supabase) found and fixed as part of this task.
- follow-ups: none identified.
- upstream docs updated: none — this is a pure UI/data-layer delta on top of Task-013; no SS, SD, or CP rules changed.
