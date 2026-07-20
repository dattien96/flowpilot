# CA-364 — flow terminal UI and settle completion

## Summary

Fixed the CP-51 flow-terminal regressions observed in `run-2383`:

- A live child run is no longer rendered both as its persisted lifecycle card and as a second active banner.
- A terminal `done` graph is authoritative for the desktop projection, closing the dangling thinking indicator and the successful internal review-outcome tool row.
- The live post-gate path now schedules the same durable settle driver used by recovery, so terminal records do not remain `settle_pending` after a successful gate pass.
- Dispatch attention now distinguishes automatic terminal settlement from operator-required uncertain or repair decisions.
- Persisted replay now orders recovery-appended events by their durable observation time before rendering them, and a terminal replay closes every stale running tool row.

## Cross-Provider Parity

The fixes are in shared desktop state and shared runner lifecycle code. New runner coverage executes the live gate-pass settlement path for Codex, Claude, and Grok normalized adapters. The desktop replay and terminal-state regressions are also asserted for all three provider keys. No provider-specific branch was introduced.

## Verification

- Added new, isolated regression tests for duplicate live agent cards, terminal flow UI cleanup, and live gate-pass settlement.
- Existing tests remain unchanged and must stay green.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Deduplicate live agent cards and finalize terminal flow UI and dispatch settlement for Codex, Claude, and Grok.
# --->8---
