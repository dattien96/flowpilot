# CA-229: Navigator Catalog Model/YOLO Mapping

## Summary

User reported the `main` orchestrator card still showed `CODEX gpt-5.4-mini` before starting a run on the built-in "Review Loop" workflow, even after `CA-227`'s priority-order fix, despite Settings clearly showing `Model override: Claude Haiku` for that same workflow. Traced to a more fundamental bug than `CA-227`: `mapNavigatorWorkflow`/`mapNavigatorStep` (`navigatorCatalog.ts`) never copied `modelOverride`/`yoloMode` (workflow) or `model`/`yoloMode` (step) from the admin catalog data at all, so the desktop's pre-run preview always saw `undefined` and fell straight to the project's default model — for every workflow and step, in every project, regardless of configuration. `CA-227`'s fix only changed priority ordering for the *post-run* runtime-meta state; it never touched this pre-run data source, which is why the symptom persisted. Fixed both mapping functions and added direct regression coverage for the exact case (non-null override) neither pre-existing test exercised. Filed `BUG-230` (fixed).

## What Changed

- `apps/desktop-flowpilot/src/app/navigatorCatalog.ts`:
  - `mapNavigatorWorkflow` now maps `model: workflow.modelOverride ?? undefined` and `yoloMode: workflow.yoloMode`.
  - `mapNavigatorStep` now maps `model: definition.model || undefined` and `yoloMode: definition.yoloMode`.
- `tests/phase1/navigatorCatalog.test.ts`:
  - Updated the two pre-existing tests' expected output to include the now-mapped `model`/`yoloMode` fields.
  - Added `mapNavigatorWorkflow carries modelOverride and yoloMode through to the navigator Workflow` and `mapNavigatorStep carries model and yoloMode through to the navigator Step`, both exercising a non-null override — the case the pre-existing tests (both using `null`/default values) never caught.
- `requirements/09-BugFix/done/BUG-230-Navigator-Catalog-Mapping-Drops-Model-And-Yolo-Mode.md`: new bug doc.

## Verification

- `npx tsx --test` across every `apps/desktop-flowpilot` unit test file plus `tests/phase1/navigatorCatalog.test.ts` — 111 passed, 2 failed (identical pre-existing, environment-specific failures already established as baseline this session: missing `localStorage` in the Node test runner, one timing-sensitive test).
- `npm run typecheck` — clean.
- `npm run build` — passes.
- Noted, not caused by this change: `npm run test:phase1` fails to compile for unrelated pre-existing reasons (`desktopSupabaseAuthRepository.test.ts`/`settingsHelpers.test.ts` fake test doubles missing newer interface methods) — confirmed via `git stash` that this pre-dates this session entirely.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-183
change_type: bugfix
summary: Fix navigator catalog mapping to carry workflow/step model and yoloMode overrides through to the desktop's pre-run preview
# --->8---
