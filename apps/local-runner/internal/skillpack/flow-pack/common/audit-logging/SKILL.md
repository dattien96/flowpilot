---
name: audit-logging
description: Write or update a change-audit note after any code change so the ledger stays complete.
version: 6
---

# audit-logging

After ANY code change, write a change-audit note.

## Trigger

This skill activates after every code change that modifies, adds, or deletes production or test files.

## Step 1 — Pick or register a feature_key

1. Open `change-audit/FEATURE-KEYS.md`.
2. Find the key whose description best matches this change.
3. **If no existing key fits**, append a new line to `FEATURE-KEYS.md` before continuing:
   ```
   - my-new-feature — short description of the feature area
   ```
   Keys must be kebab-case. Do not delete or rename existing keys (mark retired ones `superseded by <new-key>` instead).

## Step 2 — Create the CA note

Create `change-audit/CA-NNN.md` (increment NNN from the highest existing CA number).

At the bottom of the file add the required ledger block exactly as shown — the parser reads this format literally:

```
# ---8<--- flowpilot:change-ledger
feature_key: <key from FEATURE-KEYS.md>
source_doc_id: <Task-NNN | BUG-NNN | CP-NN>
change_type: <feature | bugfix | refactor | docs | hotfix | other>
summary: <one-line description of what changed>
# --->8---
```

## Rules

1. Always resolve `feature_key` from `FEATURE-KEYS.md`. If none fits, add the key there first (Step 1), then use it.
2. `source_doc_id` must match an existing document in `requirements/`.
3. One CA file per logical change (a task, a bug fix, a refactor session). Do not create a new CA file for trivial follow-up edits within the same session.
4. The CA file and any `FEATURE-KEYS.md` addition must be committed in the same commit as the code change.

## Why

The `flowpilot:change-ledger` block is parsed by the Context & Regression Engine (`enrich.go`) to assign `feature_key` and `source_doc_id` to each commit with `confidence: high`. Without it the ledger falls back to path-guessing (`confidence: low`), and the NL feature resolver loses accuracy.
