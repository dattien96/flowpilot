---
name: receiving-code-review-skill
description: Use this skill PROACTIVELY when processing reviewer comments or evaluating architectural pushback to ensure technical correctness over social comfort.
---

# A. What is this skill for?

Use this skill PROACTIVELY to process code review feedback with technical rigor. It prioritizes verification and rational evaluation over "performative agreement" or emotional acknowledgment.

# B. What are the tasks?

## 1. Feedback Analysis
Read, restate, and verify feedback against the codebase reality before implementing anything.

## 2. Technical Response Execution
Respond with factual acknowledgment ("Fixed") or reasoned technical pushback.

## 3. Order-of-Ops Implementation
Categorize and implement fixes by severity: Critical -> Important -> Simple -> Complex.

## 4. Ambiguity Resolution
Identify and resolve unclear feedback items BEFORE starting any implementation.

# C. Instructions

## 1. Response Protocol (The "No Thanks" Rule)
- **FORBIDDEN**: "You're right!", "Great point!", "Thanks for catching that!", apologies, or gratitude.
- **REQUIRED**: Technical acknowledgment or direct path to action.
- **EXAMPLE**: "Fixed. Added null-check in ProfileRepository.kt."

## 2. Pushback Strategy
- Push back if a suggestion: Breaks existing behavior, violates YAGNI, is technically incorrect for our stack, or lacks full context.
- **Example**: "This API is required for Android 10+ support. Dropping it would be a breaking change."

## 3. Handling Unclear feedback
- STOP immediately if any item is confusing.
- Ask for clarification before partial implementation.

## 4. Implementation Rules
- One item at a time.
- Test each fix individually.
- Verify no regressions.

# D. Examples (Enforce)

- [review-response-examples.md](./references/review-response-examples.md) - Scenario-based responses.

# H. FORCE rules

- **CRITICAL**: Delete any "Thanks" or performative agreement before sending responses.
- **MANDATORY**: Verify all suggested changes against the codebase before adoption.
- **ENFORCE**: Ask before assuming intent on multi-item lists.

# I. Notes

- Code review is technical evaluation, not emotional performance.
- Use GitHub API for inline replies to stay in context.



