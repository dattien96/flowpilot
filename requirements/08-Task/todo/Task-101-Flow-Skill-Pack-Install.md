# Task-101: Flow Skill Pack Install

## Metadata

- Document ID: `Task-101`
- Title: `Flow Skill Pack Install`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [CP-34: Init Tool](../../07-Coding-Plan/done/CP-34-Init-tool.md), [Task-096: Commit-History Ledger](./Task-096-Commit-History-Ledger.md), [CP-32: UnitTest Rule](../../07-Coding-Plan/done/CP-32-UnitTest-Rule.md)
- Replaces: `None`
- Tags: `skillpack, skills, git-commit-format, install, local-runner`

## AI Quick View

### Summary

- Ship a bundled, provider-agnostic flow skill pack and auto-install it into each bound project's `.claude/.codex/.gemini` dirs.
- Includes `git-commit-format` (the contract Task-096's parser reads), plus oracle-rule, audit-logging, phase-doc, context-discipline.
- Soft enforcement layer; the Flow Gate (Task-099/100) is the hard backstop.

### Current Ask

- Implement `internal/skillpack/install.go` + embedded `flow-pack/` per CP-35 §4.6 (setup-page UI stays in CP-34).

### Key Decisions

- `T-1` Reuse existing skills (`git-commit`, `audit-logging`, `phase-document-authoring`); add `oracle-rule`, `context-discipline`.
- `T-2` `git-commit-format` enforces `[Type]: <id> <desc>` (id ∈ `Task-`/`BUG-`/`CP-`) — the ledger contract.

### Constraints

- Runner-side install logic only; the desktop setup page is CP-34. Re-sync on bind; version-stamp.

### Open Questions

- None blocking.

### Source Refs

- `CP-35 §4.6` (P-6); `SD-17 D-7`, §6.4; `SS-14 AC-12`.

## 1. Goal

Every bound project has the flow-aware skills installed for each configured provider, so the AI follows the flow (and writes parseable commits) by default.

## 2. Parent Links

- coding plan: `CP-35` P-6
- tech design: `SD-17` `D-7`, §6.4
- system spec: `SS-14` AC-12
- specific upstream ids: `P-6`, `D-7`, `AC-12`

## 3. Trigger

Plane C depends on commit format; the gate works best when the AI cooperates. Both require shipping the skills into the bound project.

## 4. Exact Change

- `T-1` Embed `internal/skillpack/flow-pack/**` via `//go:embed` (5 skills, each with a `version:` header).
- `T-2` `install.go` — copy into `<target>/.claude/skills/flowpilot/`, `.codex/`, `.gemini/`; version-stamp; overwrite when newer.
- `T-3` author `git-commit-format` + `context-discipline`; reuse `oracle-rule` (Task-100 wording), `audit-logging`, `phase-doc`.
- `T-4` invoke install on bind; unit test the copy + version logic.

## 5. Touched Areas

- files: `apps/local-runner/internal/skillpack/*`, embedded `flow-pack/**`, bind path
- modules: `skillpack`
- routes: none
- tables: none

## 6. Acceptance Check

- CP-35 P-6 DoD: five skills present in each provider dir with version stamps after bind; re-bind re-syncs.
- `go test ./internal/skillpack/...` passes.

## 7. Out of Scope

- The desktop setup page (CP-34); tooling install/health (Task-102); doc normalization (CP-31).

## 8. Completion Notes

- result: planned
- follow-ups: CP-34 setup page surfaces install status
- upstream docs updated: none
