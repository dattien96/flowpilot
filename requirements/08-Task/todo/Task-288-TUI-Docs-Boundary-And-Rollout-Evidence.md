# Task-288: TUI Docs, Boundary, And Rollout Evidence (CP-56 P-9)

## Metadata

- Document ID: `Task-288`
- Title: `TUI Docs Boundary And Rollout Evidence`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-9), [CP-56-Test-Steps](../../07-Coding-Plan/inprogress/CP-56-Test-Steps.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-287](./Task-287-TUI-Resume-Headless-And-Session-Reset.md)
- Child Documents: `none`
- Related Documents: Root README; FEATURE-KEYS `cli-tui`
- Replaces: `None`
- Tags: `cli-tui, docs, dod`

## AI Quick View

### Summary

- Operator README for `apps/local-runner/internal/tui`: modes, Settings boundary, ensure-runner, slash cheatsheet.
- Boundary test: TUI packages must not import runner internals; default self-autostart documented.
- Close CP-56-Test-Steps evidence; root README one-paragraph client blurb; CA note when implementing.

### Current Ask

Close P-9 documentation + verification only after 278–287 functionally done (or run docs incrementally last).

### Key Decisions

- `T-1` Settings boundary must be explicit in README (no authoring in CLI).
- `T-2` Package boundary enforced by test (`TestPackageBoundary_TuiDoesNotImportRunnerInternals`).
- `T-3` Verify every landed implementation Task has a change-audit entry carrying `feature_key: cli-tui`; P-9 adds its own documentation audit entry rather than deferring all audit work to the end.
- `T-4` Add `.github/workflows/cli-tui.yml`: matrix `windows-latest` + `ubuntu-latest`, Go version from `apps/local-runner/go.mod`, then `go vet ./internal/tui/... ./internal/cli/...` and `go test ./internal/tui/... ./internal/cli/... -count=1`. Existing `local-runner-race.yml` is runner-only and does not cover these packages.
- `T-5` Provide an ASCII status/agent fallback (`*`, `-`, plain separators) for legacy conhost/non-UTF terminals while retaining Unicode styling on modern terminals.

### Constraints

- No feature creep. No runner business logic edits.

### Open Questions

- None.

### Source Refs

- CP-56 §10–§11, P-9; A9.1–A9.5; M11–M17.

---

## 1. Goal

Make the TUI operable by humans and prove the thin-client boundary for rollout.

## 2. Parent Links

- coding plan: CP-56 P-9, D-7
- tech design: thin-client boundary from 03/04-02
- system spec: n/a; documentation and evidence closure
- specific upstream ids: CP-56 A9.1–A9.5, M11–M17

## 3. Trigger

Tasks 278–287 are complete and need a final operability, architecture-boundary, and evidence gate.

## 4. Exact Change

### 4.1 Docs

```text
apps/local-runner/internal/tui/README.md
```

Sections:

1. Install / run (`go run ./cmd/flowpilot chat`, ensure-runner/self-spawn behavior)
2. Modes: chat / flow / step
3. Settings boundary (Desktop only)
4. Slash cheatsheet: provider, model, reasoning, yolo, skill, image, flow, step, agent, stop, new
5. Flags: chat-local flags plus inherited root `--workspace`, `--host`, `--port`
6. Troubleshooting: unresolved/mismatched runner workspace, port in use, empty flows
7. Terminal compatibility: Unicode mode and ASCII fallback

Root `README.md`: add under architecture/clients — Terminal CLI (`apps/local-runner/internal/tui`) thin Bubble Tea client.

### 4.2 Boundary test

```go
// internal/tui/app/boundary_test.go
func TestPackageBoundary_TuiDoesNotImportRunnerInternals(t *testing.T)
// inspect TUI package imports/dependencies; fail if they contain flowpilot-runner/internal/runner
func TestEnsureRunner_DefaultAutostartEnabled(t *testing.T)
// config default NoStart==false
```

### 4.3 Evidence

- Fill CP-56-Test-Steps §3–§4 checkboxes / evidence log
- `git diff` allowlist check (M12)
- Verify Task-278–287 audit entries exist and carry `feature_key: cli-tui`
- Write/update the P-9 documentation audit entry

### 4.4 Optional justfile

```just
chat *args:
  cd apps/local-runner && go run ./cmd/flowpilot chat {{args}}
```

Only if repo already uses just — do not invent heavy tooling.

## 5. Touched Areas

- files: TUI README, root README, CP-56/Test-Steps evidence, change-audit entries, additive boundary test
- CI: new `.github/workflows/cli-tui.yml`
- modules: existing `flowpilot-runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] A9.1–A9.5 green
- [ ] Manual M11–M17, including Windows/macOS self-spawn, legacy-console rendering, flow-gate, project-picker, headless-gate, and forbidden-diff evidence
- [ ] CP-56 §10 DoD checklist complete
- [ ] CP status can move to `done` after operator accept

## 7. Out of Scope

- New product features; Ink migration; desktop changes

## 8. Completion Notes

- result: pending
- follow-ups: none (CP-56 complete when this closes)
- upstream docs updated: CP-56 → done when accepted
