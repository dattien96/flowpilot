# CP-34: Init / Setup Tool — Engine Install & Health UI

## Metadata

- Document ID: `CP-34`
- Title: `Init / Setup Tool — Engine Install & Health UI`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-104: Runner Engine-Setup Endpoints](../../08-Task/todo/Task-104-Runner-Engine-Setup-Endpoints.md) (P-1), [Task-105: Engine Setup UI Tab](../../08-Task/todo/Task-105-Engine-Setup-UI-Tab.md) (P-2), [Task-106: Bind-Time Auto-Init Orchestration](../../08-Task/todo/Task-106-Bind-Time-Auto-Init-Orchestration.md) (P-3)
- Related Documents: [CP-35: Context And Regression Engine Rollout](../inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-31: Auto-Document Process](../done/CP-31-Auto-Document-Process.md), [CP-10: Integrations, Memory & Context Intelligence](../inprogress/CP-10-Integrations-Hardening.md), [CP-33: Desktop Project Chat Drive Folder Selection](./CP-33-Desktop-Project-Chat-Drive-Folder-Selection.md)
- Replaces: `None`
- Tags: `init, setup, skill-pack, tooling, gitnexus, rtk, capability-profile, admin-web, local-runner`

## AI Quick View

### Summary

- The init/setup tool is the **install + health-check home** for the Context & Regression Engine (`SD-17 §7.3`): it installs the bundled flow skill pack into each bound project and verifies external tooling (GitNexus, RTK, node).
- The Go logic already exists and is unit-tested — `skillpack.Install()` (Task-101) and `tooling.CheckAll()` / `ComputeCapabilityProfile()` (Task-102). This CP **wires them to HTTP, a gateway, and a Settings tab**; it writes no new engine logic.
- Three slices: `P-1` runner endpoints (status + init/resync), `P-2` admin-web gateway + **Engine Setup tab** in the project area, `P-3` bind-time auto-init so a freshly bound project is engine-ready without a manual click.
- Everything is **per bound project**, operates only on the target repo's `<target>/.flowpilot/`, and is **non-fatal**: a missing tool degrades the capability tier, it never blocks project use.
- Implementation note: the runner does **not** own Supabase directory bindings, so the shipped HTTP contract uses `projectId` in the path plus an explicit `workingDirectory` from admin-web. Bind-time auto-init is therefore attached to the admin-web binding save flows, not inferred inside the runner.

### Current Ask

- Give the user a visible project **Engine** surface to (1) see tooling health (gitnexus/rtk/node/skill_pack), (2) see which flow skills are installed, and (3) run install/re-sync — plus do this automatically when bindings are saved.

### Key Decisions

- `K-1` New project tab **"Engine"** (`ProjectSectionNav`) rather than burying it inside the existing `/settings` page, because it owns its own status fetch + actions.
- `K-2` Two runner endpoints only: `GET …/engine/status` (read tooling.json + skill-pack install state) and `POST …/engine/init` (run `CheckAll` + `Install`, return fresh status). Idempotent; re-run = re-sync. Because directory bindings live in admin-web/Supabase, both endpoints take an explicit `workingDirectory`.
- `K-3` The init action shows **exactly what it runs** before running external installers (`SD-17 §9` tooling-install trust); v1 only *checks* tools, it does not auto-install gitnexus/rtk (it reports `missing` and links to install docs).
- `K-4` Bind-time auto-init is **best-effort and async** — project create/update/binding-save never fails because the engine init failed.

### Constraints

- Operate only on the bound target project's workspace dir; **never** touch FlowPilot's own repo (`SS-14 AC-1`, `SD-17 §9`).
- Reuse the runner's existing `http.ServeMux` route pattern (`internal/cli/root.go`, `interactive_handlers.go`) and the admin-web `HttpLocalRunnerGateway`.
- Machine-specific results (`tooling.json`) are local-only, never synced (`SD-17 §5.1`).
- `apps/admin-web` is a customized Next/Vite + TanStack Router app — match existing route/loader/gateway conventions; read `apps/admin-web/AGENTS.md` before touching it.

### Open Questions

- `Q-1` Should v1 attempt auto-install of gitnexus/rtk (npm `-g`) or only detect + link to docs? (Default: detect-only — `K-3`.)
- `Q-2` Auto-init on every bind vs. first-bind-only with a manual "re-sync" thereafter? (Shipped default: run from binding save flows when `.flowpilot/` or `tooling.json` is missing, or the skill-pack version is stale.)

### Source Refs

- `SD-17 §3.6` (tooling availability), `§6.4` (skill-pack manifest + `checkTool`), `§7.3` (bind/setup flow), `§9` (install trust).
- `SS-14 AC-12` (skills auto-installed), `AC-13` (tooling health + degrade).
- `CP-35 §4.6/§4.7` (the `skillpack` / `tooling` packages this consumes).
- Code: `internal/skillpack/install.go`, `internal/tooling/check.go`, `internal/cli/root.go`, `internal/runner/interactive_handlers.go`, `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`, `apps/admin-web/src/components/project/project-section-nav.tsx`.

## 1. Goal

A bound project becomes engine-ready with no hidden state: the user can see tooling health and skill-pack status in a dedicated **Engine** tab, run install/re-sync on demand, and have it happen automatically on bind — all scoped to the target repo, all non-fatal.

## 2. Input Documents

- `SD-17` §3.6, §6.4, §7.3, §9 (the contracts + bind flow this implements the UI/wiring for).
- `SS-14` AC-12, AC-13 (the acceptance criteria this satisfies on the product surface).
- `CP-35` (delivered the `skillpack` + `tooling` Go packages; this CP is their consumer/UI).
- `CP-31` (doc normalization — triggered as part of bind orchestration, not re-specified here).

## 3. Implementation Strategy

- **Prerequisite (met):** `skillpack` and `tooling` packages are implemented and green (Task-101, Task-102). This CP adds no engine logic — only transport + UI + an orchestration call.
- **Order:** `P-1` (endpoints) → `P-2` (gateway + UI tab) → `P-3` (bind auto-init). `P-1` and `P-2` deliver the manual, user-visible flow first; `P-3` makes it automatic.
- **Workspace resolution:** the runner cannot resolve Supabase directory bindings by `projectId` alone in this codebase. The shipped setup endpoints therefore take `projectId` in the path for project scoping and an explicit `workingDirectory` in the request; they refuse to run if that path points at the FlowPilot repo itself.
- **No new module:** all runner code lands in a thin handler in `internal/runner/` (or `internal/cli/root.go` route block) that calls the two existing packages.

## 4. Work Breakdown

### 4.1 `P-1` Runner engine-setup endpoints — `internal/runner/`

**Endpoints (registered on the existing `mux`):**

```
GET  /client/projects/{projectId}/engine/status?workingDirectory=<abs-path>
POST /client/projects/{projectId}/engine/init
```

**`GET …/engine/status`** → uses the explicit `workingDirectory`, then returns:
```go
type EngineStatusResponse struct {
    ProjectID    string                 `json:"projectId"`
    WorkingDirectory string             `json:"workingDirectory"`
    Tooling      []tooling.ToolStatus   `json:"tooling"`       // gitnexus, rtk, node, skill_pack
    Capability   tooling.CapabilityProfile `json:"capability"`
    SkillPack    skillpack.PackStatus   `json:"skillPack"`     // pack version + per-provider presence/current
    Initialized  bool                   `json:"initialized"`   // .flowpilot/ exists
    LastInit     *EngineInitState       `json:"lastInit"`      // last manual/auto init outcome
}
```
- Reads `tooling.json` via `tooling.LoadToolingStatus(dotFlowpilot)`; if absent, runs `tooling.CheckAll` once.
- `SkillPackState` from `skillpack.Status(workingDirectory)`; status includes each bundled skill across `.claude/.codex/.gemini`.

**`POST …/engine/init`** (idempotent install/re-sync) → on the explicit working directory:
1. `tooling.CheckAll(workspaceDir, dotFlowpilot)` → writes `tooling.json`.
2. `skillpack.Install(workspaceDir)` → copies the 5 skills into `.claude/.codex/.gemini`, version-stamped, skips same-version.
3. `changeledger.Build(...)` + `featurecatalog.Build(...)` → refresh the local git-derived engine artifacts.
4. Persist `.flowpilot/engine-init.json` and return a fresh `EngineStatusResponse`.
- **Guard:** if `workspaceDir` resolves to the FlowPilot repo root, return `400` with "engine cannot target FlowPilot's own repo" (`SS-14 AC-1`).
- **Non-fatal:** any tool check failing is reported as `missing`/`stale`, not a `500`; change-ledger/catalog failures are returned as a `partial` init result instead of aborting the whole request.

### 4.2 `P-2` Admin-web gateway + Engine Setup tab

**Gateway** (`http-local-runner-gateway.ts`, extend `LocalRunnerGateway`):
```ts
getEngineStatus(projectId: string): Promise<EngineStatus>          // GET …/engine/status
initEngine(projectId: string): Promise<EngineInitResult>          // POST …/engine/init
```
Use the existing `readJson` / `fetch(new URL(...))` patterns; map snake/camel to the domain model.

**UI** — new route `apps/admin-web/src/routes/_authenticated/projects/$projectId/engine.tsx`:
- Add `{ label: "Engine", suffix: "/engine" }` to `project-section-nav.tsx` `tabs`.
- Sections:
  - **Tooling health** — a row per tool (gitnexus, rtk, node, skill_pack) with a status `Badge` (`ok`=success, `missing`/`stale`=danger/neutral), version, and a "How to install" link for missing tools (`K-3`).
  - **Capability tier** — show `structure_tier` / `decision_tier` and detected `languages` from `CapabilityProfile`, with a one-line "what this means" note (e.g. "GitNexus missing → file-level structure").
  - **Skill pack** — list the 5 skills with installed/ version badges across `.claude/.codex/.gemini`.
  - **Actions** — `Initialize / Re-sync engine` button → `initEngine.mutate()`, then `router.invalidate()`; show installed/skipped counts, last-init steps, and any errors inline.
- Match the existing settings-page visual idiom (rounded cards, `PageFrame`, `ProjectSectionNav`).

### 4.3 `P-3` Bind-time auto-init orchestration

- On project create / project update / directory-binding save (the places where directory bindings are actually established in this codebase), call `engine/init` best-effort + async (`K-4`): check tooling, install skill pack, then build `changeledger` + `featurecatalog` from git.
- Skip silently if `workspaceDir` is FlowPilot's own repo. Run only when `.flowpilot/` is absent, `tooling.json` is absent, or the bundled skill-pack version is newer than the installed stamp (`Q-2` shipped default).
- Surface the outcome on the Engine tab (`.flowpilot/engine-init.json` → last-init timestamp + result); never fail the save flow on engine-init error.
- `CP-31` doc normalization remains a follow-up: `SD-17` references it, but there is no callable normalization entrypoint in the current repo to invoke from this task without inventing new workflow behavior.

## 5. Touched Areas

- **New:** `apps/admin-web/src/routes/_authenticated/projects/$projectId/engine.tsx`; a runner handler block for the two endpoints.
- **Extended:** `interactive_handlers.go` (route registration); `http-local-runner-gateway.ts` + the `LocalRunnerGateway` interface + domain model; `project-section-nav.tsx` (new tab); project create/update/directory-binding save flows for `P-3`.
- **Reused (no change):** `internal/skillpack/`, `internal/tooling/` (consumed as-is); `PageFrame`, `Badge`, `Button`, `useMutation` UI primitives.
- **Database:** none.

## 6. Data or Migration Steps

- **None.** All state is local under `<target>/.flowpilot/` (`tooling.json`, skill-pack files). No Supabase tables, no migration. `tooling.json` is machine-local and never synced (`SD-17 §5.1`).

## 7. Validation Plan

- **Unit (Go):** endpoint handlers resolve the bound dir; FlowPilot-repo guard returns 400; status reflects `tooling.json`; init is idempotent (second call → all skills `skipped`). (Underlying `Install`/`CheckAll` already covered by Task-101/102 tests.)
- **Integration:** save a project binding → Engine tab shows real tooling status; click Re-sync → skills appear in `.claude/.codex/.gemini` with version stamps; remove gitnexus → status shows `missing` and capability tier drops to file-level.
- **Manual / E2E:** maps to `CP-35 §12` E2E-1 (store created), E2E-2 (tooling health), E2E-5 (skill pack installed). Confirm the FlowPilot repo can never be targeted.
- **Observability:** init result (installed/skipped/errors + tooling status) logged via the runner's existing log path.

## 8. Rollout and Fallback

- `P-1`→`P-2` ship the manual flow; `P-3` enables auto-init behind the binding save flows. If `P-3` misbehaves, disable auto-init and the manual button still works. No data migration to reverse; `.flowpilot/` is disposable and rebuildable from git.

## 9. Risks

- `R-1` Auto-installing external tools runs third-party installers → v1 is **detect-only** (`K-3`); auto-install deferred to `Q-1`.
- `R-2` Wrong workspace resolution could touch the wrong repo → explicit FlowPilot-repo guard + always resolve from the project's bound path.
- `R-3` Bind-time init slows bind → async + best-effort (`K-4`).
- `R-4` Skill-pack version drift across providers → `Install` is version-stamped and re-syncs on bind (`SD-17 R-4`).

## 10. Definition of Done

### Per-slice

- [x] `P-1` `GET …/engine/status` returns tooling + capability + skill-pack state for a bound project; `POST …/engine/init` runs `CheckAll` + `Install` idempotently, refreshes ledger/catalog, and refuses the FlowPilot repo. — **Task-104**
- [x] `P-2` A project **Engine** tab shows tooling health, capability tier, skill-pack status, and a working Initialize/Re-sync action. — **Task-105**
- [x] `P-3` Saving project bindings auto-runs engine init (best-effort, async, skips self-repo); outcome is shown on the Engine tab. — **Task-106**
- [x] `go test ./internal/runner ./internal/skillpack ./internal/tooling` passes; admin-web targeted tests pass; offline `vite build` passes with a manually updated route tree.

### Shared

- [x] Engine setup operates only on the bound target repo; FlowPilot's own repo is never targeted (`SS-14 AC-1`).
- [x] All checks/installs are non-fatal; a missing tool degrades the capability tier and never blocks project use (`SS-14 AC-9`, `AC-13`).

### AC coverage

| AC | Covered by |
|----|-----------|
| `SS-14 AC-12` skills auto-installed | `P-2` (manual) + `P-3` (on binding save) |
| `SS-14 AC-13` tooling health + degrade | `P-1` status + `P-2` tier display |
| `SS-14 AC-1` target-only / no self-context | FlowPilot-repo guard in `P-1`/`P-3` |

### Out of scope (tracked elsewhere)

- Auto-installing gitnexus/rtk binaries (`Q-1`).
- `CP-31` doc normalization trigger wiring (no callable entrypoint exists in the current repo).
- Flow-gate runner hook + Drive sync wiring (owned by `CP-10` / `CP-35` remaining items).

## 11. Manual Test Guide

1. Start the app surfaces:
   - local runner
   - admin-web
2. Open a project with at least one valid local directory binding.
3. Open `Project → Engine`.
4. Verify:
   - the primary binding path is shown
   - tooling rows render for `gitnexus`, `rtk`, `node`, `skill_pack`
   - capability badges and detected languages render
   - all 5 bundled skills render with provider badges for `.claude`, `.codex`, `.gemini`
5. Click `Initialize / Re-sync engine`.
6. Verify on disk inside the bound repo:
   - `.flowpilot/tooling.json`
   - `.flowpilot/engine-init.json`
   - `.flowpilot/ledger/feature_history.ndjson`
   - `.flowpilot/catalog/features.ndjson`
   - `.claude/skills/flowpilot/*/SKILL.md`
   - `.codex/skills/flowpilot/*/SKILL.md`
   - `.gemini/skills/flowpilot/*/SKILL.md`
7. Refresh the Engine tab and confirm `Last init` shows:
   - trigger
   - status
   - installed/skipped/error counts
   - step rows for tooling / skillpack / ledger / catalog
8. Negative check:
   - remove or break `gitnexus`
   - click re-sync again
   - confirm Engine still succeeds or returns `partial`, the tool row becomes `missing`, and capability drops to fallback
9. Self-repo guard:
   - point the request at the FlowPilot repo
   - confirm the runner returns `self_repo_forbidden`
10. Auto-init check:
   - create or update a directory binding
   - confirm the save succeeds even if runner init fails
   - open Engine and confirm `Last init` updates when the background auto-init succeeds
