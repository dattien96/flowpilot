# SP-06: The Oracle Rule & Schema-First Gate Enforcement

## Metadata

- Document ID: `SP-06`
- Title: `The Oracle Rule & Schema-First Gate Enforcement`
- Phase: `system_principle`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Operator, Engineering Team`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [SP-04 Safe Gate](./SP-04-safe-gate.md), [SP-05 Kill-Review](./SP-05-Kill-Review-Closed-Claim-Contract.md)
- Child Documents: [SS-14 Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20 Flow Gate Rule Semantics](../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-62 Zcode Harness Parity](../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- Related Documents: [SS-08 Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md), [CP-35 Context And Regression Engine](../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md)
- Replaces: `None`
- Tags: `oracle-rule, regression, schema-first, flow-gate, test-protection, non-determinism`
- Feature Keys: `context-regression-engine, flowgate, zcode-parity`

## AI Quick View

### Summary

- **The Oracle Rule**: Pre-existing unit and integration tests are an absolute ground truth (Oracle). A failing test must NEVER be resolved by weakening the test or modifying test expectations to force a green result. Any failure in pre-existing tests is a hard regression that triggers an immediate, un-downgradable block.
- **Schema-First 4-Tier Enforcement (T0-T3)**: The execution engine must never parse unstructured free-form prose from LLMs to make routing or state decisions. All critical decisions, reviews, and escalations must pass through typed schemas and infrastructure-level guards.
- **Tiers of Control**:
  - `T0`: Deterministic code (0-token calculation in Go).
  - `T1`: Transport-level tool-call schemas.
  - `T2`: Runner validation with at most one specific reprompt.
  - `T3`: Fail-closed escalation/parking.

---

## 1. The Oracle Rule (Regression Ground Truth)

### 1.1 The Fundamental Problem
When an LLM coding assistant breaks an existing behavior, its primary instinct is often to alter the test assert statements or delete the failing test case to make `test` command exit with code 0. This silently converts a caught regression into shipped broken code.

### 1.2 Absolute Axioms
1. **Pre-existing tests are immutable ground truth**: Tests that passed before the turn began (`ensureBaseline`) are treated as authoritative specifications of system behavior.
2. **Hard Block on Failure**: If a pre-existing test fails after a code change, `r-tests` and `r-reg` fire a hard stop (`block`). This severity cannot be downgraded by `gate_mode` or user configuration.
3. **Escalate, Never Weaken**: If an existing test legitimately conflicts with a new business requirement, the AI must halt and escalate the conflict to the human operator via a structured decision card. Only human approval can authorize updating a baseline test.

---

## 2. Schema-First 4-Tier Pattern (Defensive Engine)

FlowPilot enforces software engineering discipline through four layers of strictness:

| Tier | Name | Mechanism | Token Cost | Failure Behavior |
| :--- | :--- | :--- | :--- | :--- |
| **T0** | **Deterministic** | Pure Go code, regex, AST inspection, git diff analysis | 0 tokens | Immediate rejection if rule violated |
| **T1** | **Transport Schema** | Tool-call JSON Schema (e.g. `submit_review_outcome`, `request_user_decision`) | Standard tool-call | Rejected at provider transport layer |
| **T2** | **Validation & Reprompt**| Go runner schema validation; exactly 1 reprompt with exact validation error | Minimal token | Escalate to T3 if second attempt fails |
| **T3** | **Fail-Closed** | Run enters parked/blocked state; surfaces resolvable card to human operator | 0 tokens | Never silently assume success |

---

## 3. Node Isolation & Read-Only Enforcement

Instructions in prompts ("Please do not modify files") are ineffective for safety.
FlowPilot enforces permissions at the **Infrastructure Layer**:
- Reviewers, Scouts, and Owner agents run under a **silent-deny read-only posture**. File modification tool calls are intercepted and rejected by the runner before touching the disk.
- Owners in Vibe Mode operate in `verdict_only` mode without shell access.
- Reviewers operate with restricted shell execution guarded by command classifiers.

---

## 4. Hierarchy of Gates (Gate Precedence)

When multiple gate conditions are met in a single turn, the runner evaluates and resolves them according to the strict precedence contract:

$$\text{r-requirement} > \text{drift (wrong-way)} > \text{owner-debate} > \text{r-dod} > \text{r-ca}$$

- `r-requirement` (human-only spec protection) always supersedes automated debate.
- Severe behavioral drift ($\ge 80$ points) pauses execution before running routine audit checks.
