# CA-802 — leftover paused block must not skip validate after OK

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: After pause-gate OK, leftover BlockReason=paused still allows coder→validate so hub does not stall
# --->8---

## Why

Live run-220036: OK spawned coder; coder DONE; validate stayed `[ ]`; 2m later `hub_stalled` and synthesis WAITING. `tryAdvanceFlowFromNode` skipped because `loopIsAdvancing` treats any `blocked` as settled, including stale `paused` after confirm was cleared.

## Change

- `loopIsAdvancing`: `blocked`+`paused` + `vibeResumeConfirm==false` → advancing. Pause gate still blocks while confirm is true.
- `maybeResumeVibeCoderAfterTdd` uses the same predicate (leftover pause no longer early-returns).
- Pause-gate OK emits agent graph + persists parent so TUI does not keep `blocked: paused`.

## Tests

New `ca802_stale_pause_allows_validate_test.go`: leftover pause allows `tryAdvance` coder→inline; `hub_stalled` still blocks; OK clears loop. Old BUG-234 tests untouched.

## Providers

Agnostic Case 1: loop status helper takes no providerKey.

## Will not undo

CA-801 pause gate. BUG-234: other blocked reasons still stop advance.
