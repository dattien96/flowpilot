# Task-186: Canonical Head And Intent Signature

## Metadata

- Document ID: `Task-186`
- Title: `Canonical Head And Intent Signature`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-3), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-7, AC-8, BR-2)
- Child Documents: `None`
- Related Documents: [Task-184: Change Contract Capture](./Task-184-Change-Contract-Capture.md), [Task-187: Superseding Decision Records And Retire](./Task-187-Superseding-Decision-Records-And-Retire.md), [Task-096: Commit-History Ledger](../../08-Task/done/Task-096-Commit-History-Ledger.md), [Task-097: Feature Catalog And Resolver](../../08-Task/done/Task-097-Feature-Catalog-And-Resolver.md)
- Replaces: `None`
- Tags: `canonical-head, intent-signature, spec-drift, code-drift, flowgate, changecontract, local-runner`

## AI Quick View

### Summary

- Build the per-feature `CanonicalHead` (`.flowpilot/canonical/<feature_key>.json`) carrying `intent_signature = sha256(feature_key + governing doc ids + their content hashes + behavior_statement)`.
- Backfill Heads on bind from newest `changeledger` entry + `featurecatalog.DocRefs`; **birth** path mints a Head from the Contract's `intent` when no history exists (`spec_less` when no governing docs).
- Add drift rules `r-spec-drift`, `r-code-drift`, and the lifecycle `r-attach-spec` (re-baseline `spec_less → current`, human-approved).

### Current Ask

- Implement `SD-21 P-3`: Head store, signature, backfill+birth, `UpdateHead`, and the three status/lifecycle rules.

### Key Decisions

- `T-1` Hash **intent**, not code (`SD-21 D-1`); signature is deterministic and reproducible.
- `T-2` `spec_less` births are allowed and flagged low-confidence (`AC-2`); `attach-spec` is a re-baseline, **not** `spec_drift`.

### Constraints

- Reuse Task-096 (`changeledger`) and Task-097 (`featurecatalog.DocRefs`) for seed data; reuse `flowgate` evaluate/enforce and `SD-16` approval gate for `r-attach-spec`.
- Drift is surfaced for human reconciliation, never auto-rewritten (`BR-2`); non-fatal (`AC-9`).

### Open Questions

- Governing-doc discovery: `featurecatalog.DocRefs` vs explicit `governs:` front-matter (`SD-21 Q-3`).
- Head granularity per `feature_key` vs per code-unit (`SD-21 Q-4`).

### Source Refs

- `SD-21 §3 D-1`, `§5` (`CanonicalHead`, transitions), `§6` (`r-spec-drift`/`r-code-drift`/`r-attach-spec`), `§7` steps 7, `§13.3 T1/T3`, `§13.5 T-1/T-1b`. `CP-43 §4.3`. `SS-14 AC-7`, `AC-8`, `AC-2`, `BR-2`.

## 1. Goal

One authoritative record per feature that carries a reproducible intent signature and flips to `spec_drifted`/`code_drifted` when code and intent diverge, with a correct birth path for brand-new (possibly spec-less) features.

## 2. Parent Links

- coding plan: `CP-43` P-3
- tech design: `SD-21` §5, §6 (drift + attach rules), D-1
- system spec: `SS-14` AC-7, AC-8, AC-2, BR-2
- specific upstream ids: `P-3`, `r-spec-drift`, `r-code-drift`, `r-attach-spec`

## 3. Trigger

CP-35 packs ordered history but has no single "current truth" record and no integrity check between code and its governing spec. This task adds that authority + signature so churn need not be replayed and drift is detectable.

## 4. Exact Change

- `T-1` `changecontract/head.go` — `CanonicalHead` struct per `SD-21 §5` (incl. `spec_confidence`, `superseded_by`, `retired_at`, `status ∈ {spec_less,current,spec_drifted,code_drifted,renamed,merged,deprecated}`); store one `<feature_key>.json` under `.flowpilot/canonical/`; `Load/Save`.
- `T-2` `changecontract/signature.go` — `ComputeSignature(h CanonicalHead) string = sha256(canonicalJoin(feature_key, sort(GoverningDocIDs), sort(values(GoverningDocHashes)), normalize(BehaviorStatement)))`; `HashDoc(path) (string,error)` (sha256 of the markdown file; missing file → recorded, not panic — `F-1`).
- `T-3` `changecontract/backfill.go` — `BuildHead(featureKey) (CanonicalHead, error)`: seed `head_commit`/`behavior_statement` from newest `changeledger` entry; `governing_doc_ids` from `featurecatalog.DocRefs` (`Q-3`); **birth** — when no history, mint from the passing Contract's `intent`; set `spec_confidence="spec_less"`, `status="spec_less"` when `governing_doc_ids` is empty (`AC-2`).
- `T-4` `changecontract/update.go` — `UpdateHead(featureKey, c Contract, diff) error` on gate pass: fold realized intent into `behavior_statement`, refresh `head_commit`, recompute signature, set `status` per the §5 transition table; `RebaselineWithSpec(featureKey, docIDs)` for the attach-spec path (after human approval).
- `T-5` `flowgate/evaluate.go` drift triggers: `governing_spec_changed` (a `GoverningDocHashes` entry no longer matches the file hash) → `r-spec-drift`; `code_diverged_from_intent` (feature code in diff, out-of-contract, `behavior_statement`/docs unchanged) → `r-code-drift`; `governing_spec_added_to_spec_less` (a `spec_less` feature gains a governing doc) → `r-attach-spec`.
- `T-6` `flowgate/rules.go` — add `r-spec-drift` (warn), `r-code-drift` (warn), `r-attach-spec` (approve, reuse `SD-16` gate); seed to `flow-rules.json`.
- `T-7` Tests — signature reproducibility (same inputs → same hash) + doc-content sensitivity (edit doc → new hash); birth `spec_less` (no history/no docs); `attach-spec` re-baseline flips `spec_less → current` and is **not** classified as `spec_drift`; each status transition; missing-doc hash does not panic.

## 5. Touched Areas

- files: `apps/local-runner/internal/changecontract/{head,signature,backfill,update}.go` (+ `_test.go`), `internal/flowgate/{rules,evaluate}.go`, `settings/flow-rules.json`
- modules: `changecontract`, `flowgate`, `changeledger`, `featurecatalog`
- routes: SSE `flow_gate_violation` (existing); approval-gate event (SD-16) for `r-attach-spec`
- tables: none (local JSON); `flow_rules` mirror

## 6. Acceptance Check (DoD)

- [x] Each feature has a `CanonicalHead` JSON with a reproducible `intent_signature`; recomputing on identical inputs yields the identical hash. (`ComputeSignature`, `TestComputeSignatureReproducible`.)
- [x] Editing a governing SS/SD changes its stored hash → Head flips to `spec_drifted` → `r-spec-drift` (warn); no auto-rewrite of `behavior_statement` (`BR-2`). (`SpecDrifted`, `updateCanonicalHead` leaves the Head untouched when drifted, `r-spec-drift` rule.)
- [x] An out-of-contract code change with unchanged spec/behavior flips the Head to `code_drifted` → `r-code-drift` (warn). (`CodeDrifted`, `r-code-drift` rule.)
- [x] An in-contract, gate-passing change runs `UpdateHead`, refreshes the signature, and keeps `status=current`. (`updateCanonicalHead` calls `UpdateHead`+`SaveHead` only when not drifted/pending.)
- [x] **Birth**: a brand-new `feature_key` with no history mints a Head from the Contract `intent`; with no governing docs it is `spec_less` and flagged low-confidence. (`BuildHead`, `TestBuildHeadBirthWithNoHistoryOrDocsIsSpecLess`.)
- [x] **Attach-spec**: adding the first governing doc to a `spec_less` feature raises `r-attach-spec` (approve), and on approval re-baselines to `current` — and is **not** reported as `spec_drift`. Detection is wired (`updateCanonicalHead` sets `attachSpecPending` and freezes the Head until resolved; `r-attach-spec` fires) and the **approval-confirmation caller now exists** (added 2026-07-13, follow-up): `POST /client/projects/{projectId}/features/{featureKey}/canonical-head/rebaseline` resolves the feature's governing DocRefs from the catalog and calls `RebaselineWithSpec`, wired to a "Confirm spec & rebaseline" button in the desktop app's Canonical Head panel (`ProjectsSettings.tsx`). Verified with `TestHandleRebaselineCanonicalHead` (+ the no-governing-docs 400 case). This is an explicit user action (the human confirmation), not the SD-16 flow-gate approval modal — a deliberate design choice (see Completion Notes).
- [x] A missing/deleted governing doc does not panic; the Head is flagged, signature recompute is safe (`F-1`). (`HashDoc` returns an error not a panic; `hashGoverningDocs` records `""`; `TestSpecDriftedTrueWhenDocDeleted`, `TestHashDocReturnsErrorNotPanicOnMissingFile`.)
- [x] Backfill builds Heads for existing features from `changeledger` + `featurecatalog` on bind. (`BuildHead`, `TestBuildHeadWithGoverningDocsIsSpecBacked`, `TestBuildHeadBackfillsFromLedgerHistory`.)
- [x] `go test ./internal/changecontract/... ./internal/flowgate/...` passes; no `changeledger`/`featurecatalog` regression. (172 tests passed, `go vet` clean, 2026-07-13.)

## 7. Out of Scope

- Folding rejected-alternative Decision Records and the `r-retire` end-of-life rule (Task-187).
- Prompt packing (Head-first) and Admin panels (Task-188).
- Per-code-unit (symbol) Heads (`SD-21 Q-4`, deferred).

## 8. Completion Notes

- result: **done** — core Head store, signature, backfill/birth, drift detection, and the 3 flowgate rules are implemented and unit-tested; the attach-spec re-baseline confirmation caller (rebaseline endpoint + desktop button) was added as a follow-up on 2026-07-13, so `r-attach-spec` is now fully round-trippable (fire → human confirms → Head flips to current). All DoD items checked.
- design decision (resolves part of SD-21's Open Question): attach-spec re-baseline is an **explicit user action** in the desktop app, not the SD-16 flow-gate approval modal. Rationale: the transition is a deliberate one-off human judgement ("does this spec describe current behavior?"), better served by a Projects-settings action than by an inline gate modal that would re-appear every turn.
- dependency fixed: [BUG-280](../../09-BugFix/done/BUG-280-Features-Never-Link-To-Governing-Docs-So-Canonical-Head-Stays-Spec-Less.md) — until it, real features never got `DocRefs`, so every Head was born `spec_less` forever and the spec-backed/drift/attach-spec paths were inert in practice. Governing docs now declare `Feature Keys:` so a feature's Head can actually be `spec_backed`.
- follow-ups: Task-187 folds decisions into the Head and handles retire (done); Task-188 packs the Head first and adds desktop visibility (done). The status should be flipped `in_progress → done` and the file moved to `08-Task/done/` at commit time.
- upstream docs updated: none (doc terminology already aligned during doc-prep before Task-184 began)
