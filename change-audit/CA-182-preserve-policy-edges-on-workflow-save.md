# CA-182: Preserve Policy/Edges Fields on Workflow Save (BUG-NOTE-CP42 #14)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: saving a workflow from `WorkflowsSettings.tsx` silently wiped its existing `policyCap`/`policyOnCap`/`policyExtendBy`/`policyExtendMax`/`edges` fields, even for an edit unrelated to any of them (e.g. a plain rename).

## The bug

`SupabaseAdminRepository.saveWorkflow`'s upsert payload has no notion of "field omitted, leave unchanged" — it writes `policy_cap: workflow.policyCap ?? null`, `edges_json: workflow.edges ?? []`, etc., unconditionally. `WorkflowsSettings.tsx`'s `WorkflowDraft` type and both `saveWorkflow`/`saveNewWorkflow` call sites never carried these fields at all, so every save request sent them as `undefined` — which the `??` operator in the repository always coerces to `null`/`[]`. Any edit through this screen (even changing just the description) would collaterally null out a workflow's cap policy and edge graph.

## Fix

`WorkflowDraft` gained pass-through-only fields: `policyCap`, `policyOnCap`, `policyExtendBy`, `policyExtendMax`, `edges`. No UI control edits any of them today (confirmed — neither this bug note nor #3/#22, which specifically audit the editing surface, found one), so they're populated from the loaded `Workflow` in `mapWorkflowToDraft` and sent back unchanged on save. `createEmptyWorkflowDraft` (used only when creating a genuinely brand-new workflow, where there's nothing to preserve) defaults them to `null`/`[]`, matching the repository's own defaults for a new row. Both `saveWorkflow` and `saveNewWorkflow` call sites now include all five fields in their `admin.workflows.saveWorkflow(...)` payload.

## Verification

- `tsc` (the real compiler at `apps/desktop-flowpilot/node_modules/.bin/tsc`, not the `npx tsc` stub — see CA-168's note) against `tsconfig.phase1-tests.json`: no new type errors; the same 3 pre-existing errors in unrelated files (`desktopSupabaseAuthRepository.test.ts`, `navigatorCatalog.test.ts`, `settingsHelpers.test.ts`) remain, confirmed unrelated via `git status`.
- Full `tests/phase1/*.test.js` suite: 42 passed, 3 pre-existing failures (same unrelated files) — unchanged from baseline.
- No dedicated unit test was added: this component has no existing test harness to extend (no fixture/mock setup for `WorkflowsSettings.tsx` itself exists anywhere in the repo), and the fix is a straightforward pass-through wiring change, not new branching logic worth a bespoke harness just for this fix.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: WorkflowDraft now carries policyCap/policyOnCap/policyExtendBy/policyExtendMax/edges as pass-through fields populated from the loaded workflow, so saving an unrelated edit no longer silently nulls out a workflow's existing policy and edge graph
# --->8---
