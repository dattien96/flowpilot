# CP-34: Init / Setup Tool — Desktop Engine Page & Project Auto-Init

## Metadata

- Document ID: `CP-34`
- Title: `Init / Setup Tool — Desktop Engine Page & Project Auto-Init`
- Phase: `coding_plan`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-104: Runner Engine-Setup Endpoints](../../08-Task/done/Task-104-Runner-Engine-Setup-Endpoints.md) (P-1), [Task-105: Desktop Settings Engine Page](../../08-Task/done/Task-105-Engine-Setup-UI-Tab.md) (P-2), [Task-106: Bind-Time Auto-Init Orchestration](../../08-Task/done/Task-106-Bind-Time-Auto-Init-Orchestration.md) (P-3)
- Related Documents: [CP-35: Context And Regression Engine Rollout](../inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-31: Auto-Document Process](../done/CP-31-Auto-Document-Process.md), [CP-10: Integrations, Memory & Context Intelligence](../inprogress/CP-10-Integrations-Hardening.md), [CP-33: Desktop Project Chat Drive Folder Selection](./CP-33-Desktop-Project-Chat-Drive-Folder-Selection.md)
- Replaces: `None`
- Tags: `init, setup, desktop, engine, tooling, skill-pack, auto-init, local-runner`

## AI Quick View

### Summary

- The engine setup surface now splits machine-global and project-scoped concerns correctly: tooling is global, while skill-pack, capability profile, ledger/catalog, and last-init state are project-local.
- The shipped desktop UX is one top-level page at `Settings -> Engine`, not an embedded panel inside project details.
- The runner exposes a machine-global tooling endpoint plus project-scoped engine status/init endpoints.
- Project create/save flows still trigger best-effort auto-init because bindings are established there, not inside the runner.
- The init flow remains non-fatal: missing tools degrade project capability, but they do not block project creation, binding saves, or project usage.
- The desktop Supabase setup page also needs a manual schema-init action so an operator can push the checked-in `supabase/migrations/*.sql` files into the configured remote Supabase project from the app.

### Current Ask

- Keep one desktop Engine menu page for global tooling and selected-project engine state, and keep project auto-init wired to binding save flows.
- Add one Supabase-page action that applies the repo migration files to the configured Supabase project.

### Key Decisions

- `P-1` Global tooling is exposed separately through `GET /client/engine/tooling/status` because `gitnexus`, `rtk`, and `node` are machine-global checks and should not be framed as per-project state.
- `P-2` The desktop user-facing surface is `Settings -> Engine`, with two subareas: global tooling and selected project skill-pack/capability/init status.
- `P-3` Project engine status stays on `GET /client/projects/{projectId}/engine/status?workingDirectory=...` and init stays on `POST /client/projects/{projectId}/engine/init`, because the runner still requires an explicit workspace path.
- `P-4` Bind-time auto-init stays in desktop project create/save flows, because those are the only flows that own project bindings in the current codebase.
- `P-5` Bind-trigger skip logic is based on project-local engine state (`.flowpilot/engine-init.json`) plus current bundled skill-pack status; global tooling availability is checked live, not used as a per-project gating file.
- `P-6` Supabase schema init is operator-triggered from `Settings -> Supabase`, reads the repo's `supabase/migrations/` files in order, and applies only missing versions to the configured remote project.

### Constraints

- Operate only on the bound target project's workspace dir; FlowPilot itself is allowed when it is intentionally the bound target (`SS-14 AC-1`).
- Reuse the existing runner HTTP surface and existing desktop settings patterns.
- Do not invent runner-side binding lookup; the shipped contract still requires `workingDirectory`.
- Missing external tools remain non-fatal and must be rendered as degraded state instead of hard failure (`SS-14 AC-13`).

### Open Questions

- `Q-1` Should a later slice add guided install docs or auto-install actions for missing global tooling? Current behavior is detect-only.

### Source Refs

- `SD-17 §3.6`, `§6.4`, `§7.3`, `§9`
- `SS-14 AC-1`, `AC-12`, `AC-13`
- `CP-35 §4.6`, `§4.7`
- `Task-104`, `Task-105`, `Task-106`

## 1. Goal

Provide one desktop Engine page that shows machine-global tooling health and selected-project engine readiness, while project binding flows automatically initialize project-local engine assets in the background.

## 2. Input Documents

- [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- [CP-35: Context And Regression Engine Rollout](../inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)

## 3. Implementation Strategy

- Reuse the existing `skillpack`, `tooling`, `changeledger`, and `featurecatalog` packages; this plan only wires transport, desktop surface, and save-flow orchestration.
- Split state by scope:
  - machine-global: `gitnexus`, `rtk`, `node`
  - project-local: `skill_pack`, capability profile, `.flowpilot/engine-init.json`, ledger/catalog artifacts
- Ship the visible manual surface first through runner endpoints plus the desktop Engine page, then keep project save flows auto-initializing in the background using the same runner init path.

## 4. Work Breakdown

### `P-1` Runner engine-setup transport

- Add `GET /client/engine/tooling/status` for machine-global tooling checks.
- Keep `GET /client/projects/{projectId}/engine/status?workingDirectory=<abs-path>` for project-local engine state.
- Keep `POST /client/projects/{projectId}/engine/init` with `{ workingDirectory, trigger }` to run tooling check, skill-pack sync, change-ledger build, and feature-catalog build.
- Return non-fatal degraded status when external tools are missing.
- Allow any intentionally bound target directory, including FlowPilot itself when selected as a project.

### `P-2` Desktop `Settings -> Engine` page

- Add one top-level settings menu item named `Engine`.
- Render a `Global Tooling` section that calls `GET /client/engine/tooling/status`.
- Render a `Project Skill Pack` section that lets the user pick a project and binding, then shows project-local capability, skill-pack state, tooling summary, and last init.
- Keep manual actions on that page:
  - `Refresh Tooling`
  - `Refresh Project`
  - `Initialize / Re-sync Project`
- Remove the earlier embedded engine panel from `ProjectsSettings`.

### `P-3` Bind-time auto-init orchestration

- Keep bind-trigger auto-init in desktop project create/save flows, because that is where bindings exist.
- Call the same runner init endpoint with `trigger=bind` for each normalized binding.
- Make the call best-effort and async so project create/save succeeds even if engine init is partial or fails.
- Persist and later surface the last-init result through the Engine page.

### `P-4` Supabase schema-init action

- Add a button on `Settings -> Supabase` that applies the repo migration files to the configured remote Supabase project.
- The desktop flow uses a Supabase Management API access token provided at action time; it must not be persisted in the local runtime config file.
- The runner reads `supabase/migrations/*.sql` from the repo workspace, checks remote migration history, and applies only missing versions in ascending order.
- The action must report applied versus skipped migrations clearly so the operator can rerun safely.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/local-runner/internal/runner/engine_setup.go`
  - `apps/local-runner/internal/tooling/check.go`
  - `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
  - `apps/desktop-flowpilot/src/components/settings/EngineSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`
  - `apps/desktop-flowpilot/src/components/SupabaseSetupScreen.tsx`
  - `packages/flowpilot-client-core/src/domain/runtime.ts`
  - `packages/flowpilot-client-core/src/data/runnerRuntimeConfigRepository.ts`
  - `apps/local-runner/internal/runner/supabase_config.go`
  - `apps/local-runner/internal/cli/root.go`
- modules:
  - `runner`
  - `tooling`
  - `skillpack`
  - desktop settings
- database:
  - none
- external systems:
  - local filesystem
  - local runner HTTP API

## 6. Data or Migration Steps

- None.
- Project-local artifacts are written under `<target>/.flowpilot/`.
- Global tooling status is checked live and is not persisted as shared product data.
- Supabase remote schema changes come from the checked-in repo migration files; the temporary Management API token used to trigger the action is never persisted.

## 7. Validation Plan

- Go tests cover runner status/init behavior, runner-workspace targeting, bind-trigger skip, and the global tooling endpoint.
- Desktop build verifies the new settings navigation plus the dedicated Engine page compiles.
- Manual checks verify:
  - `Settings -> Engine` shows global tooling rows
  - selecting a project binding shows project-local engine state
  - project create/save still triggers best-effort background init
  - `Settings -> Supabase` can apply the repo migration set to the configured remote Supabase project and safely skip already-applied versions

## 8. Rollout and Fallback

- Manual engine inspection and re-sync remain available from `Settings -> Engine` even if background auto-init is disabled later.
- If bind-time auto-init causes issues, disable that call path and keep the manual Engine page workflow.
- If the schema-init action fails, operators can fix the failing SQL or remote state and rerun; already-applied versions should be skipped on retry.
- No irreversible data migration exists; `.flowpilot/` state can be regenerated.

## 9. Risks

- `R-1` Users may assume tooling is per-project if the UI mixes scopes. Mitigation: split the page into explicit global and project sections.
- `R-2` Wrong workspace resolution could modify the wrong repo. Mitigation: explicit `workingDirectory` and no runner-side binding inference.
- `R-3` Background init can slow save flows if awaited. Mitigation: async best-effort orchestration.
- `R-4` Missing tooling can appear broken if treated as fatal. Mitigation: degraded status rendering and capability fallback.
- `R-5` Schema-init needs elevated Supabase Management API access. Mitigation: require an explicit operator token only at action time and never persist it.

## 10. Definition of Done

- [x] `P-1` Runner exposes global tooling status and project engine status/init endpoints for any intentionally bound target project.
- [x] `P-2` Desktop has one top-level `Settings -> Engine` page for global tooling and selected-project engine state.
- [x] `P-2` The old embedded engine panel is removed from project details.
- [x] `P-3` Desktop project create/save flows trigger best-effort background project init through the shared runner init endpoint.
- [x] `P-4` `Settings -> Supabase` exposes a manual repo-migration apply action for remote schema initialization.
- [x] Missing tooling degrades project capability without blocking project flows.
- [x] `go test ./internal/runner ./internal/skillpack ./internal/tooling` passes.
- [x] Desktop build passes with the new Engine page.

## 11. Manual Test Guide

1. Start the local runner.
2. Start the desktop app.
3. Open `Settings -> Engine`.
4. In `Global Tooling`, verify rows render for:
   - `gitnexus`
   - `rtk`
   - `node`
5. Click `Refresh Tooling` and confirm the rows update without needing a project selection.
6. In `Project Skill Pack`, select a project that has at least one saved binding.
7. Select one binding and verify the page shows:
   - working directory
   - initialized state
   - capability tiers
   - project tooling summary including `skill_pack`
   - bundled skill-pack entries across `.claude` and `.agents` (reported for Claude, Codex, and Gemini)
   - last init summary and step details
8. Click `Initialize / Re-sync Project`.
9. Verify on disk inside the selected bound repo:
   - `.flowpilot/tooling.json`
   - `.flowpilot/engine-init.json`
   - `.flowpilot/ledger/feature_history.ndjson`
   - `.flowpilot/catalog/features.ndjson`
   - `.claude/skills/*/SKILL.md`
   - `.agents/skills/*/SKILL.md`
10. Click `Refresh Project` and confirm `Last Init` updates with trigger, status, timestamps, and step rows.
11. Negative global-tooling check:
    - remove or break `gitnexus`
    - return to `Settings -> Engine`
    - click `Refresh Tooling`
    - confirm the `gitnexus` row becomes `missing`
12. Negative project-capability check:
    - with `gitnexus` still missing, click `Initialize / Re-sync Project`
    - confirm init still returns success or partial
    - confirm project capability falls back instead of blocking the page
13. Auto-init check:
    - open `Settings -> Projects`
    - create a new project with a valid binding, or save updated bindings on an existing project
    - confirm the save succeeds
    - return to `Settings -> Engine`
    - select that project and binding
    - confirm `Last Init` reflects the background bind-trigger run when it succeeds
14. FlowPilot-as-project check:
    - bind the FlowPilot repo itself as a project
    - open `Settings -> Engine`
    - confirm project status and init work the same way as for any other bound repo
15. Supabase schema-init check:
    - open `Settings -> Supabase`
    - provide a valid Management API token
    - click `Apply Repo Migrations`
    - confirm the action reports applied/skipped versions and the remote project contains the expected schema after completion
