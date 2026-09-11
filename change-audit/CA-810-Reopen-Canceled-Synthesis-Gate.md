# CA-810 — reopen parks when reconstruct canceled the next node

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: /open parks resume when reconstruct remaps ghost RUNNING synthesis to CANCELED; Stop no longer hides the gate
# --->8---

## Why

Live /open run-220036: sidebar `[-] synthesis` is CANCELED (TUI glyph), not
WAITING. Last log was RUNNING; I-17 replay remaps in-flight RUNNING to
CANCELED before maybePark. CA-809 only treated PENDING/WAITING/ghost
RUNNING, so no PendingGate. Stop-sealed loops also skipped park.

## Change

- `vibeSuccessorNeedsResume`: unfinished = not DONE/SKIPPED. CANCELED/FAILED
  count. RUNNING only if no live turn.
- `maybeParkVibeResumeConfirm`: Stop seals auto-reinvoke, not the reopen
  gate.

## Tests

New `ca810_reopen_canceled_synthesis_gate_test.go`: reconstruct with
validate DONE + synthesis RUNNING log → CANCELED + resume from validate;
stopped loop still parks. CA-809 live-turn skip untouched.

## Providers

Agnostic Case 1.

## Will not undo

I-17 RUNNING→CANCELED replay. Stop-wins auto-reinvoke. Failed runs still
skip park.
