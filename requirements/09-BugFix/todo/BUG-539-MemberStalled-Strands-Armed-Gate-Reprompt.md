# BUG-539 — Armed child gate reprompts have no drive path; orphan-cure resets the reprompt cap (unbounded cycle)

Status: **FIXED — regression tests green; live re-verify pending**
Severity: Important — a queued child gate reprompt is undriven for tens of
minutes and, when recovered via the orphan-cure path, the reprompt-attempt
counter resets so `maxFlowGateReprompts` can never engage — an unbounded
reprompt cycle on an unsatisfiable gate.

## Symptom (live, `/tmp/fp-live4`, run-1663 / child run-2830)

1. Coder leg `run-2830` (candidate-a, devin) completed turn-6016 at 23:34:59;
   post-turn gate armed a reprompt (`child gate reprompt attempt=0`,
   `pending_gate_code_paths` flagged).
2. **46 minutes with zero dispatch** — the armed reprompt sat with no drive:
   the hub loop was `blocked` (hub_stalled 23:33:02), nothing re-flushed it,
   and at 23:45:00 the member stall watchdog fired `member_stalled` on
   run-2830 — treating a member holding queued work as silent.
3. First `continue` (00:03) re-drove only the hub (arbiter turn-6304); the
   orphan-cure scan skipped run-2830 because its status was still `running`,
   not yet `waiting_user_approval`.
4. Second `continue` (00:20) hit the orphan-cure path
   (interactive_service.go:2777-2810: `waiting_user_approval` +
   `pendingGateCodePaths` non-empty → `reinvokeMatchingFlowChild`) →
   turn-6561 dispatched — a plain "Resume: parked before post-turn gate"
   turn, **not** a `scenarioGateReprompt` delivery.
5. Because the re-drive ran as a generic resume turn,
   `startTurn`'s `scenario != scenarioGateReprompt` branch
   (interactive_service.go:10526) zeroed `repromptAttempts` — consistent with
   every gate log line printing `attempt=0` across all cycles.
6. turn-6561 completed at 00:21:30; gate flagged the same paths again →
   reprompt re-armed `attempt=0` → cycle repeats. `maxFlowGateReprompts = 2`
   is unreachable through this path → **unbounded reprompt loop**.

## Root cause (coupled gaps)

1. **No reprompt shield in `checkAndBlockStalledMembers`**
   (cohort_stall.go:181-249): skips only approval/question/live-gate members.
   The BUG-520 fix added `repromptArmed` to the hub ghost-check
   (hub_stall.go:118-131) but the member-stall path never got it — an armed
   reprompt counts as silence → stall → park.

2. **`parkFlowForAwaitingUser` wipes `pendingGateReprompt*` on children**
   (interactive_service.go:3086-3089, via hub_stalled at hub_stall.go:461).
   Recovery survives only by accident: durable `pending_gate_code_paths`
   lets the orphan-cure path synthesize a fresh prompt — the armed intent
   itself (composed reprompt text, step ID, generation) is destroyed.

3. **Reprompt arm is never persisted.** gate_hook.go:1677-1683 sets
   `pendingGateReprompt*` under `s.mu` with no following persist; run-2830's
   durable row carried `pending_gate_reprompt_gen=1` but **no
   `pending_gate_reprompt_prompt`** at every snapshot. A restart between arm
   and dispatch loses the queued turn — recovery then depends on the
   code-paths orphan cure, which itself requires a manual `continue`.

4. **Orphan-cure re-drive bypasses the reprompt cap.** The cure dispatches a
   generic Resume turn → `scenario != scenarioGateReprompt` →
   `repromptAttempts = 0` → the 2-attempt cap never engages → infinite
   reprompt→flag→reprompt cycles on an unsatisfiable gate (exactly the
   run-169/442/4014 failure mode the BUG-289 comment says was fixed for the
   reprompt-delivery path — the orphan-cure path reopened it).

## Why it matters

- The armed reprompt has **no self-drive**: between arm and the next settle
  there is no flush trigger; a blocked hub loop leaves it queued silently.
- Every cycle burns a full provider turn; on an unsatisfiable gate the child
  loops forever, holding its worktree claim (`leg_state=active`) and stalling
  the cohort barrier.
- member_stalled then parks the flow on a member that was mid-remediation —
  a self-inflicted stall.

## Reproduction (live recipe used)

1. Flow with a labeled coder child whose post-turn gate flags
   `pending_gate_code_paths` (code changed without declared contract).
2. Let the reprompt arm while the hub loop is blocked (any awaiting-user
   park) → armed intent sits, member ages to stall.
3. `continue` → orphan-cure re-drives via Resume turn → gate flags again →
   reprompt re-arms at `attempt=0` → repeat; `reprompt_attempts` never
   accumulates, cap never fires.

## Expected fix shape (for the fixer)

- Shield armed `pendingGateReprompt*`/`pendingResume*` members in
  `checkAndBlockStalledMembers` (mirror hub_stall.go:118-131).
- Either stop wiping `pendingGateReprompt*` in `parkFlowForAwaitingUser*`
  for children, or re-derive the intent durably from `pendingGateCodePaths`
  (including on restart reconstruction — the intent must not depend on RAM).
- Persist `pendingGateReprompt*` at arm time (gate_hook.go:1677-1683).
- Orphan-cure re-drives must carry `scenarioGateReprompt` semantics (or
  otherwise increment `repromptAttempts`) so the cap bounds the loop.
- Self-drive the armed reprompt when the parent loop unblocks even if the
  child step still reads `WAITING_USER_APPROVAL` — today the only recovery
  is an operator `continue` firing the orphan scan.

## Fix applied (CA-632)

- `cohort_stall.go checkAndBlockStalledMembers`: armed durable intents
  (`pendingGateRepromptPrompt`/`pendingResumePrompt`) now count as queued
  activity — mirrors the existing hub_stall.go shield. A member can no longer
  stall-fire while remediation is armed.
- `interactive_service.go resumeFlowWithFeedback`: orphan-cure now routes
  children with an armed reprompt through `resumeIDs` →
  `flushDurableTurnIntents` (reprompt channel, cap preserved) instead of the
  generic Resume mint. `resumeIDs` consults armed intents, not just parked
  status.
- `interactive_service.go startTurn`: `repromptAttempts` resets only when the
  `pendingGateCodePaths` debt is cleared (`scenario != scenarioGateReprompt
  && len(pendingGateCodePaths) == 0`). The orphan-cure Resume mint for a
  code-paths-owed child now preserves the counter → cap reachable.
- `handleMemberAction`: `skip` clears reprompt/resume/pending-turn intents and
  closes the active leg (`LegClosedReasonMemberSkipped`); `retry` clears the
  same stale intents before the fresh retry turn.
- The park wipe (`parkFlowForAwaitingUser`) and blocked-loop clear
  (`resumePendingFlowGate`) were kept per the deliberate BUG-354/run63960
  freeze contract — parked/blocked flows hold no live auto-intents, and
  `pendingGateCodePaths` carries the re-check obligation through orphan-cure.
  BUG-520/BUG-327/run-63960 tests all still pass unchanged.

Tests: `bug539_member_stall_reprompt_strand_test.go` (5 regression tests —
stall shield, no double-drive + counter preserved, skip clears+leg close,
retry clears, counter guard). All green.

## Live re-verification (2026-09-28, post-fix binary)

- Reprompt-cap reachability verified live on run-6565 (cp-harness on :4321):
  post-turn gate reprompt armed → dispatched promptly (~2 s) → counter
  incremented to attempt 2 → `Gate reprompt exhausted` → `blocked`/escalate.
  The counter now survives to the cap; the orphan-cure Resume mint no longer
  resets it.
- The stall-shield/member_stalled live leg was not re-staged: the tournament
  vehicle (run-18354) wedged earlier at `waiting_review` on a different
  defect (BUG-542) before a cohort member could arm a reprompt under a live
  stall check. Shield behavior is covered by
  `bug539_member_stall_reprompt_strand_test.go` (armed-intent member not
  counted silent; orphan-cure drives the reprompt channel; skip clears
  intents + closes leg; retry clears intents; cap counter guard) — unit-verified,
  live leg deferred to a future cohort run.
