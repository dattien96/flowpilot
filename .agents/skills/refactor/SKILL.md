---
version: 6
name: refactor-skill
description: Use this skill PROACTIVELY when improving code structure, migrating patterns, or modernizing legacy code while preserving external behavior.
---

# A. What is this skill for?

Use this skill PROACTIVELY to improve internal code quality (readability, maintainability, structure) without changing external behavior. It provides systematic patterns for extracting components, migrating to modern APIs (like StateFlow), and removing duplication.

> [!IMPORTANT]
> **Refactor vs. Rewrite:**
> - **Refactor**: Change structure WITHOUT changing external behavior.
> - **Rewrite**: Change both structure AND behavior (requires full 4C + design approval).
> - If behavior changes are needed, flag to user BEFORE proceeding.

# B. What are the tasks?

## 1. Refactor Scoping & Planning
Identify affected files, check test coverage, and break the refactor into small, incremental steps.

## 2. Structural Improvements
Extract UI components, business logic (Use Cases), or shared widgets to appropriate modules.

## 3. Pattern/API Modernization
Migrate legacy code (e.g., LiveData) to modern standards (e.g., StateFlow) or simplify complex branching logic.

## 4. Verification & Documentation
Ensure all tests pass post-refactor, update relevant documentation, and document changes in `walkthrough.md`.

# C. Instructions

## 1. Safety Rules (Hard Gates)
- **Tests First**: NEVER refactor without test coverage. Write characterization tests if missing.
- **Incremental**: One pattern/step per commit. "Big Bang" refactors are forbidden.
- **Preserve Behavior**: Refactor = Same logic, different structure. Flag behavior changes to User first.
- **Commit History**: Each logical step = one commit with `[Refactor]:` prefix.

## 2. Refactor Patterns
- **Extract Component**: Screen → smaller components with `internal` visibility. Components receive data via parameters (no direct ViewModel access).
- **Extract Use Case**: Logic in ViewModel/Repo → action-specific UseCase in `features/domain/usecase/`.
- **Extract Shared Widget**: Promoted to `core-foundation/modules/appdesign/widgets/` ONLY if used in ≥2 DIFFERENT features.
- **DRY**: Consolidate truly similar logic; avoid over-engineering.

## 3. Modernization Standards
- **Naming**: Functions MUST be verbs (`calculateTotal()`). Classes MUST be nouns (`UserProfile`). 
- **Booleans**: Use `is/has/can/should` prefixes (`isValid`, `hasPermission`).
- **Hygiene**: Avoid abbreviations (`userRepository`, not `usrRepo`). Functions MUST be < 40 lines.

## 4. Common Refactor Mistakes
- **Mistake 1: Changing Behavior**: If tests fail because logic changed (not structure), revert and focus.
- **Mistake 2: Missing Tests**: Refactoring untested code without characterization tests first.
- **Mistake 3: Breaking Public APIs**: Failing to maintain backward compatibility or coordinate changes.

# D. Examples

- Pattern Details: [refactor-patterns.md](./references/refactor-patterns.md)
- Reference: [SOLID Principles](../architecture/SKILL.md)

# H. FORCE rules

- **FORCE**: Run full verification after EVERY incremental step.
- **MANDATORY**: No suspend function is considered refactored without verifying tests.
- **CRITICAL**: Revert immediately if tests fail and root cause is logic change.

# I. Notes

- Refactor vs Rewrite: If behavior must change, use the 4C Checklist process instead.
- Use `internal` visibility for feature-specific components.
- Each refactor step should be committable independently.



