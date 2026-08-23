# CA-571: TUI /init skill and /init all — flow-pack per platform via engine/init

## What

Adds TUI slash command `/init` with Tab picker `skill | all` so the operator can init the target project from the chat TUI:

- `/init skill` — installs only the embedded flow-pack skills for the bound project's platform into `.claude/skills`, `.agents/skills`, `.grok/skills` (common + platform group; `kmm` pulls `kmm+android+ios`).
- `/init all` — full engine init, same as Desktop Engine Settings `Init` (POST `/engine/init` trigger `manual`): skill pack + tooling check + `requirements/` scaffold + change-ledger + feature catalog + gate config + post-commit hook + Drive restores/sync.

Bare `/init` or `/init ` Tab shows the two rows; unknown kind shows usage/error; no bound project shows "No project bound".

## Why

- TUI had no `/init`. Skill pack install was only reachable via Desktop `ProjectsSettings`/`EngineSettings` → `autoInitProjectEngine`/`initProjectEngine` or boot-time bind. 
- User wants TUI to respect the project `type/platform` chosen at creation and copy the corresponding `internal/skillpack/flow-pack/{common,<type>}` skills into each provider folder. Future init kinds will reuse `/init` as prefix, so Tab must show `skill` and `all` now.

## Fix

- `apps/local-runner/internal/runner/provider_event.go:353` — `Project.Platform` (omitempty).
- `apps/local-runner/internal/runner/supabase_catalog_store.go:82` — select `platform`, map to `Project.Platform`.
- `apps/local-runner/internal/runner/interactive_catalog.go:24` — fake projects carry `Platform` (reactjs, android).
- `apps/local-runner/internal/tui/client/client.go:34` — `Project.Platform`; new `EngineInitResult` + `InitEngine(projectID, cwd, platform, kind)` (POST `/engine/init` with `kind`).
- `apps/local-runner/internal/runner/engine_setup.go:26` — `engineSetupRequest.Kind`; `handleInitEngine` validates `kind` (`all` default, `skill`), threads to `runEngineInit(..., kind)`; `runEngineInit` gains `kind` param, short-circuits `shouldSkipBindInit` for skill-only, adds skill-only branch (install + tooling check only, two-step `EngineInitState`).
- `apps/local-runner/internal/tui/app/model.go:586` — `knownSlashCommands` entry `/init`.
- `apps/local-runner/internal/tui/app/init_suggestions.go` — new `filterInitSuggestions` (bare `/init` and `/init <prefix>` prefix match).
- `apps/local-runner/internal/tui/app/init_engine.go` — new `EngineInitMsg`, `cmdInitEngine(kind)` (uses `boundProjectPath`/`cfg.ProjectPath`, `project.Platform`), `handleEngineInitMsg` summary (installed/skipped/errors + warnings).
- `apps/local-runner/internal/tui/app/app.go:721` — `case EngineInitMsg`; `collectSuggestions` appends `filterInitSuggestions`; `applySuggestion`/`suggestionAcceptValue` handle `init`; `handleSlashCommand` `/init` dispatches to `cmdInitEngine`.

## Tests (additive)

- `internal/tui/app/init_suggestions_test.go` — `TestFilterInitSuggestions_BareAndWithArgs` (bare/space/prefix/non-init), `TestInit_TabCompletesAndEnterDispatches` (collectSuggestions init kind, unknown kind error, usage, no-project).
- `go vet ./internal/...` clean; `go test ./internal/tui/app -count=1` 13s pass; `go test ./internal/runner -run TestEngineInit -count=1` pass; `go test ./internal/skillpack -count=1` pass.

## Residual

- `GET /client/projects` now returns `platform` for TUI/Desktop; no migration needed (`platform` nullable, `none` falls back to common-only install).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: add TUI /init with skill (pack-only) and all (full engine) — Tab picker, per-platform flow-pack install via POST /engine/init kind
# --->8---
