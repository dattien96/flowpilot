# CA-804 — /open parks resume from last completed node

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Reopen a vibe flow parks an OK/Cancel resume gate at the last DONE node with a pending successor (coder→validate), not idle
# --->8---

## Why

Live /open run-220036 after coder DONE: no gate, no Thinking, validate `[ ]`. maybePark only armed tdd→coder when coder had not started.

## Change

- `pendingVibeResumeFromNode` + `maybeParkVibeResumeConfirm`: last DONE node whose forward target is pending.
- Store `vibeResumeFromNode`. GateReason `Resume from {node}?`
- OK: tdd → existing coder resume; otherwise `tryAdvanceFlowFromNode(from)` after Cancelled heal.

## Tests

New `ca804_reopen_resume_from_last_node_test.go`. Old CA-801 tdd-only park tests untouched.

## Providers

Agnostic Case 1.

## Will not undo

CA-801 tdd pause gate. CA-803 heal/Retry. BUG-288 Cancelled terminal.
