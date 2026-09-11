# CA-796 — vibe owner fail settles immediately, not hub_stalled

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Failed vibe-owner-debate members retry or park cap immediately; do not wait 2m for hub_stalled
# --->8---

## Why

Live run-219176 restore: `debate_trigger` done, `owner_1`/`owner_2` Failed, `debate_synthesis` never ran. Failed children are not busy (CA-616), so the hub sat empty until BUG-289 F-0 parked `hub_stalled` with Dev-shaped Retry/Stop/Revise. V6 / SS-18: after lock, user asks are `r-requirement` and Owner-cap — not a 2-minute stall card.

## Change

- `maybeSettleVibeOwnerDebate`: both owners terminal-failed and synthesis not started → retry `vibe-owner-debate` (max 2) or park `BlockReason: cap`. Never `hub_stalled`.
- Hook: `checkAndBlockStalledHub` (before park), owner `EventTurnFailed`, start-turn fail incomplete cohort, reconstruct parent.
- `defaultHubStallTimeout` stays 2m (harness/dev last-resort). Not raised to 10–15m.

## Tests

New `ca796_vibe_owner_fail_no_hub_stalled_test.go`: stall intercept, cap park not hub_stalled, harness still hub_stalled, live settle. Old CA-792 / hub_stall tests untouched.

## Providers

Agnostic: `maybeSettleVibeOwnerDebate` / stall intercept take no `providerKey`. Tests use Codex `newTestServer` (Claude/Grok controlled runtime not implemented there). Same function for all three.

## Will not undo

CA-792 debate overlay / no Dev 1/2/3 on synthesis verdict. CA-794/795 TDD signatures + `tdd=agent.code`. CA-616 terminal child not busy. BUG-289 F-0 timeout unchanged.

## Residual

Owner-cap park after retries is still a user interrupt (SS-18 AC-6), not `r-requirement`. YOLO/restore can still fail owners; we no longer wait 2m to say so. `vibeOwnerFailRetries` is RAM-only (restart resets the counter; durable `cap` park survives).
