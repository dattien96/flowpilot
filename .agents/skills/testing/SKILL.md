---
version: 6
name: testing-skill
description: Use this skill PROACTIVELY when writing unit tests to ensure 2-path coverage (Fast/Suspend) for coroutines and proper mocking with Mokkery.
---

# A. What is this skill for?

Use this skill PROACTIVELY to ensure Kotlin unit tests are reliable and comprehensive. It mandates 2-path coverage for state-machine-driven suspend functions and establishes mocking standards using Mokkery.

# B. What are the tasks?

## 1. Coroutine Suspend Audit
Identify suspend functions and ensure both "Fast Path" and "Suspend Path" (forced delay) are tested.

## 2. Test Structure Implementation
Apply the Arrange-Act-Assert pattern using `runTest` and Mokkery's `mock/everySuspend` API.

## 3. Quality Critique
Execute a self-audit against the Critique Checklist (requirements alignment, edge cases, risk).

# C. Instructions

## 1. Suspend Function Coverage
- **Problem**: Kotlin state machines have two branches (immediate vs suspended).
- **Solution**: Add a test case with a `delay()` or an actual suspend operation to cover both branches.
- **Hard Gate**: You MUST test:
    - **Path 1 (Fast)**: Immediate return using `everySuspend { ... } returns Result`.
    - **Path 2 (Suspend)**: Forced suspension using `everySuspend { ... } calls { delay(10); Result }`.

## 2. Framework Standards
- **Mocking**: Use Mokkery. `verifySuspend(exactly(1))` for verification.
- **Execution**: Use `kotlinx-coroutines-test`.
- **Target**: 100% branch coverage for the domain layer.

## 3. Report Format
If issues are found during audit: use `[ISSUE] → [SUGGESTED FIX]`.

# D. Examples

- [ViewModel Test Example](references/viewmodel-test.md): Testing UI state and use case interactions.
- [UseCase Test Example](references/usecase-test.md): Testing "Fast Path" vs "Suspend Path" for coroutines.
- [Repository Test Example](references/repository-test.md): Testing data source mocking and error propagation.
- [Coroutine Audit Example](references/coroutine-tests.md): Detailed delay-based suspend function testing.

# H. FORCE rules

- **FORCE**: Use the layer-specific [Examples](#d-examples) provided in this skill to understand the project's testing standards. DO NOT perform broad searches (e.g., `grep_search` or `find_by_name` for `*Test.kt`) to find existing test classes for reference.
- **FORCE**: No suspend function is considered tested without a `delay()` call in at least one test case.
- **MANDATORY**: Domain layer requires 100% branch coverage.
- **CRITICAL**: Use `runTest` for all coroutine-based tests.

# I. Notes

- Characterization tests should be written BEFORE refactoring existing code.
- Mocking libraries must be configured to export mocks to Swift if doing KMP testing.



