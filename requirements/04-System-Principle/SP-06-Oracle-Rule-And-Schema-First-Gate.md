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
- Child Documents: [SS-14 Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20 Flow Gate Rule Semantics](../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-62 Zcode Harness Parity](../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [CP-64 Reproduce-First TDD Gate](../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-65 Multi-Candidate Tournament Harness](../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Related Documents: [SS-08 Approve Gate](../05-System-Specs/SS-08-Approve-Gate.md), [CP-35 Context And Regression Engine](../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md)
- Replaces: `None`
- Tags: `oracle-rule, regression, schema-first, flow-gate, test-protection, reproduce-first, tournament-arbiter`
- Feature Keys: `context-regression-engine, flowgate, zcode-parity, reproduce-first-gate, tournament-harness`

## AI Quick View

### Summary

- **The Oracle Rule**: Pre-existing unit and integration tests are an absolute ground truth (Oracle). A failing test must NEVER be resolved by weakening the test or modifying test expectations to force a green result. Any failure in pre-existing tests is a hard regression that triggers an immediate, un-downgradable block.
- **The Reproduce-First Oracle (CP-64)**: For bugfix and behavior-change workflows, an agent is barred from touching production code until it authors an executable test that compiles cleanly and FAILS via assertion error (`r-reproduce`). A green test on arrival fails the reproduce gate.
- **Schema-First 4-Tier Enforcement (T0-T3)**: The execution engine must never parse unstructured free-form prose from LLMs to make routing or state decisions. All critical decisions, reviews, and escalations must pass through typed schemas and infrastructure-level guards.
- **Tournament Arbiter Consensus (CP-65)**: When review cycles or debates exceed their iteration cap, resolution shifts to deterministic scoring across parallel rollouts rather than unconstrained sequential retries.
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

### 1.3 The Reproduce-First Oracle Extension (BugFix Ground Truth)
For any defect report or regression fix (`bug-harness`, `bug-plan-harness`):
1. **Presumption of Defect**: The existing production code is presumed defective for the reported condition.
2. **Mandatory Red Execution**: Before the coder agent receives write permission to production files, the tester agent MUST produce an executable test case that compiles cleanly and fails with an explicit assertion error.
3. **Compile Error Disqualification**: A test that fails due to compilation/syntax errors does NOT satisfy reproduction.
4. **Physical Write Lock**: Once verified RED by the runner, the reproduction test file is locked read-only during the coder's implementation turn to prevent retroactive tampering.

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

$$\text{r-reproduce} > \text{r-requirement} > \text{drift (wrong-way)} > \text{owner-debate} > \text{r-dod} > \text{r-ca}$$

- `r-reproduce` (verifiable defect reproduction) gates coder entry for bug fixes.
- `r-requirement` (human-only spec protection) always supersedes automated debate.
- Severe behavioral drift ($\ge 80$ points) pauses execution before running routine audit checks.

---

## 5. Deadlock Resolution & Tournament Arbiter Principle

When autonomous review loops or debates reach their iteration cap (`review_cap_exceeded`, `owner_debate_stalled`), sequential retry becomes counterproductive due to context accumulation and patch-on-patch degradation.

FlowPilot resolves deadlocks via the **Deterministic Tournament Arbiter**:
1. **Parallel Independent Rollouts**: Spawn isolated candidates across multiple providers (e.g. Claude, Codex, Grok) within dedicated Git worktrees.
2. **Objective Score Matrix**: The winning candidate is selected strictly via deterministic calculation:
   $$\text{Score} = (0.50 \times \text{TestPassRate}) + (0.30 \times \text{LSPCleanliness}) + (0.20 \times \text{MinimalBlastRadius})$$
3. **Zero Hallucinated Consensus**: An LLM is never allowed to pick the winner subjectively; selection is performed exclusively by runner runtime metrics.
