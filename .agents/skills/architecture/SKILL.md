---
version: 6
name: architecture-skill
description: Use this skill PROACTIVELY when designing system architecture, package structure, layer boundaries, and communication patterns during Phase 2 (Architecture).
---

# A. What is this skill for?

Use this skill PROACTIVELY when designing the high-level structural blueprint of a feature or the entire system. It defines the **"Where"** and **"What"** of the codebase, ensuring Clean Architecture compliance and maintainability.

# B. What are the tasks?

## 1. Feature Blueprinting
Define the package structure and module boundaries for a new or existing feature.

## 2. Layer Definition
Properly allocate responsibilities across Presentation, Domain, Data, and Core layers.

## 3. Communication Pattern Design
Establish how components interact (MVI, Registry Pattern, Navigator) and ensure dependency inversion.

## 4. Architectural Audit
Verify that existing code follows SOLID principles and the project's standardized Screen Architecture.

# C. Instructions

## 1. Clean Architecture Layers

### 📂 Presentation Layer (`/presentation/`)
- **Focus**: UI state management and rendering.
- **Pattern**: `XxxScreen` (Entry) -> `XxxContent` (Routing) -> `XxxSuccessView` (UI).
- **Structure**:
    ```
    features/presentation/[feature]/
    ├── screen/          # Entry points & State/Intent definitions
    ├── components/      # Feature-specific reusable UI
    └── model/          # UI-only data models & enums
    ```

### 📂 Domain Layer (`/domain/`)
- **Focus**: Business logic and use cases.
- **CRITICAL**: NO Android, UI, or Compose dependencies. Pure Kotlin/KMP only.
- **Rules**:
    - **UseCases**: Single responsibility (e.g., `GetMealUseCase`).
    - **Repository Interfaces**: Defined here to be implemented in the Data layer.
    - **Domain Models**: Plain data classes representing business entities.

### 📂 Data Layer (`/data/`)
- **Focus**: Data sourcing (API, DB, Cache).
- **Rules**:
    - **Implementations**: Implements the Repository interfaces from Domain.
    - **Mappers**: MUST map technical DTOs to Domain Models before returning.

### 📂 Core Layer & Foundation
- `/core/`: Shared project-specific core modules.
- `/core-foundation`: Common UI components & design system.

## 2. Screen Architecture Pattern (MANDATORY)

Every screen must follow this hierarchy:
1. **XxxScreen**: ViewModel binding, state observation, intent dispatching.
2. **XxxContent**: State routing (Loading/Error/Success). MUST use `LoadingScreen()` and `ErrorScreen()`.
3. **XxxSuccessView**: Final UI rendering for the success state.

### 2.1. **XxxScreen** - Entry Point
- Naming format: `SettingScreen`, `LoginScreen`, `ProfileScreen`
- Responsibilities:
  - Declare ViewModel
  - Observe states from the ViewModel
  - Pass data + callbacks down to the Content layer

### 2.2. **XxxContent** - State Handler
- Naming format: `SettingContent`, `LoginContent`
- **IMPORTANT**: Content only receives **pure data** and is unaware of the ViewModel.
- Input: Pure data + callbacks (`onIntent`)
- Responsibilities depend on whether `BaseScreenState` is observed:  
**Case A: NOT observing BaseScreenState**
**Case B: Observing BaseScreenState**
- Content is only responsible for **detecting Success/Loading/Error states**.
- Delegates rendering to `XxxSuccessView`.
- **CRITICAL**: MUST use `LoadingScreen()` and `ErrorScreen()` from `appdesign`.

**Requirements:**
- `BaseScreenState.Loading` → MUST use `LoadingScreen(modifier = modifier)`
- `BaseScreenState.Error` → MUST use `ErrorScreen(errorStringResId = uiState.messageErrorResId, onRetry = {...}, modifier = modifier)`
- Imports: `com.datnguyen.a76.mplan.core.appdesign.widgets.LoadingScreen` and `ErrorScreen`

### 2.3. **XxxSuccessView** - Final UI (Only if BaseScreenState)
- Naming format: `SettingSuccessView`, `LoginSuccessView`
- Only created when Content observes `BaseScreenState`.
- This is where the actual UI for the success case is rendered.

## 3. Visibility Modifiers (Access Control)

| Function Type | Modifier | Reason |
| :--- | :--- | :--- |
| **XxxScreen** | `public` | Entry point for navigation from other modules. |
| **XxxContent / SuccessView** | `internal` | Accessible for internal testing and previews; hidden from external modules. |
| **Helper Components** | `private` | Used only within a single file. |

## 4. Core SOLID Principles

- **SRP**: One class/function/package/module = one responsibility.
- **OCP**: Open for extension, closed for modification.
    + Prefer using `extensions`, `delegates`, and `higher-order functions` for extension.
    + Limit complex nested `if/else` and `switch/case` statements.
    + Use `abstraction` (interfaces/abstract classes) for easy scaling/extension.
- **DIP**: Depend on abstractions (interfaces), not concrete implementations. Use Koin for DI, No dependence on static/global state.
- **ISP**: Small, focused interfaces.
- **Testability**: No global state. Inject time/random providers.

## 5. Component Classification

### 5.1 Decision Tree for UI Components
1.  **Is it a Main Screen?** → `/screen/` (Must have ViewModel).
2.  **Used in ≥2 features?** → `:core-foundation:appdesign`.
    - Components used by **MULTIPLE features**
    - Example: `AppSelectableCard` (used in preferences + auth), `SimpleOutlinedTextField` (used in multiple features)
3.  **Used only in 1 feature?** → `/components/` of that feature.
    - Components that can be separated but are **ONLY used within a single feature**.
    - **IMPORTANT**: Even if used multiple times WITHIN that feature, it remains feature-specific.
    - Example: `ProfileHeader` (only used in profile feature), `RecipeCard` (only used in recipe feature).
    - Path: `features/presentation/[feature]/components/`

### 5.2  Main Screen → `/screen/`
- The primary screen of the feature.
- **CRITICAL**: Screens MUST have a ViewModel binding (declare ViewModel, observe state).
- **RULE**: If a file is named `XxxScreen` but DOES NOT have a ViewModel → It is a Component and MUST be renamed to `XxxComponent` and moved to `/components/`.
- Example: `LoginScreen.kt`, `HomeScreen.kt`, `ProfileScreen.kt`
- Path: `features/presentation/[feature]/screen/`
## 7. Screen Intent & State → `/screen/[screen-name]/`**
- **CRITICAL**: ONLY 2 types of files are permitted in `/screen/`: Intent and ScreenState.
- **MANDATORY NAMING PATTERN:**
  - Intent: `XxxIntent.kt` (sealed class)
  - ScreenState: `XxxScreenState.kt` or `XxxSuccessDataState.kt` (data class)
- Located in the same folder as the Screen and ViewModel.

- **Pattern**: `MVI` (Intent/State).
- **Rule**: ViewModels expose a single `UiState` via `StateFlow` and receive `Intents`.
- **Rule**: Use `StateFlow` for state management.

## 8. Workflow Example

**Request**: "Review package XXX and change Text to CommonText"

**Action Steps**:
1. Find all `Text(` in the package.
2. Check if `CommonText` already exists in `appdesign`.
3. Replace `Text` → `CommonText`.
4. Verify imports and parameters.

**Request**: "Create a new LoginScreen"

**Action Steps**:
1. Check if `appdesign` has `CommonTextField` and `CommonButton`.
2. Use common components if available.
3. Create main screen → `features/presentation/login/screen/LoginScreen.kt`.
4. Create feature-specific components → `features/presentation/login/components/LoginForm.kt`.
5. Create UI models (if needed) → `features/presentation/login/model/LoginUiState.kt`.
6. If a component is shared across multiple features, create it in `appdesign/widgets/`.
7. **Ensure every @Composable has `modifier: Modifier = Modifier`**.

# D. Examples

Check [examples.md](examples.md) for detailed structural examples and implementation patterns.
- Reference: [Clean Architecture Overview](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- File: @features/presentation/login/screen/LoginScreen.kt

# H. Mandatory Skills (always load)
- [`lazy-loading-context-search-skill`](../lazy-loading-context-search/SKILL.md) ⭐ **Domain-specific search, max 25 files**

# I. FORCE rules

- **MANDATORY**: Every Screen MUST have a ViewModel.
- **CRITICAL**: UseCases MUST NOT have UI dependencies.
- **ENFORCE**: Every @Composable MUST accept a `modifier: Modifier = Modifier`.

- Strictly follow **Clean Architecture**.
- Divide the project into multiple features (feature-based modularization).
- Each feature has levels: `domain`, `data`, `presentation`.
- _Exception:_ If a feature is small (like onboarding), the 3 layers can be combined into one module, but they must be clearly separated by package.
- Adhere to **Low Coupling, High Cohesion**.
- UseCases follow **SAM (Single Abstract Method)** syntax.
- Strictly design for **code reusability**.

# I. Notes

- If a feature is small, layers can be combined into one module but must be separated by package.
- Features should NOT depend on each other; use a central `Navigator`.



