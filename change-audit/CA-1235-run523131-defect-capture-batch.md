# CA-1235 — Live-run-523131 (vibe-sprint CP-11) defect capture batch — 17 new BUG docs

## Summary

Docs-only capture of defects observed while babysitting one real
`vibe-sprint` run end-to-end (PrivateVault CP-11, provider Devin). Each
BUG doc carries the live evidence chain (run ids, event ids, file:line)
recorded at the moment the defect was hit — written so the fixer never
has to re-derive the wedge from scroll.

## Defects captured (BUG-644..660)

| BUG | Defect | Sealed wedge cost |
|-----|--------|-------------------|
| 644 | Freeze summary prose drifts from contract JSON | audit distrust |
| 645 | submit_review_outcome: approved + blocked ACs → advancing | false pass |
| 646 | agent-loop/amend mints contract, never unparks leg | silent park |
| 647 | completed child leg never settles step mirror (RUNNING) | ~4h wedge + 1 wasted leg |
| 648 | sprint `done` while tdd FAILED → silent task skip | empty task shipped |
| 649 | decisionTarget consumes gateBlock without arming proposalTurnPending | repeat reblock |
| 650 | resume-with-feedback prose-only, no verdict/dispatch | dead "running" |
| 651 | InferFromDiff mints wrong contract vs frozen contract (root defect) | contract-bind loop ×3 |
| 652 | debate loop repeats identical card/verdict, no circuit breaker | quota burn ×3 rounds |
| 653 | dead-dispatch sweep skips runs with pendingFlowGateSettle | park shields wedge |
| 654 | agent-loop/continue re-dispatches parked hub vs advancing edge | burned rounds |
| 655 | zero_delta_progress fires on non-writing hub/debate turns | 419 false drift events |
| 656 | stale `running` child blocks resume-confirm gate forever | unresumable run |
| 657 | leg-level run_stop fence has no release path (parentRunID keying) | permanently fenced leg |
| 658 | debate owner legs killed by provider_limit have no redrive | abandoned cohort |
| 659 | resume-at-tdd abandons frozen contracts; once-freeze never re-runs | contract wall |
| 660 | loop `done` but run stays `running` until manual /resume | terminal limbo |

Also committed: orphaned capture docs BUG-633..636 (written in the
CA-1234 session, never committed) — they are live-capture records for
the same run batch and belong in the ledger.

## Root-defect callouts for fix priority

- **BUG-651** is the cascade root for the Task-113 wedge (contract
  binding ignores frozen scope on dirty worktrees).
- **BUG-648** is the worst correctness defect (silent task skip).
- **BUG-656 + 657** chained to make one interrupt turn a leg permanently
  dead: stale stamp blocks resume AND the fence blocks its redrive.
- **BUG-647 + 653 + 660** are one settle-propagation family at three
  levels: leg→step, park-shielded-step, loop→run.

## Verification

Docs-only change; no code touched. Numbering continues BUG-643
(committed) — no collisions. Cross-references to BUG-397/402/425/439/
464/467/471/540/561/637/641/643 verified as related-but-distinct.
