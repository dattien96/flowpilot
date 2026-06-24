---
name: phase-doc
description: Follow the SS-13 document contract when writing or updating requirements and specification docs.
version: 5
---

# phase-doc

Follow the SS-13 AI-Followable Document Contract when authoring any requirements or specification document.

## Required Document Structure (SS-13)

Every SS, SD, CP, Task, or BugFix document MUST contain:

### 1. Metadata Block (top of file)

```markdown
---
id: <DOC-ID>
title: <Human-readable title>
status: <draft | active | done | superseded>
version: <integer>
created: <ISO-8601 date>
updated: <ISO-8601 date>
owner: <name or role>
linked: [<DOC-ID>, ...]
---
```

### 2. AI Quick View

A short block immediately after the metadata:

```markdown
## AI Quick View
- **What**: <one sentence>
- **Why**: <one sentence>
- **Key constraint**: <one sentence>
```

### 3. Numbered Sections

All sections must be numbered (e.g., `## 1. Overview`, `## 2. Requirements`).
Sub-sections use decimal notation (`### 2.1 Detail`).

## Rules

1. Never write a requirements document without the metadata block.
2. Never write a requirements document without the AI Quick View.
3. Section numbers must be stable — inserting a section renumbers subsequent ones; update all cross-references.
4. When updating a document, increment `version` and update `updated`.
5. Superseded documents must set `status: superseded` and add a `superseded_by: <DOC-ID>` field.

## Rationale

SS-13 ensures every document is machine-parseable by the Context & Regression Engine (SD-17) without ambiguity. Inconsistent structure breaks automated feature history extraction.
