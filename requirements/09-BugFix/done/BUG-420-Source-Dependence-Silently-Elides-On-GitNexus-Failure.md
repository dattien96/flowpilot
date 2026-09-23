# BUG-420: `source.dependence` silently elides when `npx gitnexus impact` fails (repo-name ≠ dir basename)

## Metadata

- Document ID: `BUG-420`
- Title: `GitNexus per-target lookups fail on name-mismatched/cloned beds → errors swallowed, section vanishes with no warning; Available() hardcoded true`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-44-Pluggable-Context-Source-Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp44/RESULT.md` (BUG-LIVE-1) + `~/fp-beds/lt-evidence/cp49/RESULT.md` (BUG-LIVE-6)
- Feature Keys: `context-produce`, `source-dependence`, `gitnexus`, `knowledge`

## AI Quick View

### Summary

- `gitNexusProvider.Dependents` shells out to `npx gitnexus impact <target> --repo <dir-basename>`; on beds whose GitNexus index name differs from the directory basename (e.g. bed `lt-cp44` indexed as `gate-sandbox`), every call returns `Repository "lt-cpNN" not found`.
- `renderDependenceBody` swallows each per-target error (`continue`) → empty body → section elided at render; `Available()` is unconditionally `true` so the "GitNexus not indexed" guidance body never fires either — the failure is indistinguishable from a legitimately-zero blast radius.
- Same mismatch repeated on cloned beds: `Repository "lt-cp49" not found` during knowledge bootstrap/context enrichment (repo list contains `gate-sandbox`, `flowpilot`, not the project dir name).

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

`flow_context_package` `source.dependence` section arrives with `Body:""`, `SourceRef:""`, `Warnings:null` — rendered prompt shows no `## Dependence` and no warning. `feature.history` by contrast correctly records `no change history found for feature: calc-core`.

### Expected

- When every per-target GitNexus lookup fails, the section surfaces a warning (e.g. `source.dependence` warning `gitnexus impact failed: repo not found`) instead of vanishing silently.
- `Available()` reflects real index reachability so the "not indexed" guidance can fire.
- Repo-name resolution does not assume index name == directory basename on cloned/renamed workspaces.

### Actual

- Runner log fills with `Error: Repository "lt-cp44" not found. Available: … gate-sandbox …` — manual `npx gitnexus impact calc.go --repo lt-cp44` reproduces verbatim.
- The section is elided; no `Warnings` entry; downstream prompts carry zero signal that dependence data was attempted and lost.
- On `lt-cp49` (cloned from `gate-sandbox`, `.gitnexus/meta.json` names repo `gate-sandbox` while project id/dir is `lt-cp49`): repeated `Repository "lt-cp49" not found` — knowledge bootstrap/context enrichment fails every time.

### Impact

A default context source can disappear with zero observable signal — degrade-contract/observability gap. Blast-radius awareness (a CP-44/CP-66 value-add) is silently absent exactly on cloned or renamed workspaces, i.e. common live-test/CI shapes.

## Reproduction

1. Index a workspace under a repo name ≠ its directory basename (or make `npx gitnexus impact` fail), e.g. clone a `gate-sandbox` bed to `lt-cpNN` keeping `.gitnexus/meta.json`.
2. Run any flow with a frozen contract that reaches `context.produce` (e.g. bug-harness).
3. Inspect `flow_context_package.sections[source.dependence]` → empty body, no warnings; runner log shows `Repository "<dir>" not found`.

## Root cause

- `apps/local-runner/internal/structure/gitnexus.go:31-32` — `Dependents` invokes `npx gitnexus impact <target> --repo <dir-basename>`; `repoNameFromDir` (:60) uses the basename, which diverges from the indexed repo name on cloned/renamed beds.
- `apps/local-runner/internal/runner/context_source_dependence.go:95` — `renderDependenceBody` `continue`s past each per-target error → `any=false` → empty `Body` → `renderFlowContextSection` elides the section.
- `apps/local-runner/internal/structure/gitnexus.go:19` — `Available()` returns `true` unconditionally → the "GitNexus not indexed" guidance body never fires either.

## Evidence

- `~/fp-beds/lt-evidence/cp44/RESULT.md` — §8 BUG-LIVE-1 (`fcp-26b3529d` section dump; `L-44-3-probes.txt` manual repro; `runner.log` error lines).
- `~/fp-beds/lt-evidence/cp49/RESULT.md` — BUG-LIVE-6 (`runner.log` repeated `Repository "lt-cp49" not found`; bed `.gitnexus/meta.json` names `gate-sandbox`).
- Verified on main worktree HEAD `435e336b`: `gitnexus.go:19` `Available()` unconditional; `context_source_dependence.go:95-96` error-swallowing `continue`.

## Severity

- `medium` — silent loss of a default context section + repeated GitNexus failures on name-mismatched workspaces; no crash, but zero observability of the degradation.

## Completion Notes (implemented 2026-09-23, CA-924)

- Root cause: `repoNameFromDir` used the current directory basename as the
  GitNexus repo name — wrong on beds renamed/cloned after indexing — and
  `renderDependenceBody` swallowed per-target errors with `continue`, so a
  full failure run produced an empty section with no signal.
- Fix: `repoNameFromDir` reads `.gitnexus/meta.json` `repoPath` first
  (basename = registered name), basename fallback when metadata is absent;
  per-target failures now collect into `section.Warnings`
  (`gitnexus impact failed for N/M target(s): <err>`), which merge into
  `FlowContextPackage.Warnings`. `Available()` is unchanged —
  `structure_test.go` pins `Available()==true` unconditionally and may not
  be weakened; observable degradation is delivered via warnings carrying
  the real error text (stronger than the generic guidance proposed).
- Tests: `internal/structure/gitnexus_bug420_test.go` + runner
  `TestBug420_*` (red by assertion pre-fix).
- Live: `/tmp/fp-live-i` bed indexed as `ws` then served via a `runner-ws`
  symlink — `--repo runner-ws` reproduces `Repository "runner-ws" not
  found`; `--repo ws` (the meta.json-resolved name) reaches the index.
