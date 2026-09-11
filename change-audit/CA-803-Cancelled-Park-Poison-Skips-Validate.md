# CA-803 — park-poison Cancelled must not skip validate

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Cancelled+running is pause-park poison, not Stop; coder→validate and hub_stalled Retry dispatch validate
# --->8---

## Why

Live run-220036: `coder→validate` resolved with loop running, then `flow_inline_dispatch_skipped_terminal`. Parent `status=cancelled` from pause-park. Retry reinvoked hub; validate stayed PENDING.

## Change

- Pause-gate OK heals Cancelled→Running before coder spawn; **Stop/done loop does not heal**.
- `hub_stalled` Retry: validate predecessor from edges (not hardcoded coder); missing step row still advances; Failed/sealed loop return false so hub prose is not skipped as "handled".
- `flowRunTerminalLocked` unchanged (BUG-288 Cancelled-always-terminal).

## Tests

New `ca803_cancelled_park_poison_validate_test.go` including Stop-no-heal and Failed-no-claim. Old Cancelled-always-terminal test untouched.

## Providers

Agnostic Case 1.

## Will not undo

BUG-288 P1-12: Cancelled/Failed stay terminal in `flowRunTerminalLocked`. CA-801/802 pause gate.
