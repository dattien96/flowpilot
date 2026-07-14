# CP-48: Standardize Existing Requirements Docs To The Phase Contract

## Metadata

- Document ID: `CP-48`
- Title: `Standardize Existing Requirements Docs To The Phase Contract`
- Feature Keys: `context-regression-engine`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [BUG-280](../../09-BugFix/done/BUG-280-Features-Never-Link-To-Governing-Docs-So-Canonical-Head-Stays-Spec-Less.md), [Task-097 Feature Catalog](../../08-Task/done/Task-097-Feature-Catalog-And-Resolver.md), [CP-43](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md); `reqscaffold` package; skills `phase-document-authoring`, `phase-document-compliance`
- Replaces: `None`
- Tags: `context-regression-engine, doc-standardization, reqscaffold, phase-contract, conformance`

## AI Quick View

### Summary

- Give FlowPilot a **repeatable conformance engine** for requirements docs: scan every SS/SD/CP/Task/BugFix against the `SS-13` contract, report what is non-conformant, then fix it.
- Split fixes three ways: **auto** (deterministic Go codemod), **assisted** (agent fills semantic fields with human diff-review), **manual** (flag only).
- The standard **evolves** — e.g. `Feature Keys` was just added (BUG-280) and every old doc instantly became non-conformant. This engine is what back-fills such a rule across a whole project instead of by hand.
- Scope is **conforming docs that already exist** (P-1→P-3). Generating docs from a brief and ingesting loose non-standard docs are explicitly out of scope here.

### Current Ask

- Deliver a deterministic **scanner + conformance report** (P-1), deterministic **auto-fixers** (P-2), and an **assisted standardize flow** with human review (P-3).

### Key Decisions

- `P-1` The rule set is **encoded in Go** (fast, offline, deterministic); tests assert it stays in sync with the `FORMAT-REFERENCE-*.md` files. `SS-13` remains the human-readable spec.
- `P-2` Fix class per rule ∈ {`auto`, `assisted`, `manual`}; only `auto` writes without a human, `assisted` always shows a diff first (matches the explicit-human-confirm pattern used for Canonical Head lifecycle actions).
- `P-3` Reuse existing assets — `SS-13` + `FORMAT-REFERENCE-*` as the contract, `reqscaffold` for structure, `phase-document-authoring`/`phase-document-compliance` skills for the assisted pass — do not reinvent the standard.

### Constraints

- Non-destructive: never overwrite doc **content**; only normalize structure + fill missing metadata. Every assisted write is reviewable before it lands.
- The scanner must run offline with zero LLM cost (it is the backbone that re-runs whenever the standard changes).
- Do not weaken `reqscaffold`'s existing greenfield structure scaffold; this builds on top of it.

### Open Questions

- Does doc-standardization warrant its own `feature_key` (`doc-standardization`) split from `context-regression-engine`, and a dedicated `SD` doc, or is `SS-13` + `FORMAT-REFERENCE-*` a sufficient design source for a CP grounded directly in the contract?
- Where does the fix pass surface — a CLI command, a desktop action, or a built-in flow? (Leaning: scanner as CLI/desktop report; assisted fix as a flow with review.)

### Source Refs

- `SS-13` (metadata block, `AI Quick View`, phase section order, traceability, downstream-vs-upstream authority).
- `FORMAT-REFERENCE-{SS,SD,CP,TASK,BUGFIX}.md` (per-phase section order).
- `phase-document-compliance` skill severity rules (Critical/Important/Minor).
- `BUG-280` (motivating case: an added rule that had to be back-filled by hand).

## 1. Goal

Turn the prose doc contract (`SS-13` + `FORMAT-REFERENCE-*`) into an executable, re-runnable conformance engine that reports and fixes non-conformant requirements docs in a target project — so that when the standard evolves, bringing every doc back into line is a command, not manual toil.

## 2. Input Documents

- system spec: `SS-13` (the contract being enforced).
- design source: `FORMAT-REFERENCE-{SS,SD,CP,TASK,BUGFIX}.md` (per-phase structure). No dedicated `SD` today — see Open Questions.
- prior art: `reqscaffold` (greenfield **structure** scaffold), `phase-document-authoring`/`phase-document-compliance` skills (manual per-doc authoring/audit), `flowgate` rule-registry pattern (model for the rule set).

## 3. Implementation Strategy

- overall approach: encode each contract rule as a `{id, phase-scope, detector, fixClass}` unit in Go (mirrors `flowgate.DefaultRules`). A deterministic scanner runs all detectors → a structured report. A fix pass consumes the report: `auto` rules run codemods; `assisted` rules dispatch an agent (using `phase-document-authoring`) that proposes a diff; `manual` rules are flagged.
- sequencing logic: P-1 (scanner/report) is the prerequisite for everything. P-2 (auto-fix) and P-3 (assisted) build on the report. Ship P-1 first so the scale of non-conformance is visible before investing in fixers.
- dependencies: `SS-13` is stable; `FORMAT-REFERENCE-*` are the section-order source of truth (tests pin Go rules to them). `reqscaffold` already guarantees the folder structure the scanner walks.

## 4. Work Breakdown

- `P-1` **Conformance scanner + report (Go, deterministic).** New package (e.g. `internal/docconform`): a markdown structure parser (metadata block, headings, ID tokens like `AC-*`/`D-*`/`P-*`/`T-*`/`V-*`), a rule registry with detectors, and a report model (per file → per rule → severity per `phase-document-compliance`). Rules include: metadata-block presence + required fields; `AI Quick View` subsection presence; phase section order matches `FORMAT-REFERENCE`; filename/folder-by-status convention; required ID presence; and `Feature Keys` present on governing SS/SD/CP (the BUG-280 rule). Tests assert rule expectations match the checked-in `FORMAT-REFERENCE-*` files.
- `P-2` **Auto-fixers (deterministic codemods).** For mechanical rules only: insert a missing metadata key with a placeholder value, reorder sections to the reference order, move a file to the folder matching its `Status` (todo/inprogress/done), normalize filename. Each auto-fix is idempotent and content-preserving; emits a per-file change list.
- `P-3` **Assisted standardize flow (agent + human review).** For semantic rules: fill/repair `AI Quick View`, infer and add `Feature Keys`, infer `Parent Documents` links (and verify targets resolve under `requirements/`), draft a missing required section from the doc's own body. Runs the `phase-document-authoring` skill scoped to *normalize structure + fill metadata, never rewrite content*; presents a diff the human approves before writing. `manual` findings (e.g. downstream overriding upstream truth) are surfaced, never auto-applied.

## 5. Touched Areas

- files: new `apps/local-runner/internal/docconform/` (scanner, rules, report + tests); wiring into a CLI/desktop entry point + an assisted flow definition (exact surface per Open Questions); reuse `reqscaffold`.
- modules: `docconform` (new), `reqscaffold`, `featurecatalog` (consumer of `Feature Keys`), skills `phase-document-authoring`/`phase-document-compliance`.
- database: none (operates on markdown files on disk).
- external systems: none (scanner offline; assisted pass uses the existing agent runtime).

## 6. Data or Migration Steps

- schema: none.
- data backfill: running the engine on this repo itself back-fills `Feature Keys` and any other newly-added rule across existing docs (dogfood; the BUG-280 manual edits become the first auto-detected/auto-suggested case).
- config updates: none required; rule set ships in code.

## 7. Validation Plan

- tests to add: detector unit tests per rule (conformant vs each violation); auto-fixer idempotency + content-preservation tests; a golden test that the Go rule set matches `FORMAT-REFERENCE-*`; a scan of this repo's `requirements/` as a fixture producing a stable report.
- manual checks: run scanner on this repo → review report; run assisted fix on a deliberately-broken doc → confirm diff is structure-only and correct.
- failure cases: malformed/empty doc must not panic (report as Critical); a doc already conformant yields zero findings and auto-fix is a no-op; assisted fix declined by human writes nothing.

## 8. Rollout and Fallback

- rollout order: P-1 (report, read-only, safe to ship first) → P-2 (auto-fix behind explicit invocation) → P-3 (assisted, human-gated).
- fallback path: the engine is invoked explicitly; if it misbehaves, docs are unchanged (report-only) or revertible via git (auto-fix writes are ordinary file edits).
- monitoring: the report itself is the signal (count of findings by severity trends to zero as docs conform).

## 9. Risks

- `R-1` Go rules drift from `SS-13`/`FORMAT-REFERENCE` prose. Mitigation: golden tests pinning rules to the reference files; `SS-13` change → rule + test update in the same PR.
- `R-2` Assisted pass rewrites content, not just structure. Mitigation: scope the authoring prompt to normalization only + mandatory human diff-review before write.
- `R-3` Over-eager auto-fix corrupts a doc (bad section-reorder). Mitigation: idempotency + content-preservation tests; auto-fix limited to mechanical rules; git-revertible.
- `R-4` Feature-key inference (assisted) attaches the wrong feature. Mitigation: human review; scanner only *flags* missing `Feature Keys`, the value is human-confirmed (consistent with BUG-280's explicit-declaration decision).

## 10. Definition of Done

- [ ] `P-1` `docconform` scans every `requirements/05-09` doc and emits a per-file, per-rule, severity-graded report; runs offline with no LLM; does not panic on malformed docs.
- [ ] `P-1` Rule set covers: metadata block + required fields, `AI Quick View` subsections, phase section order, folder-by-status, required IDs, and `Feature Keys` on governing SS/SD/CP; golden tests pin it to `FORMAT-REFERENCE-*`.
- [ ] `P-2` Auto-fixers resolve the mechanical rules idempotently and content-preservingly; a conformant doc is a no-op.
- [ ] `P-3` Assisted flow fills semantic fields (`AI Quick View`, `Feature Keys`, parent links, missing sections) via the authoring skill, shows a diff, and writes only on human approval; `manual` findings are flagged, never auto-applied.
- [ ] Running the engine on this repo brings its own docs (incl. the BUG-280 `Feature Keys` back-fill) to zero Critical findings.
- [ ] Out of scope (tracked separately if pursued): greenfield content generation from a brief, and ingestion/classification of loose non-standard docs into the SS/SD/CP taxonomy.

## 11. Out of Scope

- Generating real SS/SD/CP content from a product brief (beyond `reqscaffold`'s empty structure) — a future greenfield-authoring CP.
- Classifying/importing arbitrary pre-existing docs (READMEs, ad-hoc specs) into the phase taxonomy — a future brownfield-ingestion CP.
- Changing the contract itself (`SS-13`); this CP enforces the contract, it does not redefine it.
