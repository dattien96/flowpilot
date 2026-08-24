---
version: 6
name: add-new-task
description: Use when the user invokes `$add-new-task` or asks for a new Android/KMP task or feature slice that should be planned, implemented, tested, and closed out through the repo's normal workflow artifacts.
---

# Add New Task

Use this skill to take a scoped Android task from request to completion.

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

- Small, clear task: use the lean workflow.
- Multi-file or cross-layer task: use the hybrid workflow.
- Ambiguous, risky, or broad task: use the multi-agent workflow and `feature-orchestrator`.

If the request is missing the key facts needed to proceed, stop and ask for clarification before coding.

## 3. Plan The Task

1. Capture the goal, scope, and constraints.
2. Build or update `task.md` when the repo workflow expects it.
3. Create `4c_summary.md` if the request is ambiguous.
4. If the task is large, produce `implementation_plan.md` before coding.
5. Keep the task narrow and traceable.

Do not convert a task into a broad redesign unless the user explicitly asked for that.

## 4. Implement The Task

1. Make the requested code changes.
2. Add or update tests.
3. Keep changes inside the relevant Android/KMP boundaries.
4. Preserve the repo's layer rules and test requirements.

## 5. Finish The Work

After the change is in place:

- run the relevant tests
- update `skill_audit.md` for the changed files if the workflow uses it
- update `walkthrough.md` with the result and verification summary
- update docs in `docs/` if the behavior change affects documented usage

If the work is large enough to require delegated phases, keep the repo's `planner` -> `coder-agent` -> `reviewer-agent` flow intact.

## 6. Force Rules

- Do not skip `android/.codex/AGENTS.md`.
- Do not start coding before the workflow choice and scope are clear.
- Do not treat a task as complete without tests or a clear verification note.
- Do not leave the user with only code when the repo expects task, audit, or walkthrough artifacts.
- Do not silently expand a task into unrelated feature work.
