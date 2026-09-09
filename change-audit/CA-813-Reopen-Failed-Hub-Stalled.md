# CA-813 — /open must not restore stale hub_stalled after Failed stop-fence

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Reopen heals Failed+hub_stalled vibe runs to a Resume-from-last-DONE gate; stop-fence cancel no longer stamps Failed
# --->8---

## Why

Live /open run-220036 showed hub_stalled Retry/Stop immediately. Disk:
status=failed, loop blocked hub_stalled. maybePark skips Failed, so the
stale 2m stall card won. Failed came from stop-fence TurnFailed
(emitLocked maps TurnFailed→Failed).

## Change

- `healVibeFailedForReopenPark` before maybePark: unfinished vibe Failed →
  Cancelled so park can run. Genuine Failed with no unfinished successor
  unchanged (CA-806).
- Stop-fence linearize cancel uses parkCancelSuppress so the run stays
  non-Failed.

## Tests

ca813_reopen_failed_hub_stalled_gate_test.go. CA-806 Failed-no-arm
untouched.

## Providers

Agnostic Case 1.

## Will not undo

CA-806 genuine Failed skip. hub_stalled watchdog for live hangs.
