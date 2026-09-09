# CA-811 — /open must not hold s.mu while cancelling turns

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Park releases the service mutex before turnCancel; boot summary scan uses deferred reconstruct so it cannot park
# --->8---

## Why

Live /open run-220036 stuck on "Opening chat…". Health 200, GET run hung —
InteractiveService.mu held. CA-810 made reconstruct park; parkFlowForAwaitingUser
called turnCancel while holding s.mu. finishTurn/emitLocked need that mutex.

Isolated resume of the same session returns in 0.7s; the live process was
deadlocked.

## Change

- Collect cancel funcs under lock, unlock, then cancel, then diag.
- ScanPersistedChatsForSummaries uses reconstructRunDeferred (no park).

## Tests

ca811_open_resume_hang_test.go: turnCancel that re-locks s.mu must not deadlock;
deferred reconstruct does not set vibeResumeConfirm.

## Providers

Agnostic Case 1.

## Will not undo

CA-810 CANCELED successor park. Stop-wins auto-reinvoke. Locked park path
(emitLocked) unchanged.
