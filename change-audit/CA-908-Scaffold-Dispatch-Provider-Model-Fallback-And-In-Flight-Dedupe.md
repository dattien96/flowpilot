# CA-908: Scaffold Dispatch Drops Provider/Model + Concurrent Turn Race

## Summary

- Bug (live repro): after onboarding a react-native project with
  provider=devin / model=devin/swe-2-max, the post-init AI Scaffold turn
  spawned `codex --sandbox workspace-write exec` with no model and died
  with `MODULE_NOT_FOUND` for the orphaned codex npm shim. Run artifacts
  showed two scaffold turns fired ~300 ms apart: the create_project
  auto-trigger correctly resolved `devin --model swe-2-max`, while the
  TUI `/init all` follow-up dispatch (`POST /client/projects/{id}/scaffold`,
  trigger=init_all) carried no providerKey/modelName, so
  `resolveScaffoldProviderKey` fell back to the registry default = codex.
- Root causes (two):
  1. `Client.DispatchScaffold` / `cmdDispatchScaffold` never forwarded the
     session provider/model, and `handleDispatchScaffold` had no fallback to
     the project's stored `default_model` — only platform/path fell back
     via `lookupProject`.
  2. `handleDispatchScaffold` ignored `scaffoldInFlight` entirely, so a
     manual dispatch could stack a second concurrent AI turn on top of the
     in-flight create_project trigger — two AI processes writing into the
     same workspace.
- Fix:
  - `handleDispatchScaffold` now falls back to `project.Model`
    (projects.default_model) when the body omits modelName, before the
    registry default is consulted. Explicit providerKey > explicit
    modelName > project default_model > registry default.
  - The in-flight claim is shared: extracted `claimScaffold` /
    `releaseScaffold` on InteractiveService, used by both
    `autoTriggerScaffold` and `handleDispatchScaffold`. A dispatch landing
    while a turn runs returns `status: skipped` ("a scaffold turn is
    already in progress") instead of spawning a duplicate.
  - TUI `DispatchScaffold` sends `providerKey` + `modelName` from the
    session (`m.provider` / `m.model`); `cmdDispatchScaffold` passes them.

## Files

- `apps/local-runner/internal/runner/scaffold_handler.go`
  (default_model fallback in handleDispatchScaffold, claim/release
  helpers, in-flight skip result)
- `apps/local-runner/internal/runner/interactive_service.go`
  (scaffoldInFlight comment — now shared with HTTP dispatch)
- `apps/local-runner/internal/tui/client/client.go`
  (DispatchScaffold signature + providerKey/modelName body fields)
- `apps/local-runner/internal/tui/app/init_engine.go`
  (cmdDispatchScaffold forwards m.provider/m.model)
- `apps/local-runner/internal/runner/scaffold_handler_test.go`
  (new TestScaffoldDispatch_FallsBackToProjectDefaultModel +
  TestScaffoldDispatch_SkipsWhileTurnInFlight — red before, green after)
- `apps/local-runner/internal/tui/app/init_engine_test.go`
  (new TestInitEngine_ScaffoldDispatchForwardsSessionProviderAndModel —
  red before, green after)

## Out of Scope

- `TestCreateProject_AutoTriggersScaffoldForCapablePlatform` and
  `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold` fail in this
  working tree with and without this change (verified via targeted stash):
  broken by in-progress CP-71 work in the same package, not by this fix.
- The user's machine has an orphaned codex npm shim
  (`@openai/codex/bin/codex.js` missing) — environment issue; the fix
  removes the silent fallback that reached it, but does not repair codex.
- `gitnexus` MCP unreachable during this change (tool listing failed);
  impact assessed manually: `handleDispatchScaffold` is only bound to the
  scaffold route, `DispatchScaffold`'s only caller is
  `cmdDispatchScaffold`, `claimScaffold`/`releaseScaffold` are new private
  helpers — leaf-level, no signature consumers outside this change.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: live-repro-tui-init-all-scaffold
change_type: bugfix
summary: scaffold dispatch falls back to project default_model, forwards session provider/model, dedupes in-flight turns
# --->8---
