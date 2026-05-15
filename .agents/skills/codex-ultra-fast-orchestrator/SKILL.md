---
name: codex-ultra-fast-orchestrator
description: Direct implementation flow for small bugs or tasks with pre-defined plans.
---

# Codex Ultra-Fast Orchestrator (Direct Coding)

Use this skill for small bugs or tasks where the plan is already clear or provided by the user. It skips the Architecture/Planning phase and goes directly to implementation and review.

## Phase Ownership & Tooling

| Phase | Responsibility | Tool / Agent |
| :--- | :--- | :--- |
| **Phase 1: Coding** | Implementation | **Main Agent (Gemini)** |
| **Phase 2: Review** | Final Audit | `codex exec -m gpt-5.4 -c reasoning_effort=high` |

## Operating Rules

- **Direct Action**: Skip all planning/architecture artifacts. Implementation starts immediately based on user instructions.
- **Verification (FORCE)**: Before proceeding to Phase 2 (Review), the Main Agent MUST ensure:
  1. **Build Success**: Gradle build completes without issue (`./gradlew assembleDebug` or similar).
  2. **Test Success**: All unit tests pass (`./gradlew test`).
- **Constraint**: DO NOT trigger Phase 2 (Review) if build or tests fail. Fix all issues locally first.
- **Feedback Loop**: If Phase 2 (Review) identifies issues, the Main Agent MUST fix them and re-trigger Phase 2 **only after** re-verifying that the build is successful and all unit tests pass.

## Phase 1: Coding (Gemini)
- **Action**: Implement the fix or feature directly into the codebase based on the user's instructions.
- **Goal**: High-speed resolution for well-defined tasks.

## Phase 2: Review (Codex)
- **Action**: Run `codex exec -m gpt-5.4 -c reasoning_effort=high "Review implementation in [FILES] against the user request. Produce walkthrough.md in the project root. Check for missing parts, bugs, or edge cases."`
- **Goal**: Adversarial audit to ensure quality.
