---
version: 6
name: common-mistakes-skill
description: Use this skill PROACTIVELY as a self-check before starting any new task or during the final review phase to avoid common AI agent pitfalls like missing context or overlapping tasks.
---

# A. What is this skill for?

Use this skill PROACTIVELY to prevent systemic errors in task execution and review. It serves as a quality gate to ensure the agent has sufficient context, clear boundaries, and appropriate verification loops before committing to changes.

# B. What are the tasks?

## 1. Context Verification
Ensure the 4Cs (Context, Command, Constraints, Criteria) are fully understood before starting work.

## 2. Request Decomposition
Identify and break down "crammed" requests into manageable, single-objective sub-tasks.

## 3. Workflow Validation
Enforce the use of review loops and ensure that failure is met with targeted fixes rather than global restarts.

## 4. Assumption Sanity Check
Identify implicit assumptions about business logic or API contracts and seek clarification immediately.

# C. Instructions

## ⚠️ Common Mistakes Checklist

Immediately self-check for these pitfalls:

### 1. Missing Context
- **Sign**: Vague request or unclear module/package.
- **Action**: Stop and ask for the 4Cs.

### 2. Overlapping Tasks
- **Sign**: Multiple verbs like "and" or "simultaneously."
- **Action**: Decompose into separate small tasks.

### 3. Skipping Review
- **Sign**: Using the first result without iteration.
- **Action**: Iterate through 1-2 review cycles per task.

### 4. Unclear Completion Criteria
- **Sign**: "Done" is subjective or undefined.
- **Action**: Confirm technical completion criteria (tests, lint, coverage).

### 5. Infinite Failure Loops
- **Sign**: Redoing everything after an error instead of fixing the root cause.
- **Action**: Analyze the failure and rewrite only the offending section (layer/logic/naming).

### 6. Implicit Assumptions
- **Sign**: Assuming API contracts or DB schemas without verification.
- **Action**: Ask for documentation or search the codebase for the source of truth.

# H. FORCE rules

- **FORCE**: The Planner Agent MUST run this checklist when receiving a request.
- **ENFORCE**: The Reviewer Agent MUST run this checklist when evaluating output.

# I. Notes

- This skill is primarily a cognitive self-check for the AI agent rather than a coding rulebook.



