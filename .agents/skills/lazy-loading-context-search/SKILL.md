---
name: lazy-loading-context-search-skill
description: Use this skill PROACTIVELY to enforce domain-specific search patterns and 25-file limits to save tokens and prevent loading entire codebase categories.
---

# A. What is this skill for?

Use this skill PROACTIVELY to prevent token waste and improve response accuracy by loading ONLY files relevant to the current request. It enforces narrow search patterns and a 25-file limit per search operation.

# B. What are the tasks?

## 1. Request-Driven Search Design
Extract keywords from the user request to identify the target feature domain and construct constrained glob patterns.

## 2. Incremental Discovery
Start with narrow patterns and broaden only if no matches are found, avoiding global searches.

## 3. Path-Constrained Grep
Apply path constraints to all ripgrep operations to focus on relevant modules or layers.

## 4. Search Scope Verification
Audit search results against the 25-file limit and refine patterns if the result set is too large.

# C. Instructions

## 1. Constraints
- **FORBIDDEN**: Broad patterns like `**/*UseCase*.kt` without domain constraints.
- **LIMIT**: Max 25 files per search. If more are needed, explain trade-offs and get explicit User confirmation.
- **GREP**: ALWAYS add a `limit` of 5-10 and a `path:` constraint (e.g., `path:**/meal/**`).
- **READ**: For files >100 lines, use `offset` and `limit` to read only relevant sections.
- **ASK**: If a domain search is likely to yield >10 matches, ask the USER for specific file names or sub-paths.

## 2. Decision Tree
```
Request → Extract Keywords → Identify Domain
     ↓
Search: **/{domain}/**/*{Keyword}*.kt
     ↓
Found ≤ 25 files? 
├─ YES → Read files ✅
└─ NO → Refine pattern or ask User for scope
```

## 3. Token Budget Rules
- **Planning**: Max 50 files total.
- **Architecture**: Max 100 files total.
- **Coding**: Max 25 files total (focused scope).

# D. Examples

- ✅ `Glob: '**/meal/**/*UseCase*.kt'`
- ✅ `Grep: 'class.*Meal.*UseCase' path:**/meal/**`
- ❌ `Glob: '**/features/**/usecase/*.kt'` (Loads too many domains)

# H. FORCE rules

- **CRITICAL**: NEVER load 25+ files without explicit confirmation.
- **MANDATORY**: `grep_search` MUST use a `limit` (target 5-10).
- **MANDATORY**: Files >100 lines MUST be read using `offset` and `limit`.
- **ENFORCE**: Path constraints are MANDATORY for all Grep calls.
- **MANDATORY**: Summarize search constraints in the Skill Loading Receipt.

# I. Notes

- Use Option A/B/C Batching approach if the domain is exceptionally large.



