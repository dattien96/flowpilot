# CA-1118 — BUG-567: debate overlay wedge after restart redrives synthesis

## Why

Live run-100368 (PrivateVault Task-025): a restart while the owner-debate
overlay was mounted left the run with both owner legs DONE, `debate_synthesis`
PENDING, and the sprint graph parked. The cohort buffer is RAM-only, so no
join event ever arrived post-restart — nothing re-drove the synthesis hub
turn, and the overlay stayed mounted forever. While mounted, `activeFlowNodes`
contained only the debate graph, so `submit_review_outcome` tool offers and
verdict-record checks (which resolve the `review` cohort on the active node
set) could not land — the live run needed manual HTTP `flow-control` surgery.

## What changed

`apps/local-runner/internal/runner/vibe_debate.go`:

- New `maybeResumeVibeDebateSynthesis`: when the active graph is the
  owner-debate graph with parked sprint nodes — replay `restoreVibeFlowAfterDebate`
  if `debate_synthesis` is already DONE; do nothing while synthesis is
  RUNNING/waiting; otherwise, when both owners are DONE but synthesis never
  ran, revive the reconstructed run (status/autoOrchestrate/loop, matching the
  vibe resume-helper pattern — a normalized-cancelled run would refuse the
  reinvoke guard) — and re-drive the debate hub with a recovery prompt
  (`vibeDebateSynthesisResumePrompt`).

`apps/local-runner/internal/runner/interactive_resume.go`:

- The reconstruct-time resume sweep calls `maybeSettleVibeOwnerDebate` then
  `maybeResumeVibeDebateSynthesis` in order, so a fresh owner retry is never
  mistaken for already-settled evidence.

## Invariant

A restarted run mid-debate must recover deterministically: settled owners
must produce the owed synthesis turn (or replay the owed restore) instead of
an orphan overlay that swallows downstream verdict surfaces.

## Tests

`bug567_568_debate_restart_test.go` (red → green):

- `TestBUG567ResumeRedrivesDebateSynthesis` — restart with owners DONE /
  synthesis PENDING re-drives a synthesis hub turn
- `TestBUG567ResumeRestoresWhenSynthesisAlreadyDone` — DONE synthesis
  replays the parked-flow restore instead of double-driving
