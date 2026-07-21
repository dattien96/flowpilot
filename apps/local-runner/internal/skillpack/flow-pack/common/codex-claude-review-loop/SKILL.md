---
name: codex-claude-review-loop
description: Run a Codex-orchestrated implementation loop where Claude Sonnet medium writes or fixes code, Codex 5.6 Terra reviews the result, and Claude is called again for every confirmed issue until Codex reports no blocking issues. Use when a task, bug fix, or review cycle should be implemented by Claude and gated by Codex review.
version: 6
---

# Codex Claude Review Loop

## Overview

Use this skill when Codex should coordinate the work but Claude Sonnet medium should be the implementation engine. Codex 5.6 Terra is the review gate and the loop only stops after Codex returns OK with no blocking issues.

## Agent Roles

| Phase | Responsibility | Command |
| :--- | :--- | :--- |
| Implementation | Plan and edit code | `claude -p --model sonnet --effort medium` |
| Local verification | Run focused tests/build checks | Current agent or Claude, depending on context |
| Review gate | Find remaining bugs, regressions, missed requirements, or test gaps | `codex exec -m gpt-5.6-terra -c model_reasoning_effort="high"` |
| Loop | Feed every confirmed Codex finding back to Claude | Claude + Codex |

## Workflow

1. Capture the user request verbatim, including acceptance criteria, bug symptoms, and any required files.
2. Gather minimal repository context needed for Claude to implement safely.
3. Start implementation with Claude Sonnet medium:

```bash
claude -p --model sonnet --effort medium --permission-mode acceptEdits "You are the implementation owner. Implement this request in the current repository: <USER_REQUEST>. Follow repo instructions, keep changes focused, run or name the relevant verification, and report changed files plus verification results."
```

4. Inspect the changed files and run focused verification locally when Claude did not already do so.
5. Review with Codex 5.6 Terra:

```bash
codex exec -m gpt-5.6-terra -c model_reasoning_effort="high" "Review the implementation for this request: <USER_REQUEST>. Inspect changed files and tests. Identify remaining bugs, missed requirements, regressions, insufficient tests, edge cases, or repo-rule violations. Return OK only if there are no blocking issues; otherwise list actionable findings."
```

6. After every Codex review, append the review outcome to `review_result.md` at the repository root. Create the file if it does not exist. Include the review pass number, reviewer, request summary, and either `OK` or the full actionable findings.
7. If Codex reports any blocking or plausible actionable finding, send those findings back to Claude Sonnet medium:

```bash
claude -p --model sonnet --effort medium --permission-mode acceptEdits "Codex review found these issues in the implementation: <CODEX_FINDINGS>. Fix every confirmed issue, reject only with concrete repository evidence, run focused verification, and summarize changed files plus verification results."
```

8. Repeat Codex review after every Claude fix. Do not finish while Codex still reports unresolved blocking issues.
9. Finish only when Codex returns OK or when every remaining finding is explicitly disproven with evidence.

## Operating Rules

- Treat Codex review findings as the gate, not as suggestions to ignore silently.
- After each review pass, persist the exact Codex result by appending it to `review_result.md` in the repository root. Never overwrite earlier passes within the same task.
- Feed the exact Codex findings into the next Claude turn so the loop has continuity.
- Reject a Codex finding only after inspecting the code and naming the evidence.
- Keep each loop bounded by evidence. If the same ambiguity repeats, ask the user for missing requirements or reproduction details.
- Follow all repo instructions before edits and before commits.
- In the final response, report the Claude implementation pass, Codex review result, loop count, verification run, files changed, and residual risk.





