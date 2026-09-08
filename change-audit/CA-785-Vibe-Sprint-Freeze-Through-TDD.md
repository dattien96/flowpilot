# CA-785 — vibe-sprint freeze binds coder through TDD

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: contract.freeze no longer escalates when TDD (agent.delegate) sits between freeze and coder; bind coder and follow the done-edge so TDD still runs first
# --->8---

## Why

Live run-214306: slicer chained vibe-sprint, then freeze parked
`no reachable agent.code writer target`. resolveFreezeWriterTarget only hops
context.produce → agent.code. SD-24 tdd is agent.delegate, so freeze→context→tdd→coder fail-closed.

Skipping TDD in the freeze-chain would spawn coder first (wrong for V5).

## Change

`freezeWriterBinding`: direct CP-55 chain unchanged; otherwise bind first
agent.code and `advanceToNextInlineOrDelegate` (context then TDD then coder).

## Tests

`ca785_vibe_sprint_freeze_tdd_test.go`. Old freeze-chain tests untouched.

## Will not undo

CA-426 fail-closed on safety-node skip. CA-783 slicer→sprint.
