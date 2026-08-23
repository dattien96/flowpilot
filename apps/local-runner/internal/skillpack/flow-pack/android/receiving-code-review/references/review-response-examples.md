# Receiving Code Review Examples

## Example 1: Handling Multi-Item Feedback

### Review Feedback Received

```markdown
## Issues Found

### Critical
1. Missing unit tests for GetConversationUseCase

### Important
2. Function `procData` is unclear - rename
3. Magic number 300 in line 45
4. Missing error handling in repository call

### Minor
5. Import order not standardized
6. Comment on line 78 doesn't add value
```

### ❌ WRONG Response
```
You're absolutely right! Great catches! Let me fix all of these now.
Thanks for the detailed review!

[Starts fixing items 1,2,3,6 without asking about 4,5]
```

### ✅ RIGHT Response

```
Understand items 1,2,3,6. Need clarification on 4 and 5 before proceeding.

**Item 4 (Missing error handling)**: Which repository call? There are 3 repository calls in the diff:
- `repository.getConversation()` - already has try/catch (line 22)
- `repository.saveMetadata()` - no error handling (line 35)
- `repository.updateIndex()` - has Result wrapper (line 50)

**Item 5 (Import order)**: Currently using default Android Studio import layout. Should I switch to project-specific import order? If so, what's the config?

Will proceed with items 1,2,3,6 after clarification.
```

---

## Example 2: Verifying Before Implementing

### Review Feedback

```markdown
### Important
- Remove this legacy API support (lines 45-67). It's unused and complicates the code.
```

### ❌ WRONG Response
```
Good point! Let me remove that legacy code now.
[Deletes code without checking]
```

### ✅ RIGHT Response

```
Checking legacy API usage...

**Findings:**
- API requires Android 13+ (current minSdk is Android 10)
- Used by: `LegacyCompatHelper.kt` (called from 2 places)
  - `OnboardingFlow.kt` line 34
  - `MigrationWorker.kt` line 89

**Options:**
A) Keep legacy support (needed for Android 10-12 users, ~15% of base)
B) Fix the bundle ID issue and drop pre-13 support (breaking change)
C) Extract to separate module for optional inclusion

Current implementation has wrong bundle ID - should fix regardless.

Which direction should we go?
```

---

## Example 3: YAGNI Check

### Review Feedback

```markdown
### Important
- Implement proper metrics tracking with database storage, date filters, and CSV export functionality
```

### ❌ WRONG Response
```
Excellent idea! I'll implement a full metrics system with all those features.
```

### ✅ RIGHT Response

```
Grepped codebase for metrics usage:

**Findings:**
- `trackMetric()` is called in 0 places
- No UI components read metrics data
- No analytics or reporting features in current roadmap

**Question:** This endpoint isn't called anywhere. Should we:
A) Remove it (YAGNI) - can add when actually needed
B) Keep minimal stub for future implementation
C) There's usage I'm missing?

If removing, I'll delete:
- MetricsRepository.kt
- MetricsDao.kt
- database/metrics_table.sql
- trackMetric() call in ViewModel
```

---

## Example 4: Pushing Back with Technical Reasoning

### Review Feedback

```markdown
### Important
- This function is too long (35 lines). Split it into smaller functions.
```

### ❌ WRONG Response
```
You're right, let me split it up!
```

### ✅ RIGHT Response

```
Function is 35 lines. Checking code-style-best-practices skill:

**Skill limit:** Functions should be ≤40 lines (current is within limit)

**Current function:** `renderContent()` handles state routing (when-expression with 5 branches)

**Analysis:**
- Single responsibility: routing based on BaseScreenState
- Each branch is 5-7 lines (already small)
- Splitting would create 5 separate functions called from one place

**Trade-off:**
- Current: Clear state machine, easy to follow flow
- Split: More functions, but each branch isolated

Should I:
A) Keep as-is (within skill guidelines, single responsibility)
B) Split into `renderLoading()`, `renderError()`, `renderSuccess()`, etc.?

If B, what should the parent function be named? `routeToScreen()`?
```

---

## Example 5: Correct Feedback Acknowledgment

### Review Feedback

```markdown
### Critical
- Missing unit tests for GetProfileUseCase
```

### ❌ WRONG Response
```
You're absolutely right! Thanks for catching that! Let me add tests now.
```

### ✅ RIGHT Response

```
Added. Created GetProfileUseCaseTest.kt with 5 test cases:

1. `invoke - returns success with valid data`
2. `invoke - returns error when repository fails`
3. `invoke - returns error with null data`
4. `invoke - verifies repository call`
5. `invoke - handles exception thrown by repository`

File: features/domain/usecase/GetProfileUseCaseTest.kt
```

---

## Example 6: Gracefully Correcting Your Pushback

### Scenario
You pushed back on feedback, reviewer provided more context, you were wrong

### Reviewer Follow-up

```
Actually, line 35 `repository.saveMetadata()` is NOT wrapped in try/catch.
If it throws, the entire transaction fails silently.

See: Crash report #4521 - NullPointerException in saveMetadata
```

### ❌ WRONG Response
```
Oh no! I'm so sorry! I totally missed that! I should have checked more carefully!
I feel terrible about this. Let me fix it right away with a huge apology!
```

### ✅ RIGHT Response

```
You were right - I checked line 35 and it does throw without catch.
My initial check was incomplete (only looked at line 22).

Adding try/catch around saveMetadata() call. Fixed in commit abc123.
```

---

## Example 7: Handling Unclear Scope

### Review Feedback

```
Fix items 1-8 in the authentication flow.
```

### ❌ WRONG Response
```
Will do! [Starts implementing without clarification]
```

### ✅ RIGHT Response

```
I understand items 1,2,3,5,6,8. Need clarification on 4 and 7 before implementing.

**Item 4**: "Improve error messages" - Which errors specifically?
- Network errors? (currently shows "Connection failed")
- Auth errors? (currently shows "Invalid credentials")
- Server errors? (currently shows "Server error")

**Item 7**: "Add loading state" - Where?
- During login attempt? (already has LoadingScreen)
- During password reset? (currently no loading indicator)
- Both?

Once clarified, I'll implement all 8 items in order.
```

---

## Example 8: External Reviewer (CI Bot) Feedback

### CI Bot Feedback

```
❌ CI Check Failed: Function complexity too high (cyclomatic: 15, limit: 10)
```

### ❌ WRONG Response
```
The bot is wrong! This function is fine!
```

### ✅ RIGHT Response

```
Checking function complexity...

**Function:** `processUserInput()` - 15 branches
**Current structure:** Single function with when-expression (15 cases)

**Options to reduce complexity:**
A) Extract each case to separate function (15 small functions)
B) Group related cases (5 medium functions, ~3 branches each)
C) Use Strategy pattern (15 strategy classes - overkill?)

**Context:** This is a parser for user commands. Each case handles a different command type.

**Recommendation:** Option B - group by command category:
- `processNavigationCommands()` (3 cases)
- `processDataCommands()` (4 cases)
- `processSettingsCommands()` (3 cases)
- etc.

Implementing Option B. Will create 5 category functions.
```

---

## Example 9: Conflicting Feedback

### Scenario
External reviewer suggests change that conflicts with Architect's prior decision

### Review Feedback

```
### Important
- Remove Koin dependency injection, use Hilt instead (standard for Android)
```

### ❌ WRONG Response
```
Good point! Hilt is the standard. Let me migrate to Hilt.
```

### ✅ RIGHT Response

```
Checking architectural decisions...

**Finding:** Architect's decision in `implementation_plan.md` Section 3.2:

> "Using Koin instead of Hilt for this project because:
>  - Multiplatform support (iOS share DI framework)
>  - Lighter weight (Hilt adds 2MB to APK)
>  - Team already familiar with Koin"

**Conflict:** This feedback conflicts with architectural decision.

**Action:** Keeping Koin as per Architect's decision.

@Architect - Please confirm if this decision still stands or if we should revisit Hilt migration.
```

---

## Example 10: GitHub Thread Reply

### Scenario
Reviewer left inline comment on PR, need to reply

### ❌ WRONG Response
```
[Posts as top-level PR comment]
Fixed this in commit abc123!
```

### ✅ RIGHT Response

```bash
# Reply in the comment thread using GitHub API
gh api repos/{owner}/{repo}/pulls/{pr}/comments/{comment_id}/replies \
  -X POST \
  -F body="Fixed in commit abc123. Extracted to private const val."
```

Or manually: Click "Reply" on the inline comment, not "Add general comment".

---

## Response Pattern Quick Reference

| Situation | ❌ WRONG | ✅ RIGHT |
|-----------|----------|----------|
| Correct feedback | "You're right! Thanks!" | "Fixed. [Brief description]" |
| Unclear item | [Implement anyway] | "Need clarification on [X]" |
| Disagree | "You're wrong" | "Checking... [technical reasoning]" |
| YAGNI suggestion | [Implement everything] | "Grepped - unused. Remove it?" |
| Was wrong in pushback | Long apology | "Verified - you're right. Fixing." |
| Conflicting feedback | [Blindly follow] | "Conflicts with [decision]. Confirm?" |

---

## The Bottom Line

**External feedback = suggestions to evaluate, not orders to follow.**

1. **Verify** against codebase reality
2. **Question** if something seems wrong
3. **Implement** after confirmation
4. **No performative agreement**
5. **Technical rigor always**
