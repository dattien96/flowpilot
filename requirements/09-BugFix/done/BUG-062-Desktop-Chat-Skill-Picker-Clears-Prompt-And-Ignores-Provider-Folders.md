# BUG-062: Desktop Chat Skill Picker Clears Prompt And Ignores Provider Folders

## Metadata

- Document ID: `BUG-062`
- Title: `Desktop Chat Skill Picker Clears Prompt And Ignores Provider Folders`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../../08-Task/done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- Child Documents: `none`
- Related Documents: [CA-075: Desktop Chat Mode Split And BUG-060 History Fix](../../change-audit/CA-075-desktop-chat-mode-split-and-bug060-history-fix.md)
- Replaces: `none`
- Tags: `desktop, bugfix, regression, skills, provider, chat`

## AI Quick View

### Summary

- Chat-mode prompt text was being cleared when the skill picker closed after selecting a skill from the grouped UI.
- The interactive runner skill endpoint still returned a static fake catalog and ignored provider-specific folders plus the selected workspace path.
- Follow-up fixes now cover Supabase project binding path resolution, provider-specific project skill folder rules, and skill-picker ordering/label/layout behavior.

### Current Ask

- Reopened on `2026-06-16`: the runner could load workspace skills when `cwd` was supplied, but desktop state still refreshed skills without the selected project path in some project/provider flows.
- Follow-up on `2026-06-16`: Supabase-backed projects could return an empty or stale `Project.path` because the desktop navigator did not receive machine-valid workspace binding paths.
- Done. Prompt text stays intact when selecting skills from the chat UI, provider skill lists now load from the correct project/account folders, and the picker UI now reflects the current catalog with the expected ordering and labels.

### Key Decisions

- `V-1` Keep slash-triggered picker cleanup behavior, but do not clear normal prompt text when the grouped skill picker closes.
- `V-2` Reuse the existing markdown skill parser and provider-home discovery helpers instead of introducing a second skill metadata format.
- `V-3` Merge project-local and provider-home skill sources by skill name, with project-local entries taking precedence over account-home defaults.
- `V-4` Resolve the selected project path centrally in desktop state so initial, project-change, and provider-change skill loads request the same workspace-aware catalog.
- `V-5` Supabase project catalog responses must expose a local workspace path selected from project bindings, preferring a binding that exists on the current machine.
- `V-6` Project-local skill folders are provider-specific: `codex` and `gemini` use project `.agents/skills`, while `claude` uses project `.claude/skills`, then all merge with provider account-home defaults.
- `V-7` The picker UI should present only two user-facing source labels, `Project` and `Account`, and should keep selection-order reshuffling deferred until the next time the picker opens.

### Constraints

- Keep the bugfix narrow to desktop chat input and local runner skill discovery.
- Do not alter turn execution or provider contract shapes.

### Open Questions

- None for this slice.

### Source Refs

- `Task-044`
- `Task-049`
- `CA-075`

## 1. Issue Summary

In desktop chat mode, users could type a prompt, open the grouped skill picker, choose a skill, and then lose the prompt text when closing the picker. Separately, the provider skill list was not loading the real skills from provider-specific folders and the selected workspace, because the runner endpoint still served a static fake list.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../../08-Task/done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- impacted tech design: [SD-05: Workflow Engine](../../06-System-Tech-Design/SD-05-Workflow-Engine.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` + `apps/local-runner`
- reproduction steps:
  - open desktop chat mode
  - type a prompt in the composer
  - open the skill picker from the grouped skill button
  - choose a skill and close the picker
  - observe that the prompt text is cleared
  - request provider skills for a provider/workspace that has real skill folders
  - observe that only the static fake list is returned
- frequency: always

## 4. Expected vs Actual

- expected:
  - closing the grouped skill picker should preserve the current prompt text
  - provider skill loading should read real skills from provider-home and selected workspace folders
- actual:
  - grouped skill picker close logic cleared the whole prompt text
  - interactive skill loading ignored `provider` and `cwd` and returned a static list

## 5. Impact

- users affected: desktop chat users relying on grouped skill selection
- workflows affected: normal chat mode skill attachment and provider-specific skill discovery
- severity: medium

## 6. Root Cause

- hypothesis:
  - the grouped picker close handler was treating all picker closes like slash-query cleanup
  - the interactive skill endpoint never moved off the fake catalog implementation
- confirmed cause:
  - `ChatInput` cleared `text` on any outside click while the picker was open
  - `interactiveCatalog.listSkills` ignored both `provider` and `cwd` and returned `c.skills`
  - follow-up: desktop state still had skill refresh paths that called `listSkills(provider)` without the selected project `cwd`, so Codex could show only provider-home/default skills such as `imagegen` and `openai-docs`
  - follow-up: `SupabaseCatalogStore.ListProjects` selected only `id,name`, so Supabase-backed projects had no usable `path` even when `project_workspace_bindings` contained the correct local checkout for the current Mac
  - follow-up: project-local skill folder rules were still ambiguous, so Codex/Gemini could incorrectly look at provider-specific project folders instead of project `.agents/skills`
  - follow-up: the picker UI exposed internal source names (`flowpilot`, `workspace`, `provider`) and immediately re-sorted the open list after each selection, which made long-list selection harder
- evidence:
  - UI code path in `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - state refresh path in `apps/desktop-flowpilot/src/state/store.ts`
  - Supabase project catalog path in `apps/local-runner/internal/runner/supabase_catalog_store.go`
  - runner code path in `apps/local-runner/internal/runner/interactive_catalog.go`

## 7. Fix Strategy

- `F-1` Preserve prompt text when closing the grouped picker, while still clearing slash-only picker queries.
- `F-2` Discover provider skills from the selected workspace provider folder, workspace `.agents/skills`, and provider-home skill folders.
- `F-3` Deduplicate merged skills by name with project-local entries taking precedence over provider account-home defaults.
- `F-4` Add a runner regression test covering precedence and real directory loading.
- `F-5` Reload skills with the selected project path from desktop state when projects load, when the provider changes, and when the selected project changes.
- `F-6` Populate `Project.path` from Supabase `project_workspace_bindings.local_path`, choosing a binding that exists locally before falling back to stored primary/stale paths.
- `F-7` Enforce provider-specific project skill folders: `codex` and `gemini` read project `.agents/skills`; `claude` reads project `.claude/skills`; all providers still merge account-home default skills.
- `F-8` Update the picker UI so skill source badges collapse to `Project` and `Account`, selected-skill counts reflect the current live catalog, the modal rows use the compact two-line layout, and selected skills sort to the top only on the next picker open.

## 8. Validation

- `V-1` `go test ./internal/runner -run TestSkills -count=1`
- `V-2` `npm run typecheck --prefix apps/desktop-flowpilot`
- `V-3` `git diff --check`
- `V-4` `go test ./internal/runner -run 'TestSkills(ServedLocallyWithInjectedStore|MergeClaudeProjectAndProviderHomeWithPrecedence|MergeCodexProjectAgentsAndProviderHomeWithPrecedence|MergeGeminiProjectAgentsAndProviderHomeWithPrecedence)' -count=1`
- `V-5` `go test ./internal/runner -run TestSupabaseCatalogStoreShaping -count=1`
- `V-6` `go test ./internal/runner -run 'TestSkills(ServedLocallyWithInjectedStore|MergeClaudeProjectAndProviderHomeWithPrecedence|MergeCodexProjectAgentsAndProviderHomeWithPrecedence|MergeGeminiProjectAgentsAndProviderHomeWithPrecedence)' -count=1`
- `V-7` `npm run typecheck --prefix apps/desktop-flowpilot`

## 9. Regression Guard

- tests: added provider-specific project/account skill merge coverage in `phase8_a1_test.go`
- alerts: none
- audit checks: GitNexus impact reviewed for `ChatInput`, `listSkills`, `handleListSkills`, `loadProjects`, `selectProject`, `selectProvider`, and `loadSkills`

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - no new frontend test harness was added in this slice
