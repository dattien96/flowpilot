# CA-369: Regression gate dismiss terminal action

## Summary

Changed the desktop regression-gate decision modal so it cannot hide an unresolved child gate locally. Option-bearing r-reg cards now expose `Stop flow`, which reuses the existing parent-and-child cascade-stop action; plain informational gate cards remain dismissible.

## Verification

- `npx tsx --test apps/desktop-flowpilot/src/components/gateBlockActions.test.ts`: 2 passed.
- `npm --prefix apps/desktop-flowpilot run build`: passed.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-291
change_type: bugfix
summary: Replace local regression-gate dismissal with a cascade Stop flow action to prevent orphaned child runs.
# --->8---
