# CA-1208 — Vibe sprint take resets the per-task round budget (live run-262417)

## Evidence
run-262417 (PrivateVault CP-04, vibe-tasks, 5-task plan): Task-041 closed
its sprint having consumed ~7 rounds of escalates/continues; the boundary
auto-advanced to Task-042 and the desktop round chip still read `7/20`.
The vibe contract gives every Task its own `defaultVibeTaskRoundCap` (20)
budget — `vibe-sprint.yaml` policy comment: "cap 20 per Task; a CP's total
cap is len(taskPlan) × 20". With no reset, a contested early task starves
every later task: two ~10-round tasks exhaust the whole CP's shared 20 and
`advanceRound` seals the loop `stopped` ("round cap reached") mid-sprint.

## Root cause
`AgentLoopState` is one shared loop across all sprints of a CP run.
`advanceRound` increments `Round` forever; the only existing reset is
`resetPlanPhaseRound` (plan → code phase, dual-loop flows). The cross-sprint
reset block in `takeNextVibeSprintLocked` already clears verdicts, decision
cards, AC caches, and debate-mount counters on `d.Start` (Task-350,
BUG-595 — "a new sprint takes a fresh remediation budget"), but never
touched `Round`, `ExtendCount`, or `NegotiationRound`. Both take paths —
`maybeStartNextVibeSprint` (chain/initial mount) and
`continueVibeSprintBoundary` → `startTakenVibeSprint` (boundary advance) —
funnel through `takeNextVibeSprintLocked`, so the gap applied to every
task transition.

## Fix
Inside `takeNextVibeSprintLocked`'s `d.Start` branch, one
`agentOrchestrator.mutateLoop` resets `Round`, `ExtendCount`, and
`NegotiationRound` to 0 under the established `s.mu → o.mu` lock order —
the take IS the task-start decision, so index++ and the fresh budget land
in the same critical section. `ExtendCount` must clear so the flow-mount
seed (`st.ExtendCount == 0 || st.Cap < cap` in `startResolvedFlow`)
re-applies the policy cap=20 instead of carrying a prior task's extension.
`NegotiationRound` is the same phase-scoped budget class. Non-Start takes
(Locked/Budget/Done) return before the reset — a peeking take never
touches the in-flight sprint's budget, and a spawn-failure rollback
leaving Round=0 is harmless (the previous task is already done; the
re-offer takes again anyway).

## Tests
- `vibe_sprint_boundary_test.go`
  `TestVibeSprintBoundary_TakeResetsTaskRoundBudget`: armed boundary with
  Round=7/ExtendCount=1/NegotiationRound=3 → boundary continue → all
  zero (fails pre-fix: round=7 carried).
- `TestVibeSprintBoundary_NonStartTakeKeepsRoundBudget`: vibeAwaitingLock
  → Locked take leaves Round=7 untouched.
- Full `TestVibeSprint*|TestVibe*|Boundary|takeNext|Debate|SprintHandoff`
  sweep green; `TestSwitchInitializesTranscriptWriterBeforeHandoff`
  flake is unrelated (passes solo, transcript-writer path).
