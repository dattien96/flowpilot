# Task-225: File Artifact Output Heading Gate

## Metadata

- Document ID: `Task-225`
- Title: `File Artifact Output Heading Gate`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `Codex / owner`
- Created: `2026-07-12`
- Last Updated: `2026-07-12`
- Parent Documents: [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [Task-224: Flow Prompt Scoping And Coder Output Why Template](../done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md)
- Child Documents: `None`
- Related Documents: [Task-223: File Artifact Output Contract And Review Input Chain](../done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [BUG-276: File Artifact Input Path-Only](../../09-BugFix/done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md), [CA-289](../../../change-audit/CA-289-task-224-flow-prompt-scoping-and-coder-why-template.md)
- Replaces: `None`
- Tags: `file-artifact, flow-gate, write-contract, heading-gate, phase-2, agent-flow-engine`

## AI Quick View

### Summary

- Phase-2 polish on top of Task-223 / Task-224: after a required `file_artifact.v1` **OUTPUT** path exists on disk, optionally **enforce** that the written markdown contains the required headings **`## What`**, **`## Why`**, and **`## Baseline`**.
- Today (v1): prompt **guidance only** — missing headings do not fail the turn if the file exists (`r-artifact-output` checks existence only).
- Goal: make deliverables reliably reviewable without re-injecting full ledger history into review prompts (Task-224 deliverable-centric model).

### Current Ask

- Implement a cheap, deterministic heading check after required file OUTPUT existence passes; reprompt (or warn) when headings are missing. Do **not** NLP-score Why quality in v1 of this task.

### Key Decisions

- `T-1` Trigger only for **required** `file_artifact` **OUTPUT** paths that already exist (after Task-223 existence gate).
- `T-2` Required headings (case-sensitive markdown ATX): `## What`, `## Why`, `## Baseline` (match Task-224 template guidance).
- `T-3` Default action: **reprompt** once/twice via existing flow-gate family (extend `r-artifact-output` or sibling rule `r-artifact-output-headings`); max attempts align with current gate reprompt cap.
- `T-4` Check is **structural only** (heading presence). Do not validate that Why lists closed decisions correctly.
- `T-5` Gate mode `warn` may downgrade reprompt to warn (same as other soft rules).
- `T-6` Does **not** reintroduce full file-body paste into prompts (BUG-276 remains law for INPUT).

### Constraints

- Depends on Task-223 existence gate and Task-224 template text already shipping.
- Must not block CP-45 closeout (this task is optional polish).
- Prefer extending `flowgate` / `runChildArtifactOutputGate` over a parallel enforcement path.
- No new Supabase tables.

### Open Questions

- `Q-1` Accept alternate heading spellings (`## why`, `### Why`)? Default: **ATX `## What` / `## Why` / `## Baseline` only** (case-sensitive) for determinism.
- `Q-2` Apply to all required OUTPUT files in one turn or fail-fast on first missing heading set? Default: **collect all missing headings across all required paths** in one reprompt message.

### Source Refs

- Task-224 `T-4` Phase-2 note; Task-223 `r-artifact-output`; SD-23 `D-8` OUTPUT write contract.
- Code today: `appendRequiredOutputArtifactPrompt`, `MissingRequiredFileArtifactOutputs`, `runChildArtifactOutputGate`.

## 1. Goal

When a coder (or any step) must write a required `file_artifact` path, ensure the file not only exists but includes the structural What/Why/Baseline sections so review can stay path-first without full history inject.

## 2. Parent Links

- coding plan: [CP-45](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md) (residual Phase-2; CP itself is `done`)
- tech design: [SD-23](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md) `D-8` (OUTPUT write + optional heading gate), `D-11` (prompt-assembly helpers)
- system spec: [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) (doc contract); [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) `US-10` / `AC-17` (typed artifacts as flow I/O)
- specific upstream ids: [Task-223](../done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [Task-224](../done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md) `T-4`/`Q-4`, [BUG-276](../../09-BugFix/done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md)

## 3. Trigger

Task-224 deliberately deferred heading enforcement so CP-45 could close with prompt guidance only. Product may still want a hard gate if live agents skip `## Why` / `## Baseline` and review quality suffers.

## 4. Exact Change

- `T-1` After existence check for required OUTPUT paths, read each file (workspace-safe) and require presence of the three headings.
- `T-2` On failure, emit flow-gate violation with explicit missing headings + path list in `RepromptPrompt` remediation.
- `T-3` Unit tests: present headings pass; missing Why fails; outside-workspace still fails existence first.
- `T-4` Docs: Task-224 completion note that Phase-2 is this task; SD-23 already points here for heading gate.

## 5. Touched Areas

- files (expected):
  - `apps/local-runner/internal/flowgate/*` and/or `apps/local-runner/internal/runner/gate_hook.go`
  - possibly `artifact_type_registry.go` shared path helpers
  - tests under `flowgate` / `runner`
- modules: flow gate, child artifact output gate
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] `AC-1` Required file exists with all three headings → no heading violation.
- [ ] `AC-2` Required file exists missing `## Why` → reprompt names path + missing heading(s).
- [ ] `AC-3` Required file missing entirely → still handled by existence gate (not heading check).
- [ ] `AC-4` INPUT path-only inject unchanged (BUG-276).
- [ ] `AC-5` Template guidance text remains; gate is additive.

## 7. Out of Scope

- NLP quality scoring of Why content.
- Enforcing What/Why on non-file artifacts.
- Changing INPUT path-only semantics.
- Full document management / versioning.

## 8. Completion Notes

- result: `draft` — authored 2026-07-12 after CP-45 closeout; not started.
- follow-ups: implement when owner prioritizes Phase-2 heading enforcement; no code until approved.
- upstream docs updated: CP-45 §13.3 residual + SD-23 `D-8`/`D-11` note Task-225; Task-224 `T-4`/`Q-4` points here.
