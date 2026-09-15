# SS-22: Runtime Intelligence And Drift Correction

## Metadata

- Document ID: `SS-22`
- Title: `Runtime Intelligence, Token Budgeting, and Drift Correction Ladder`
- Phase: `system_spec`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Operator, Engineering Team`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [Product Vision](../01-Vision/Product-vision.md), [SP-04 Safe Gate](../04-System-Principle/SP-04-safe-gate.md), [SP-06 Oracle Rule & Schema-First](../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md)
- Child Documents: [SD-10 Memory And Prompt Architecture](../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md), [SD-20 Flow Gate Rule Semantics](../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-23 Auto-Learn-To-Skill](../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)
- Related Documents: [SS-09 Context Management](./SS-09-Artifact-Memory-Context-Retrieval.md), [SS-14 Code Context And Regression Safety](./SS-14-Code-Context-And-Regression-Safety.md)
- Replaces: `None`
- Tags: `runtime-intelligence, budget-packer, drift-detector, wrong-way, correction-ladder, auto-skill`
- Feature Keys: `runtime-intelligence, context-budget, drift-detector, auto-skill`

## AI Quick View

### Summary

- Define the operational specification for FlowPilot's **Runtime Intelligence Engine**:
  1. **Budget Packer**: Section-based token packing with fixed limits, prioritizing compact summaries over raw source code to prevent context overflow.
  2. **Drift & Wrong-Way Detector**: Real-time behavioral scoring (`drift_score`) tracking circular reasoning, repetitive test failures ($\ge 2$), zero delta outputs, and out-of-scope modifications (`r-scope`).
  3. **Correction Ladder & Drift Pause**: Multi-stage mitigation (Advisory prompt $\to$ Context contraction $\to$ Execution Pause at $\ge 80$ points requiring human intervention).
  4. **Mistake-to-Skill Promotion**: Automated distillation of recurring failure modes into persistent Skill guidelines vetted by the operator.

### Key Decisions

- `AC-1` **Budget Ceiling**: The prompt packer strictly enforces section budgets; if token count exceeds limits, raw history is pruned in favor of `artifact_memories` summaries.
- `AC-2` **Deterministic Drift Scoring**: Initial drift scoring relies on zero-token deterministic telemetry (test exit codes, diff paths, repetition count) rather than secondary LLM calls.
- `AC-3` **Drift Pause at 80+**: If `drift_score \ge 80`, the runner must halt automated turns and display a structured `drift_pause` card to the user on Desktop and TUI.

---

## 1. Goal

Safeguard long-running agent workflows from cognitive degradation, context exhaustion, repetitive apology loops, and silent deviations from the assigned task scope.

## 2. Problem

In prolonged coding sessions:
1. LLMs fill their context window with raw transcripts and irrelevant file dumps, leading to degraded reasoning ("lost in the middle").
2. When stuck on a bug, agents often enter circular loops, apologizing and trying variations of the same broken approach while burning tokens.
3. Errors made in one session are repeated in subsequent sessions because there is no mechanism to extract lessons into long-term habits.

## 3. Scope

- In scope:
  - Budget Packer context packaging.
  - Telemetry aggregation and drift scoring.
  - Correction ladder and Drift Pause (`drift_pause.go`).
  - Skill candidate extraction (`skilllearn`).
- Out of scope:
  - Fine-tuning foundational models.

## 4. Acceptance Criteria

- `AC-1` The prompt compiler must pack context strictly according to the active `ContextProfile` and token ceilings.
- `AC-2` Any turn that touches files outside the declared task scope immediately contributes $\ge 30$ points to the run's `drift_score`.
- `AC-3` When `drift_score \ge 80`, automated turns are locked, and the operator is presented with 3 options: Reset context to baseline, Override and continue, or Abort run.
- `AC-4` Resolved recurring mistakes can be exported as candidate skills into `.agents/skills/` upon human operator approval.
