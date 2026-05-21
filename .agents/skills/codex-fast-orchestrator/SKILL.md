---
name: codex-fast-orchestrator
description: Lean hybrid workflow for small tasks and bug fixes, focusing on speed.
---

# Codex Fast Orchestrator (Lean Workflow)

Use this skill for small features or bug fixes where a full TDD/Planning overhead is not required. It focuses on rapid Architecture -> Implementation -> Review.

## Phase Ownership & Tooling

| Phase | Responsibility | Tool / Agent |
| :--- | :--- | :--- |
| **Phase 1: Architecture** | Implementation Plan | `codex exec -m gpt-5.4 -c reasoning_effort=high` |
| **Phase 2: Coding** | Implementation | **Main Agent (Gemini)** |
| **Phase 3: Review** | Final Audit | `codex exec -m gpt-5.4 -c reasoning_effort=high` |

## Operating Rules

- **Delegation**: When a phase is assigned to `codex exec`, the Main Agent must construct a precise prompt containing all relevant local context and file paths.
- **Artifacts**: `implementation_plan.md` and `walkthrough.md` MUST be written directly to the project root directory.
- **Approval**: Wait for user confirmation after Phase 1 (`implementation_plan.md`) before proceeding to implementation.
- **Feedback Loop**: If Phase 3 (Review) identifies bugs or missing parts, the Main Agent MUST fix them and re-trigger Phase 3 **only after** verifying that the build is successful and all unit tests pass.

## Phase 1: Architecture (Codex)
- **Action**: Run `codex exec -m gpt-5.4 -c reasoning_effort=high "Perform Architecture design for [TASK]. Produce implementation_plan.md directly to the project root. Skip 4C summary and TDD signatures."`
- **Goal**: Rapidly define the technical approach.

## 🛑 Gate: User Approval (MANDATORY)
- **Action**: STOP and ask the user: "Architecture Phase is complete. Do you approve the `implementation_plan.md`? Please provide feedback or type 'Approve' to continue."

## Phase 2: Coding (Gemini)
- **Action**: Implement the logic described in `implementation_plan.md`.
- **Verification (FORCE)**: Before proceeding to Phase 3, the Main Agent MUST ensure:
  1. **Build Success**: Gradle build completes without issue (`./gradlew assembleDebug` or similar).
  2. **Test Success**: All unit tests pass (`./gradlew test`).
- **Constraint**: DO NOT trigger Phase 3 (Review) if build or tests fail. Fix all issues locally first.
- **Goal**: High-speed implementation using Gemini's large window.

## Phase 3: Review (Codex)
- **Action**: Run `codex exec -m gpt-5.4 -c reasoning_effort=high "Review implementation in [FILES] against implementation_plan.md. Produce walkthrough.md in the project root. Check for missing parts, bugs, or edge cases."`
- **Goal**: Fast adversarial review.
