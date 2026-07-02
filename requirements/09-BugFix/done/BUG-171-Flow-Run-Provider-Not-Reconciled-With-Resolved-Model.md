# BUG-171: Flow Run Provider Not Reconciled With Resolved Model

## Metadata

- Document ID: `BUG-171`
- Title: `Flow Run Provider Not Reconciled With Resolved Model`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-165-Implement-Step-Flow-Project-Default-Model-Resolution.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-162-Coder-Reviewer-Steps-Must-Default-To-Claude-Haiku.md`, `requirements/09-BugFix/done/BUG-163-Flow-Pack-Mirror-Sync-Stamps-Codex-Provider-Override.md`, `requirements/09-BugFix/done/BUG-164-Remove-Workflow-Steps-Model-Provider-Reasoning-Overrides.md`
- Replaces: `none`
- Tags: `agent-flow-engine, ai-providers, runner, flow-mode, model-resolution, regression`

## AI Quick View

### Summary

- In Flow Mode, a run was stamped with provider `codex` but model `claude-haiku`, so its turn died with `The 'claude-haiku' model is not supported when using Codex with a ChatGPT account.` The step-timeline sidebar also showed a wrong `CODEX` provider tag next to the `claude-haiku` model. The identical setup works in normal chat, because chat couples provider+model in the UI.
- Root cause: for a workflow/flow launch the desktop sends `providerKey: selectedProvider` (e.g. `codex`) but no model, and the runner resolves the model server-side (Step > Flow > Project > default, BUG-165) — which lands on `claude-haiku` (BUG-162 makes coder/reviewer default to Claude Haiku). The runner only derived the provider from the resolved model when `providerKey` was empty, which a flow launch never is — so the Claude model stayed pinned to the Codex provider.
- A prefixed model (`claude-*`, `gpt-*`, `gemini-*`) is only runnable on its matching provider, so the resolved model must be authoritative over a conflicting launched/inherited provider.

### Current Ask

- A Flow Mode run whose model resolves to a Claude model must run on Claude (and likewise for gpt-*/gemini-* models), regardless of whatever provider the launch happened to carry — both so the turn succeeds and so the step-timeline provider tag is correct.

### Key Decisions

- `V-1` In `createRun`, when the resolved model has a known provider prefix, that provider wins over the explicit/inherited `in.ProviderKey`. The provider is only taken from `in.ProviderKey` (or the default) when the model implies no known provider.

### Constraints

- Must not change `normal_chat`: there the UI couples provider+model so they never conflict (a `gpt-*` model already implies codex, a `claude-*` model implies claude) — the change is a no-op for chat, preserved by `TestCreateRunNeverInjectsResolvedModelForChat`.
- Fix is centralized in `createRun`, the single chokepoint every run (hub launch AND spawned child) passes through, so the child-spawn path is covered by the same change without touching `spawnChildRun`.

### Open Questions

- None. (Follow-up now resolved: the Agents panel's `main` card previously hardcoded a `CODEX` provider badge, so a Claude-/Gemini-driven hub still read "CODEX" there. Fixed in this same pass — the badge and model now source from the run's real provider (`workflowStepRuntimeMeta.provider`/`.model` in Flow Mode, else the selected provider/model). See Fix Strategy `F-2`.)

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go` (`createRun` provider resolution — the fix)
- `apps/local-runner/internal/runner/provider_registry.go:172` (`providerKeyFromModel` — the prefix→provider map)
- `apps/desktop-flowpilot/src/state/store.ts:1041` (flow launch sends `providerKey: selectedProvider`, no model)
- `apps/local-runner/internal/runner/interactive_handlers.go:970` (step-runtime meta provider = `rs.providerKey`, so the fix also corrects the sidebar tag)
- `requirements/09-BugFix/done/BUG-162-*` (why coder/reviewer resolve to `claude-haiku`)

## 1. Issue Summary

Launching a Flow Mode workflow ("Review Loop") produced a run stamped `codex` + `claude-haiku`. The turn failed with a provider/model-mismatch error from Codex, and the step-timeline sidebar showed `CODEX` next to `claude-haiku`. The same model works fine in normal chat mode (image 3 in the report: normal chat correctly on Claude Haiku via the Claude provider).

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot on branch `task/gemini-adapter` + local runner; a project whose resolved Flow Mode model is a Claude model (`claude-haiku`), launched with the Codex provider selected/carried.
- reproduction steps:
  1. In Flow Mode, launch a workflow whose Step/Flow/Project model resolution yields `claude-haiku` (BUG-162 default for coder/reviewer), while the launch carries `providerKey: codex`.
  2. Observe the step-timeline sidebar shows `CODEX` + `claude-haiku`.
  3. The run's turn fails: `The 'claude-haiku' model is not supported when using Codex with a ChatGPT account.`
  4. Switch to normal chat with Claude Haiku — works (provider and model are coupled by the UI).
- frequency: deterministic whenever the resolved flow model's provider differs from the launched provider.

## 4. Expected vs Actual

- expected: the run runs on the provider its resolved model requires (Claude for `claude-haiku`); the step-timeline tag reads `CLAUDE`.
- actual: the run kept the launched `codex` provider against a `claude-haiku` model → invalid pairing, failed turn, wrong `CODEX` tag.

## 5. Impact

- users affected: anyone running Flow Mode where the resolved model's provider differs from the provider carried by the launch (notably the coder/reviewer Claude-Haiku default from BUG-162 with a Codex-selected client).
- workflows affected: all Flow Mode / workflow runs; the run fails to execute, not just a display issue.
- severity: high — Flow Mode runs fail outright.

## 6. Root Cause

- confirmed cause: `createRun` (`interactive_handlers.go`) resolves the model for a workflow run via Step > Flow > Project > default (BUG-165), but its provider resolution was `providerKey := in.ProviderKey; if providerKey == "" { deriveFromModel() }`. The desktop's flow launch sends `providerKey: selectedProvider` (non-empty — `store.ts:1041`), so the `deriveFromModel` branch never ran and the resolved Claude model stayed pinned to the launched Codex provider. `normal_chat` never hit this because its provider and model are chosen together in the UI and are always consistent.
- evidence: `store.ts:1041` sends a provider but no model for workflow launches; `providerKeyFromModel` (`provider_registry.go:172`) maps `claude-*`→claude / `gpt-*`→codex / `gemini-*`→gemini; the runner error text `"the 'claude-haiku' model is not supported when using Codex with a ChatGPT account"`.

## 7. Fix Strategy

- `F-1` **(implemented)** In `createRun`, the resolved model's implied provider is now authoritative: `if pk, ok := providerKeyFromModel(resolvedModel); ok { providerKey = pk } else if providerKey == "" { providerKey = default }`. A known-prefix model wins over a conflicting explicit/inherited provider; the explicit provider (or default) is only used when the model implies no known provider. This is centralized in `createRun`, so both the hub launch and spawned children (which also go through `createRun`) are covered, and — because the step-runtime meta provider is read straight off `rs.providerKey` (`interactive_handlers.go:970`) — the step-timeline tag now shows the correct provider too.
- `F-2` **(implemented)** Fixed the Agents-panel `main`-card badge, which was hardcoded to `CODEX` (`AgentsPanel.tsx`). It now renders `mainProvider`/`mainModel` sourced from `workflowStepRuntimeMeta.provider`/`.model` (the run's real provider in Flow Mode, made authoritative by `F-1`), falling back to `selectedProvider`/`selectedModel` for normal chat and to `codex` only when nothing is known yet. So a Claude-driven hub now reads `CLAUDE` there, matching the (now-correct) step-timeline tag.

## 8. Validation

- `V-1` Added `TestCreateRunReconcilesProviderToResolvedModel` (`workflow_model_resolution_test.go`): a workflow launch with explicit `providerKey=codex` and a project model of `claude-haiku` now yields a run stamped `claude` + `claude-haiku`. Uses a test registry with Claude marked Available (`registryWithClaudeAvailable`) so the reconciled provider is selectable in-test. Passes.
- `V-2` `TestCreateRunNeverInjectsResolvedModelForChat` and the four `TestCreateRun*` model-tier tests still pass — the change is a no-op for chat and for gpt-* workflow models.
- `V-3` All change-relevant groups green: `TestCreateRun* / TestListAgentRunSummaries* / TestSpawnAgent* / TestResumeRun*` — 23 passed, 0 failed.
- `V-4` Ran the full runner `go test ./...`; the failures present are all pre-existing and environment-specific (missing `codex`/`agy`/`go` binaries, OS path quoting) — confirmed identical on a clean `git stash` baseline, none introduced by this change.
- `V-5` `npm run typecheck` in `apps/desktop-flowpilot` — clean (covers the `F-2` Agents-panel change).
- `V-6` Not executed: a live Flow Mode launch confirming the turn now succeeds on Claude and both the sidebar tag and the Agents-panel `main` badge read `CLAUDE` — no runner + provider account available in this environment. The user should confirm on next use.

## 9. Regression Guard

- tests: `TestCreateRunReconcilesProviderToResolvedModel` (new) guards the reconciliation; existing `TestCreateRunNeverInjectsResolvedModelForChat` guards no regression to chat. The `F-2` badge change is presentational (no render-test harness for this component, consistent with prior desktop UI fixes).
- alerts: none.
- audit checks: recorded in `change-audit/CA-208-flow-run-provider-reconciled-with-model.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the desktop still sends `providerKey: selectedProvider` for flow launches — harmless now that the server reconciles, and kept because it remains a sensible fallback provider for a resolved model with no known prefix.
