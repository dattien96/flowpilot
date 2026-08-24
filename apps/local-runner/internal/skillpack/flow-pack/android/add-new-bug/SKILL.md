---
version: 6
name: add-new-bug
description: Use when the user invokes `$add-new-bug` or reports a bug, regression, broken workflow, or incorrect behavior in the Android/KMP repo and wants the issue investigated, fixed, tested, and wrapped up with the repo's normal planning, audit, and walkthrough artifacts.
---

# Add New Bug

Use this skill to take a bug from report to fix inside the Android repo workflow.

## 1. Start With Repo Context

Read these first:

- `android/.codex/AGENTS.md`
- `shared-rules/entry_instrution/entry-android.md`
- `shared-rules/workflows/workflow-selection-guide.md`
- `shared-rules/skills/planner/SKILL.md`
- `shared-rules/skills/testing/SKILL.md`
- `shared-rules/skills/requesting-code-review/SKILL.md`

Then inspect the smallest relevant set of files:

- current code in `android/`
- any related `task.md`
- any related `implementation_plan.md`
- any related `skill_audit.md`
- any related `walkthrough.md`

## 2. Choose The Right Workflow

Use the repo's workflow guide, not ad hoc judgment.

- Small, clear, low-risk bug: use the lean workflow.
- Multi-file or cross-layer bug: use the hybrid workflow.
- Ambiguous, risky, or broad bug: use the multi-agent workflow and `feature-orchestrator`.

If the report is missing the key facts needed to proceed, stop and ask for clarification before coding.

## 3. Fix The Bug

1. State the symptom, expected behavior, actual behavior, and impact.
2. Reproduce or confirm the issue from the current code.
3. Find the root cause.
4. Make the smallest correct fix that resolves the bug end to end.
5. Add or update tests for the regression.
6. Keep the fix inside the relevant Android/KMP layer boundaries.

If the bug reveals a broader design or behavior issue, flag it and update the supporting plan/docs rather than hiding the change only in code.

## 4. Finish The Work

After the fix is in place:

- run the relevant tests
- update `skill_audit.md` for the changed files if the workflow uses it
- update `walkthrough.md` with the result and verification summary
- update docs in `docs/` if the behavior change affects documented usage

If the work is large enough to require delegated phases, keep the repo's `planner` -> `coder-agent` -> `reviewer-agent` flow intact.

## 5. Force Rules

- Do not skip `android/.codex/AGENTS.md`.
- Do not start coding before the workflow choice is clear.
- Do not treat a bug fix as complete without tests or a clear verification note.
- Do not leave the user with only a code patch when the repo expects audit or walkthrough artifacts.
- Do not silently expand a bug into unrelated feature work.
