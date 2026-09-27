# CA-1043 — Worktree sweeps close stale leg claims; respawn refuses shared/dead dirs (BUG-535)

## Summary

Deep review of the BUG-533/534 fix surfaced a durable-state hole the live
run had already materialized: candidate worktrees are keyed
`candidate-<nodeID>` per workspace — no run/attempt/leg namespacing — and
nothing reconciled `leg_state` claims against the dirs. `/tmp/fp-live3`
ended with **three** `leg_state=active` rows claiming
`candidate-candidate-a` (a vetoed leg, a dead-run residue leg, and a
quota successor). Any two of them running turns concurrently would have
interleaved writes in one directory.

## Changes

- `internal/runner/chat_ssot.go`: new `LegClosedReasonWorktreeSwept`
  (`"worktree_swept"`) — durable reason for legs closed because their
  dedicated worktree was swept.
- `internal/runner/tournament_dispatch.go`:
  - `sweepCandidateWorktrees` + `closeLegsBoundToWorktree` — every service
    candidate-worktree removal (spawn abort, stale-dir retry, orphan
    sweep, arbiter retry/discard, merge-done losers) now also closes legs
    whose session rows claim the dir. Claims can no longer outlive dirs.
  - `liveChild` → `liveClaim`: the Create-failure self-heal checks for
    any non-terminal `active` leg claiming the dir (any run, any cohort),
    not just same-attempt children of this parent. A live foreign
    claimant now parks the flow instead of losing its workspace.
- `internal/runner/quota_gate.go` (`respawnChildOnRoute`): when the leg's
  `WorkspaceCwd` is a dedicated binding (≠ parent's cwd), refuse the
  respawn if another non-terminal `active` leg claims the dir or the dir
  no longer exists. Refusal keeps the vetoed leg open per BUG-534; the
  next admission re-enters the gate.
- Tests: `TestBug535_WorktreeSweepClosesStaleLegClaims`,
  `TestBug535_LiveClaimantBlocksSweep`,
  `TestBug535_RespawnRefusesDirClaimedByLiveLeg`,
  `TestBug535_RespawnRefusesSweptWorktree`.
- Docs: `BUG-535-*.md` marked fixed; R8 runbook updated.

## Red test

The four `TestBug535_*` tests assert contracts that fail pre-change:
sweeps left stale legs `active`, a foreign live claimant's dir was
removable, and the respawn shared/absent dirs without complaint.
(`LegClosedReasonWorktreeSwept` is new, so the sweep tests cannot compile
pre-change — the live durable state is the reproduction evidence.)

## Verification

- `go test -count=1 -run "TestBug535|TestBug533|TestBug534"` + the wider
  BUG-5xx tournament/quota batch: PASS.
- `go test -race` on the touched batch: PASS; `go vet` + `go build`: OK.
- Merge/Leg/Switch/Worktree-adjacent batch: 3 fails, all classified —
  `TestBug334` (missing `opencode` binary, env baseline) and
  `TestBug516`/`TestStartTurnGrokCrossAccount` (TempDir cleanup flake;
  both pass isolated — same flake class recorded in CA-1042).

## Live evidence basis

`/tmp/fp-live3/.flowpilot/chats/sessions.ndjson` is the reproduction:
run-799 (attempt-0, veto-parked), run-1408 (attempt-1, dead parent), and
run-4150 (BUG-534 successor) all `leg_state=active` on
`candidate-candidate-a`. Post-fix the equivalent sequence cannot produce
overlapping active claims: sweeps close prior claims and the respawn
refuses live-claimed or missing dirs.

## Provider parity

Worktree lifecycle + leg bookkeeping — provider-agnostic by construction.

## Follow-ups

- Dir namespacing (`candidate-<run>-attempt<N>-<id>`) remains a possible
  hardening but is no longer required for correctness — claims serialize.
- A vetoed leg closed by a cohort-retry sweep keeps its pending quota
  card; answering it now fails honestly (dir gone → respawn refuses).
  A cosmetic follow-up could invalidate the card on leg close.
