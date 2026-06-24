# CA-123 — Platform-Scoped Skill Pack Install

## Scope

Extend the flow skill pack (Task-101 / CP-35 P-6) so installs are driven by a project's platform. Restructure the embedded pack into a mandatory `common/` group plus one folder per project type, and thread the selected platform from the desktop create/save flow and the Engine tab through the runner engine-init endpoints into `skillpack.Install/Status`.

Feature area: `context-regression-engine` (SD-17 skill pack slice, `SS-14 AC-12`). Implements Task-107.

## Completed

- **Skill pack restructure** (`apps/local-runner/internal/skillpack/flow-pack/`): moved the 5 existing skills (`git-commit-format`, `oracle-rule`, `audit-logging`, `phase-doc`, `context-discipline`) under `common/`. Added 12 per-type folders, each with one versioned placeholder `SKILL.md` (`<type>-conventions`, `version: 2`): android, ios, kmm, react-native, flutter, reactjs, vuejs, angularjs, golang, java, python, nodejs.
- **`install.go`**: added `platformGroups()` / `skillsForPlatform()` / `normalizePlatform()`; `Install`, `Status`, and `SkillNames` now take a `platform` arg. `common` always installs; a recognised platform adds its same-named folder; `kmm` additionally pulls `android` + `ios`; `none`/empty/unknown installs common only. Skills still install flat at `<root>/<skill>/SKILL.md`.
- **`engine_setup.go`**: `engineSetupRequest` gained `Platform`; the status handler reads a `platform` query param; `runEngineInit` and `buildEngineStatusResponse` thread `platform` into `skillpack.Install/Status`.
- **Desktop (`projectEngine.ts`, `ProjectsSettings.tsx`, `EngineSettings.tsx`)**: optional `platform` added to `fetchProjectEngineStatus`/`initProjectEngine`/`autoInitProjectEngine`; project create/save and the Engine tab now pass the project platform. Platform dropdown replaced with a shared `PROJECT_PLATFORM_OPTIONS` list (`none` default + 12 types).
- **client-core**: `ProjectPlatform` union replaced (`none` + 12 types); repository platform fallback `"multi"` → `"none"`.

## Verification

- `go build ./...` — success.
- `go test ./internal/skillpack/...` — all pass, including new platform-mapping, kmm-composite, `SkillNames`, and `Status` tests.
- Desktop `tsc --noEmit` — clean. Phase1 `tsc -p tsconfig.phase1-tests.json` — clean. Phase1 `supabaseAdminRepository` node tests — 5/5 pass.
- 14 pre-existing runner test failures (codex CLI absent, Windows home-path expectations, provider skill-merge fixtures) reproduce identically on a clean `main` worktree; none reference `skillpack`.

## Residual Notes

- Per-type skill content is placeholder only (`<type>-conventions` stubs) — intended for later authoring by the user.
- admin-web keeps its own local platform union and zod enum (`android|ios|web|multi`); not touched.
- Existing DB rows with legacy platform values (`web`/`multi`/`nextjs`/`react`) are not migrated; they degrade gracefully to common-only install and show no matching dropdown option.
- `PackVersion` unchanged (`2`); placeholders declare `version: 2` so status stays current with no forced re-sync.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-107
entries:
  - symbol: apps/local-runner/internal/skillpack/install.go
    layer: domain
    change: modified
    class: interface
  - symbol: apps/local-runner/internal/skillpack/flow-pack
    layer: domain
    change: modified
    class: structural
  - symbol: apps/local-runner/internal/runner/engine_setup.go
    layer: domain
    change: modified
    class: interface
  - symbol: apps/desktop-flowpilot/src/components/settings/projectEngine.ts
    layer: ui
    change: modified
    class: interface
  - symbol: apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx
    layer: ui
    change: modified
    class: behavioral
  - symbol: apps/desktop-flowpilot/src/components/settings/EngineSettings.tsx
    layer: ui
    change: modified
    class: behavioral
  - symbol: packages/flowpilot-client-core/src/domain/adminModels.ts
    layer: domain
    change: modified
    class: interface
# --->8---
