---
id: CA-925
title: Init/standardize fixes — pack version stamping at install, truthful hook/ledger steps, docscan audit lines, workspace-rooted /standardize (BUG-415, 416, 422, 423)
type: BugFix
feature: skill-anchored-init
date: 2026-09-23
status: done
---

## Context

The live-verification wave exposed four defects on the init/standardize
surface:

- 46/47 `golang/`, 4/5 `reactjs/`, 1/10 `react-native/` flow-pack SKILL.md
  files ship without a `version:` marker. `fileMatchesVersion` reports them
  stale on every read, so `skillPack.current=false` forever, `skill_pack`
  tooling stays `stale`, and `shouldSkipBindInit` never fires — every bind
  rewrites ~184 files and re-runs ledger/catalog.
- `InstallPostCommitHook` unconditionally ran
  `os.MkdirAll(<dir>/.git/hooks)` — on a directory that is not a git work
  tree it fabricated a `.git/` skeleton, and the `hook_install` step still
  reported `ok`. `changeledger_build` always detailed
  `feature_history.ndjson` even when `Build` was a no-op (zero commits /
  non-git) and the file never existed.
- CP-48 mandates two audit lines — `docscan_scan_completed
  files_scanned=N issues_found=M` after every scan and
  `docscan_autofix_applied file=… changes=…` per repaired file — and no
  code path emitted either.
- `/standardize` resolved its root from `os.Getwd()` unless a test pinned
  `SetStandardizeWorkspaceRoot`; `cli/root.go` never pinned, so a runner
  serving `--workspace A` from cwd B scanned/wrote under B.

## Changes

### BUG-415 — install stamps the pack version marker

`internal/skillpack/install.go`: `Install` now writes
`ensurePackVersionMarker(srcBytes, PackVersion)`. The helper inserts (or
replaces) `version: <v>` inside existing YAML frontmatter — every shipped
SKILL.md already has a `---` block — and falls back to the legacy
first-line `version: <v>` format when frontmatter is absent. Stamping at
install rather than editing 51 embedded sources keeps the fix self-healing:
any future skill added without a marker still installs correctly
versioned, and genuinely older installs (e.g. `version: 5`) remain
detected-stale because `fileMatchesVersion` still governs the skip/write
decision.
Tests: `internal/skillpack/bug415_version_stamp_test.go` (4 tests: all
installed files versioned, reinstall idempotent, Status current, stale
v5 still detected).

### BUG-416 — truthful hook_install / changeledger_build outcomes

`internal/changeledger/hook.go`: new sentinel `ErrNotGitRepo` and
`resolveHooksDir` — `.git` absent → sentinel (no MkdirAll, nothing
fabricated); `.git` directory → `.git/hooks`; `.git` file (linked
worktree) → the `gitdir:` pointer's `hooks/` (previously this case failed
on mkdir of a `.git/` dir that can never exist). `engine_setup.go`:
`hook_install` maps `ErrNotGitRepo` → `outcome=skipped, "not a git work
tree"`; `changeledger_build` stats the ledger file after `Build` —
present → `ok` + path, absent → `skipped, "no commits recorded; ledger
file not created"`, error → existing error path.
Tests: `internal/changeledger/bug416_hook_nonrepo_test.go`,
`internal/runner/bug416_init_step_truth_test.go`.

### BUG-422 — docscan audit lines

`internal/docscan/scanner.go`: `ScanDirectory` logs
`docscan_scan_completed files_scanned=N issues_found=M` on success —
covers the full-project conformance scan and the SS-Lock post-publish
scan. `internal/runner/standardize_cmd.go`: `scanDocFiles` (scoped/mixed
paths) logs the same line. `internal/docscan/autofix.go`: new
`AutoFixDocumentDetailed` returns the applied repair-stage labels
(`metadata-inserted|fixed`, `ai-quick-view-inserted|fixed`,
`sections-rebuilt`); `AutoFixDocument` delegates. `autoFixDraftDocs` logs
`docscan_autofix_applied file=<path> changes=<count>` per actually
rewritten draft.
Tests: `internal/runner/bug422_docscan_audit_test.go` (3 tests covering
both scan paths + the per-file autofix line).

### BUG-423 — /standardize defaults to the runner workspace

`internal/runner/interactive_service.go` `AttachRunner`: when no explicit
`SetStandardizeWorkspaceRoot` pin exists, the service pins
`standardizeRootBySvc[s] = r.workspace`. Every real serve path attaches
the runner (`cli/root.go:158`), so `/standardize` now operates on the
configured workspace; explicit pins still win, so test sandboxes are
unchanged.
Tests: `internal/runner/bug423_standardize_root_test.go` (root
resolution, pin precedence, ExecuteStandardize e2e under a foreign cwd).

## Safe-fix-contract compliance

- Reproduce-first: all 12 Cluster J tests failed by assertion (or the
  missing `ErrNotGitRepo` symbol) on the pre-fix tree before any
  production change.
- Additive tests only; zero existing tests touched.
- Provider-agnostic by construction: pack install, hook resolution,
  ledger bookkeeping, docscan, and workspace-root resolution are all
  deterministic in-process Go — no provider session, no LLM call on any
  touched path. The embedded SKILL.md files are shared across all
  provider install roots (.claude/.agents/.grok/.opencode), so the stamp
  lands identically everywhere; live-verified end-to-end via the HTTP
  surface.

## Verification

- Focused: `go test -count=1 ./internal/skillpack/ ./internal/changeledger/
  ./internal/docscan/ ./internal/runner/ -run 'TestBug415|TestBug416|
  TestBug422|TestBug423'` — 12/12 green post-fix.
- Package suites: `changeledger`, `docscan` clean; `skillpack` has two
  pre-existing failures identical on baseline d191004f
  (`TestScaffoldYAMLIsNotTreatedAsSkill`, `TestInstall_CommonOnlyForNonePlatform`
  — count pins written before the pack gained skills); runner suite delta
  recorded in `requirements/07-Coding-Plan/done/CP-Test-Progress-Tracking.md`.
- Live (`flowpilot runner serve --workspace /tmp/fp-live-j/ws` from cwd
  `/tmp/fp-live-j/cwd`, port 4399):
  - `engine/init` manual+golang on the git bed → `skillpack_install` ok;
    0 of 248 installed SKILL.md files lack `version: 6`; a follow-up
    `bind` init → `status=skipped` ("engine already initialized for
    current skill-pack version") and `skillPack.current=true`.
  - `engine/init` on a non-git dir → `hook_install=skipped (not a git
    work tree)`, `changeledger_build=skipped (no commits recorded; ledger
    file not created)`, and `/tmp/fp-live-j/nongit/.git` was NOT created.
  - `POST /client/standardize {"path":"features/auth"}` → conformance
    mode scanning `ws/requirements`; server log shows
    `docscan_scan_completed files_scanned=2 issues_found=57`.
  - `POST /client/standardize {}` → full-project scan logged
    `files_scanned=8 issues_found=70` plus
    `docscan_autofix_applied file=…/todo/SS-99-Demo.md changes=3`; the
    draft gained the required blocks on disk.
  - cwd `/tmp/fp-live-j/cwd` remained empty throughout — no
    requirements/ tree was written outside the workspace.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: BUG-415
change_type: bugfix
summary: Init and standardize fixes; feature migrated engine-init -> skill-anchored-init (BUG-442)
# --->8---
