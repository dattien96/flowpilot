# BUG-280: Features Never Link To Governing Docs So Canonical Head Stays Spec-Less

## Metadata

- Document ID: `BUG-280`
- Title: `Features Never Link To Governing Docs So Canonical Head Stays Spec-Less`
- Feature Keys: `context-regression-engine`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [Task-097](../../08-Task/done/Task-097-Feature-Catalog-And-Resolver.md), [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-186](../../08-Task/todo/Task-186-Canonical-Head-And-Intent-Signature.md), [CP-43](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [CA-299](../../../change-audit/CA-299-feature-governing-doc-linking.md)
- Replaces: `None`
- Tags: `featurecatalog, doc-refs, canonical-head, spec-drift, done`

## AI Quick View

### Summary

- `featurecatalog.loadDocRefs` only attached a doc to a feature whose **key equals the doc's filename stem** (`c.Get(stem)`) — which never holds for real semantic keys (`change-contract`, `chat-ui`), so `DocRefs` on real features was **always empty**.
- Consequence: every `CanonicalHead` (CP-43/Task-186) was born `spec_less` forever, `r-spec-drift` could never fire, and the attach-spec rebaseline always failed with "no governing docs".
- Fix: governing docs declare `Feature Keys:` in metadata; `loadDocRefs` parses it and attaches the doc's stem to that real feature's `DocRefs`. Also extended the doc glob to include `06-System-Tech-Design/SD-*`.

### Current Ask

- Keep closed; annotate more governing docs with `Feature Keys:` as features need spec-backing.

### Key Decisions

- `F-1` Linkage is **doc-declares-feature** (SD-21 Q-3's explicit-front-matter option), not filename-stem matching and not fuzzy keyword inference (avoids R-5 "hash the wrong spec").

### Constraints

- Catalog is a derived artifact rebuilt from git each engine init, so the linkage source must live in git-tracked docs — hence a doc metadata field, not a UI/catalog edit.

### Open Questions

- None. (Auto-discovery beyond explicit declaration was rejected as too fuzzy.)

### Source Refs

- `featurecatalog/catalog.go` `loadDocRefs`/`parseDocFile`; `SS-13` metadata contract; `SD-21 Q-3`, `R-5`.

## 1. Issue Summary

Real features never received governing-doc references, leaving the entire Canonical Head spec-backing / drift machinery inert.

## 2. Parent Links

- impacted task: `Task-097` (Feature Catalog), consumed by `Task-186` (Canonical Head)
- impacted tech design: `SD-17` (catalog), `SD-21` (Q-3 governing-doc discovery)
- impacted system spec: `SS-13` (adds optional `Feature Keys` metadata field)

## 3. Environment and Reproduction

- environment: any project; engine init builds `features.ndjson`.
- reproduction: build the catalog, inspect any real feature (e.g. `change-contract`) → `DocRefs` empty; look up its Canonical Head → `status: spec_less` regardless of how many SS/SD/CP docs describe it.
- frequency: always (pre-fix).

## 4. Expected vs Actual

- expected: a feature described by SS-14/SD-21/CP-43 has those docs in `DocRefs` and its Head is `spec_backed`.
- actual: `DocRefs` empty; Head permanently `spec_less`.

## 5. Impact

- `r-spec-drift` (Task-186) never fires for real features — a changed governing spec goes undetected.
- attach-spec rebaseline endpoint/button always 400s ("no governing docs are registered").
- CP-43's spec-integrity value proposition was effectively dead for real features.

## 6. Root Cause

- confirmed: `loadDocRefs` computed `stem = docStem(path)` and only linked via `c.Get(stem)`. Real feature keys (from `FEATURE-KEYS.md`) are semantic kebab-case and never equal a doc filename stem, so the match always fell to the else-branch that creates a doc-stem-named *phantom* feature — real features were never touched.

## 7. Fix Strategy

- `F-1` `parseDocFile` now also extracts a `Feature Keys:` metadata line (`parseFeatureKeysLine`, backtick-tolerant, singular/plural, comma-separated).
- `F-2` `loadDocRefs` attaches each doc's stem to every declared real feature's `DocRefs` (real keys are loaded from `FEATURE-KEYS.md` in `Build` step (a), before step (b), so they resolve). The legacy stem-self behavior is kept for backward compat.
- `F-3` Extended the governing-doc glob to include `requirements/06-System-Tech-Design/SD-*.md` (walked for nested folders).
- `F-4` `SS-13` documents the optional `Feature Keys` field; `SS-14`/`SD-21`/`CP-43` annotated with `Feature Keys: change-contract`.

## 8. Validation

- `V-1` `TestBuild_GoverningDocAttachesToDeclaredFeature` — a doc declaring `Feature Keys: calc-core` attaches its stem to `calc-core.DocRefs`.
- `V-2` `TestParseFeatureKeysLine` — label parsing incl. negative cases (`Parent Documents:`, prose).
- `V-3` `go test ./internal/featurecatalog/... ./internal/changecontract/...` — 80 passed; `go build ./...` + `go vet` clean.

## 9. Regression Guard

- tests: `TestBuild_GoverningDocAttachesToDeclaredFeature`, `TestParseFeatureKeysLine`.
- audit: `CA-299`.

## 10. Follow-Up Document Updates

- `SS-13` metadata contract updated (optional `Feature Keys`).
- Task-186 / CP-43 P-3 completion notes updated to record the discovery gap is now closed.
