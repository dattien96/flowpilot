---
name: kmp-interop-skill
description: Use this skill PROACTIVELY when implementing native iOS features, bridging KMP logic to Swift, or handling iOS-specific platform APIs.
---

# A. What is this skill for?

Use this skill PROACTIVELY to manage Kotlin-Swift boundaries efficiently. It enforces the Registry Pattern for iOS bridges, ensures correct function type mapping, and provides a strategy for sharing UI vs logic between Compose and Swift.

# B. What are the tasks?

## 1. Registry Pattern Implementation
Enforce initialization of iOS bridges in `AppDelegate` via a centralized Registry object.

## 2. Type Mapping Verification
Correctly map `Unit` vs `Void` and handle mandatory discards for Unit-returning Kotlin functions in Swift.

## 3. Native Feature Architecture
Implement the "UI from Compose, Logic from Swift" pattern for platform-specific features (Sign-In, Camera, etc.).

# C. Instructions

## 1. iOS Bridge Architecture (Registry Pattern)
- **Hard Gate**: Bridges MUST NOT be initialized in SwiftUI View `init()`.
- **Initialization**: Centralize bridge factories in a Kotlin `object` (Registry) in `iosMain`.
- **AppDelegate**: Initialize all bridges in `AppDelegate.application(didFinishLaunching)`.

## 2. Kotlin-Swift Type Mapping
- **Swift Calls Kotlin**: Use `_ =` to discard `KotlinUnit`.
- **Swift Conforms to Kotlin**: Parameter `() -> Unit` becomes `() -> Void`.
- **Shared Framework**: Interfaces MUST be exported correctly for Swift access.

## 3. Native Feature Strategy
- **Compose UI**: Reuse design system typography/colors in `iosMain`.
- **Logic Location**: Data generation/config stays in Kotlin; Swift only executes the native SDK call.
- **Integration**: Keep callbacks simple; return raw data to Kotlin for processing.

# D. Examples (Enforce)

Check these reference files for patterns:
- [examples.md](references/examples.md) - General patterns.
- [bridge-patterns.md](references/bridge-patterns.md) - Registry implementation.
- [type-mapping.md](references/type-mapping.md) - Type conversion rules.

# H. FORCE rules

- **CRITICAL**: No initialization of bridges in SwiftUI initialization blocks.
- **MANDATORY**: Use the Registry pattern for ALL iOS bridges.
- **ENFORCE**: `_ =` for discarding Unit results in Swift.

# I. Notes

- Import the `shared` module in Swift to access shared interop interfaces.
- Maximize code reuse by keeping the Swift implementation minimal.



