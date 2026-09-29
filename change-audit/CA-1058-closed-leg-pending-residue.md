# CA-1058 — closed legs drop armed residue; read paths never resurrect them

## What changed

Live run-945 / run-400 class: a leg closed by provider switch, quota stop,
worktree sweep or crash-heal kept its durable "armed" state —
`pending_resume_*`, `pending_gate_reprompt_*`, `pendingFlowGateSettle`,
`waiting_user_approval` — so the row kept reading actionable after the leg was
dead. Two read paths made it worse than cosmetic:

- `reconstructPendingChildSessions` (`needSessionWithIntent`) never consulted
  `legState`, so a closed-but-armed child row was reconstructed into `s.runs`
  on parent resume and its intent re-driven (only the `leg_closed` startTurn
  guard stood between it and a ghost turn).
- `rehydratePendingGatesLocked` rebuilt pending approval/question cards on a
  closed leg as live actionable surfaces.

Mutation side — new `clearClosedLegPendingLocked` (interactive_resume.go)
drops continuation intents, the parked flow-gate settle, the buffered turn
prompt and live card handles, and collapses `waiting_*` status to `cancelled`.
Gen high-water marks stay (idempotency); `legState`/`legClosedReason` keep the
closure audit. Called at every leg-close site that was missing it:
`switchChatLeg` phase C, `healChatLegsLocked` two-active window, the
dispatch-failed switch abort, quota-gate stop, `closeLegsBoundToWorktree`
(in-memory branch) and the flow-done settle (previously cleared only the
resume kind). `clearClosedLegPendingSession` is the `ProviderSessionState`
twin for the non-resident durable-row sweep.

Read side — defense for stale rows already on disk and any future close path
that misses the clear:

- `needSessionWithIntent` returns false for `LegState == closed`.
- `rehydratePendingGatesLocked` returns early on a closed leg.
- `normalizeResumedFlowStatus` / `agentStatusFromSession` /
  `resumedChildRunStepStatus` project closed legs through the new
  `closedLegResumedStatus` — waiting-* collapses to cancelled (a card on a
  dead leg), everything else flows through `normalizeResumedStatus`.

## Red → green

`internal/runner/bug55x_closed_leg_pending_residue_test.go` (new), all red
before the fix:

- `TestClosedLegClearsArmedPendingStateOnClose` — worktree sweep on a resident
  leg + a durable-only row: intents/settle cleared, `waiting_*` → cancelled,
  `legClosedReason` and `PendingResumeGen` high-water preserved.
- `TestReconstructSkipsClosedLegWithArmedIntent` — closed+armed child no
  longer lands in `s.runs` via `reconstructPendingChildSessions`.
- `TestClosedLegProjectsTerminalNotArmed` — `listAgentRunSummaries` projects
  `cancelled`, `resumedChildRunStepStatus` → `canceled`.
- `TestClosedLegDoesNotRehydratePendingCards` — pending approval sidecar on a
  closed leg stays inert.

`go test -count=1 ./internal/runner/` — 29 failures, identical set to HEAD
(gate-warn baseline, provider/env deps). Two diffs vs the HEAD run
(`TestDeleteChatSessionRemovesCodexStableAndTurnLogRolloutsAcrossAccounts`,
`TestTask453_AuditEntryRecorded`) pass in isolation in this tree, and the HEAD
run's own failures (`TestDeleteChatSessionCascadesToStoredChildAgentRuns`,
`TestListProviderAccountsRecoversManagedCodexSlotsFromDisk`) flap both ways —
suite-ordering flakes, not regressions.

## Residuals

- `pendingRestart*` provenance fields are intentionally untouched — they mark
  that a replacement was minted, not a re-drive intent on the closing row.
- Durable pending approval/question sidecar rows on a closed leg stay on disk
  (audit trail) but are inert under the rehydrate guard.
