---
name: git-commit-skill
description: Use this skill PROACTIVELY when preparing commit messages or finalizing tasks to enforce strict formatting like [Feature], [BugFix], or [Refactor].
---

# A. What is this skill for?

Use this skill PROACTIVELY to ensure all Git commit messages follow the project's standardized prefix system. Consistent commit formatting improves changelog readability and helps track feature vs. fix distributions.

# B. What are the tasks?

## 1. Commit Message Drafting
Apply the correct prefix based on the nature of the changes (Feature, BugFix, Refactor, etc.).

## 2. Formatting Audit
Verify that manual commit message drafts satisfy the concise, English-only, and accurately descriptive requirements.

# C. Instructions

## 1. Mandatory Prefixes
Every commit message MUST start with one of:
- `[Feature]: xxxx` - New feature addition.
- `[BugFix]: xxx` - Code bug fix.
- `[HotFix]: zzz` - Urgent production fix.
- `[Docs]: xxx` - Documentation updates.
- `[Refactor]: xxx` - Structural changes without logic modification.
- `[Test]: xxx` - Unit Test additions or modifications.

## 2. General Rules
- **Language**: Use English consistently.
- **Tone**: Be concise and accurate.
- **Authorization**: Do NOT use custom prefixes without explicit user approval.

# D. Examples

- `[Feature]: Implement user profile screen`
- `[BugFix]: Fix crash in landing page navigation`
- `[Refactor]: Extract common widgets to appdesign module`

# H. FORCE rules

- **FORCE**: Final step of any task MUST involve a commit check if changes were made.
- **CRITICAL**: No commits without authorized prefixes.
- **FORCE**: Remove any `Co-authored-by` lines and DO NOT include AI authorship attribution in commit messages.

# I. Notes

- Commit messages should be succinct (max 72 characters for the first line).



