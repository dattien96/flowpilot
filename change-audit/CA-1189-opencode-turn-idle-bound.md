# CA-1189 — opencode ACP turn had no liveness bound (BUG-1189)

## Defect

`opencodeAdapter.SendTurn` waited on three channels with no upper bound:
`session/prompt` RPC result, ctx cancel, and the session notification stream.
A dead or wedged ACP peer that never answers the RPC AND never emits (or
closes) notifications left `SendTurn` blocked forever — the run displayed
"running" with nothing able to settle it, the exact dead-end class that
poisoned the live CP-03 hub. Separately, the post-result blocking drain reset
its inactivity timer on every notification, so an endless keepalive/usage
stream could extend a finished turn indefinitely.

## Fix

- `opencodeTurnIdleBound` (5m): idle timer around the main turn select; resets
  on each session notification. On expiry SendTurn emits `session/cancel` and
  returns an `idle` error, so the turn settles as a failure the coordinator
  can observe instead of hanging silently.
- `opencodePostResultDrainCap` (90s): absolute wall-clock cap on the
  post-result blocking drain, independent of the per-notification inactivity
  reset.

Both bounds are package vars so tests can shrink them.

## Regression tests

`TestBug1189_SendTurnFailsOnSilentPeer` — fake ACP peer answers `session/new`
but never answers `session/prompt` and keeps the notification stream open and
silent; SendTurn must return an `idle` error within the shrunk bound instead
of hanging.
