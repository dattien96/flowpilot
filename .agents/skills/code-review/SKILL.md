---
version: 6
name: code-review-skill
description: Use this skill PROACTIVELY after completing tasks, implementing major features, or before merging to verify work meets requirements and catch issues early via the reviewer subagent.
---

# Code Review Skill

Use this skill after completing tasks, implementing major features, or before pushing/merging changes.

## Tasks

1. Identify the review scope from the modified files, git diff, implementation plan, or explicit user request.
2. Delegate independent review to `reviewer-agent` when the change is non-trivial.
3. Triage findings into Critical, Important, and Minor.
4. Fix all Critical findings before continuing.

# A. What is this skill for?

Use this skill PROACTIVELY to dispatch a specialized `reviewer-agent` subagent. It ensures that internal quality checks (testing, style, architecture) are performed independently before work is merged or before the next sub-task begins.

# B. What are the tasks?

## 1. Review Scope Identification
Define the git range or specific files modified that require evaluation.

## 2. Reviewer Subagent Dispatch
MANDATORY: Launch the `reviewer-agent` via the Task tool with a detailed context prompt.

## 3. Feedback Triage
Categorize feedback into Critical, Important, and Minor severities based on implementation plan compliance and quality standards.

# C. Instructions

## 1. Review Scope

- Include modified source files.
- Include matching tests.
- Include `implementation_plan.md`, `tdd_signatures.md`, and `logic_sync_report.md` when they exist.
- Include user requirements or acceptance criteria when available.

## 2. Delegation Protocol

Use `reviewer-agent` for independent review. Provide:

- the exact files to inspect
- the approved plan or task summary
- the test expectations
- known constraints and out-of-scope areas
- the expected output format: findings by severity plus pass/fail recommendation

## 3. Severity Actions

- Critical: fix immediately; blocking.
- Important: fix before the next task or push.
- Minor: batch later if non-blocking.

# D. References

- Scenario guidance: [review-request-examples.md](./references/review-request-examples.md)

# E. Force Rules

- Do not rely only on self-review for non-trivial implementation work.
- Fix all Critical issues immediately.
- Implementation must match the approved plan unless the user approves a change.


