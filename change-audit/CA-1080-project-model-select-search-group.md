# CA-1080: searchable provider-grouped model select on project settings

Date: 2026-10-01
Refs: CA-1076 (SearchableModelSelect introduced for step/workflow Model
fields), user request: apply the same to the project detail settings page.

## Change

`ProjectsSettings.tsx` project-detail **Default Model** field now uses the
CA-1076 `SearchableModelSelect` (search by name/id/provider, grouped by
provider) instead of the flat native `<select>` over all 200+ enabled
`ai_supported_models` rows. Options come from the same
`buildModelOptions(models)` helper — `SupportedModel[]` satisfies its
input shape directly, so behavior (enabled-only, `sort_order` preserved)
is identical; only the picker UX changes.

## Files

- `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
  — import + single select-site replacement.

## Verification

- `tsc --noEmit` clean.
- Component logic covered by `stepModelOptions.test.ts` (grouping,
  filtering, provider fallback) — no new logic introduced.
