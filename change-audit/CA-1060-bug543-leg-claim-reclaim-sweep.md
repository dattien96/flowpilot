# CA-1060 — BUG-543 residual: conservative leg-claim reclamation sweep

## Scope
- `apps/local-runner/internal/runner/leg_claim_sweep.go` (new)
- `apps/local-runner/internal/runner/chat_ssot.go` (`LegClosedReasonReclaimed`)
- `apps/local-runner/internal/runner/provider_event.go` (`EventLegClaimReclaimed`)
- `apps/local-runner/internal/runner/local_file_session_store.go` (sidecar whitelist)
- `apps/local-runner/internal/runner/interactive_service.go` (boot sweep in `AttachRunner`)
- `apps/local-runner/internal/cli/root.go` (`StartLegClaimSweep` wiring)
- `apps/local-runner/internal/runner/bug543_leg_claim_reclaim_sweep_test.go` (new, additive)

## Why
Live residue (run-18660 / run-23156 / run-28790): park/stop-cancelled and
completed flow children keep `leg_state=active` forever. The BUG-543
assessment established that status-driven closure is unsafe — a
cancelled/waiting/completed child is still legally re-drivable on the same
leg via `reinvokeMatchingFlowChild`, `resumePendingLoopWork`, and
orphan-cure. The residual accepted outcome: orphaned claims under a sealed
parent are durable garbage needing a conservative sweep keyed on
sealed-loop age.

## What changed
`sweepStaleLegClaims` (boot via `AttachRunner`, in-session via
`StartLegClaimSweep` on `legClaimSweepInterval`) closes a claim only when
every signal proves the child can never be re-driven:

- `leg_state=active` AND `ParentRunID` set (children only — root/chat legs
  keep heal/reattach semantics);
- terminal `Status` (completed/failed/cancelled) — waiting/running keep;
- `UpdatedAt` older than `legClaimReclaimMinAge` (24h) — the sealed-loop-age
  "never resumed" evidence;
- no armed intents (resume/reprompt/flow-gate-settle/restart);
- no pending approval/question durable card (re-drive surface via
  orphan-cure); card reader errors fail closed to keep;
- no in-flight merge (`WorktreeResolutionPhase` / `merge_pending`);
- not remote-origin (`SourceMachineID`/`RestoredFrom`) — the source machine
  owns that leg;
- parent provably sealed: resident live loop `stopped`/`done`, or durable
  loop `stopped`/`done`, or terminal parent with no loop, or parent row
  deleted. Parent read errors keep the claim (mirrors `worktreeGCVerdictFor`'s
  gcUnknown rule).

Reclaim flips `leg_state→closed` + `leg_closed_reason=claim_reclaimed`, runs
the CA-1058 closed-leg cleanup, and emits durable `leg_claim_reclaimed`
(added to `isFlowSidecarEventType`) so the decision survives restart. Both
resident runs and durable-only rows converge. Idempotent — a leg already
closed by an explicit seam is untouched.

## Non-changes
- Stop still keeps child leg claims (`TestBug543_StopKeepsChildLegClaim`
  pins it; the sweep's age + sealed-parent predicate never fires on a
  just-stopped loop).
- No close-on-cancel/stop anywhere — claim ownership still transfers only
  through the four explicit seams plus this sweep.

## Evidence
- `go test -count=1 -run TestBug543_ ./internal/runner` — 11/11 (9 new
  reclaim/keep cases + 2 pinned contract tests).
- Full `./internal/runner` suite: failure set equivalent to HEAD
  (`TestBug377_UnarmedSettleRepromptStillDispatches` reproduced on clean
  HEAD ea744b3c — pre-existing; remainder env: missing claude/codex/agy/
  opencode/gitnexus binaries + documented flakes).
