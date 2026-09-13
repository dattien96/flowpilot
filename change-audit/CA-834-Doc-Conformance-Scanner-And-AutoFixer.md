# CA-834 — Task-332: docscan conformance scanner + non-destructive autofix (CP-48)

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-332
change_type: feature
summary: add internal/docscan package — SS-13 conformance scanner (9 deterministic rules, phase tables from FORMAT-REFERENCE samples) and non-destructive AutoFixDocument codemod
# --->8---

## Why

CP-48 P-1/P-2: when the doc contract grows a new rule (e.g. `Feature Keys` in BUG-280), the whole requirements tree must be brought back to conformance by a command, not by hand. Task-332 ships the offline 0-LLM machinery; `/standardize` wiring lands with Task-333.

## Change

- `docscan/rules.go` (new): `Severity`/`ScanIssue`/`ScanReport`/`ConformanceRule` per Code Guide, `DefaultConformanceRules()` (9 rules — mandated 6 + `missing_ai_quick_view_subsection` + `missing_required_section`), per-phase canonical metadata fields / AI-Quick-View sub-items / numbered-section tables taken verbatim from the five FORMAT-REFERENCE samples, `NormalizePhase`.
- `docscan/scanner.go` (new): `DetectPhase` (directory + filename prefix), `ScanDocument`, `ScanDirectory`, fenced-code-aware block parser, structural checks; legacy Vietnamese-titled headings intentionally mismatch canonical English names (documented deferral to Task-333 — scanner must never translate/rename).
- `docscan/autofix.go` (new): `AutoFixDocument` — metadata field insertion (e.g. `Feature Keys: None`), section reorder with canonical renumbering, TODO skeletons for missing sections; byte-for-byte identity on compliant input; errors on non-UTF-8 and unsupported phase. FORMAT-REFERENCE-* guides exempt from `missing_feature_keys` per SS-13 §11.
- New package imports only stdlib (`go list -deps`: itself only) — offline by construction.

## Tests

`docscan_test.go`: all 8 Task-332 §10 signatures implemented fully + 15 additional (FORMAT-REFERENCE sync = 0 issues on all 5 real samples, perf 120 files ~12ms (<500ms budget; 768-file real tree ~0.14s), CRLF, round-trip byte preservation, duplicate sections, tilde/unclosed fences). 23/23 pass; gofmt/vet clean.

## Providers

Case 1 agnostic: pure-Go offline markdown scanning with deterministic input validation — no LLM, no provider adapters; identical behavior across Claude/Codex/Grok by construction.

## Prior claims intact

CA-695 / CA-442 / CA-441 (flowgate gates) untouched — docscan is an additive leaf package with zero repo-internal callers. CP-48 closed: doc moved to `07-Coding-Plan/done/` with DOD checked (P-3 Assisted-Fixer remains explicitly deferred to a later CP).
