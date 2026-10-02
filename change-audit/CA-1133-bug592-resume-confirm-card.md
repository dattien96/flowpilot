# CA-1133 — BUG-592: Armed vibe resume-confirm gate never reaches the desktop

## What changed

`apps/local-runner/internal/runner/vibe_sprint.go`:

- `maybeParkVibeResumeConfirm` previously set `vibeResumeConfirm` +
  `vibeResumeFromNode` and mutated the loop to `blocked`, then returned
  without emitting a graph event, marking the mux lane dirty, or persisting
  the parent session.
- The decision payload builder (`gateDecisionPayload`) already projects a
  `vibe-gate-resume:` decision — but the lane fingerprint never changed, so
  it was never flushed to the desktop.
- Now mirrors the BUG-365 requirement-park emit: `emitAgentGraph`,
  `markRunRealtimeDirty`, and `persistParentSession` after the mutation.

## Live evidence (run-139670)

- After the tournament leg completed, the parent parked with
  `gateReason: "Resume from reviewer?"` while `/questions` and `/approvals`
  were both empty — a phantom wait with no card to answer.
- Operator had to call `gate-decision ok` over HTTP, which additionally
  surfaced BUG-591 (resume resolved against the ingest topology).

## Invariant

Every armed vibe gate must have a projected decision on the mux lane —
arming state without flushing it is a silent wedge.

## Tests

`internal/runner/bug592_resume_confirm_card_test.go` — red-first:
`maybeParkVibeResumeConfirm` must mark the run dirty and the drained
projection must carry a `vibe-gate-resume:` decision ID.
