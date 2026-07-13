# CA-299 — Feature ↔ governing-doc linking (BUG-280)

## Scope

Fix [BUG-280](../requirements/09-BugFix/done/BUG-280-Features-Never-Link-To-Governing-Docs-So-Canonical-Head-Stays-Spec-Less.md): real features never received governing-doc references, so every Canonical Head (CP-43/Task-186) was born `spec_less` forever and `r-spec-drift` / attach-spec could never work. Governing docs now declare which feature they govern.

## Changes

- `apps/local-runner/internal/featurecatalog/catalog.go`:
  - New `parseFeatureKeysLine` + `featureKeysLineRe`: parses a `Feature Keys:` metadata line (case-insensitive, singular/plural, backtick-tolerant, comma-separated).
  - `parseDocFile` now returns the declared `featureKeys` alongside title/summary/keywords.
  - `loadDocRefs` attaches each governing doc's stem to every declared **real** feature's `DocRefs` (real keys come from `FEATURE-KEYS.md`, loaded in `Build` step (a) before doc parsing in step (b)). The legacy doc-stem-self linkage is kept for backward compatibility.
  - Extended the governing-doc glob to include `requirements/06-System-Tech-Design/SD-*.md` (walked for nested folders), so tech-design specs count as governing docs too.
- `requirements/05-System-Specs/SS-13-...md`: documented the optional `Feature Keys` metadata field (the doc contract addition).
- `requirements/05-System-Specs/SS-14-...md`, `requirements/06-System-Tech-Design/SD-21-...md`, `requirements/07-Coding-Plan/todo/CP-43-...md`: annotated with `Feature Keys: change-contract` so the `change-contract` feature becomes spec-backed.

## Why this design (SD-21 Q-3 resolved)

The catalog (`features.ndjson`) is a derived artifact rebuilt from git on each engine init, so the linkage source must live in git-tracked docs — a doc metadata field, not a UI/catalog edit that would be wiped. Chosen over: filename-stem matching (never matches real keys — the bug), and fuzzy keyword inference (SD-21 R-5 "hash the wrong spec"). Docs declaring their own feature is explicit, re-derivable, and low-false-positive.

## Effect

Once a project's catalog is rebuilt (engine init), a feature with annotated governing docs gets a non-empty `DocRefs`; its Canonical Head is then born `spec_backed` (or can be rebaselined from `spec_less`), and editing any governing doc trips `r-spec-drift`. This makes the attach-spec rebaseline endpoint/button (CA-298) actually usable in normal flow, not just via hand-editing `features.ndjson`.

## Verification

- `go build ./...` clean; `go vet ./internal/featurecatalog/...` clean.
- `go test ./internal/featurecatalog/... ./internal/changecontract/...` — 80 passed (new: `TestBuild_GoverningDocAttachesToDeclaredFeature`, `TestParseFeatureKeysLine`).

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: BUG-280
change_type: bugfix
summary: featurecatalog now links a governing SS/SD/CP doc to the real feature it declares via a Feature Keys metadata line, instead of the old filename-stem matching that never matched real feature keys — this was why every Canonical Head stayed spec_less forever and spec-drift/attach-spec could not work
# --->8---
