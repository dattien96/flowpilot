---
name: codex-orchestrator
description: Hybrid workflow orchestrator using Codex for Planning, Architecture, TDD, and Review, and Gemini Flash for Implementation.
---

# Codex Orchestrator (Hybrid Workflow)

Use this skill for feature development where Codex (GPT-5/O3) handles high-level design and review, while Gemini Flash handles the tactical coding implementation.

## Phase Ownership & Tooling

| Phase | Responsibility | Tool / Agent |
| :--- | :--- | :--- |
| **Phase 1-3: Design** | Plan, Arch, TDD | `codex exec -m gpt-5.4 -c reasoning_effort=high` |
| **Phase 4: Logic Validation**| Sync Report | **Main Agent (Gemini)** |
| **Phase 5: Coding** | Implementation | **Main Agent (Gemini)** |
| **Phase 6: Review** | Final Audit | `codex exec -m gpt-5.4 -c reasoning_effort=high` |

## Operating Rules

- **Delegation**: When a phase is assigned to `codex exec`, the Main Agent must construct a precise prompt containing all relevant local context and file paths.
- **Codex Syntax**: Use `-m gpt-5.4 -c reasoning_effort=high` for all Codex calls to ensure maximum reasoning capability.
- **Artifacts**: All artifacts (4c_summary, task, implementation_plan, tdd_signatures, walkthrough) MUST be written directly to the project root directory by the Codex agent.
- **Audit Logging**: After review passes and before the final commit or handoff, the Main Agent MUST use `audit-logging-skill` to create or update the relevant `change-audit/` entry for the completed feature or bug fix.
- **Context Loading**: Always read `.codex/AGENTS.md` and relevant KIs before initiating a Codex call.
- **Approval**: Wait for user confirmation after the Phase 1-3 artifact batch before proceeding to implementation.
- **Feedback Loop**: If Phase 6 (Review) results in a `Fail` or identifies missing implementation, the Main Agent MUST automatically return to Phase 5, address the feedback, and re-trigger Phase 6 **only after** verifying that the build is successful and all unit tests pass.

## Phase 1-3: Design (Codex)
- **Action**: Run `codex exec -m gpt-5.4 -c reasoning_effort=high "Perform Planning, Architecture, and TDD for [TASK]. 1) Produce 4c_summary.md and task.md. 2) Produce implementation_plan.md. 3) Produce tdd_signatures.md. All files must be written directly to the project root."`
- **Goal**: Leverage high-effort reasoning to generate the entire design blueprint in one pass.

## 🛑 Gate: User Approval (MANDATORY)
- **Action**: STOP and ask the user: "Design Phase (1-3) is complete. Do you approve the `implementation_plan.md` and `tdd_signatures.md`? Please provide feedback or type 'Approve' to continue."
- **Constraint**: DO NOT proceed to Phase 4 or 5 without explicit user confirmation.

## Phase 4: Logic Validation (Gemini)
- **Action**: Use `ut-logic-sync-skill` to verify that the generated design aligns with the existing codebase.
- **Goal**: Ensure the Main Agent (Gemini) fully understands the design before starting Phase 5.

## Phase 5: Coding (Gemini)
- **Action**: Implement the logic described in `implementation_plan.md` and `tdd_signatures.md`.
- **Verification (FORCE)**: Before proceeding to Phase 6, the Main Agent MUST ensure:
  1. **Build Success**: Gradle build completes without issue (`./gradlew assembleDebug` or similar).
  2. **Test Success**: All unit tests pass (`./gradlew test`).
- **Constraint**: DO NOT trigger Phase 6 (Review) if build or tests fail. Fix all issues locally first.
- **Goal**: High-speed implementation using Gemini's large window.

## Phase 6: Review (Codex)
- **Action**: Run `codex exec -m gpt-5.4 -c reasoning_effort=high "Review implementation in [FILES] against implementation_plan.md and tdd_signatures.md. Produce walkthrough.md in the project root. Besides reviewing the main flow, finally check for any missing parts, bugs, or edge cases that need to be fixed."`
- **Goal**: Use Codex as a critical adversarial reviewer. 
- **Auto-Correction**: If the review returns a `Fail`, immediately loop back to Phase 5 to fix the findings.

## Phase 7: Audit Logging
- **Action**: Use `audit-logging-skill` to create or update the detailed `change-audit/` entry for the completed scope.
- **Constraint**: Do not skip this phase for completed features or bug fixes that changed behavior, schema, routes, runtime logic, or delivery flow.
- **Goal**: Leave a durable engineering record before commit or handoff.
