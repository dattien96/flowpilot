# BUG-423: `/standardize` ignores the runner workspace and operates on the process cwd

## Metadata

- Document ID: `BUG-423`
- Title: `standardizeWorkspaceRoot falls back to os.Getwd(); SetStandardizeWorkspaceRoot only called in tests — drafts land in the wrong requirements/ tree`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp49/RESULT.md` (BUG-LIVE-5)
- Feature Keys: `standardize`, `docscan`, `workspace-root`

## AI Quick View

### Summary

- `POST /client/standardize` has no workspace field; `standardizeWorkspaceRoot()` returns the pinned root or `os.Getwd()` — and the only caller of `SetStandardizeWorkspaceRoot` is test code (`task333_standardize_test.go`); the `serve` path never pins it.
- Running `runner serve --workspace /repo/A` from a different cwd scans and writes drafts into the **cwd's** `requirements/` tree, not the workspace's.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

`/client/standardize` scans/fixes docs under the runner process's current working directory, not the `--workspace` directory the runner was launched with.

### Expected

`/standardize` resolves its doc root from the runner's configured workspace (or a request field), consistent with every other workspace-scoped endpoint.

### Actual

Doc root = pinned root (never set in production) → `os.Getwd()`. A runner started as `runner serve --workspace /repo/A` from cwd `/repo/B` writes SS/SD drafts into `/repo/B/requirements/`.

### Impact

Drafts land in the wrong project's `requirements/` tree (or nowhere meaningful) whenever the runner's cwd differs from the workspace — a common shape (runner started from a bin dir, service manager cwd, etc.). Silent mislocation; no error surfaced.

## Reproduction

1. `cd /tmp && flowpilot runner serve --workspace /path/to/project --port N`.
2. `POST /client/standardize` → observe scan/write activity under `/tmp/requirements/` instead of `/path/to/project/requirements/`.

## Root cause

- `apps/local-runner/internal/runner/standardize_cmd.go:95-105` — `standardizeWorkspaceRoot()` returns `standardizeRootBySvc[s]` or `os.Getwd()`.
- `apps/local-runner/internal/runner/standardize_cmd.go:89` — `SetStandardizeWorkspaceRoot` exists but is only invoked by tests (`task333_standardize_test.go`); the `serve` wiring (`internal/cli/root.go`) never pins it, and the request body carries no workspace field (`standardize_cmd.go` handler ~:77-102 region per evidence; verified `os.Getwd` fallback at :101).

## Evidence

- `~/fp-beds/lt-evidence/cp49/RESULT.md` — BUG-LIVE-5: `standardize_cmd.go` L77-102 + L483-502, `internal/cli/root.go` serve path (no pin call).
- Verified on main worktree HEAD `435e336b`: `standardize_cmd.go:95-105` (`os.Getwd()` at :101); grep shows `SetStandardizeWorkspaceRoot` referenced only in test files.

## Severity

- `low` — wrong-root writes in non-default launch shapes; no corruption (docs land under a requirements/ tree of the cwd), but the operation targets the wrong workspace silently.

## Completion Notes (implemented 2026-09-23, CA-925b)

- Root cause confirmed: `standardizeWorkspaceRoot()` fell back to
  `os.Getwd()` and only tests ever called `SetStandardizeWorkspaceRoot` —
  the serve path never pinned, so `/standardize` operated on the process
  cwd.
- Fix: `InteractiveService.AttachRunner` now pins
  `standardizeRootBySvc[s] = r.workspace` when no explicit pin exists —
  the single seam every real serve path flows through (`cli/root.go`).
  Explicit pins still win, so test sandboxes are unchanged.
- Tests: `internal/runner/bug423_standardize_root_test.go` — root
  resolution from attached runner, pin precedence, and an
  `ExecuteStandardize` e2e under a foreign cwd.
- Live: server started with `--workspace /tmp/fp-live-j/ws` from cwd
  `/tmp/fp-live-j/cwd`; `POST /client/standardize {"path":"features/auth"}`
  returned `mode=conformance` scanning `ws/requirements` (pre-fix this
  would have hit `scope path not found` or polluted cwd); cwd stayed
  empty.
