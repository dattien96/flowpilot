# Requesting Code Review Examples

## Example 1: Review After Task Completion

### Scenario
Just completed Task 2: Add conversation verification function

### Step 1: Identify Review Scope
```bash
# Get the git range for this task
BASE_SHA=$(git rev-parse HEAD~1)  # Last commit from Task 1
HEAD_SHA=$(git rev-parse HEAD)    # Current commit (Task 2 complete)
```

### Step 2: Dispatch Reviewer Subagent

```kotlin
task(
    description = "Code Review - Task 2: Conversation Verification",
    subagent_type = "general-purpose",
    prompt = """
        You are the Code Reviewer for Task 2.

        CONTEXT:
        - What was implemented: Conversation verification and repair functions
        - Plan/Requirements: Task 2 from implementation_plan.md
          - Add verifyIndex() function to check conversation integrity
          - Add repairIndex() function to fix common issues
          - Write unit tests for both functions
        - Modified files:
          - ConversationVerifier.kt
          - ConversationRepairer.kt
          - ConversationVerifierTest.kt
          - ConversationRepairerTest.kt

        SKILLS TO READ (MANDATORY):
        - .claude/skills/testing/SKILL.md
        - .claude/skills/code-style/SKILL.md
        - .claude/skills/architecture/SKILL.md

        REVIEW CHECKLIST:
        1. Plan Compliance: Does implementation match Task 2 requirements?
        2. Test Coverage: Are there unit tests for verifyIndex() and repairIndex()?
        3. YAGNI: Any unnecessary features or over-engineering?
        4. DRY: Any code duplication?
        5. Architecture: Clean Architecture boundaries respected?
        6. Code Style: Function size < 40 lines, clear naming?

        OUTPUT FORMAT:
        ## Strengths
        [What was done well]

        ## Issues by Severity

        ### Critical (Blocks Progress)
        - [ ] Issue → Suggested fix

        ### Important (Fix Before Proceeding)
        - [ ] Issue → Suggested fix

        ### Minor (Note for Later)
        - [ ] Issue → Suggested fix

        ## Assessment
        [Ready to proceed / Needs fixes before Task 3]
    """
)
```

### Step 3: Reviewer Returns Feedback

```markdown
## Strengths
- Clean separation between Verifier and Repairer (SRP)
- Unit tests cover success and error paths
- Good use of sealed classes for issue types

## Issues by Severity

### Important (Fix Before Proceeding)
- [ ] Missing progress indicators during verification → Add LoadingScreen state to show user what's being checked
- [ ] Magic number (100) for reporting interval → Extract to `private const val REPORTING_INTERVAL_MS = 100L`

### Minor (Note for Later)
- [ ] Function `checkAndFix` is 45 lines → Consider splitting into `checkIssues()` and `applyFixes()`
- [ ] Variable name `tmp` in line 87 → Rename to `temporaryIndex` for clarity

## Assessment
Needs fixes before Task 3. Please address Important issues.
```

### Step 4: Act on Feedback

```kotlin
// Fix 1: Add progress indicators
sealed class VerificationState {
    object Loading : VerificationState()
    data class Verifying(val currentCheck: String) : VerificationState()
    data class Repairing(val issueType: IssueType) : VerificationState()
    data class Complete(val issuesFound: Int, val issuesFixed: Int) : VerificationState()
}

// Fix 2: Extract magic number
private const val REPORTING_INTERVAL_MS = 100L

class ConversationVerifier {
    // ... use REPORTING_INTERVAL_MS instead of 100L
}
```

### Step 5: Confirm and Proceed

```
✅ Fixed Important issues:
  - Added VerificationState with Loading/Verifying/Repairing/Complete states
  - Extracted REPORTING_INTERVAL_MS constant

⏳ Deferred Minor issues to batch fix later:
  - Split checkAndFix function (will do after Task 4)
  - Rename tmp variable (will do in refactor pass)

Proceeding to Task 3...
```

---

## Example 2: Review Before Merge (Final Gate)

### Scenario
All tasks complete, ready to merge feature branch to main

### Dispatch Final Review

```kotlin
task(
    description = "Final Code Review - Emotion Tracking Feature",
    subagent_type = "general-purpose",
    prompt = """
        You are the Code Reviewer for final merge approval.

        CONTEXT:
        - Feature: Emotion Tracking (Tasks 1-5 complete)
        - Branch: feature/emotion-tracking → main
        - Plan: implementation_plan.md (all 5 tasks)

        REVIEW SCOPE:
        - All commits since origin/main
        - Full feature integration

        MANDATORY CHECKS:
        1. ✅ All tasks from implementation_plan.md completed
        2. ✅ Unit tests for ALL UseCases, Repositories, ViewModels
        3. ✅ No Critical or Important issues outstanding
        4. ✅ Architecture boundaries respected
        5. ✅ No breaking changes to existing APIs
        6. ✅ Documentation updated (if needed)

        OUTPUT:
        ## Merge Readiness

        ### Task Completion
        - [ ] Task 1: ...
        - [ ] Task 2: ...
        - [ ] Task 3: ...
        - [ ] Task 4: ...
        - [ ] Task 5: ...

        ### Test Coverage
        - [ ] All UseCases have tests
        - [ ] All Repositories have tests
        - [ ] All ViewModels have tests

        ### Outstanding Issues
        - Critical: [count]
        - Important: [count]
        - Minor: [count]

        ### Recommendation
        [✅ Ready to Merge / ❌ Not Ready - list blockers]
    """
)
```

### Reviewer Returns

```markdown
## Merge Readiness

### Task Completion
- ✅ Task 1: Emotion data model created
- ✅ Task 2: EmotionTracker use case implemented
- ✅ Task 3: Repository and data source added
- ✅ Task 4: UI screen and components complete
- ✅ Task 5: Unit tests written (85% coverage)

### Test Coverage
- ✅ All UseCases have tests (EmotionTrackerUseCaseTest.kt)
- ✅ All Repositories have tests (EmotionRepositoryTest.kt)
- ✅ All ViewModels have tests (EmotionViewModelTest.kt)

### Outstanding Issues
- Critical: 0
- Important: 0
- Minor: 2 (naming suggestions, non-blocking)

### Recommendation
✅ **Ready to Merge**

Minor issues can be addressed in follow-up PR.
```

---

## Example 3: Review When Stuck (Fresh Perspective)

### Scenario
Stuck on bug: Conversation index keeps corrupting after repair

### Request Review for Fresh Perspective

```kotlin
task(
    description = "Code Review - Debug Index Corruption Bug",
    subagent_type = "general-purpose",
    prompt = """
        You are the Code Reviewer helping debug a tricky issue.

        PROBLEM:
        - Conversation index corrupts again after repair completes
        - Happens intermittently (~30% of repairs)
        - Cannot reproduce consistently

        CONTEXT:
        - Modified files in last 3 commits
        - Suspect: Race condition in repairIndex()
        - Already tried: Added logging, increased delay

        REQUEST:
        1. Review repairIndex() implementation
        2. Look for race conditions or async issues
        3. Check if state is being mutated unexpectedly
        4. Suggest debugging strategies

        FILES TO REVIEW:
        - ConversationRepairer.kt
        - ConversationVerifier.kt
        - Recent commits (HEAD~3..HEAD)
    """
)
```

### Reviewer Returns with Insight

```markdown
## Review Findings

### Root Cause Identified
Found potential race condition in `repairIndex()`:

```kotlin
// Line 67-72: Problem area
suspend fun repairIndex() {
    val currentIndex = repository.getIndex()  // ← Read
    val repaired = applyFixes(currentIndex)
    repository.saveIndex(repaired)            // ← Write
    // ← Another coroutine could modify between read/write!
}
```

### Suggested Fix
Use atomic compare-and-swap or mutex:

```kotlin
private val mutex = Mutex()

suspend fun repairIndex() {
    mutex.withLock {  // ← Prevent concurrent modifications
        val currentIndex = repository.getIndex()
        val repaired = applyFixes(currentIndex)
        repository.saveIndex(repaired)
    }
}
```

### Additional Finding
Line 45: `tmp` variable is actually a shared mutable state → Rename to `sharedIndexBuffer` and make private.

### Assessment
High confidence this is the issue. Implement mutex fix and test.
```

---

## Example 4: Review After Refactor

### Scenario
Just completed refactoring ProfileScreen (400 lines → 80 lines + 3 components)

### Request Refactor Review

```kotlin
task(
    description = "Code Review - ProfileScreen Refactor",
    subagent_type = "general-purpose",
    prompt = """
        You are the Code Reviewer for a refactor task.

        REFACTOR SCOPE:
        - Before: ProfileScreen.kt (400 lines, monolithic)
        - After: ProfileScreen.kt (80 lines) + 3 components

        REFACTOR PATTERNS APPLIED:
        - Extract Component: ProfileHeader, ProfileInfoSection, ProfileActionsSection
        - All components are internal visibility
        - Screen remains only component with ViewModel binding

        MANDATORY CHECKS FOR REFACTOR:
        1. ✅ No behavior changes (UI looks identical)
        2. ✅ All existing tests still pass
        3. ✅ No new public APIs exposed
        4. ✅ Components follow architecture-navigation skill
        5. ✅ Modifier parameter on all @Composable functions

        FILES CHANGED:
        - ProfileScreen.kt (reduced)
        - ProfileHeader.kt (new)
        - ProfileInfoSection.kt (new)
        - ProfileActionsSection.kt (new)
    """
)
```

### Reviewer Returns

```markdown
## Refactor Review

### Verification Results

✅ No behavior changes
- UI matches before/after (tested on light/dark theme)
- All user interactions work identically

✅ All tests pass
- ProfileScreenTest.kt: 12/12 passing
- No test modifications needed (good sign!)

✅ Architecture compliance
- ProfileHeader: internal, /components/ ✓
- ProfileInfoSection: internal, /components/ ✓
- ProfileActionsSection: internal, /components/ ✓
- ProfileScreen: public, /screen/, ViewModel binding ✓

✅ Code style
- All @Composable functions have modifier parameter
- Function sizes: 15-25 lines (well under 40 limit)
- Clear naming following conventions

### Assessment
✅ **Refactor Successful**

No issues found. Ready to proceed.
```

---

## Pre-Review Checklist Template

Before requesting any code review, verify:

```markdown
## Pre-Review Checklist

- [ ] Code compiles without errors
- [ ] All new logic has unit tests
- [ ] No TODO comments left in code
- [ ] Imports are organized
- [ ] Modified files are staged/committed
- [ ] Git diff is clean (no accidental changes)
- [ ] Linting passes (no warnings)
```
