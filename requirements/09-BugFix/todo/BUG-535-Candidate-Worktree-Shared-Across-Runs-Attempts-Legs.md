# BUG-535 — tournament candidate worktree is keyed per node-ID, not per run/attempt/leg → multiple live legs can own the same directory

**Status:** fixed (CA-1043) — sweep-side leg terminalization + claim-aware
guards; unit-verified on all three legs of the hole.

## Reproduction

Worktree identity is `candidate-<nodeID>` inside the run's workspace —
no parent-run or attempt namespacing. Three paths converge on the same dir:

1. **Cross-attempt retry**: vetoed/parked leg keeps `leg_state=active` +
   `WorkspaceCwd` pointing at `candidate-<id>`. Arbiter `retry` →
   `resumeTournamentChoice` → `mgr.Cleanup(cwd, ids)` sweeps **all**
   candidate dirs unconditionally (no live-leg guard) →
   `spawnTournamentCandidates(attempt=N+1)` recreates the same path for the
   new cohort. The old vetoed leg is still `active` and still claims the
   dir.
2. **Quota respawn**: `respawnChildOnRoute` passes `WorkspaceCwd:
   child.workspaceCwd` to the successor — no ownership check that another
   leg now holds that path. Successor lands on the dir a newer cohort (or
   another run's leg) owns.
3. **Cross-run**: two tournament runs in one workspace both map
   `candidate-a` to `<ws>/.flowpilot/worktrees/candidate-candidate-a`.
   A dead-run leg keeps `leg_state=active` forever in `sessions.ndjson`
   (restart marks run cancelled but never touches `leg_state`), so the
   durable record shows multiple `active` owners permanently.

## Live evidence (/tmp/fp-live3, sessions.ndjson)

Three legs `leg_state=active` on `/tmp/fp-live3/.flowpilot/worktrees/
candidate-candidate-a` simultaneously:

| run | leg | provider | cohort | why it holds the claim |
|-----|-----|----------|--------|------------------------|
| run-799  | active | grok | attempt-0 | veto-parked, never closed (run-348) |
| run-1408 | active | grok | attempt-1 | retry cohort (run-348, later killed) |
| run-4150 | active | devin | attempt-0 | BUG-534 successor of run-3107 (run-2634) |

No concurrent writes occurred only because all three were veto-parked or
their parent runs were dead. If any two had running provider turns they
would interleave writes in the same directory and corrupt each other's
diff/audit evidence.

## Root cause

- `WorktreeManager` keys worktrees by owner node-ID only; nothing in
  `candidate-<id>` encodes which run/attempt/leg the dir belongs to.
- `resumeTournamentChoice` retry cleanup has no live-leg guard.
- `respawnChildOnRoute` inherits `WorkspaceCwd` blindly — a vetoed leg's
  successor can land on a dir already claimed by a newer cohort.
- A vetoed/dead leg is never terminalized: `leg_state` stays `active` in
  the durable row even after its parent run is cancelled, so ownership
  claims never expire.

## Fix (CA-1043)

Two complementary pieces (options 2+3 from below; namespacing left as a
possible follow-up since claims now serialize correctly without it):

- **`sweepCandidateWorktrees`** (`tournament_dispatch.go`): every service
  candidate-worktree sweep — spawn aborts, stale-dir retry, orphan sweep,
  arbiter retry/discard, merge-done losers — now also runs
  `closeLegsBoundToWorktree`, closing every leg whose session row claims
  the dir with `leg_closed_reason=worktree_swept` (new reason). Stale
  `active` residue on cancelled runs can no longer outlive the dir.
- **`liveClaim` replaces `liveChild`**: the Create-failure self-heal now
  checks for *any* non-terminal leg claiming the dir (any run, any cohort)
  — a live foreign claimant blocks the sweep and parks the flow
  (fail-closed), while dead/residue claims are swept + closed.
- **Respawn revalidation** (`respawnChildOnRoute`): when the leg's
  `WorkspaceCwd` is a dedicated binding (differs from the parent's cwd —
  a shared main workspace legitimately overlaps), the respawn refuses if
  another non-terminal `active` leg claims it, or if the dir no longer
  exists. Refusal keeps the vetoed leg open under the BUG-534 contract.

## Regression tests

- `TestBug535_WorktreeSweepClosesStaleLegClaims` — cancelled run with an
  `active` leg claiming the dir → spawn sweeps, leg closes
  `worktree_swept`, new candidate spawns.
- `TestBug535_LiveClaimantBlocksSweep` — a different run's live leg owns
  the dir → no sweep, dir survives, claimant untouched, flow parks.
- `TestBug535_RespawnRefusesDirClaimedByLiveLeg` — parent running, rival
  live leg on the dir → respawn errors, leg stays open, no commit.
- `TestBug535_RespawnRefusesSweptWorktree` — dedicated dir already
  deleted → respawn errors, leg stays open, no commit.

## Suggested fix (original)

Pick one or combine:

- **Namespace the dir**: `candidate-<parentRunID>-attempt<N>-<nodeID>` so
  cohorts can never collide; respawn inherits the original leg's dir only
  while that cohort still owns it.
- **Ownership revalidation at respawn**: before reusing `WorkspaceCwd`,
  check no other non-terminal leg claims it; if claimed, mint a fresh
  worktree for the successor leg instead of sharing.
- **Leg terminalization on sweep**: when `Cleanup` removes a worktree,
  close every still-`active` leg bound to it (`leg_closed_reason=
  worktree_swept`) so durable claims can't outlive the dir.

The last option also fixes the lingering `active`-on-dead-runs residue.

## Context

Pre-existing lifecycle gap, not introduced by BUG-533/534 — the BUG-533
sweep (`liveChild` guard + post-loop orphan cleanup) matches the retry
path's existing unguarded semantics: it protects dirs owned by a live
child of the *same attempt* and sweeps everything else. BUG-534's
respawn inherits `WorkspaceCwd` the same way the old code did; the new
ordering just made the reuse visible in live state.
