# Task-160: Self-Hosting Safety Via Worktree-Isolated Self-Targeting

## Metadata

- Document ID: `Task-160`
- Title: `Self-Hosting Safety Via Worktree-Isolated Self-Targeting`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-34: Init / Setup Tool — Desktop Engine Page & Project Auto-Init](../../07-Coding-Plan/done/CP-34-Init-tool.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [CP-15-01: Project Workspace Bindings](../../07-Coding-Plan/done/CP-15-01-Project-Workspace-Bindings.md), [CP-15-02: Project Launch And Workspace UX](../../07-Coding-Plan/done/CP-15-02-Project-Launch-And-Workspace-UX.md), [CP-15-04: Workflow Start And Runner Runtime](../../07-Coding-Plan/done/CP-15-04-Workflow-Start-And-Runner-Runtime.md), [SD-02: Architecture](../../06-System-Tech-Design/SD-02-Architecture.md), [Task-104: Runner Engine-Setup Endpoints](../done/Task-104-Runner-Engine-Setup-Endpoints.md), [Task-106: Bind-Time Auto-Init Orchestration](../done/Task-106-Bind-Time-Auto-Init-Orchestration.md)
- Replaces: `None`
- Tags: `self-hosting, dogfooding, worktree, workspace-binding, engine-init, isolation, guard, runbook`

## AI Quick View

### Summary

- **The ask is dogfooding: drive FlowPilot to change FlowPilot's own code.** The capability already exists — a run targets whatever `Cwd` the bound project resolves to ([claude_process.go:120](../../../apps/local-runner/internal/runner/claude_process.go), [interactive_service.go](../../../apps/local-runner/internal/runner/interactive_service.go)), and binding FlowPilot as its own target is **explicitly allowed** (`SS-14 AC-1`, `SD-17 D-2`, `CP-34` constraints). The missing piece is doing it **without the in-flight edits disrupting the very session making them.**
- **What is and is not safe when the target == the live checkout.** Go source edits do **not** change the running compiled engine — until a rebuild/restart. The real disruption surface is everything the live session **re-reads from the same checkout**: skills (`injectSkillContent`, per turn — [runner.go:903](../../../apps/local-runner/internal/runner/runner.go)), gate config (`.flowpilot/settings/gate-config.json`, per turn), the `.flowpilot/` ledger/catalog/state ([engine_setup.go:247](../../../apps/local-runner/internal/runner/engine_setup.go)), `FEATURE-KEYS.md` / `CA-*` notes, the post-commit hook ([changeledger/hook.go](../../../apps/local-runner/internal/changeledger/hook.go)), the shared git working tree (`captureGitHead`), and the admin-web Vite dev server (HMR). A supervisor restart ([scripts/supervisor.js](../../../scripts/supervisor.js) runs `go run`) would also recompile **half-finished** source.
- **The fix is workspace isolation via a git worktree, not new isolation machinery.** Add a worktree on a `task/`/`bugfix/` branch (`git worktree add`), bind **that path** as the FlowPilot project target. A linked worktree has its own working tree, its own `HEAD`, and its own repo-local `.flowpilot/` state ([engine_setup.go:247](../../../apps/local-runner/internal/runner/engine_setup.go) keys state to `workingDirectory`), so the running engine — built from the primary checkout — is untouched. Review, merge, then **deliberately** rebuild + restart the engine. This reuses the existing many-paths-per-project binding model (`CP-15`) and the harness's native worktree support; it adds **no** new runtime architecture.
- **One small code guard makes the safe path the obvious path.** Today nothing tells the user they have bound the live checkout. Add a **non-fatal warning** at engine init when the bound target's git top-level equals the top-level of the checkout the running engine was launched from — pointing them to a worktree. We **warn, never reject** (binding FlowPilot to itself stays allowed per `CP-34`).

### Current Ask

- Make FlowPilot safely self-hostable for code changes: (1) document the worktree self-hosting workflow as a followable runbook/skill, and (2) add a non-fatal engine-init guard that detects "you bound the live checkout this engine is running from" and steers the user to a worktree — without changing the binding model or rejecting the bind.

### Key Decisions

- `T-1` **Author a self-hosting runbook (the primary deliverable).** A followable doc/skill: create a worktree (`git worktree add ../flowpilot-dev -b task/<slug>`), bind that path as the FlowPilot project target, run edits/commits there, review + merge the branch, then **deliberately** rebuild + restart the engine. State explicitly what is safe (the running binary is immutable until restart) and what is not (live-read skills/gate/`.flowpilot`/working-tree, Vite HMR).
- `T-2` **Add a self-target guard at engine init (non-fatal warning).** In `runEngineInit` ([engine_setup.go:241](../../../apps/local-runner/internal/runner/engine_setup.go)), after resolving the working directory, append a `warnings[]` entry when `git -C <workingDirectory> rev-parse --show-toplevel` equals the top-level of the engine's own launch checkout. The warning names the risk and the remedy (`just self-worktree <branch>`). Init still proceeds — `CP-34` allows self-targeting.
- `T-3` **Give the guard a reliable self-anchor.** Capture the engine's own source-repo top-level once at `runner serve` startup: prefer `FLOWPILOT_SELF_REPO` injected by [supervisor.js](../../../scripts/supervisor.js); fall back to walking up from `os.Executable()` / `os.Getwd()` to the nearest git top-level. A worktree of the same repo has a **different** `--show-toplevel`, so it never trips the guard.
- `T-4` **Add a `just self-worktree <branch>` convenience recipe.** Creates the worktree on a new branch and prints the absolute path to bind, so the safe path is one command. Convenience only — the runbook works without it.
- `T-5` **Tests.** See §6.

### Constraints

- **Warn, never block.** Self-targeting FlowPilot is explicitly permitted (`CP-34` constraint, `SS-14 AC-1`). The guard is advisory and non-fatal — it must never fail or short-circuit bind/init (match the silent-swallow pattern in `ledger_live.go` / `gate_hook.go`).
- **Reuse the binding + run-resolution model as-is.** Do not modify `CP-15`'s many-paths-per-project model or the runner's path-resolution sequence (`CP-15-04`); the worktree is just another bound local path.
- **No engine hot-reload.** The Go runner is not hot-reloaded; rebuild + restart stays a deliberate, manual, post-merge step. This task does not add a watcher/auto-restart.
- **Per-project `.flowpilot/` isolation is the safety boundary.** State is keyed to `workingDirectory` ([engine_setup.go:247](../../../apps/local-runner/internal/runner/engine_setup.go)); the worktree's `.flowpilot/`, ledger, and post-commit sentinel are separate from the primary checkout's. Do not introduce shared/global engine state that would break this.
- **Out of band with the rejected isolation modes.** Pinned-binary/separate-clone and container/devcontainer isolation are explicitly **not** implemented here (see §7).

### Open Questions

- `Q-1` How to capture the engine's own source repo for `T-3` — env var from the supervisor, or `os.Executable()`/`Getwd()` walk-up? (Proposal: env-first with walk-up fallback, so it works whether launched via supervisor or bare `go run`.)
- `Q-2` Where does the guard surface — engine-init warning + engine status only, or also a per-turn flow-gate rule? (Proposal: init + status warning only; cheap, non-blocking, no per-turn cost. Promote to a gate later only if ignored.)
- `Q-3` A linked worktree shares `.git/common-dir` (hooks, config) with the primary checkout. Editing shared git infrastructure (e.g. `.git/hooks/`) could still affect the primary. (Proposal: note as a residual caveat in the runbook; the dominant risks — running binary, working tree, repo-local `.flowpilot/` — are covered by the worktree.)
- `Q-4` Should the desktop binding UI also show the warning inline (not just the runner status payload)? (Proposal: out of scope here; the warning rides the existing `EngineStatusResponse.warnings` so the UI can render it without new wiring — confirm before adding UI work.)

### Source Refs

- `SS-14 AC-1` (all context/history built against the **currently bound target**; FlowPilot-as-target allowed and isolated). `SD-17 D-2` (engine operates only on the bound target; per-project, local-first). `CP-34` constraint ("operate only on the bound target project's workspace dir; FlowPilot itself is allowed when it is intentionally the bound target"). `CP-15-04` (runner path-resolution: choose one usable bound local path).
- Code: `apps/local-runner/internal/runner/{engine_setup.go,runner.go,interactive_service.go,claude_process.go}`, `apps/local-runner/internal/changeledger/hook.go`, `apps/local-runner/internal/cli/` (serve startup), `scripts/supervisor.js`, `Justfile`.

## 1. Goal

Make it safe and obvious to use a running FlowPilot to edit FlowPilot's own code: the AI edits and commits inside an **isolated git worktree** bound as the project target, while the engine that drives the session runs from the primary checkout and is untouched until a deliberate rebuild + restart. Ship a followable runbook plus a non-fatal engine-init guard that detects "you bound the live checkout" and steers the user to a worktree — reusing the existing binding model and adding no new runtime isolation machinery.

## 2. Parent Links

- coding plan: `CP-34` (engine init / setup on a bound target — this task narrows its "FlowPilot itself is allowed when intentionally bound" constraint into a safe operational contract); reuses `CP-15-04` (runner path resolution) and `CP-15-01/02` (bindings model).
- tech design: `SD-17` `D-2` (per-project, bound-target-only engine); `SD-02` (3-tier: runner is a long-lived process decoupled from any single workspace).
- system spec: `SS-14` `AC-1` (context/history bound to the target; FlowPilot-as-target allowed and isolated).
- specific upstream ids: operationalizes the self-target allowance recorded in `CP-34` and `SS-14 AC-1`.

## 3. Trigger

The user wants to drive FlowPilot to change FlowPilot's own task/bug code. Binding FlowPilot to itself already works, but if the bound target is the **same checkout the engine was launched from**, the in-flight session re-reads assets that the AI is concurrently editing, and a rebuild/restart recompiles unfinished source:

1. **Live-read assets in the same checkout.** Skills are re-read per turn ([runner.go:903](../../../apps/local-runner/internal/runner/runner.go)); gate config is read per turn; the `.flowpilot/` ledger/catalog and `FEATURE-KEYS.md`/`CA-*` notes drive context — all inside the bound repo. Edits to them change the running session's behavior mid-task.
2. **Shared git working tree.** The AI's commits/branch switches mutate the same tree the engine reads `HEAD` and the ledger from; the post-commit hook fires on the AI's own commits.
3. **Rebuild/restart hazard.** [scripts/supervisor.js](../../../scripts/supervisor.js) launches the runner via `go run` and restarts on crash/port conflict — a restart would compile **half-finished** source and kill the engine doing the work.
4. **admin-web HMR.** Editing frontend source hot-reloads the UI the user is driving the run from.

A git worktree removes 1–3 (its own working tree, `HEAD`, and repo-local `.flowpilot/`) and lets the engine binary stay pinned to the primary checkout until a deliberate restart. Nothing tells the user this today — hence the runbook + guard.

## 4. Exact Change

- `T-1` **runbook/skill** — author a self-hosting runbook (e.g. `.agents/skills/self-hosting/SKILL.md` or a doc under `docs/`) covering: worktree creation on a `task/`/`bugfix/` branch, binding that path as the FlowPilot project target (via the existing project-binding UX, `CP-15-02`), running edits + commits there, the review → merge → **deliberate** rebuild+restart loop, and an explicit "safe vs not-safe" table (running binary immutable until restart; live-read skills/gate/`.flowpilot`/working-tree/Vite HMR are not).
- `T-2` **`runner` (engine_setup.go)** — in `runEngineInit` ([engine_setup.go:241](../../../apps/local-runner/internal/runner/engine_setup.go)), compute `boundTop = gitTopLevel(workingDirectory)`; if `boundTop` equals the engine's own launch top-level (`T-3`), append a `warnings[]` entry: *"This target is FlowPilot's own live checkout (the source this engine runs from). Edits here can disrupt the running session and a restart recompiles unfinished code. Use a worktree: `just self-worktree <branch>`, then bind that path."* Non-fatal; init proceeds unchanged. The warning already flows out via `EngineStatusResponse`.
- `T-3` **`cli` (serve startup) + `scripts/supervisor.js`** — resolve and cache the engine's own source-repo top-level at `runner serve` start: read `FLOWPILOT_SELF_REPO` if set (supervisor injects it), else walk up from `os.Executable()` then `os.Getwd()` to the nearest git top-level. Store it where `runEngineInit` can read it (runner field / package value). Worktrees of the same repo report a different `--show-toplevel`, so they never match.
- `T-4` **`Justfile`** — add a `self-worktree <branch>` recipe: `git worktree add ../flowpilot-<branch> -b <branch>` (idempotent if the branch/worktree exists) and echo the absolute path to bind.
- `T-5` **tests** — see §6.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/engine_setup.go` (guard in `runEngineInit`), `apps/local-runner/internal/cli/` (serve-startup self-repo resolution), `scripts/supervisor.js` (inject `FLOWPILOT_SELF_REPO`), `Justfile` (`self-worktree` recipe), new runbook (`.agents/skills/self-hosting/SKILL.md` or `docs/`).
- modules: `runner`, `cli`, `scripts`, docs/skills.
- routes: none new — the warning rides the existing `GET /client/projects/{projectId}/engine/status` and `POST /client/projects/{projectId}/engine/init` payloads (`Task-104`).
- tables: none (binding + per-project `.flowpilot/` state unchanged).

## 6. Acceptance Check

- **(T-2/T-3)** Binding the **live checkout** (the dir the running engine was launched from) returns a non-fatal warning in the engine init/status payload that names the risk and the worktree remedy; init still completes.
- **(T-2/T-3)** Binding a **git worktree of the same repo** returns **no** warning (different `--show-toplevel`), and binding an **unrelated repo** returns no warning.
- **(isolation)** A worktree target has its own `.flowpilot/` ledger/catalog and post-commit sentinel; an edit/commit in the worktree leaves the primary checkout's `.flowpilot/` and working tree untouched.
- **(T-1)** The runbook is followable end-to-end on this repo: create worktree → bind → edit via a run → commit → merge → rebuild+restart, with the safe/not-safe surface stated.
- **(T-4)** `just self-worktree <branch>` creates the worktree and prints a bindable absolute path; re-running is non-destructive.
- **(non-regression)** Binding/run resolution (`CP-15-04`) and existing engine-init behavior are unchanged for all non-self targets; `go test ./internal/{runner,cli}/...` passes.

## 7. Out of Scope

- **Pinned-binary / separate-clone mode and container/devcontainer isolation** — the alternative isolation strategies considered and not chosen; not implemented here.
- **Engine hot-reload / auto-restart-on-change** — rebuild + restart stays a deliberate manual step; no file watcher or supervisor auto-rebuild.
- **Automatic worktree lifecycle** beyond the `self-worktree` convenience recipe — no auto-merge-back, auto-prune, or auto-bind.
- **Rejecting or special-casing self-target binds** — the guard only warns; self-targeting remains allowed (`CP-34`, `SS-14 AC-1`).
- **Changes to the binding model or run path resolution** (`CP-15`) — reused as-is.
- **Desktop binding-UI rendering of the warning** (`Q-4`) — confirm before adding UI work; the runner-side warning is the deliverable here.

## 8. Completion Notes

- result: planned
- follow-ups: if users keep binding the live checkout despite the warning (`Q-2`), consider promoting the guard to an optional flow-gate rule; if worktree friction proves high, revisit the pinned-binary mode as a future hardening; address `Q-3` (shared `.git` infrastructure caveat) and `Q-4` (desktop UI surfacing) if they bite in practice.
- upstream docs updated: none yet — add a one-line operational note to `CP-34` (safe self-target contract) and, if the guard later becomes a formal gate rule, to `SD-20`; record in `SD-17 §`/`SS-14` only if the isolation contract changes (it does not — this operationalizes the existing allowance).
