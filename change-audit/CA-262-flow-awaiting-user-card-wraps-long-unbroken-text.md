# CA-262: Flow Awaiting-User Card Wraps Long Unbroken Text

## Scope

Fixed BUG-264, found live during CP-36 Scenario 11 testing (`run-11120`): the "Needs your decision" card's escalation text appeared cut off with no visible way to read the rest, because a long unbroken token (e.g. a git diffstat line) overflowed the card's width instead of wrapping — the existing vertical scroll (`max-height`/`overflow-y: auto`) was already correct but didn't help since the content was clipped horizontally, not vertically.

## Changes

- `styles.css`: `.flow-awaiting-user-detail` gained `overflow-wrap: anywhere` (wraps long unbroken tokens) and `white-space: pre-wrap` (preserves the source text's own line breaks).

## Verification

- CSS-only change; no automated test covers rendered text wrapping in this repo. Reviewed against the standard fix for this failure mode; a live visual re-test is recommended on the next Scenario 11 pass.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-264
change_type: bugfix
summary: wrap long unbroken tokens and preserve line breaks in the flow-awaiting-user card's escalation text so it reads correctly within its existing scroll area
# --->8---
