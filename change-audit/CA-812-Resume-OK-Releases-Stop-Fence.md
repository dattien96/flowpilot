# CA-812 — vibe resume OK releases durable stop fence

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Operator OK on Resume from N? releases the hub run-stop fence so synthesis is not cancelled before send
# --->8---

## Why

Live run-220036: gate showed, OK started synthesis RUNNING, then
`Turn failed: turn cancelled before send (run stop fence)`. Session still
had `stop_generation: 1` from an earlier Stop. BUG-308 only releases the
fence for plain-chat follow-up (`turnStartedAfterLoopDone`), not vibe
resume OK.

## Change

- SubmitGateDecision OK: `releaseHubStopFenceForFollowUp` before advance.
- Drop `loopSealedForReinvoke` early-return on that OK — the gate is the
  unseal. Auto-reinvoke stays fenced.

## Tests

`ca812_resume_ok_releases_stop_fence_test.go`: post-Stop fence + resume
from validate → fence cleared, no stop-fence TurnFailed, synthesis hub
dispatched.

## Providers

Agnostic Case 1 (dispatch V2).

## Will not undo

BUG-308 child fence (generation stays elevated). Failed runs still skip.
Stop-wins auto-reinvoke.
