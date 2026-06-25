# Task-107: Platform-Scoped Skill Pack Install

## Metadata

- Document ID: `Task-107`
- Title: `Platform-Scoped Skill Pack Install`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-101: Flow Skill Pack Install](./Task-101-Flow-Skill-Pack-Install.md), [Task-104: Runner Engine Setup Endpoints](./Task-104-Runner-Engine-Setup-Endpoints.md), [Task-105: Engine Setup UI Tab](./Task-105-Engine-Setup-UI-Tab.md), [CP-34: Init Tool](../../07-Coding-Plan/inprogress/CP-34-Init-tool.md), [CA-123: Platform-Scoped Skill Pack Install](../../change-audit/CA-123-platform-scoped-skill-pack-install.md)
- Replaces: `None`
- Tags: `skillpack, skills, project-platform, install, local-runner, desktop`

## AI Quick View

### Summary

- Restructures the embedded flow skill pack into a mandatory `common/` group plus one folder per project type, and makes install platform-aware.
- A project's platform now drives which skills install: `common` is always installed; a recognised type adds its same-named folder; `kmm` additionally pulls `android` + `ios`; `none`/unknown installs common only.
- Replaces the hardcoded `android|ios|web|multi` dropdown with the full type list (`none` default + 12 types) and threads `platform` from the desktop create/save flow through the runner engine-init endpoints into `skillpack.Install/Status`.

### Current Ask

- Implement platform-scoped skill install so picking a project type and pressing install lays down `common` + that type's skills.

### Key Decisions

- `T-1` `flow-pack/` is now `common/<skill>` + `<type>/<skill>`; the 5 existing skills moved under `common/`.
- `T-2` Platform→group map is by-name, except `kmm` → `common + kmm + android + ios`; `none`/unknown → common only.
- `T-3` Each new type folder ships one versioned placeholder `SKILL.md` (`<type>-conventions`) for the user to expand later.
- `T-4` `ProjectPlatform` union replaced with `none` (default) + android, ios, kmm, react-native, flutter, reactjs, vuejs, angularjs, golang, java, python, nodejs.

### Constraints

- Skills still install **flat** at `<root>/<skill>/SKILL.md` (no nested per-group dir) to preserve the existing layout and provider-merge logic.
- `PackVersion` stays `2`; all placeholder skills declare `version: 2` so status stays current.
- admin-web keeps its own local platform union and is out of scope.

### Open Questions

- None blocking. Per-type placeholder content is intentionally a stub for later authoring.

### Source Refs

- `CP-35 §4.6` (P-6); `SD-17 D-7`, §6.4; `SS-14 AC-12`.

## 1. Goal

When a user selects a project type and triggers engine init/install (on create, save, or the manual Engine tab), the bound project receives the mandatory common skill pack plus the skills specific to that type, in each provider directory.

## 2. Parent Links

- coding plan: `CP-35` P-6 (extends Task-101)
- tech design: `SD-17` `D-7`, §6.4
- system spec: `SS-14` AC-12
- specific upstream ids: `P-6`, `D-7`, `AC-12`

## 3. Trigger

Task-101 shipped a single, platform-agnostic skill pack. The product now needs per-language/per-platform skills installed only for the relevant project type, so the AI gets type-appropriate guidance without polluting every project.

## 4. Exact Change

- `T-1` Move `flow-pack/{audit-logging,context-discipline,git-commit-format,oracle-rule,phase-doc}` → `flow-pack/common/...`.
- `T-2` Add `flow-pack/<type>/<type>-conventions/SKILL.md` placeholders for: android, ios, kmm, react-native, flutter, reactjs, vuejs, angularjs, golang, java, python, nodejs (each `version: 2`).
- `T-3` `install.go`: add `platformGroups(platform)`, `skillsForPlatform(platform)`; change `Install(target, platform)`, `Status(target, platform)`, `SkillNames(platform)`; common always + by-name map + `kmm`→`+android +ios`; unknown/`none`→common only.
- `T-4` `engine_setup.go`: add `Platform` to `engineSetupRequest`; read `platform` query param on status; thread `platform` into `runEngineInit`/`buildEngineStatusResponse` and the `skillpack.Install/Status` calls.
- `T-5` `projectEngine.ts`: add optional `platform` to `fetchProjectEngineStatus`, `initProjectEngine`, `autoInitProjectEngine`.
- `T-6` `ProjectsSettings.tsx`: default platform `none`; shared `PROJECT_PLATFORM_OPTIONS` rendered in both selects; pass `platform` into both `autoInitProjectEngine` calls.
- `T-7` `EngineSettings.tsx`: pass `selectedEntry?.project.platform` into status/init calls.
- `T-8` `adminModels.ts`: replace `ProjectPlatform` union; `supabaseAdminRepository.ts`: platform fallback `"multi"`→`"none"`.
- `T-9` Update `skillpack_test.go` for the new structure/signatures + add platform-mapping, SkillNames, and Status tests.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/skillpack/install.go`, `skillpack_test.go`
  - `apps/local-runner/internal/skillpack/flow-pack/**` (restructured + 12 new placeholders)
  - `apps/local-runner/internal/runner/engine_setup.go`
  - `apps/desktop-flowpilot/src/components/settings/projectEngine.ts`, `ProjectsSettings.tsx`, `EngineSettings.tsx`
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`, `src/data/supabaseAdminRepository.ts`
- modules: `skillpack`, runner `engine_setup`, desktop settings, client-core admin models
- routes: `POST /client/projects/{projectId}/engine/init`, `GET /client/projects/{projectId}/engine/status` (now accept `platform`)
- tables: none

## 6. Acceptance Check

- `go build ./...` succeeds; `go test ./internal/skillpack/...` passes including platform-mapping, kmm composite, SkillNames, and Status tests.
- `Install(dir,"none")` → 5 common skills only; `Install(dir,"android")` → common + `android-conventions`; `Install(dir,"kmm")` → common + `kmm-conventions` + `android-conventions` + `ios-conventions`.
- Desktop `tsc --noEmit` clean; phase1 `tsc` clean; phase1 repository node tests pass.
- Desktop create/save and the Engine tab send the project platform to the runner.

## 7. Out of Scope

- Authoring real per-language skill content (placeholders only).
- admin-web project create flow and its local platform union/zod enum.
- Migrating existing DB rows that hold legacy platform values (`web`/`multi`/`nextjs`/`react`); they degrade gracefully to common-only.
- Any change to the Flow Gate, ledger, catalog, tooling, or Drive sync slices.

## 8. Completion Notes

- result: implemented and verified. `skillpack` package green (all tests incl. new platform cases); desktop `tsc --noEmit` clean; phase1 `tsc` clean; phase1 repo node tests 5/5. Pre-existing runner test failures (codex CLI absent, Windows home paths) reproduced identically on clean `main` and are unrelated.
- follow-ups: author real per-type skill content; consider surfacing the resolved skill set in the Engine setup UI; optional DB migration mapping legacy platform values.
- upstream docs updated: none required — change is a downstream execution delta extending CP-35 P-6 / Task-101; behaviour stays within `SS-14 AC-12` intent.
