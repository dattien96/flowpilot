# BUG-549 — Scaffold Progress Feed Not Scoped To Selected Binding

## Metadata

- Document ID: `BUG-549`
- Title: `Scaffold progress feed not scoped to selected binding`
- Phase: `bugfix`
- Status: `done`
- Owner: `devin`
- Reviewers: `tiendat`
- Created: `2026-09-28`
- Last Updated: `2026-09-28`
- Parent Documents: `CP-68 scaffold observability (CA-916/CA-917)`
- Child Documents: ``
- Related Documents: `CA-916 live scaffold feed`, `CA-917 manual scaffold trigger`
- Replaces: ``
- Tags: `engine-settings, scaffold, binding, ux`

## AI Quick View

### Summary

- Engine Settings lets the operator pick a project binding, but the "AI Scaffold"
  transcript card renders identically under every binding — the feed is keyed by
  `projectId` only.
- `loadScaffoldProgressTail` replays the persisted NDJSON via `project.Path`
  (the first/Primary binding), so transcripts of scaffolds that ran on a
  non-primary binding never replay after a runner restart.

### Current Ask

- Scope `GET /client/projects/{id}/scaffold/progress` to the caller's
  `workingDirectory` so the card only shows the run that belongs to the
  selected binding, and replay the persisted log from that same directory.

### Key Decisions

- `V-1` Hub events are served only when `hub.workspace` equals the normalized
  `workingDirectory` filter; a mismatched filter falls through to the filtered
  directory's persisted tail (never another workspace's events).
- `V-2` The query param is optional: absent = legacy project-level feed
  (TUI, Projects Settings keep current behavior byte-for-byte).

### Constraints

- `scaffoldProgressSnapshot(projectID, after)` signature is frozen by existing
  tests (oracle-rule) — add a scoped variant, keep the old signature delegating.
- No provider-specific code touched; fix is provider-agnostic (parity N/A).

### Open Questions

- ``

### Source Refs

- Reporter: tiendat (DnStudio project, bindings `Primary`/`mac`, 2026-09-28)
- `apps/local-runner/internal/runner/scaffold_progress.go`
- `apps/desktop-flowpilot/src/components/settings/ScaffoldActivity.tsx`

## 1. Issue Summary

In Engine Settings the operator selects binding `mac` and runs AI Scaffold. The
turn correctly executes inside `mac`'s working directory, but the live "AI
Scaffold" transcript card also renders when the `Primary` binding (a different
path) is selected — making it look like the scaffold ran on the wrong binding.

## 2. Parent Links

- impacted coding plan:
- impacted tech design:
- impacted system spec:

## 3. Environment and Reproduction

- environment: desktop-flowpilot + local-runner on macOS; project with two
  bindings pointing at different directories
- reproduction steps: Engine Settings → pick a non-primary binding → Run AI
  Scaffold → switch Binding dropdown to `Primary` → the same transcript card
  renders, implying the run belongs to `Primary`
- frequency: always when a project has ≥2 bindings

## 4. Expected vs Actual

- expected: the scaffold feed under binding X shows only the scaffold run that
  targeted X's working directory
- actual: the feed is project-scoped; it renders under every binding, and the
  post-restart replay only ever reads the first (Primary) binding's directory

## 5. Impact

- users affected: Desktop operators with multi-binding projects
- workflows affected: scaffold observability — misleading attribution + lost
  transcript replay for non-primary bindings
- severity: medium (UX confusion + observability gap; dispatch itself is correct)

## 6. Root Cause

- hypothesis:
- confirmed cause: `fetchScaffoldProgress(projectId)` sends no
  `workingDirectory`; the runner hub is `map[projectID]` and the persisted-tail
  replay resolves `project.Path` (binding[0]) rather than the dispatched
  workspace.
- evidence: `EngineSettings.tsx` mounts `<ScaffoldActivity projectId=…>` with no
  binding; `scaffold_progress.go` `scaffoldProgressSnapshot` has no workspace
  concept; `loadScaffoldProgressTail` uses `project.Path`.

## 7. Fix Strategy

- `F-1` Runner: `handleScaffoldProgress` reads an optional `workingDirectory`
  query param (normalized via `resolveEngineWorkingDirectory`); a new
  `scaffoldProgressSnapshotScoped(projectID, after, workspaceFilter)` serves hub
  events only when `hub.workspace` matches the filter, else replays the filtered
  directory's persisted tail.
- `F-2` Desktop: `fetchScaffoldProgress` gains an optional `workingDirectory`
  arg; `ScaffoldActivity` gains an optional `workingDirectory` prop (poll resets
  on change); `EngineSettings` passes `selectedBindingPath`. `ProjectsSettings`
  intentionally stays project-level.

## 7a. Code Guide Signatures

```go
// apps/local-runner/internal/runner/scaffold_progress.go
func (s *InteractiveService) scaffoldProgressSnapshot(projectID string, after int64) ScaffoldProgressSnapshot // unchanged signature; delegates to scoped variant
func (s *InteractiveService) scaffoldProgressSnapshotScoped(projectID string, after int64, workspaceFilter string) ScaffoldProgressSnapshot // F-1
func (s *InteractiveService) loadScaffoldProgressTail(projectID string, workspaceFilter string) []ScaffoldProgressEvent // F-1
func sameWorkspacePath(a, b string) bool // F-1
func (s *InteractiveService) handleScaffoldProgress(w http.ResponseWriter, r *http.Request) // F-1 — unchanged signature
```

```ts
// apps/desktop-flowpilot/src/components/settings/projectEngine.ts
export async function fetchScaffoldProgress(projectId: string, after?: number, signal?: AbortSignal, workingDirectory?: string | null): Promise<ScaffoldProgressSnapshot> // F-2
```

```ts
// apps/desktop-flowpilot/src/components/settings/ScaffoldActivity.tsx
export function ScaffoldActivity({ projectId, workingDirectory }: Props): React.ReactElement | null // F-2
```

## 8. Validation

- `V-1` `go test -count=1 ./internal/runner/ -run 'Bug549|ScaffoldProgress'` green
- `V-2` Desktop typecheck + `test:phase1` scaffold tests green
- `V-3` Manual: two bindings → scaffold on `mac` → card hidden under `Primary`

## 8a. Test Signatures

- `TestBug549_ProgressFilterHidesOtherWorkspaceFeed` — dispatch on dir A; `?workingDirectory=B` returns no events/inactive; `?workingDirectory=A` returns the run; no param keeps the legacy feed
- `TestBug549_ProgressFilterReplaysFilteredWorkspaceTail` — hub empty; persisted log only under dir B; `?workingDirectory=B` replays it (previously resolved `project.Path` → A → empty)
- `fetchScaffoldProgress` appends `workingDirectory` to the query when provided (frontend)

## 9. Regression Guard

- tests: bug549 test file + existing scaffold_progress tests (all untouched)
- alerts:
- audit checks: CA-1046

## 10. Definition of Done

- [x] Root cause confirmed with evidence (§6) — not a guess
- [x] §7a signatures landed; §8a tests exist, green, additive-only
- [x] Every pre-existing test untouched and green; old failure → STOP + report, never edit-to-green (oracle-rule) — remaining suite failures are env flakes identical on clean HEAD
- [x] Claude / Codex / Grok parity proven or tested for shared paths — provider-agnostic transport change, no per-provider code
- [x] Prior CA claims for this feature_key not undone
- [x] CA ledger entry written under the correct feature_key (CA-1046, `skill-anchored-init`)

## 11. Follow-Up Document Updates

- upstream docs that must change:
- notes left unchanged on purpose: ProjectsSettings keeps the project-level feed
  (that surface has no binding selector; seeing the project's latest run is the
  intended scope there)
