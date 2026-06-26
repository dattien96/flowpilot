# CA-128 — r-task Rule: Missing Task Doc Reprompt

- Feature: `context-regression-engine`
- Date: 2026-06-24
- Author: DatNguyen
- Related: `Task-113`, `CP-35`, `CA-126` (r-bug / r-ca reprompt work)

## What Changed

Added `r-task` as the 6th default Flow Gate rule. When the AI's final message references a `Task-NNN` identifier but no Task document appears in the git diff, the gate issues a `reprompt` with explicit file-creation instructions naming the exact path and FORMAT-REFERENCE to follow.

## Files Changed

| File | Change |
|------|--------|
| `apps/local-runner/internal/flowgate/rules.go` | Added r-task rule definition |
| `apps/local-runner/internal/flowgate/observe.go` | Added `HasTaskDoc()` with FORMAT-REFERENCE guard |
| `apps/local-runner/internal/flowgate/evaluate.go` | Added `taskIDRegex`, `task_referenced` case in `checkRule` |
| `apps/local-runner/internal/flowgate/enforce.go` | Added `task_referenced` case in `remediationFor()` |
| `apps/local-runner/internal/flowgate/flowgate_test.go` | Updated `TestDefaultRules` (5→6); added 4 r-task tests |

## Why

The same enforcement gap that existed for r-bug (AI completes a task but forgets the BugFix doc) existed symmetrically for Task docs. AI steps that close tracked tasks (e.g., "Completed Task-113 implementation") were not required to produce the corresponding Task doc.

## Design Notes

- Trigger heuristic: `\bTask-\d+\b` regex on `FinalMessage` — same pattern as r-bug's message scan
- FORMAT-REFERENCE guard on `HasTaskDoc` prevents scaffold-created templates from satisfying the predicate (proactive application of the BUG-141 lesson)
- Reprompt text explicitly says "Do NOT edit the change-audit note" to avoid the AI misidentifying an existing doc as the solution

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: TASK-113
change_type: feature
summary: r-task Rule: Missing Task Doc Reprompt
# --->8---
