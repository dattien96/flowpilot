# CA-433 — Fix GitNexus Structure Provider Always Returns Empty (BUG-323)

## Scope

Fixed [BUG-323](../requirements/09-BugFix/done/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md): `structure.gitNexusProvider.Dependents` now invokes the real GitNexus CLI contract (`npx gitnexus impact <target> --repo <basename(repoDir)>` — no `--json`), parses the current impact JSON schema (`impactedCount`, `affected_processes`, `affected_modules`, `byDepth`), keeps the legacy `{dependents,nearest,flows}` parser as a fallback, and returns a **non-nil error** for CLI/`{"error":...}` failures so callers can distinguish query failure from a legitimate zero blast radius.

## Changes

- `apps/local-runner/internal/structure/gitnexus.go`: rewrote `Dependents`, added `repoNameFromDir`, `gitNexusResponseError`, `tryParseGitNexusImpactV2`, `collectGitNexusImpactNames`, `finalizeDependentsSummary`; legacy `tryParseJSON`/`tryParseText` preserved for existing tests.
- `apps/local-runner/internal/structure/gitnexus_bug323_test.go` (new, additive only): v2 parser tests, error-vs-empty distinction, live smoke against `ScopeDiff` + missing target.

## Design notes

- **F-1/F-2/F-3 from BUG-323 delivered.** `--json` removed; `--repo` derived from `filepath.Base(repoDir)`; v2 schema tried before legacy JSON.
- **Errors are visible again.** `Dependents` returns `fmt.Errorf(...)` on non-zero exit (when stdout is not a structured error payload), on `{"error":"..."}`, and includes stderr when present. `changecontract.HighSeverity` already skips errors non-fatally (`continue`) — unchanged, AC-9 preserved.
- **Legitimate zero hits stay success.** `impactedCount: 0` parses to `Count: 0`, `err == nil`.
- **Out of scope (BUG-323 non-goals, unchanged):** `HighSeverity` still passes **file paths** to `Dependents`; GitNexus resolves **symbols** only — `r-scope` block on out-of-scope **file** edits still does not fire until a follow-up maps paths→symbols or populates `DeclaredSymbols` (BUG-323 Q-2 / Task-259). Task-259 remains blocked on that input work, not on this provider fix.

## Verification

- `go test ./internal/structure/... -count=1 -v` — **21 tests, all green** (13 pre-existing unchanged + 8 new).
- `go test ./internal/changecontract/... -count=1 -run 'TestHighSeverity|TestScopeDiff' -v` — **11 tests, all green**.
- Manual CLI parity (2026-08-11): `npx gitnexus impact ScopeDiff --repo flowpilot` → `impactedCount: 4`; `NonExistentSymbolXYZ` → `{"error":"Target ... not found"}` (matches new error path).

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: BUG-323
change_type: bugfix
summary: gitNexusProvider.Dependents now uses the real gitnexus impact CLI (--repo, current JSON schema) and returns errors on failure instead of silently empty summaries
# --->8---
