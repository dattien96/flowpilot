# Task-225: File Artifact Instance Output Structure

## Metadata

- Document ID: `Task-225`
- Title: `File Artifact Instance Output Structure`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex / owner`
- Created: `2026-07-12`
- Last Updated: `2026-07-12`
- Parent Documents: [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-224: Flow Prompt Scoping And Coder Output Why Template](./Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md)
- Child Documents: `None`
- Related Documents: [Task-223: File Artifact Output Contract And Review Input Chain](./Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [BUG-276: File Artifact Input Path-Only](../../09-BugFix/done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md), [CA-287](../../../change-audit/CA-287-file-artifact-output-contract-and-review-input-chain.md), [CA-289](../../../change-audit/CA-289-task-224-flow-prompt-scoping-and-coder-why-template.md)
- Replaces: prior draft title “File Artifact Output Heading Gate” (same `Task-225` id; global What/Why/Baseline enforcement discarded). Also drops dual `format`+`structure` — **only `structure`**.
- Tags: `file-artifact, artifact-instance, output-structure, flow-gate, write-contract, agent-flow-engine`

## AI Quick View

### Summary

- `file_artifact.v1` is a **generic path I/O type**. Different instances (coder summary, test report, plan outline, raw notes) must **not** all be forced into the coding What/Why/Baseline memo shape.
- Extend instance `config_json` with **optional `structure` only** (list of section titles). No separate `format` / preset field — coder memo is just an instance seeded with `sections: ["What","Why","Baseline"]`. Aligns with SD-23 `D-3`.
- **Existence gate** (Task-223) still applies to every **required OUTPUT** path.
- **Prompt template + structural section gate** apply **only** when the bound instance declares `structure`; matching is **flexible** (ATX level, case, whitespace), not NLP quality scoring.
- Fixes today’s global What/Why/Baseline inject in `appendRequiredOutputArtifactPrompt` (Task-224 residual over-reach).

### Current Ask

- Spec + implement per-instance `structure` for required `file_artifact` OUTPUT: schema, prompt assembly, optional section gate, seed coder instance(s) with explicit sections. No UI picker required in v1 if config can be set via instance JSON / seed.

### Key Decisions

- `T-1` **Always-on:** required `file_artifact` **OUTPUT** → write contract + **existence** only (`r-artifact-output`). Never invent structure from step role (coding vs testing vs planning).
- `T-2` **Only one config knob: `structure`.** Optional on `config_json`. Missing / empty = **no** section template and **no** section gate. **Do not add `format`.**
- `T-3` **v1 shape:**
  ```json
  {
    "paths": ["docs/coder-summary.md"],
    "structure": {
      "kind": "markdown_sections",
      "sections": ["What", "Why", "Baseline"]
    }
  }
  ```
  - `structure` optional; when present, `kind` must be `"markdown_sections"` and `sections` non-empty string array.
  - Paths-only instance: `{ "paths": ["docs/test-report.md"] }`.
- `T-4` **Flexible section match** (when structure present and file exists): ATX `#`–`######`; case-insensitive title; trim whitespace; light trailing punctuation (`:`, `.`). No synonym NLP (`Rationale` ≠ `Why`).
- `T-5` **Gate:** sibling rule `r-artifact-output-structure` after existence; only structured required OUTPUTs; aggregate missing sections; action `reprompt` (warn softens).
- `T-6` **Prompt:** always list required OUTPUT paths; append section template **only** for bindings with `structure`. Remove global hard-coded What/Why/Baseline. Optional Why guidance bullets only when those section titles are in the instance’s `sections` list (e.g. includes `"Why"`), not via a separate format type.
- `T-7` **Seed:** coder deliverable instance(s) on the built-in artifact flow get explicit `structure.sections = ["What","Why","Baseline"]`. Other instances stay paths-only unless configured.
- `T-8` **INPUT / BUG-276 unchanged.**
- `T-9` No new tables; extend `file_artifact.v1` `config_schema` via migration upsert (optional `structure` only).

### Constraints

- CP-45 remains `done`.
- Prefer `flowgate` + `runChildArtifactOutputGate`; no parallel post-turn path.
- v1 may be runner + schema + seed only; Artifacts UI for editing `structure` is follow-up.
- Paths-only `{ "paths": [...] }` must keep working.

### Open Questions

- `Q-1` **RESOLVED:** no synonym aliases in v1.
- `Q-2` **RESOLVED:** aggregate missing sections across structured paths in one violation.
- `Q-3` **RESOLVED:** no `format` field; presets are seed data only.
- `Q-4` Per-path structure inside multi-path instance? **Out of v1** — one `structure` for all paths on that instance.

### Source Refs

- SD-23 `D-3`, `D-8`, `D-11`; Task-223; Task-224; BUG-276.
- Code: `appendRequiredOutputArtifactPrompt`, `requiredFileArtifactOutputPaths`, `MissingRequiredFileArtifactOutputs`, `runChildArtifactOutputGate`, migration `20260709093000_add_file_artifact_type.sql`.

## 1. Goal

Make `file_artifact` OUTPUT **instance-correct**: required paths must exist; only instances with `structure` get section template + gate—so coding memos, test reports, and plan docs share the type without sharing the section list.

## 2. Parent Links

- coding plan: [CP-45](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md) (done; residual)
- tech design: [SD-23](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md) `D-3`, `D-8`, `D-11`
- system spec: [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md); [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) `US-10` / `AC-17`
- specific upstream ids: [Task-223](./Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [Task-224](./Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md), [BUG-276](../../09-BugFix/done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md)

## 3. Trigger

1. Task-224 injected **global** What/Why/Baseline for every required file OUTPUT.
2. A global heading gate would force testing/planning files into coding memo shape.
3. Owner: structure on the **instance**; **one field only** (`structure`), no parallel `format`.

## 4. Exact Change

### 4.1 Config contract (`file_artifact.v1`)

- `T-1` Keep required `paths: string[]`.
- `T-2` Add optional `structure` to `config_schema`:
  - `{ "kind": "markdown_sections", "sections": string[] }` with non-empty `sections` when `structure` is present.
  - **No `format` property** in schema or runner.
- `T-3` Migration upsert `artifact_types.config_schema` (backward compatible).

### 4.2 Prompt assembly

- `T-4` Refactor `appendRequiredOutputArtifactPrompt`:
  - Always: required paths + write-contract / existence wording.
  - If binding has `structure.sections`: list those section titles as required headings for those paths; if `"Why"` is among them, may include Task-224-style Why bullets under that heading guidance.
  - Paths-only: **no** What/Why/Baseline block.
- `T-5` Read `structure` from binding/`ConfigJSON`, not step display name. Iterate **bindings**, not flat path-only lists that drop config.

### 4.3 Gate

- `T-6` Keep `r-artifact-output` = existence only.
- `T-7` After existence: for required existing paths with structure, flex-match section titles; aggregate gaps; reprompt.
- `T-8` `runChildArtifactOutputGate` evaluates existence + structure family.
- `T-9` Missing file → existence only.

### 4.4 Seeds / built-ins

- `T-10` Coder deliverable instance(s): set  
  `structure: { "kind": "markdown_sections", "sections": ["What", "Why", "Baseline"] }`.
- `T-11` Do not attach that structure to unrelated file instances.

### 4.5 Tests & docs

- `T-12` Tests: paths-only; parse structure; prompt omit/include; gate skip/pass/fail; existence-first; INPUT path-only.
- `T-13` SD-23 / CP-45 residual: per-instance `structure` only (no `format`).

## 5. Touched Areas

- files: `artifact_type_registry.go` (+ tests), `flowgate/*`, `gate_hook.go`, migration for `config_schema`, coder instance seed, this task + residual cross-links
- modules: artifact type registry, flowgate, child gate hook
- routes: none
- tables: no new tables; `artifact_types.config_schema` + optional `artifact_instances.config_json` seed

## 6. Acceptance Check

- [ ] `AC-1` Required OUTPUT, **no** `structure`: existence only; prompt paths only; no global What/Why; no structure gate.
- [ ] `AC-2` With `structure`, flex headings present (e.g. `### why`, `## What `): pass structure gate.
- [ ] `AC-3` With `structure`, missing section: reprompt path + missing title(s).
- [ ] `AC-4` Missing file: existence only.
- [ ] `AC-5` Seeded coder instance has explicit What/Why/Baseline `structure` and gets template + gate.
- [ ] `AC-6` INPUT path-only unchanged (BUG-276).
- [ ] `AC-7` Paths-only configs valid under new schema; **no** `format` field required or documented.
- [ ] `AC-8` Targeted `go test` for `flowgate` + `runner` pass.

## 7. Out of Scope

- Separate `format` / preset enum field.
- NLP body scoring; synonym aliases; per-path structure; new artifact types; full Artifacts UI; INPUT / context_artifact changes; reopening CP-45.

## 8. Completion Notes

- result: `done` 2026-07-12 — CA-290: per-instance `structure` prompt + `r-artifact-output-structure` gate; paths-only existence only; flex ATX match; migration schema + coder-summary heuristic; same-path section merge.
- follow-ups: Artifacts UI to edit `structure.sections`; more seeded examples; optional aliases.
- upstream docs: SD-23 / CP-45 / Task-224 cross-links; walkthrough.md.
