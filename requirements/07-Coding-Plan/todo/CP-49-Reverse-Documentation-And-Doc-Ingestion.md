# CP-49: Reverse-Documentation From Code And Loose-Doc Ingestion

## Metadata

- Document ID: `CP-49`
- Title: `Reverse-Documentation From Code And Loose-Doc Ingestion`
- Feature Keys: `context-regression-engine`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [CP-48: Standardize Existing Requirements Docs](./CP-48-Standardize-Doc.md) (consumes CP-49's output), [Task-097 Feature Catalog](../../08-Task/done/Task-097-Feature-Catalog-And-Resolver.md); `reqscaffold` package; GitNexus code-intelligence (symbols / relationships / execution flows / wiki); skill `phase-document-authoring`
- Replaces: `None`
- Tags: `context-regression-engine, reverse-documentation, doc-ingestion, gitnexus, brownfield`

## AI Quick View

### Summary

- For a legacy project with **no requirements docs** (or only loose README/ad-hoc specs), bootstrap the SS/SD/CP taxonomy by (a) **ingesting** existing loose docs and (b) **reverse-documenting from code**.
- **Hard ceiling, designed-in**: code tells you *what* it does, not *why*. So generate strong **as-built SD** + partial **CP/Task/BugFix** from real evidence, but **SS (intent/acceptance criteria) is a skeleton + `TODO` only — never fabricated**.
- **Anti-hallucination rule**: every generated statement cites evidence (a GitNexus symbol/flow, a commit/changeledger entry, or a source line) or is marked `TODO: human intent needed`. Nothing becomes "truth" without human accept.
- Output is **draft** docs; [CP-48](./CP-48-Standardize-Doc.md) then normalizes + validates them against the contract. Clean seam: CP-49 generates, CP-48 conforms.

### Current Ask

- Deliver an **evidence-collection layer** (P-1), **loose-doc ingestion** (P-2), and **code→as-built draft generation** (P-3), all gated by **human review + accept** (P-4).

### Key Decisions

- `P-1` Ground truth comes from **evidence**, not raw code fed to an LLM: GitNexus graph (structure/flows/clusters) + git history/changeledger + loose docs. Degrade gracefully when GitNexus is absent (lighter static/file scan, everything flagged lower-confidence).
- `P-3` **SS is skeleton + `TODO`, not fabricated** — code cannot recover business intent. Only SD/CP/Task/BugFix are generated from evidence, each statement provenance-linked + confidence-labeled.
- `P-4` Generated docs are **draft**; a human accepts before they are authoritative, then [CP-48](./CP-48-Standardize-Doc.md) conforms/validates them. No auto-publish.

### Constraints

- Non-fabrication is absolute: no evidence → `TODO`, never invented prose.
- Read-only on source code; writes only draft docs under `requirements/` (or a staging area) pending human accept.
- Reuse GitNexus + `reqscaffold` + `phase-document-authoring`; do not build a second code-intelligence layer.

### Open Questions

- Staging: do generated drafts land directly in `requirements/**/todo/` (marked `draft` + low-confidence) or in a separate `requirements/_generated/` review area until accepted?
- Granularity: one SD per GitNexus **cluster** (functional area) vs one per top-level module — which yields the most reviewable drafts?
- Does this warrant its own `feature_key` (`doc-generation`) split from `context-regression-engine`?

### Source Refs

- `SS-13` (target doc contract); `FORMAT-REFERENCE-*` (phase structure); GitNexus resources (clusters, processes, execution flows, wiki); `changeledger` (commit history + feature keys); CP-48 (downstream conformance).

## 1. Goal

Let a brownfield project with poor or no documentation reach a *reviewed, evidence-grounded* first draft of its SS/SD/CP set — SD/CP reverse-documented from real code and history, SS scaffolded for humans to fill — without fabricating intent, then hand off to CP-48 for conformance.

## 2. Input Documents

- system spec: `SS-13` (the target structure to generate into).
- design source: `FORMAT-REFERENCE-{SS,SD,CP,TASK,BUGFIX}.md`; GitNexus code-intelligence output (the evidence graph).
- downstream: `CP-48` (validates/normalizes generated drafts).
- prior art: `reqscaffold` (structure), `phase-document-authoring` skill (writing conforming docs), `changeledger`/`featurecatalog` (history + feature keys).

## 3. Implementation Strategy

- overall approach: an **evidence → draft → human-accept** pipeline. Collect structured evidence deterministically; feed only that evidence (not raw code dumps) to a per-phase drafting agent that must cite it; label confidence; require human accept; emit `draft` docs for CP-48 to conform.
- sequencing logic: P-1 (evidence) is prerequisite. P-2 (ingest loose docs) and P-3 (code→draft) both consume P-1 and can proceed in parallel. P-4 (review/accept) gates all writes.
- dependencies: GitNexus for the richest evidence (degrade if absent); `reqscaffold` guarantees the folder targets exist; CP-48 must exist to validate output (soft dependency — CP-49 can emit drafts before CP-48 ships).

## 4. Work Breakdown

- `P-1` **Evidence collection layer (deterministic).** Aggregate: GitNexus clusters/processes/execution-flows/relationships; `changeledger` commit history + feature keys; discovered loose docs (globs beyond `requirements/` — README, `docs/**`, `*.md`); repo file/module structure. Produce a structured **evidence corpus** (per functional area → symbols, flows, commits, doc excerpts). No LLM. Degrade to file/AST-lite scan when GitNexus is unavailable, marking those areas lower-confidence.
- `P-2` **Loose-doc ingestion.** Classify each loose-doc section by phase intent (WHAT/why → SS, HOW/architecture → SD, plan/work → CP/Task, bug → BugFix). Restructure into the phase template **preserving the original text as evidence**; leave template sections the source never covers empty (`TODO`), never invented. Dedupe against any existing `requirements/` docs. Output: draft docs + an "unclassifiable" residue list (honest, not force-fit).
- `P-3` **Code→as-built draft generation (evidence-cited, per phase).** Using the P-1 corpus via `phase-document-authoring`:
  - **SD (as-built):** one draft per functional cluster/flow — modules, responsibilities, dependencies, execution flow — each claim linked to a GitNexus symbol/flow. High confidence.
  - **CP/Task:** derive "what work exists" from `changeledger`/commit clusters per feature key.
  - **BugFix:** from revert-type / bug-labeled commits.
  - **SS:** **skeleton only** — headings + empty acceptance-criteria placeholders + any intent hints found in loose docs/commit messages, all flagged `TODO / low-confidence`. Never a fabricated spec.
  Every generated doc carries a provenance/confidence header.
- `P-4` **Human review & accept → hand off.** Present each draft with its evidence + confidence; human edits/accepts/rejects; accepted drafts are written as `draft`-status docs and passed to CP-48 for conformance + `Feature Keys` linkage. Nothing is auto-published.

## 5. Touched Areas

- files: new evidence-collection + generation modules (e.g. `apps/local-runner/internal/docgen/`); a generation flow definition + review surface; reuse `reqscaffold`, `changeledger`, `featurecatalog`, GitNexus client.
- modules: `docgen` (new), `changeledger`, `featurecatalog`, `reqscaffold`, GitNexus integration; `phase-document-authoring` skill.
- database: none (markdown output; evidence read from GitNexus + git).
- external systems: GitNexus (code-intelligence); the existing agent runtime for drafting.

## 6. Data or Migration Steps

- schema: none.
- data backfill: on a legacy repo, produces the first SS/SD/CP draft set; on this repo, a way to regression-check generation quality against known-good hand-written docs.
- config updates: none required.

## 7. Validation Plan

- tests to add: evidence-corpus builder unit tests (deterministic, GitNexus-present and -absent); classifier tests for loose-doc → phase mapping (incl. "unclassifiable" path); a provenance test asserting every generated non-`TODO` statement carries an evidence link; a "no-fabricated-SS" guard (generated SS acceptance-criteria are `TODO` unless evidence exists).
- manual checks: run on a small legacy repo → review SD accuracy against actual code; confirm SS came out as skeleton, not invented; confirm accepted drafts pass CP-48 conformance.
- failure cases: GitNexus stale/absent → degrade + flag, never fail hard; loose doc with no mappable content → listed as unclassifiable, not force-fit; human rejects a draft → nothing written.

## 8. Rollout and Fallback

- rollout order: P-1 (evidence corpus, read-only) → P-2 (ingestion drafts) / P-3 (code drafts) → P-4 (review/accept). Everything read-only or staged until human accept.
- fallback path: drafts live in a staging area / `draft` status; rejecting or ignoring them leaves the repo unchanged; git-revertible.
- monitoring: share of generated statements with evidence links (should be ~100% for non-`TODO`); human accept/reject ratio as a quality signal.

## 9. Risks

- `R-1` **Fabricated specs that look authoritative** (worst risk). Mitigation: evidence-or-`TODO` rule; SS skeleton-only; confidence labels; mandatory human accept; provenance test in CI.
- `R-2` GitNexus unavailable/stale → weak evidence. Mitigation: degrade to static/file scan, flag lower-confidence, never hard-fail.
- `R-3` Loose-doc misclassification into the wrong phase. Mitigation: human review; preserve original text; "unclassifiable" bucket instead of force-fit.
- `R-4` Reverse-doc treated as final truth. Mitigation: always `draft` + confidence header; CP-48 validation + human accept before authoritative; `Feature Keys` still human-confirmed (BUG-280 rule).
- `R-5` Scope creep into "understand product intent from code". Mitigation: hard line — SS intent is never generated, only scaffolded.

## 10. Definition of Done

- [ ] `P-1` Deterministic evidence corpus builds from GitNexus + git/changeledger + loose docs; degrades (flagged) without GitNexus; no LLM; no panic on a bare repo.
- [ ] `P-2` Loose docs are classified + restructured into phase drafts preserving original text; uncovered sections are `TODO`; unmappable content is listed, not force-fit.
- [ ] `P-3` SD/CP/Task/BugFix drafts are generated with per-statement provenance + confidence; **SS is skeleton + `TODO` only**; a test proves no non-`TODO` statement lacks evidence.
- [ ] `P-4` Every write is human-accepted; accepted drafts are `draft`-status and pass into CP-48 conformance; rejecting writes nothing.
- [ ] Demonstrated on a legacy repo: reviewed SD matches real code; SS came out as a fill-in skeleton, not a fabricated spec.

## 11. Out of Scope

- Fabricating SS business intent / acceptance criteria from code (hard-excluded by design).
- The conformance/normalization engine itself (that is [CP-48](./CP-48-Standardize-Doc.md); CP-49 only *produces* drafts for it).
- Changing the doc contract (`SS-13`).
- Building a new code-intelligence layer (reuse GitNexus).
