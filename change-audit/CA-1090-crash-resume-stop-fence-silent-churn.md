# CA-1090: shutdown stop fence silently kills every post-resume turn — stopped_before_send churn

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — after a runner
restart the flow redrive minted hub turns that died at the send CAS with
`stop_outcome="stopped_before_send"`; no provider bytes, no terminal event,
loop stuck "running" but forever quiet.

## Symptom (live, post-restart)

- 19:17:04 — `flow_loop_redrive_after_crash` fired; `turn-27229` minted.
- dispatch.ndjson: `prepared → send_claimed → terminal_cancelled`,
  `cancel_requested=true`, `stop_generation=1` — **the turn was fenced
  before a single byte was sent**.
- Same for `turn-27267` on the next resume. Goroutine exits silently —
  nothing emits a turn event, so the run just sits quiet until the next
  redrive mints another fenced turn. Infinite silent churn.

## Root cause

Runner shutdown writes a durable `run_stop` tombstone
(`RequestRunStop`/`stopAgentLoop` on drain) and stamps `cancel_requested`
on recoverable dispatch records. On resume, `redriveQuietFlowLoop` (BUG-410)
heals `rs.status` cancelled→running and re-drives the hub — but never
releases the durable stop fence. `linearizeSendStarted`'s CAS then hits
`ErrRunStopFence` and commits `terminal_cancelled` before send — by design
that path suppresses the user-facing failure event (`parkCancelSuppress`),
so the wedge is invisible: mint → fence → quiet → redrive → repeat.

`releaseHubStopFenceForFollowUp` already exists for exactly this release,
but was only wired to the post-Stop plain-chat follow-up path
(`turnStartedAfterLoopDone`). The crash-resume seams never called it.

## Fix (`internal/runner` only)

- `redriveQuietFlowLoop`: release the durable run_stop fence before the
  re-driven hub mints a turn.
- `resumePendingLoopWork`: release before `deliverPendingRestart`
  (survived stall-retry intent) and before the durable child-resume flush
  (children are fenced by `ErrParentStopFence`).
- Release keeps the generation elevated — children that saw the Stop fence
  stay invalid; no-op when the fence isn't armed or V2 is inactive.

## Verification

- Red→green: `ca1090_crash_resume_stop_fence_redrive_test.go` —
  (a) crash-resumed redrive: minted turn reaches the adapter + fence
      released;
  (b) pendingRestart drain: same;
  (c) guard: without a resume the fence stays armed and a bare send is
      still fenced (no regression on Stop semantics).
- Live: run-3362's dispatch trace shows the exact `stopped_before_send`
  pattern on both post-restart turns.

## Files

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/ca1090_crash_resume_stop_fence_redrive_test.go`
