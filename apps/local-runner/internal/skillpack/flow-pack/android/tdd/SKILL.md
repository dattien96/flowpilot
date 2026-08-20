---
name: tdd-skill
description: Use this skill PROACTIVELY during the TDD phase (after Architecture) to define test signatures and expectations based on the implementation plan.
---

# A. What is this skill for?

This skill converts an architectural blueprint into a set of test signatures BEFORE any code is written. It ensures "What" to test is clear before "How" to implement.

**CRITICAL**: Do NOT write implementation test code.

# B. What are the tasks?

## 1. Signature Definition
Identify logic paths and create function signatures for tests.

## 2. Expectation Mapping
Document clear `Input` and `Expected Output` for each test case.

# C. Instructions

## 1. Process

### Step 1: Read the Plan
- Read `implementation_plan.md`.
- Identify new/modified usecases, repositories, and ViewModels.
- Identify new/modified usecases, repositories, and handlers.
- **READ**: For existing test files (>100 lines), read ONLY the structure (first 50 lines) using `offset/limit` to identify patterns.

### Step 2: Define Logic Paths
For each component, identify:
- **Success Path**: The primary "happy path".
- **Error Path**: Expected failures (invalid input, network error, etc.).
- **Boundary/Edge Cases**: Empty states, null values, list limits.

### Step 3: Write Test Signatures
Write the test class and function signatures in the target test files. 
**CRITICAL**: Do NOT write implementation code. Only specify:
- Function Name (describing behavior).
- Parameter signatures.
- **CONDENSED**: Use bullet points for `Input` and `Expected Output`. Avoid verbose KDoc to save tokens.

## 2. Signature Format

Check `signature-format.md` for examples.

# G. Mandatory Skills (always load)
- [`lazy-loading-context-search-skill`](../lazy-loading-context-search/SKILL.md) ⭐ **Domain-specific search, max 25 files**

# H. Forbidden in Phase 3 (STRICT)

- ❌ `val mockGateway = mockk<EmotionGateway>()`
- ❌ `coEvery { mockGateway.saveEmotion(any()) } returns ...`
- ❌ `assertEquals(AppResult.Success(Unit), result)`
- ❌ `mock(EmotionGateway::class.java)`

**Validation Rule**: If the test file would COMPILE and RUN in Phase 3, it's **TOO ADVANCED** and is a violation.

# I. Handover Gate (MANDATORY)

Before proceeding to Phase 4, verify:
Define "What" to test without defining "How" to implement.

Before proceeding to Phase 4, verify:
- [ ] NO `mockk()`, `mock()` calls in test files.
- [ ] NO `coEvery {}`, `every {}` stubbing.
- [ ] NO `verify {}`, `assertEquals()` assertions.
- [ ] ONLY function signatures with KDoc comments.
- [ ] `tdd_signatures.md` is complete and approved.

This step is complete ONLY when ALL logical paths defined in the plan have a corresponding commented signature in the test files.

- **STRICTLY FORBIDDEN**: Writing actual implementation code during Phase 3.
- **MANDATORY**: Every signature MUST have clear Input/Expectation comments.



