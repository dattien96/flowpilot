---
name: ut-logic-sync-skill
description: Use this skill PROACTIVELY to verify that existing Unit Tests (UT) correctly follow current business logic requirements. It identifies missing, outdated, or misaligned test logic.
---

# A. What is this skill for?

This skill prevents "logic drift" where Unit Tests pass but verify the wrong business rules (e.g., Sprint 1 requirement was 6 chars, Sprint 2 changed to 8, but tests still check for 6).

It bridges the gap between human-language documentation and technical test implementation.

# B. Process

## 1. Source Discovery
Find the business logic source file. This file must follow the standard format defined in `references/business-logic-spec.md`.
- Search for files matching `*business-logic*`, `*requirements*`, or `*spec*`.
- Ask the user if the source is not found.

## 2. Requirement Extraction
Parse the source file to identify:
- **Requirement ID/Name**.
- **Core Logic/Rule** (e.g., "Min 8 characters").
- **Target Components/Tests** (e.g., `RegisterUseCase`, `AuthRepositoryTest`).

## 3. Test Alignment Check
For each requirement, locate the corresponding test files and analyze them:

| Status | Meaning | Action |
| :--- | :--- | :--- |
| **[SYNCED]** | Test exists and logic (assertions/inputs) matches documentation. | No action. |
| **[OUTDATED]** | Test exists but verifies old logic (e.g., `6` instead of `8`). | **FLAG FOR UPDATE**. |
| **[MISSING]** | No test found for this requirement. | **FLAG FOR CREATION**. |

## 4. Reporting
Generate a `logic_sync_report.md` summarizing the findings.

# C. Instructions

- **Assertions Analysis**: Look for specific values in `assertEquals`, `assertTrue`, or constructor parameters in tests.
- **Context Awareness**: Use the `lazy-loading-context-search-skill` to find related test files if not explicitly named.
- **Proactive Correction**: After identification, offer to update the outdated tests or create missing signatures.

# D. Handover Gate

Before finishing:
- [ ] Every requirement in the source file has been checked against the codebase.
- [ ] `logic_sync_report.md` is created and lists all discrepancies.
- [ ] All "SYNCED" tests are verified to actually pass.



