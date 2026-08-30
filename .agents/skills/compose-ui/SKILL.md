---
version: 6
name: compose-ui-skill
description: Use this skill PROACTIVELY when developing Jetpack Compose UI, handling multi-platform layouts, or optimizing component reuse.
---

# A. What is this skill for?

Use this skill PROACTIVELY to ensure Compose UI development follows adaptive design principles and maximize component reusability across the project. It provides the standards for layout variations and structural UI audits.

# B. What are the tasks?

## 1. Adaptive Layout Implementation
Properly apply `SceneStrategy` (navigation-level) or `AdaptiveLayout` (screen-level) tools to handle multiple screen sizes.

## 2. Component Reuse Audit
Check for existing common components in `:core-foundation:appdesign` before creating new ones.

## 3. Extraction & Modularization
Extract repeated UI patterns (≥2 occurrences) into feature-specific components or the core design system.

## 4. UI Consistency Verification
Scan existing packages for duplicate patterns and ensure correct placement based on the UI Decision Tree.

# C. Instructions

## 1. Adaptive UI Standards
**There are 2 levels of Adaptive UI - DO NOT confuse them:**

### 1.1 **Navigation-Level**: Use `SceneStrategy` (e.g., `rememberListDetailSceneStrategy()`) for master-detail patterns on tablets.

- **WHEN**: List-detail patterns, master-detail layouts.
- **USE**: `SceneStrategy` from Nav3 (e.g., `rememberListDetailSceneStrategy()`).
- **PURPOSE**: Controls the simultaneous display of multiple destinations (list + detail on tablet).
- **EXAMPLE**: Settings screen with list items + detail panel, Email list + reading pane.


### 1.2 **Screen-Level**: Use `AdaptiveLayout` from `:core-foundation:appdesign` for portrait vs. landscape or phone vs. tablet variations within a screen.

- **WHEN**: Layout variations within a single screen (portrait vs landscape, phone vs tablet UI).
- **USE**: `AdaptiveLayout` from `:core-foundation:appdesign`.
- **PURPOSE**: Adjusts the layout/spacing/arrangement of components within the screen.
- **EXAMPLE**: IntroductionScreen (vertical stack vs horizontal row), Login screen scrollable content.
- Path: `core-foundation/modules/appdesign/src/commonMain/kotlin/com/datnguyen/a76/mplan/core/appdesign/adaptive/AdaptiveUtils.kt`

### 1.3 **CRITICAL**: Avoid hardcoded `isTablet` checks or manual `BoxWithConstraints`.

- ALWAYS use SceneStrategy or AdaptiveLayout depending on the level.
- You can combine both: SceneStrategy at navigation level + AdaptiveLayout within the screen content.

## 2. Component Extraction Decision Tree
```
Is the component reusable?
│
├─ Used in ≥2 DIFFERENT features?
│  └─ Move to :core-foundation:appdesign
│
├─ Used multiple times WITHIN a single feature?
│  └─ Move to /presentation/[feature]/components/
│
└─ Used only once as a private function?
   └─ Keep it in the Screen file
```

### 2.1 If a component already exists:
- `CommonText` instead of `Text`
- `CommonButton` instead of `Button`
- `CommonCard`, `CommonDivider`, etc.
- Path: `/core-foundation/modules/appdesign/src/commonMain/kotlin/com/datnguyen/a76/mplan/core/appdesign/`

### 2.2. Create Common Component if Necessary (When creating new ones)
When creating a new screen and identifying a component that can be **reused**:
- Extract it into a common component.
- Place it in `:core-foundation:appdesign`.
- Apply the design system (colors, typography, spacing).

### 2.3. CRITICAL - Component Extraction Check (When checking existing code)
**MANDATORY**: When checking an existing UI package, you MUST scan the entire code to find:

## 3. Scanning Checklist for Extraction
- List all `@Composable` functions in a package.
- Identify duplicate UI patterns (headers, overlays, form groups).
- Verify components accept a `modifier: Modifier = Modifier`.
- Ensure components are placed according to the Decision Tree.

# D. Examples

Check [examples.md](examples.md) for concrete adaptive UI implementations and extraction patterns.
- Reference: [Android Adaptive Layouts](https://developer.android.com/develop/ui/compose/adaptive)

# H. FORCE rules

- **MANDATORY**: Every @Composable MUST accept a `modifier`.
- **ENFORCE**: ALWAYS check `appdesign` for common components before writing custom UI.
- **CRITICAL**: Extraction is MANDATORY for patterns appearing ≥2 times.

# I. Notes

- Combine `SceneStrategy` and `AdaptiveLayout` for complex multi-platform features.
- SuccessViews should handle the bulk of feature-specific Success data rendering.



