---
version: 6
name: coding-skill
description: Use this skill PROACTIVELY during Phase 3 (Coding) to enforce tactical implementation standards, including UI patterns, visibility, naming, and KMP type safety.
---

# A. What is this skill for?

Use this skill PROACTIVELY during the implementation phase to ensure code follows project-specific tactical standards. It defines the **"How"** of the code, covering UI components, naming, interop, and general hygiene.

# B. What are the tasks?

## 1. UI Rule Enforcement
Ensure every @Composable has a modifier and correct visibility (`public`/`internal`/`private`).

Also make sure the skill [`compose-ui`](../compose-ui/SKILL.md) is adapted.

## 2. Naming & Structure Audit
Verify that files and classes follow the mandated naming patterns (e.g., `XxxScreenState`, `XxxIntent`, `XxxDto`).

Also make sure the skill [`code-style`](../code-style/SKILL.md) is adapted.

## 3. KMP Interop Management
Handle specific Kotlin-Swift boundary issues, such as Unit/Void mapping and discarding unit results.

Also make sure the skill [`kmp-interop`](../kmp-interop/SKILL.md) is adapted.

## 4. Coding Hygiene & Documentation
Maintain small functions, enforce immutability, and add KDoc headers for complex components.

# C. Instructions

## 1. UI Implementation (Compose)
- **Hard Gate**: Every `@Composable` MUST have a `modifier: Modifier = Modifier` parameter.
- **Visibility**: 
  - `XxxScreen`: `public`
  - `XxxContent` / `XxxSuccessView`: `internal`
  - Helper components: `private` (if file-specific)

## 2. Naming Conventions
- **Domain**: `XxxUseCase.kt` (SAM syntax), `XxxGateway.kt` (interface).
- **Data**: `XxxRepository.kt` (implementation), `XxxMapper.kt`, `XxxDto.kt`.
- **Presentation (MVI)**: 
  - `XxxViewModel.kt`
  - `XxxScreen.kt` (Entry point)
  - `XxxContent.kt` (State routing)
  - `XxxSuccessView.kt` (UI rendering)
  - `XxxIntent.kt` (Sealed class)
  - `XxxScreenState.kt` / `XxxSuccessDataState.kt` (Data class)
  - `[Name]UiItem.kt` (UI model in `/model/`)

## 3. KMP & Interop Tactics
- **Common Widgets**: ALWAYS prefer `CommonText` over `Text` and `CommonButton` over `Button`. Scan `:core-foundation:appdesign` before creating new ones.
- **Swift Boundaries**: Use `_ =` to discard `KotlinUnit`. Use `() -> Void` in Swift interfaces for Kotlin `() -> Unit`.

## 4. Coding Hygiene
- **Immutability**: Prefer `val`. All Data Classes MUST be immutable.
- **Null Safety**: Avoid `!!`. Use `?.let` or `?:`.
- **Functions**: MUST be < 40 lines. One responsibility per function.

# D. Examples

Check [examples.md](examples.md) for concrete coding patterns and interop examples.
- Reference: [Jetpack Compose Best Practices](https://developer.android.com/develop/ui/compose/performance/bestpractices)

Use the Read tool to load:

- **FORCE**: [`code-style-skill`](../code-style/SKILL.md) (Always load for every coding task).
- **CONDITIONAL**: [`compose-ui-skill`](../compose-ui/SKILL.md) (Load if task involves UI/Compose).
- **CONDITIONAL**: [`kmp-interop-skill`](../kmp-interop/SKILL.md) (Load if task involves iOS/Swift/KMP boundaries).
- **CONDITIONAL**: [`refactor-skill`](../refactor/SKILL.md) → If refactoring existing code

# H. FORCE rules

- **FORCE**: Load [`code-style`](../code-style/SKILL.md) for every coding task.
- **ENFORCE**: Every Composable MUST accept a `modifier`.
- **CRITICAL**: No `!!` in production code.

# I. Notes

- Add KDoc headers to complex components detailing applied skills (e.g., `[coding-skill]: Modifier required`).



