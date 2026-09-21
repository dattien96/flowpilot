# CA-892 — CP-68 Skill-Anchored Scaffold & AI-Guided Init (Task-383..386)

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: feature
summary: Implement the single-command /init AI Scaffold flow — recipe discovery + integrity gate, runner scaffold dispatcher, compiler verification self-healing loop, and TUI/Desktop auto-run surfaces with graceful ignore
# --->8---

## Why

CP-34's `/init` only copied static skill files; a platform with a verified scaffold recipe still required the user to hand-prompt the AI to build Step 0. CP-68 makes `/init` single-command: after the existing engine init, the runner evaluates the platform's `scaffold.yaml` capability and, when verified, auto-runs one skill-anchored AI Scaffold Turn closed by a fail-closed Compiler Verification Gate. Platforms without a verified recipe degrade to exactly today's behavior (graceful ignore, zero AI calls).

## Change

1. **Recipe Discovery & Skill Integrity (Task-383, `internal/skillpack/scaffold_recipe.go`)**:
   - `ScaffoldRecipe`/`VerificationGateConfig` (yaml+json) parsed from the embedded `flow-pack/<platform>/scaffold.yaml`.
   - `LoadScaffoldRecipe` (fail-safe `(nil, false, nil)` on missing/malformed manifest), `VerifyRecipeSkills` (platform-scoped `SKILL.md` integrity), `HasScaffoldCapability`.
   - `skillsForPlatform` untouched; `scaffold.yaml` never leaks into the installable skill set (react-native stays 23 skills).
   - `gopkg.in/yaml.v3 v3.0.1` promoted from `go.sum` to a direct `go.mod` require (module cache only).

2. **Scaffold Dispatcher + Prompt (Task-384, `internal/runner/scaffold_dispatcher.go`, `internal/agentpack/flow-pack/prompts/scaffold-step0-bootstrap.md`)**:
   - `ScaffoldDispatcher.Dispatch`: recipe gate → Zero-Prompt Injection (template rendered with platform/workspace/skills/gate command) → one AI turn via the injectable `ScaffoldPromptExecutor` seam (production: `Runner.ExecutePrompt`, provider-agnostic claude/codex/gemini/grok/opencode) with `SkillIds = recipe.ScaffoldSkills` (Context Profile resolves to the skills init installed in `<workspace>/.agents/skills`), `AllowWrite + YoloMode`, 30-minute turn budget.
   - Skips (zero AI calls): no recipe / disabled / missing declared skills / already scaffolded (`.flowpilot/scaffold-status.json` replay guard).
   - Prompt registered in `flow-pack/manifest.yaml`.

3. **Compiler Verification Gate & Self-Healing (Task-386, `internal/runner/compiler_gate.go`)**:
   - Recipe command run via `sh -c` (`cmd /c` on Windows) in the workspace, env inherited, per-attempt timeout (default 300s), `WaitDelay` anti-hang guard.
   - Fail-closed: `Passed` only on exit 0; timeout/"cannot start" are non-healable (a stalled install never burns AI turns).
   - Heal loop: parsed `file:line`/`error TSxxxx` diagnostics (bounded, de-duplicated) fed back as a `[Compiler Gate Thất Bại]` turn with the same attached skills, at most `cap` gate attempts; over cap ⇒ error result with the full log. Pass ⇒ write `.flowpilot/scaffold-status.json`.

4. **Trigger Surfaces (Task-385)**:
   - Runner: `GET /client/projects/{projectId}/scaffold/status` (`capable` + recipe + replay status — Desktop hides the option entirely when false) and `POST /client/projects/{projectId}/scaffold` (200 for done/skipped/error; 4xx/5xx only for malformed body / unavailable runner); provider defaults from the registry, platform defaults from the catalog.
   - TUI: after a successful `/init` with kind `all` (bare `/init` normalizes to `all`), the scaffold auto-runs when capable and logs exactly `scaffold: skipped (no verified recipe)` otherwise; `/init skill` is 100% CP-34 and never calls AI; `init_suggestions.go` untouched (picker stays `skill` | `all`).
   - Desktop: `handleCreateProject` → `autoTriggerScaffold` (async, non-blocking) layered on the CA-890 `runEngineInit` hook.
   - TUI client: `DispatchScaffold` (60-minute budget) + `ScaffoldStatus`.

## Verification

- `go build ./...` PASS; `go vet ./internal/runner/ ./internal/skillpack/` clean.
- `go test ./internal/skillpack/...` PASS (21 tests, incl. 8 new); 35 runner tests covering the CP-68 surface (`TestCompilerGate*`, `TestParseCompilerErrors*`, `TestScaffoldDispatcher*`, `TestScaffoldStatus*`, `TestScaffoldDispatch*`, `TestCreateProject_AutoTriggersScaffold/SkipsScaffold/AutoTriggerRejectsEscapingDirectory`, `TestNewScaffoldDispatcher*`) PASS; `go test ./internal/tui/app/ -run 'TestInitSuggestions_NoScaffoldRow|TestInitEngine_'` PASS (6 new tests); `go test ./internal/agentpack/... ./internal/tui/client/` PASS (manifest still validates with the new prompt entry).
- Old tests untouched (additive only): `engine_setup_test.go`, `project_auto_init_test.go`, `skillpack_test.go`, `init_suggestions_test.go` all still green.
- Known pre-existing failures (NOT introduced here): `internal/tui/app` `TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle` and `TestApprovalBarAndStopAreClickable` reproduce identically on the clean base commit 0555a8d5 (verified via a throwaway worktree); left untouched per the additive-tests-only contract.

## Scope Guard

- No Supabase schema changes, no cloud changes, no change to `/init skill` behavior, no new subcommand in the `/init` picker.
- Platform-name spoofing ("react-native" in a request) cannot authorize anything by itself: it only selects a recipe from the runner's own embedded pack; `HasScaffoldCapability` then requires the bundled `scaffold.yaml` + all declared `SKILL.md` files to exist, and the dispatcher rejects out-of-boundary workspaces before any prompt is built.
- Relative-path workspaces are rejected when they resolve outside the runner's process tree (`resolveEngineWorkingDirectory` / dispatcher guard, error `working_directory_outside_boundary`); `handleCreateProject` now returns that 400 instead of silently skipping engine init, so no AI turn is ever armed on an escaped path. Absolute-path policies stay with the operator/host (unchanged scope).
- Prompt-injection hardening is a deliberate line, not a boundary: the scaffold template labels pre-existing workspace content as data (not instructions) and requires contradictions to be reported, not obeyed. Full content-trust for generated code remains the compiler gate's job (exit 0), which cannot be talked away by workspace text.
- Manual TUI flow (empty dir + react-native project → `/init`) remains the operator's live check for real `pnpm` output; unit tests cover the orchestration with fake executors/recipes so no LLM or network is required.