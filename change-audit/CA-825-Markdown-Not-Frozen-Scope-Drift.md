# CA-825 — frozen-scope gate ignores markdown

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-370
change_type: bugfix
summary: Frozen coder-gate skips *.md and requirements/** so FEATURE-KEYS.md and tdd-signatures.md do not park; extra.go and flow-rules.json still drift
# --->8---

## Why

Live run-678326 parked on `FEATURE-KEYS.md` and
`requirements/.flowpilot/vibe/tdd-signatures.md`. The coder then asked
"What is the FlowPilot failure blocking Continue?" on every retry.

Operator: markdown is not product code. Ignore it on this gate.

## Change

- `IsMarkdownDocPath` in changecontract
- `gate_hook` codeOnlyWritten skip
- `TestBUG327_FeatureKeysWriteStillBlocks` now expects no park (operator AC)
- `TestBUG366_FeatureKeysAllowDoesNotRepark` first-write no longer parks

## Tests

New `TestBUG370_*`. extra.go and flow-rules.json still park.

## Providers

Agnostic Case 1.

## Will not undo

CA-427 flow-rules.json still drifts. CA-* notes still exempt. BUG-366 Allow
for non-md extras (Makefile still 422).
