# Task-112: Requirements Scaffold On Bind

## Metadata

- Document ID: `Task-112`
- Title: `Requirements Scaffold On Bind`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: none
- Related Documents: [Task-101: Flow Skill Pack Install](./Task-101-Flow-Skill-Pack-Install.md), [Task-106: Bind-Time Auto-Init Orchestration](./Task-106-Bind-Time-Auto-Init-Orchestration.md)
- Replaces: none
- Tags: `context-regression-engine, reqscaffold, bind, requirements, format-reference`

## AI Quick View

### Summary

- When binding a project that has no `requirements/` folder (or is missing 05–09 subfolders), the engine init now auto-creates them and copies embedded FORMAT-REFERENCE files into each one.
- The FORMAT-REFERENCE files are embedded in the runner binary via `//go:embed` (same pattern as the skill pack) — they are not shipped as loose files alongside the binary.
- Existing FORMAT-REFERENCE files in a target project are **never overwritten** — a project may customise them freely.
- The scaffold runs as a new `req_scaffold` step inside `runEngineInit`, reported in the `EngineInitState.Steps` list and factored into the success/partial summary.

### Current Ask

- Create `internal/reqscaffold/` package, embed the five FORMAT-REFERENCE files, hook the scaffold into bind-time engine init, and verify with unit tests.

### Key Decisions

- `T-1` Embed FORMAT-REFERENCE files via `//go:embed scaffold-pack` so no loose files are needed outside the binary.
- `T-2` Never overwrite an existing FORMAT-REFERENCE file — install-once semantics (unlike the skill pack's version-upgrade model).
- `T-3` Also create `todo/`, `done/`, `inprogress/` child dirs where the phase workflow uses them (CP 07, Task 08, BugFix 09).
- `T-4` `IsScaffolded(repoDir)` returns true only when all five subfolders exist — used as a cheap guard.
- `T-5` `Scaffold()` is idempotent: calling it on an already-scaffolded project produces zero errors and zero new file writes (only directory creation via `os.MkdirAll` which is a no-op on existing dirs).

### Constraints

- Must not overwrite any file already present in the target project's `requirements/` tree.
- The scaffold must be non-fatal: failures are collected in `ScaffoldResult.Errors` and surfaced as the `req_scaffold` step outcome; they do not abort the rest of the init sequence.
- Do not copy 01–04 folders (they contain project vision/business docs, not phase-workflow format files).

### Open Questions

- Q-1: Should `IsScaffolded` be used in `shouldSkipBindInit` to skip only the scaffold step (not the whole init) on re-bind? Deferred — the scaffold is idempotent and cheap.

### Source Refs

- `CP-35` P-6 pattern (skill pack embed); `SD-17 §5.1`; `SS-14 AC-12` (bind-time init).
- Code: `internal/reqscaffold/scaffold.go`, `internal/runner/engine_setup.go` (`runEngineInit`).

## 1. Goal

When an engineer binds a fresh project in the Engineering tab, the bound project automatically receives the `requirements/05-09` folder structure with FORMAT-REFERENCE files. This means the AI can immediately author phase-compliant documents (SS, SD, CP, Task, BugFix) without the human needing to copy files manually from the FlowPilot repo.

## 2. Parent Links

- coding plan: `CP-35`
- tech design: `SD-17 §5.1`
- system spec: `SS-14 AC-12`, `AC-13`
- specific upstream ids: `CP-35 P-6` (embed pattern), `Task-101` (skillpack precedent), `Task-106` (bind orchestration hook point)

## 3. Trigger

When working on bound projects (such as the gate-sandbox used for E2E tests), the AI was unable to author BugFix/Task/SS/SD documents because the FORMAT-REFERENCE files were not present in the target project — only in the FlowPilot repo itself. The user had to manually copy them, which is error-prone and incompatible with fresh repos.

## 4. Exact Change

- `T-1` Create `apps/local-runner/internal/reqscaffold/scaffold-pack/` with five subdirs, each containing the relevant FORMAT-REFERENCE-*.md file (copied from `requirements/0X-*/` at time of task).
- `T-2` Create `apps/local-runner/internal/reqscaffold/scaffold.go` with `//go:embed scaffold-pack`, `Scaffold(targetRepoDir)`, `IsScaffolded(targetRepoDir)`, and `ScaffoldResult` types.
- `T-3` Add the `req_scaffold` step to `runEngineInit` in `engine_setup.go` immediately after `tooling_check`.
- `T-4` Update `summarizeEngineInitStatus` signature to include `scaffoldErr`.
- `T-5` Add five unit tests in `scaffold_test.go` covering: folder creation, file copy, skip-existing, `IsScaffolded`, and idempotency.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/reqscaffold/scaffold.go` (new)
  - `apps/local-runner/internal/reqscaffold/scaffold_test.go` (new)
  - `apps/local-runner/internal/reqscaffold/scaffold-pack/**` (5 embedded files, new)
  - `apps/local-runner/internal/runner/engine_setup.go`
- modules:
  - `reqscaffold` (new)
  - `runner` (engine_setup.go updated)
- routes: none
- tables: none

## 6. Acceptance Check

- [x] `go test ./internal/reqscaffold/...` passes (5 tests).
- [x] `go build ./...` succeeds.
- [x] Bind to a fresh temp dir → `requirements/05-System-Specs/FORMAT-REFERENCE-SS.md` and four sibling files exist.
- [x] Bind a second time → no errors, no file overwrites.
- [x] Pre-create `requirements/08-Task/FORMAT-REFERENCE-TASK.md` with custom content → second bind does not overwrite it.
- [x] `EngineInitState.Steps` includes `req_scaffold` with `outcome: "ok"`.
- [x] `EngineStatusResponse.LastInit.Status` is `"success"` when all steps pass.

## 7. Out of Scope

- Folders 01–04 (vision, business docs) — not part of the phase-workflow FORMAT-REFERENCE set.
- Updating FORMAT-REFERENCE files when the runner ships a newer version (install-once; version upgrades deferred).
- Desktop UI surface for scaffold status (follow-up; currently visible only in `EngineInitState.Steps`).

## 8. Completion Notes

- result: implemented and all tests pass. The `req_scaffold` step appears in `runEngineInit` immediately after `tooling_check`.
- follow-ups: Q-1 (targeted skip gate for scaffold-only re-bind); optional: version stamp on FORMAT-REFERENCE to enable future upgrades without full overwrite.
- upstream docs updated: CP-35 referenced in task header; no semantic change to SD-17 or SS-14 required.
