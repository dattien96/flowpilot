# CA-1076: searchable provider-grouped Model select in Workflows/Steps settings

Date: 2026-10-01
Refs: CA-1075 (builtin mirror fix that surfaced the issue), BUG-290
(stepModelVisibility dependency-light test pattern), Task-320 (per-node
model tier column).

## Problem

Step/Workflow "Model" and "Model override" fields rendered a flat native
`<select>` over every enabled `ai_supported_models` row — 212 options at
time of report. The catalog is sorted by `sort_order`, which packs ~100
detected opencode/opencode-go rows between the seed tier and the Devin
rows, so entries like `devin/swe-2-high` sat past the fold in a list with
no way to filter. Reported as "the Step settings model list doesn't show
swe-2 even though chat mode has it" — chat filters by the selected
provider; the settings select showed everything, just unreachable.

## Solution

New `SearchableModelSelect` component replaces all three model selects in
`WorkflowsSettings.tsx` (step-editor Model, create-workflow Model
override, edit-workflow Model override):

- Trigger shows the selected display name (or placeholder / `(inherit)`
  when unset).
- Popover with autofocused search input filtering on display name, model
  id, and provider key/label — "swe" finds `Devin SWE-2 (High)`, "devin"
  lists every Devin row.
- Options grouped under provider headers (Claude / Codex / Gemini / Grok
  / OpenCode / Devin, unknown keys capitalized), preserving catalog
  sort_order inside and across groups.
- `(inherit)` stays the "" option for the step-editor select; Enter picks
  the first filtered option; Escape / outside-click closes.

`buildModelOptions` moved to `stepModelOptions.ts` (dependency-light for
standalone tsx tests, same constraint as `stepModelVisibility.ts`) and now
carries `providerKey`; the DEFAULT_MODEL empty-catalog fallback stays in
`modelOptionsFor` locally.

## Files

- `src/components/settings/stepModelOptions.ts` — `buildModelOptions`,
  `filterModelOptions`, `groupModelOptions`, `modelProviderLabel`.
- `src/components/settings/SearchableModelSelect.tsx` — the component.
- `src/components/settings/WorkflowsSettings.tsx` — three select sites
  replaced; local `buildModelOptions` → `modelOptionsFor`.
- `src/styles.css` — `.model-select-*` styles.
- `src/components/settings/stepModelOptions.test.ts` — 4 node:test cases
  (enabled-only build, provider grouping/order + swe-2 reachability,
  query filtering by name/id/provider, label fallback).

## Verification

- `npx tsx --test stepModelOptions.test.ts` — 4/4 pass.
- `npm run typecheck` clean.
- `npm run test:phase1` — 714/726 pass; the 12 failures are pre-existing
  unrelated domains (jira integration, history-replay ordering, style
  token lint, HttpRunnerRepository) — none touch WorkflowsSettings or the
  model select.

## Runtime note

Setting a model only takes effect on `agent.delegate` / `agent.code`
steps (`resolveFlowNodeModel`, BUG-352); `contract-planner`/scout ignore
DB rows entirely (CA-616). The select deliberately still shows every
provider — a Devin pin on `coder` is a legitimate cross-provider choice.
