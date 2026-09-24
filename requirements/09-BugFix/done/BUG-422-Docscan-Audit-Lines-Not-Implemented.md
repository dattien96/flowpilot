# BUG-422: Required `docscan_scan_completed` / `docscan_autofix_applied` audit log lines do not exist

## Metadata

- Document ID: `BUG-422`
- Title: `CP-48-Test-Steps §5 audit lines absent from code and logs — standardize/docscan path is completely silent`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-48-Test-Steps](../../07-Coding-Plan/done/CP-48-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp48/RESULT.md` (BUG-LIVE-CP48-3)
- Feature Keys: `standardize`, `docscan`, `audit-logging`

## AI Quick View

### Summary

- CP-48-Test-Steps §5 requires audit lines `docscan_scan_completed files_scanned=… issues_found=…` and `docscan_autofix_applied file=… changes=…`; neither string exists anywhere in `apps/local-runner` (grep: 0 hits) and `runner.log` contains zero `docscan`/`standardize` audit lines across both live `/client/standardize` calls.
- `standardize_cmd.go` and `internal/docscan/*.go` contain no `log.`/`slog.` calls at all — the scan+fix path is completely silent.
- Contract gap: the audit surface the test plan asserts was never implemented.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

Two live `POST /client/standardize` calls (scan: 15 files / 65 issues; autofix: repaired all 4 `todo/` drafts) produced zero `docscan_*` audit lines in `runner.log`; grepping the codebase for `docscan_scan_completed`/`docscan_autofix_applied` returns nothing.

### Expected

Per CP-48-Test-Steps §5: `docscan_scan_completed files_scanned=N issues_found=M` after each scan and `docscan_autofix_applied file=… changes=…` per repaired file, so the scan/fix activity is observable in logs.

### Actual

The entire scan + autofix pipeline runs silently — response JSON only, no audit trail. (The functional path itself is verified PASS: correct rule_ids/severities, content preserved verbatim, idempotent second run.)

### Impact

Observability/compliance gap: operators cannot tell from logs that a doc scan ran, how many issues were found, or which files were auto-rewritten — the auto-fix path modifies requirement docs with no audit line.

## Reproduction

1. Seed nonconforming `todo/` drafts in a bed; `POST /client/standardize {"mode":"conformance"}` twice.
2. `grep docscan runner.log` → empty; `grep -rn "docscan_scan_completed\|docscan_autofix_applied" apps/local-runner` → 0 hits.

## Root cause

- `apps/local-runner/internal/runner/standardize_cmd.go` and `apps/local-runner/internal/docscan/*.go` — no `log.`/`slog.` calls anywhere in the scan/fix path; the CP-48-Test-Steps §5 audit contract was never implemented.

## Evidence

- `~/fp-beds/lt-evidence/cp48/RESULT.md` — §"Bugs found" BUG-LIVE-CP48-3; `runner.log` (grep empty); code grep over `internal/docscan` + `standardize_cmd.go` (0 hits).
- Live scans: `L-48-1-standardize-1.json` (15 files / 65 issues), `L-48-2-standardize-2.json` (14/15 conforming, idempotent), `L-48-2-autofix.diff`.

## Severity

- `medium` — contract/observability gap on a doc-rewriting feature; function itself works.

## Completion Notes (implemented 2026-09-23, CA-925b)

- Root cause confirmed: neither `docscan_scan_completed` nor
  `docscan_autofix_applied` existed anywhere in the runner.
- Fix: `docscan.ScanDirectory` logs `docscan_scan_completed
  files_scanned=N issues_found=M` after every successful scan (covers
  full-project conformance and the SS-Lock post-publish scan);
  `scanDocFiles` (scoped/mixed paths) logs the same line. New
  `docscan.AutoFixDocumentDetailed` returns applied repair-stage labels;
  `autoFixDraftDocs` logs `docscan_autofix_applied file=<path>
  changes=<count>` for every draft actually rewritten.
- Tests: `internal/runner/bug422_docscan_audit_test.go` — both scan paths
  emit the line; a repaired todo/ draft emits the autofix line.
- Live: `POST /client/standardize {"path":"features/auth"}` →
  `docscan_scan_completed files_scanned=2 issues_found=57`;
  `POST /client/standardize {}` → `files_scanned=8 issues_found=70` +
  `docscan_autofix_applied file=…/todo/SS-99-Demo.md changes=3` (draft
  gained the required blocks on disk).
