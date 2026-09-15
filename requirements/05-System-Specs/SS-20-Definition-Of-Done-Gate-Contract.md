# SS-20: Definition-Of-Done Gate Contract

## Metadata

- Document ID: `SS-20`
- Title: `Definition-Of-Done Gate Contract (r-dod-present, r-dod-complete)`
- Phase: `system_spec`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Operator, Engineering Team`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [Product Vision](../01-Vision/Product-vision.md), [SP-04 Safe Gate](../04-System-Principle/SP-04-safe-gate.md), [SP-06 Oracle Rule & Schema-First](../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [SS-13 AI-Followable Document Contract](./SS-13-AI-Followable-Document-Contract.md)
- Child Documents: [SD-20 Flow Gate Rule Semantics](../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-47 DOD Gate](../07-Coding-Plan/done/CP-47-DOD-Gate.md)
- Related Documents: [SS-14 Code Context And Regression Safety](./SS-14-Code-Context-And-Regression-Safety.md), [SS-19 Engineering Harness Flow Family](./SS-19-Engineering-Harness-Flow-Family.md)
- Replaces: `None`
- Tags: `definition-of-done, dod, flowgate, rules, r-dod, gate-contract`
- Feature Keys: `context-regression-engine, flowgate`

## AI Quick View

### Summary

- Define the business and operational contract for the **Definition-of-Done (DOD) Gate family** (`r-dod`).
- Ensure every engineering artifact (`Task-*`, `BUG-*`) has explicit, verifiable criteria rather than vague prose statements.
- **Two distinct lifecycle enforcement points**:
  - `r-dod-present`: Evaluated whenever a task or bugfix document is authored or modified. Must contain a `## Definition of Done` section with at least one binary checkbox (`- [ ]`). Violations trigger an immediate `reprompt`.
  - `r-dod-complete`: Evaluated when an artifact attempts to transition to `Status: done` or is moved into a `done/` directory. All checkboxes must be checked (`[x]` or `[X]`), OR each open item must be explicitly justified in a structured `or-explained` field. Unjustified open items trigger an unyielding `block`.
- Completely deterministic, 0-token offline parsing in the Go runner.

### Key Decisions

- `AC-1` **No Prose Done**: Completion cannot be declared by natural language assertions alone; only checkbox states or structured explanations satisfy the gate.
- `AC-2` **Zero Token Cost**: DOD parsing is handled natively by the Go runner using markdown AST/regex, saving LLM calls.
- `AC-3` **Hotfix Bypass**: If a hotfix only updates `change-audit/` without modifying or creating a `BUG-*` file, the DOD gate does not block execution.

---

## 1. Goal

Eliminate premature task completion and ensure that every item specified in a Task or BugFix Definition of Done is either verified as done or formally waived with clear rationale before code reaches production branches.

## 2. Problem

AI agents frequently suffer from "completion bias": they declare a task complete in their final summary despite having skipped several secondary requirements, edge-case unit tests, or documentation updates specified in the plan.

## 3. Scope

- In scope:
  - All `Task-*.md` and `BUG-*.md` documents in `requirements/08-Task` and `requirements/09-BugFix`.
  - Evaluation of `r-dod-present` and `r-dod-complete` in the runner's `flowgate` module.
- Out of scope:
  - Free-form discussion or scratch notes.

## 4. Acceptance Criteria

- `AC-1` (`r-dod-present`): Any turn that writes or updates a `Task-*` or `BUG-*` file without a `## Definition of Done` section containing $\ge 1$ checkbox item must be rejected with a reprompt.
- `AC-2` (`r-dod-complete`): Any turn that attempts to set `Status: done` or move a file to `done/` with unchecked items (`- [ ]`) and no structured `or-explained` rationale must be blocked.
- `AC-3` (`or-explained` verification): An explanation must provide legitimate justification (e.g., "Deferred to Task-XYZ because of external dependency"), which surfaces on the user decision card.
