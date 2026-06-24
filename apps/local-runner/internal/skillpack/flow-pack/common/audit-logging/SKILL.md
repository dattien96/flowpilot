---
name: audit-logging
description: Write or update a change-audit note after any code change so the ledger stays complete.
version: 2
---

# audit-logging

After ANY code change, write a change-audit note.

## Trigger

This skill activates after every code change that modifies, adds, or deletes production or test files.

## What to Write

Create or append to `change-audit/CA-NNN.md` (increment NNN from the highest existing CA number).

### Required Block (flowpilot:change-ledger §13)

```markdown
---
flowpilot:change-ledger:
  feature_key: <value from FEATURE-KEYS.md>
  source_doc_id: <Task-NNN | BUG-NNN | CP-NN>
  change_type: <feature | bugfix | refactor | docs | hotfix | other>
  summary: <one-line description of what changed>
  committed_at: <ISO-8601 timestamp>
  confidence: <high | low>
---
```

## Rules

1. Look up the correct `feature_key` in `FEATURE-KEYS.md` before writing the block — do not invent keys.
2. `source_doc_id` must match an existing document in `requirements/`.
3. `confidence` is `high` when the change is well-scoped to a single feature; `low` when the change is speculative or touches multiple features.
4. One CA file per logical change (a task, a bug fix, a refactor session). Do not create a new CA file for trivial follow-up edits within the same session.
5. The CA file must be committed in the same commit as the code change.

## Why

The change-audit trail feeds the Context & Regression Engine (SD-17). Without it, feature history is incomplete and the regression oracle cannot track deltas.
