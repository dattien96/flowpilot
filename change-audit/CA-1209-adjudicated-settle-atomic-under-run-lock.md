# CA-1209 — Adjudicated-predecessor settle is atomic under s.mu (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04, vibe-tasks): sprint-042 mounted at
20:15:11.49x (all nodes PENDING per the BUG-577 reset) — and 50ms later
`synthesis` stamped DONE at 20:15:11.542 while nothing in sprint-042 had
run. At 20:16:49 `audit` auto-activated on its stale done-edge pred,
`vibe_task_doc_settled` moved Task-042's doc, `flow_audit_draft_built` →
`flow_control: done` → `markFlowRunComplete` stamped the RUNNING tdd leg
DONE and everything else SKIPPED. **The sprint sealed complete in ~15
seconds with zero implementation written** — while the scaffold-architect
leg was still mid-turn writing `tdd-signatures.md`.

## Root cause
`settleAdjudicatedDonePredecessors` evaluated evidence and stamped in two
unlocked steps: `persistedCompletedChildExists` read `rs.vibeSprintIndex`
under its own lock, `setFlowStepStatus` wrote much later with no shared
critical section. The sprint-041 audit's post-settle `flowAdvance` evaluated
"synthesis has a durably completed leg" while `vibeSprintIndex` was still 1
(legit evidence for sprint-041's own row), then its goroutine was
descheduled across the boundary take (index 1→2 + PENDING reseed) and the
DONE stamp landed on **sprint-042's fresh row** at .542 — a sprint-N
settle fabricating evidence for sprint-N+1.

## Fix
eval and stamp now share ONE `s.mu` hold inside
`settleAdjudicatedDonePredecessors`: the sprint index is re-read in the
same critical section that issues `setFlowStepStatusLocked`. Either order
is safe — settle wins the lock first → stamps the prior sprint's rows and
the mount's reseed still wins; take wins first → the new index scopes the
session scan to the new sprint's legs (BUG-622's
`sessionBelongsToVibeSprint`) and the stale stamp is refused.

Supporting refactors:
- `vibeNodeHasLiveWork` extracted `vibeNodeHasLiveWorkLocked` — the settle
  can check live legs inside the same hold (Go mutexes don't re-enter).
- `persistedCompletedChildExists` extracted `completedChildSessionInList`
  — an in-memory scan over a hoisted session snapshot, so the locked
  section does no store I/O except the stamp itself. A snapshot that
  misses a just-completed leg degrades fail-closed (no stamp), the safe
  direction.

## Tests
- `ca1209_atomic_adjudicated_settle_test.go`
  `TestCA1209_SettleVsBoundaryTakeMustNotStampNewSprintRow` — gated
  `ListAllProviderSessions` parks the settle mid-eval; test advances index
  + reseeds; pre-fix stamps DONE on the new sprint's row (RED confirmed),
  post-fix refuses (PENDING).
  `TestCA1209_SettleStampsWithinSameSprint` — same-sprint evidence still
  stamps correctly.
- BUG-1197/BUG-622/vibe-sprint/boundary regression suite green.
