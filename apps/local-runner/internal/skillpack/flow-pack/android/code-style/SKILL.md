---
version: 6
name: code-style-skill
description: Use this skill PROACTIVELY when referencing general rules for clean code, naming, comments, null safety, and security to ensure implementation quality.
---

# A. What is this skill for?

Use this skill PROACTIVELY to maintain high standards for code readability, maintainability, and security. It covers tactical clean code principles that apply across all layers of the application.

# B. What are the tasks?

## 1. Clean Code Audit
Apply principles like KISS, DRY, and YAGNI to simplify logic and remove duplication.

## 2. Naming & Documentation
Ensure variables, functions, and classes follow clear naming conventions and are documented via KDoc where necessary.

## 3. Safety & Hygiene
Enforce null safety rules (avoiding `!!`), immutability (`val`), and serialization requirements for data transfer.

## 4. Security Enforcement
Prevent hardcoded secrets and ensure sensitive data is handled in secure storage.

# C. Instructions

## 1. General Principles
- **KISS**: Keep It Simple, Stupid.
- **DRY**: Don't Repeat Yourself.
- **YAGNI**: You Ain't Gonna Need It.
- Favor **composition** over inheritance.
- Use **extensions, delegates, and higher-order functions** as extension points.

## 2. Naming & Comments
- **Rule**: Names must reflect meaning; use clear verbs for functions.
- **Rule**: Avoid confusing abbreviations or unnecessary prefixes (e.g., `mUser`).
- **Convention**: Follow the standardized naming patterns for domains and layers (e.g., `XxxUseCase`, `XxxRepository`, `XxxViewModel`) as defined in [`coding-skill`](../coding/SKILL.md).
- **Comments**: Only comment what the code cannot explain; use **KDoc** for APIs.
- **TODO**: Use for incomplete tasks; delete commented-out code blocks.

## 3. Data Integrity & Safety
- **Null Safety**: Avoid `!!`. Use `?.let`, `?: return`, or `requireNotNull`.
- **Immutability**: Prefer `val` over `var` and immutable Data Classes.
- **Serialization**: Every UI state or entity passed to a Screen must be `@Serializable`.

## 4. Security
- **No Secrets**: Never commit hardcoded API keys or secrets.
- **Secure Storage**: Use `EncryptedSharedPreferences` (Android) or `Keychain` (iOS) for tokens.
- **Logging**: Do NOT log PII (Personally Identifiable Information).

# D. Examples

Check [clean-code.md](references/clean-code.md) for examples of how to apply these rules.
- Reference: [Kotlin Style Guide](https://kotlinlang.org/docs/coding-conventions.html)

# H. FORCE rules

- **CRITICAL**: Do NOT use `!!` unless absolutely necessary and verified.
- **MANDATORY**: All data classes used in Screen states MUST be `@Serializable`.
- **ENFORCE**: Never log sensitive tokens or PII.

# I. Notes

- Use the IDE's linting tools to supplement these rules.
- High-level architectural rules live in `architecture-skill`.



