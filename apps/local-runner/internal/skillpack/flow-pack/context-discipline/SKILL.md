version: 1

# context-discipline

Always build on the newest entry in the feature history; never undo or duplicate prior work.

## Core Principle

The change ledger records the canonical history of every feature. Before writing any code or document, load the most recent entry for the relevant feature and treat it as the starting point.

## Rules

1. **Cite before you code.** At the start of every task, identify the `feature_key` (from `FEATURE-KEYS.md`) and the most recent `CA-NNN.md` entry for that key. State them explicitly.
2. **Never undo prior work.** If a prior entry records that something was implemented, do not re-implement it from scratch or overwrite it without a deliberate refactor decision captured in a new CA entry.
3. **Never duplicate prior work.** If the feature history shows a function, module, or behavior already exists, reuse or extend it — do not create a parallel copy.
4. **Cite the source doc id.** Every substantive decision in code or docs must be traceable to a `Task-NNN`, `BUG-NNN`, or `CP-NN` document. Include it in comments, commit messages, and CA entries.
5. **When in doubt, read history first.** Run `gitnexus_context` or inspect the change-audit ledger before assuming a feature is absent.

## What "Building on" Means

- Check the latest CA entry's `summary` to understand the current state.
- Extend, not replace: add new behavior alongside existing behavior unless the spec says to remove the old.
- If the new task contradicts a prior entry, surface the conflict to the user before proceeding.

## Why

Without context discipline, AI agents re-implement completed work, create divergent copies of the same feature, or silently undo previous fixes. This rule ensures the codebase converges rather than oscillates.
