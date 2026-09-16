# SS-19: Engineering Harness Flow Family

## Metadata

- Document ID: `SS-19`
- Title: `Engineering Harness Flow Family (task-harness, bug-plan-harness, cp-harness, tournament-harness)`
- Phase: `system_spec`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Operator, Engineering Team`
- Created: `2026-09-15`
- Last Updated: `2026-09-15` (added tournament-harness CP-65, reproduce-first CP-64, living-knowledge CP-66)
- Parent Documents: [Product Vision](../01-Vision/Product-vision.md), [SP-02 Workflow-First](../04-System-Principle/SP-02-workflow-first.md), [SS-04 Workflow](./SS-04-Workflow.md), [SS-16 Agent Flow Engine](./SS-16-Agent-Flow-Engine.md)
- Child Documents: [SD-19 Agent Flow Engine](../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20 Flow Gate Rule Semantics](../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-58 Bug/Task/CP Harness](../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md), [CP-62 Zcode Harness Parity](../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [CP-64 Reproduce-First TDD Gate](../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-65 Multi-Candidate Tournament Harness](../07-Coding-Plan/todo/CP-65-Multi-Candidate-Tournament-Harness.md), [CP-66 Living Knowledge Base](../07-Coding-Plan/todo/CP-66-Living-Knowledge-Base-Context-Source.md)
- Related Documents: [SS-14 Code Context And Regression Safety](./SS-14-Code-Context-And-Regression-Safety.md), [SS-15 Agent Review Loop](./SS-15-Agent-Review-Loop-Until-Clean.md), [SS-18 Vibe Working Mode](./SS-18-Vibe-Working-Mode.md), [SP-06 Oracle Rule And Schema First Gate](../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- Replaces: `None`
- Tags: `harness, agent-flow, flow-definition, pipeline, tdd, review-loop, task-harness, tournament-harness, reproduce-first`
- Feature Keys: `agent-flow-engine, flowgate, zcode-parity, tournament-harness, reproduce-first-gate, living-knowledge-base`

## AI Quick View

### Summary

- Define the standard **Harness Flow Family**: the production-grade, multi-stage engineering pipelines (`task-harness`, `bug-plan-harness`, `cp-harness`, `tournament-harness`) that translate human intent into tested, verified, and audited code.
- Unlike ad-hoc prompt chains, each harness flow follows a deterministic, closed topology:
  $$\text{preflight} \to \text{freeze} \to \text{context} \to \text{tdd/reproduce} \to \text{coder} \to \text{validate} \to \text{synthesis} \to \text{audit}$$
- **Reproduce-First in Bug Flows (CP-64)**: For bugfix flows, the TDD stage mandates writing an executable test that compiles and FAILS before coder write access is unlocked (`r-reproduce`).
- **Tournament Harness & Escalation (CP-65)**: Provides `tournament-harness` for multi-candidate parallel rollouts across independent providers (Claude, Codex, Grok) with automated arbiter scoring. Functions also as an escalation fallback when sequential review loops or debates stall.
- **Living Knowledge Context (CP-66)**: Injects distilled execution flow summaries (`knowledge.flow`) into scout and planner profiles, avoiding whole-codebase exploratory reads.

### Current Ask

- Standardize the business contract for the Harness Flow Family as reusable FlowDefinitions within the SSOT specifications.

### Key Decisions

- `AC-1` **Closed Topology**: Every harness execution must pass through the mandatory verification and gate stages; steps cannot be skipped.
- `AC-2` **TDD Mandatory**: Implementation code is never generated without prior test signatures or failing reproduction tests.
- `AC-3` **Audit Trail & Handoff**: The terminal stage (`audit`) must produce both the commit preparation and the `sprint_handoff.v1` artifact to seed context for subsequent runs.
- `AC-6` **Reproduce-First Defect Gating**: In bugfix harness topologies, the tester stage must output an executable failing test (`r-reproduce`).
- `AC-7` **Multi-Candidate Arbitration**: Complex bugs and review stalls resolve via parallel rollout across isolated worktrees scored deterministically by the tournament arbiter.
- `AC-8` **Living Knowledge Injection**: Planning nodes consume `knowledge.flow` context distilled from the project's execution flows.

---

## 1. Goal

Provide an enterprise-ready, rigorous execution harness that prevents AI hallucinations, eliminates scope drift, and ensures that code changes are backed by automated tests, independent code reviews, and change-audit records before touching the Git history.

## 2. Problem

Ad-hoc AI coding (such as single-prompt code generation) frequently causes:
1. Blind implementation without verifying architecture compatibility.
2. Skipping unit test authoring or writing tests after code that merely mirror the code's flaws.
3. Prematurely declaring victory without running real build or test suites.
4. Unaudited, chaotic Git commit messages lacking traceability.
5. Unbounded sequential retries on difficult bugs causing cognitive lock-in and context dilution.

## 3. Scope

- In scope:
  - Standard harness flow topologies (`task-harness`, `bug-plan-harness`, `cp-harness`, `tournament-harness`).
  - Stage contracts and transitions (including `reproduce_test` for bug flows).
  - Acceptance node requirements (`validate`, `synthesis`, `tournament_arbiter`, `audit`).
  - Fallback escalation routing when review cap is exceeded.
- Out of scope:
  - Vibe mode non-tech auto-debate (covered in SS-18).

## 4. User Stories

- `US-1` As a Developer, I want to trigger `task-harness Task-123`, so that FlowPilot plans, writes tests, implements, reviews, and validates the task with minimal manual babysitting.
- `US-2` As an Engineering Lead, I want all tasks to be verified by a deterministic validation node before entering review, so that reviewers only review working code.
- `US-3` As an Architect, I want difficult bugs to run through a tournament across multiple AI providers, so that the solution with the highest test score and cleanest compiler diagnostics is chosen objectively.

## 5. Acceptance Criteria

- `AC-1` Stage sequencing must strictly honor: Preflight $\to$ Freeze Scope $\to$ Context Retrieval $\to$ TDD/Reproduce $\to$ Coder $\to$ Validate $\to$ Review Synthesis $\to$ Audit.
- `AC-2` The `validate` node must execute the real project test suite command (e.g., `go test`, `gradle test`, `npm test`) via local runner execution.
- `AC-3` If `validate` fails, the flow must loop back to `coder` with the exact error output (bounded retry $\le 3$).
- `AC-4` The `synthesis` node receives structured verdicts from independent reviewers and requires a clean verdict across all acceptance criteria.
- `AC-5` The final `audit` node drafts the change-audit note, updates knowledge base incrementally, and updates task frontmatter to `Status: done`.
- `AC-6` (Reproduce-First): For `bug-plan-harness`, the TDD stage must produce an executable test that compiles and asserts failure (`r-reproduce`), locking the test read-only during the coder turn.
- `AC-7` (Tournament Flow & Escalation): `tournament-harness` executes parallel candidates in isolated worktrees. When standard review loops hit `review_cap_exceeded`, the engine automatically transitions to tournament escalation.
- `AC-8` (Living Knowledge Flow): Planning nodes in all harness flows include `knowledge.flow` in their `contextProfile`.

## 6. Definition of Done

- All 4 harness topologies exist as tested FlowDefinitions in `flow-pack`.
- Automated test suites confirm node isolation, reproduce-first gating, tournament arbitration, and fail-closed termination.

