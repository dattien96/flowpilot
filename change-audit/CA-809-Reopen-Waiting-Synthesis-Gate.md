# CA-809 — reopen parks when successor is WAITING or ghost RUNNING

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Reopen resume gate treats WAITING and ghost RUNNING successors as unfinished, not only PENDING
# --->8---

## Why

Live /open run-220036 after validate passed: synthesis `[-]` WAITING / ghost
RUNNING (hub reinvoke deferred, no turn). `pendingVibeResumeFromNode` only
matched empty/PENDING successors, so no resume UI.

## Change

- `vibeSuccessorNeedsResume`: PENDING, WAITING_USER_APPROVAL, or RUNNING
  with no live hub turn / child.
- `vibeNodeHasLiveWork` skips park while a real turn is in flight.

## Tests

New `ca809_reopen_waiting_synthesis_gate_test.go`. Old CA-804 PENDING path
untouched.

## Providers

Agnostic Case 1.

## Will not undo

CA-804 last-DONE park. CA-808 orchestration restore. Live hub turns.
