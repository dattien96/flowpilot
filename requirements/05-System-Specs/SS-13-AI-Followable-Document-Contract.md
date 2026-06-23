# SS-13: AI-Followable Document Contract

## 1. Goal

Define one consistent document contract for the project phases below so both humans and AI agents can follow work with low ambiguity:

- `05-System-Specs`
- `06-System-Tech-Design`
- `07-Coding-Plan`
- `08-Task`
- `09-BugFix`

This contract is required for enterprise use where project history must stay auditable, reviewable, and retrievable across long-running AI work.

## 2. Problem

Today the project already separates business specs, tech design, coding plan, tasks, and bug fixes.
That phase split is correct, but the file structure inside each phase is still too inconsistent.

This creates five practical failures:

1. AI cannot reliably identify the source of truth for the current topic.
2. Follow-up tasks and bug fixes can drift away from the original requirement and design.
3. The same feature is described with different section shapes in different files.
4. Context retrieval becomes expensive because the system must read whole files instead of stable sections.
5. Enterprise review is weaker because ownership, approval, and document lineage are not always explicit.

## 3. Scope

This spec defines:

- the shared structure every phase document must follow
- the role of each phase in the document chain
- traceability rules between phases
- AI-oriented summary rules
- escalation rules for when lower-level work must update upstream documents
- reference formats for each phase

This spec does not define:

- runtime vector retrieval implementation
- prompt packing internals
- workflow engine execution logic

Those remain covered by `SS-09`, `SD-10`, and related runtime plans.

## 4. Core Model

One topic should move through one traceable chain:

```text
System Spec -> Tech Design -> Coding Plan -> Task or BugFix -> Execution Result
```

The responsibilities are:

- `System Spec`: business truth and acceptance criteria
- `Tech Design`: technical response to the spec
- `Coding Plan`: implementation breakdown and delivery order
- `Task`: a scoped execution delta for planned work
- `BugFix`: a scoped execution delta for incorrect behavior or regression

Important rules:

- `Task` and `BugFix` are not replacements for `System Spec`, `Tech Design`, or `Coding Plan`
- `Task` and `BugFix` may narrow scope, but they must not silently redefine upstream intent
- downstream documents may extend details, but must not contradict approved upstream documents without updating them

## 5. Shared Contract For Every Phase File

### 5.1 Required metadata block

Every file in these five phases must start with a fixed metadata block near the top.

Minimum fields:

- `Document ID`
- `Title`
- `Phase`
- `Status`
- `Owner`
- `Reviewers`
- `Created`
- `Last Updated`
- `Parent Documents`
- `Child Documents`
- `Related Documents`
- `Replaces`
- `Tags`

Recommended status values:

- `draft`
- `reviewing`
- `approved`
- `in_progress`
- `done`
- `superseded`
- `cancelled`

### 5.2 Required AI Quick View block

Every file must contain a short AI-oriented summary block with the same subsection names.

Required subsections:

- `Summary`
- `Current Ask`
- `Key Decisions`
- `Constraints`
- `Open Questions`
- `Source Refs`

Rules:

- keep this block compact
- prefer bullets over long prose
- do not restate the whole document
- update it whenever the document meaning changes

This block is the first thing an AI should read before loading the rest of the file.

### 5.3 Section stability rules

All phase files must follow stable section names and stable section order.

Rules:

- use numbered top-level headings
- do not merge unrelated concerns into one section
- use explicit IDs where traceability matters, such as `AC-1`, `D-1`, `P-1`
- avoid free-form notes in the middle of canonical sections
- move unfinished thoughts into `Open Questions` or `Notes`

### 5.4 One topic per file

A phase file should represent one main topic only.

Allowed:

- one feature
- one major design decision set
- one implementation plan
- one task slice
- one bug or regression

Not allowed:

- mixing many unrelated features in one file
- using one task file to track multiple unrelated fixes
- redefining the whole architecture from a bug note

## 6. Required Structure By Phase

### 6.1 System Spec

Purpose:

- define the user and business truth

Minimum sections:

1. `Goal`
2. `Problem`
3. `Scope`
4. `Non-Goals`
5. `User Stories` or `Primary Use Cases`
6. `Acceptance Criteria`
7. `Business Rules`
8. `Edge Cases`
9. `Dependencies`
10. `Open Questions`
11. `Definition of Done`

Output quality rule:

- acceptance criteria must be explicit and referenceable

### 6.2 Tech Design

Purpose:

- explain how the approved spec will be implemented technically

Minimum sections:

1. `Goal`
2. `Input Documents`
3. `Architecture Decision`
4. `Component Impact`
5. `Data Model`
6. `Interfaces and Contracts`
7. `Execution Flow`
8. `Failure and Edge Handling`
9. `Security and Operational Concerns`
10. `Risks and Trade-Offs`
11. `Validation Strategy`
12. `Traceability to Spec`

Output quality rule:

- every major design decision should trace back to a spec requirement or constraint

### 6.3 Coding Plan

Purpose:

- break the design into implementation slices that can be executed and verified

Minimum sections:

1. `Goal`
2. `Input Documents`
3. `Implementation Strategy`
4. `Work Breakdown`
5. `Touched Areas`
6. `Data or Migration Steps`
7. `Validation Plan`
8. `Rollout and Fallback`
9. `Risks`
10. `Definition of Done`

Output quality rule:

- work breakdown should be actionable and ordered

### 6.4 Task

Purpose:

- define one scoped execution slice under an existing plan

Minimum sections:

1. `Goal`
2. `Parent Links`
3. `Trigger`
4. `Exact Change`
5. `Touched Areas`
6. `Acceptance Check`
7. `Out of Scope`
8. `Completion Notes`

Output quality rule:

- a task must narrow work, not replace the upstream plan

### 6.5 BugFix

Purpose:

- define one incorrect behavior, its impact, and the correction path

Minimum sections:

1. `Issue Summary`
2. `Parent Links`
3. `Environment and Reproduction`
4. `Expected vs Actual`
5. `Impact`
6. `Root Cause`
7. `Fix Strategy`
8. `Validation`
9. `Regression Guard`
10. `Follow-Up Document Updates`

Output quality rule:

- a bug file must separate symptom from confirmed root cause

## 7. Traceability Rules

### 7.1 Required upstream links

Every downstream file must link to its parent documents.

Minimum traceability:

- `Tech Design` -> one or more `System Spec` files
- `Coding Plan` -> one or more `Tech Design` files and upstream `System Spec` files
- `Task` -> one `Coding Plan` and any impacted `Tech Design` or `System Spec`
- `BugFix` -> the impacted `System Spec`, `Tech Design`, or `Coding Plan` when known

### 7.2 Referenceable IDs

Important items should use stable IDs:

- acceptance criteria: `AC-1`, `AC-2`
- design decisions: `D-1`, `D-2`
- plan slices: `P-1`, `P-2`
- task items: `T-1`, `T-2`
- bug validations: `V-1`, `V-2`

This allows lower-level files to cite exact upstream clauses instead of whole documents.

### 7.3 Upstream update triggers

Lower-level work must update upstream documents when:

- a bug changes expected business behavior
- a task changes architecture or interface contracts
- a coding detail becomes a reusable project rule
- the original acceptance criteria are no longer correct
- a technical constraint invalidates the approved design

Do not leave these changes only in `Task` or `BugFix`.

## 8. AI Context Rules

### 8.1 Read order

When an AI agent loads context for one topic, the preferred order is:

1. metadata block
2. AI Quick View block
3. exact referenced section IDs
4. parent document summaries
5. only then deeper document sections if still needed

### 8.2 Retrieval unit

The preferred retrieval unit is:

- one section
- one acceptance criterion
- one design decision
- one implementation slice

Not:

- the entire folder
- all prior task notes
- long free-form history by default

### 8.3 Authority order

If documents disagree, authority should default to:

1. approved `System Spec`
2. approved `Tech Design`
3. approved `Coding Plan`
4. latest linked `Task`
5. latest linked `BugFix`

`Task` and `BugFix` may only override upstream meaning after the upstream documents are updated.

### 8.4 AI output citation rule

Whenever an AI produces a derived artifact, it should cite the exact source documents and section IDs it relied on.

Minimum expected references:

- document IDs
- section names or requirement IDs

## 9. Approval and Lifecycle Rules

For enterprise usage, each phase must expose:

- explicit owner
- explicit reviewers
- explicit status
- visible superseded history

Recommended lifecycle:

1. `draft`
2. `reviewing`
3. `approved`
4. `in_progress` or `done` where execution applies
5. `superseded` when replaced

Rules:

- downstream execution should prefer approved upstream documents
- superseded documents remain readable for audit
- replacing a document must update `Replaces` and `Child Documents` links where relevant

## 10. File Naming Rules

Use the existing folder conventions and keep names stable:

- `SS-xx-Topic.md`
- `SD-xx-Topic.md`
- `CP-xx-Topic.md`
- `Task-xxx-Topic.md`
- `BUG-xxx-Topic.md`

Reference-format files may use explicit names such as:

- `FORMAT-REFERENCE-SS.md`
- `FORMAT-REFERENCE-SD.md`

## 11. Reference Files

This spec is paired with reference-format files in:

- `05-System-Specs`
- `06-System-Tech-Design`
- `07-Coding-Plan`
- `08-Task`
- `09-BugFix`

These files are not business truth.
They are writing guides that show the exact section order and minimum expected content.

## 12. Definition of Done

- each phase has one stable reference format file
- all new files in phases `05` to `09` can be written using the same metadata and AI Quick View structure
- downstream files can trace back to upstream documents without reading whole folders
- `Task` and `BugFix` files are treated as deltas, not silent replacements for upstream documents
- AI agents can load the top of a file and determine scope, authority, and next required reads quickly

## 13. Machine-Readable Change-Ledger Block (change-audit notes)

`change-audit/` notes are not one of the five governed phases, but they are the primary record of *what changed* and are consumed by the Context & Regression Engine (`SD-17`, Plane C). To let that engine fill the feature change ledger deterministically instead of re-reading diffs, every `change-audit/CA-###` note should carry one fenced, machine-readable block in addition to its human prose.

### 13.1 Block format

A single fenced `yaml` block delimited by stable markers:

```yaml
# ---8<--- flowpilot:change-ledger
feature_key: <stable feature slug, e.g. notifications>
source_doc_id: <Task-xxx | BUG-xxx | CP-xx | commit ref>
entries:
  - symbol: <qualified symbol or file path>
    layer: data | domain | ui
    change: added | modified | deleted
    class: behavioral | cosmetic | structural | interface
# --->8---
```

### 13.2 Rules

- The block is additive; it never replaces the human `## Scope` / `## Completed` / `## Verification` sections required by the audit-logging skill.
- `feature_key` should be stable across notes that touch the same feature, so history is groupable. Stability is maintained by **one registry file — `change-audit/FEATURE-KEYS.md`** — the source of truth for canonical kebab-case keys (`key — description`, one per line). When writing a note, pick an existing key from the registry; if none fits, append a new line. The Feature Catalog (`SD-17 §3.3`) seeds from this registry, so notes, ledger, and catalog share one vocabulary. As a committed repo file it travels with the repo via git — no separate sync is needed (it is not part of the `.flowpilot/` Drive sync).
- Parsing is deterministic; model extraction is used only to fill missing fields or normalize legacy notes that predate this block.
- One block per note. If a note legitimately spans features, list all entries under the dominant `feature_key` and note the structural exception in prose.

### 13.3 Producer and consumer

- Producer: the `.claude/skills/audit-logging` skill emits this block when writing a `CA-###` note.
- Consumer: `SD-17 §6.3` parses it into `feature_change_manifests` / `feature_change_entries`.
