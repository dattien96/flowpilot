# BUG-675 — Dead Owner-Debate Mount Burns the Sprint Cap (Mount-Cap Cohort Kill)

## Metadata

- Document ID: `BUG-675`
- Title: `vibeDebateMounts ledgered at stash time; a mount that dies before the overlay lands is never refunded — false cap parks the sprint and kills the live cohort`
- Phase: `bugfix`
- Status: `FIXED — CA-1238`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Parent Documents: `BUG-595` (mount budget), `BUG-1182` (per-entity ledger), `BUG-594`, `CA-1095`
- Tags: `vibe-debate, mount-cap, cohort, severity:high`

## Evidence

- `stashVibeFlowForDebate` increments `vibeDebateMounts` and
  `vibeDebateMountsByEntity[entity]` **before** `startResolvedFlow` mounts
  the overlay (`vibe_cp.go:1125`).
- `maybeReleaseVibeDebateClaimIfMountDied` releases the claim when the
  overlay never materializes — but never refunds the charged mount.
- Three dead mounts (dispatch died mid-resolve) reach
  `maxVibeDebateMountsPerSprint` → `escalateVibeDebateMountCap` parks the
  sprint `blocked/cap` and `parkFlowForAwaitingUser` freezes/cancels the
  live cohort underneath: phantom mounts counting as real remediation kill
  real work.

## Fix

- `maybeReleaseVibeDebateClaimIfMountDied` (non-foreign branch — the
  provably-dead mount) refunds `vibeDebateMounts` and the entity ledgered
  first at stash (`vibeParkedGatedRunIDs[0]` via `vibeDebateEntityKeyLocked`,
  falling back to the `"hub"` bucket for un-gated diverts), floored at 0.
- A claim whose overlay DID mount and conclude keeps its budget — refund
  applies only to the mount-died release path.

## Regression Tests

- `bug675_mount_cap_test.go` — dead mount refunds total+entity; concluded
  debate (debate graph live) keeps its mount.
