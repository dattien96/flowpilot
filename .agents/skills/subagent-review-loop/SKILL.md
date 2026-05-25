---
name: subagent-review-loop
description: Orchestrate a main-agent planning and review loop that delegates implementation to a coding sub-agent, then iterates until remaining bugs and review findings are resolved.
---

# Subagent Review Loop

Use this skill when a task should be handled as a controlled loop:
main agent plans, `coder-agent` codes, `reviewer-agent` audits, and the process repeats until the implementation is clean.

## Workflow

1. Main agent defines the goal, constraints, exact files, and acceptance criteria.
2. Main agent checks impact and risk before any code change.
3. Main agent delegates the bounded implementation to `coder-agent`.
4. `coder-agent` edits only the assigned scope and reports changed files, tests run, and open risks.
5. Main agent starts `reviewer-agent` and waits for the result.
6. If `reviewer-agent` returns pass, main agent runs `audit-logging-skill` and records the completed scope in `change-audit/`.
7. If `reviewer-agent` returns findings, main agent sends a narrow fix request back to `coder-agent`.
8. Repeat from step 4 until `reviewer-agent` returns pass.

## Delegation Rules

- Use `coder-agent` for concrete implementation work, not for open-ended exploration.
- Use `reviewer-agent` for audit only.
- Give each agent explicit ownership of files or modules.
- Do not overlap file ownership across agents.
- Keep each agent task small enough to finish in one pass.
- The main agent keeps control of scope, acceptance, and final approval.

## Main Agent Responsibilities

- Gather just enough context to make the next decision.
- Run impact analysis before editing any symbol.
- Decide whether `coder-agent`, `reviewer-agent`, or another coding pass is needed.
- Do not perform the final code review yourself; wait for `reviewer-agent`.
- Use `audit-logging-skill` after the review passes and before the final commit or handoff.
- Stop only when `reviewer-agent` returns pass, the audit note is updated, and no further issues are found.

## Sub-Agent Prompt Template

Use a prompt like this for `coder-agent`:

> Implement only the assigned files and scope. Do not touch unrelated files. Make the smallest correct change, run targeted tests if possible, and report the files changed, what was verified, and any remaining risks.

Use a prompt like this for `reviewer-agent`:

> Review the implementation against the plan and acceptance criteria. Focus on correctness, regressions, missing tests, and boundary violations. Return only actionable findings or a clean pass.

## Review Loop Template

Use this after the sub-agent returns:

1. Check whether the diff matches the assigned scope.
2. Verify behavior against the acceptance criteria.
3. Check for regressions, missing tests, and edge cases.
4. If anything is off, issue a targeted fix request to `coder-agent`.
5. Otherwise, update `change-audit/` via `audit-logging-skill` and then finalize.

## Good Fit

- Feature work with a clear implementation boundary
- Bug fixes that need one coder pass and one reviewer pass
- Refactors where the main agent must keep control of scope

## Not a Good Fit

- Pure research or brainstorming
- Tasks that need no delegation
- Ambiguous work with no stable file ownership
