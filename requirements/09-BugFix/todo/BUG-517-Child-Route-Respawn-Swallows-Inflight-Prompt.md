# BUG-517 — Cross-provider child respawn swallows the in-flight prompt

Status: FIXED (unit; organic child-route live leg pending)
Filed: 2026-09-26 (Grok deep-review round 3)
CA: CA-1023

## Symptom

A flow child whose pinned account hard-vetoes admission enters the quota
gate. On a cross-provider commit, `respawnChildOnRoute` seeded the
replacement child from `child.lastPrompt` — the 100-char display-
truncated title of a PREVIOUS turn. The turn that triggered routing
(`in.Prompt`) was returned as `quota_route_committed` and never stored,
so the replacement child started on a stale, truncated instruction: the
blocked prompt was swallowed.

## Root cause

`startTurn` returns `quota_route_committed` before the
`lastPrompt`/`lastFullPrompt` assignment runs — the in-flight prompt
existed only in `in.Prompt` and died with the call. `respawnChildOnRoute`
then read `child.lastPrompt`, which is both stale (previous turn) and
lossy (`truncateDisplayField(..., 100)`).

## Fix (CA-1023)

- `QuotaResolution.PendingPrompt` (in-memory, `json:"-"`) carries the
  refused turn's prompt through the commit.
- `startTurn` sets `res.PendingPrompt = in.Prompt` before
  `CommitQuotaResolution` on the rotate path.
- `respawnChildOnRoute` takes the pending prompt and prefers:
  pendingPrompt → `child.lastFullPrompt` → `child.lastPrompt` — never
  the truncated title when a fuller prompt exists.

## Tests

`bug517_respawn_inflight_prompt_test.go` —

- `TestBug517_RespawnSeedsInflightPrompt`: replacement child's handoff
  prompt is exactly the refused turn's prompt (asserted via the parent
  agent-bus handoff message, the synchronous record of `in.Prompt`).
- `TestBug517_RespawnFallsBackToFullPromptNotTruncatedTitle`: no pending
  prompt → the FULL prior prompt (200 chars) is used, not the 100-char
  title; old leg still closes.

## Round 4 (2026-09-26) — admission-path regression test

Grok review round 4: both tests above call `respawnChildOnRoute` with the
pending prompt already in hand — deleting `res.PendingPrompt = in.Prompt`
would leave them green.

Added `TestBug517_AdmissionCarriesInflightPromptToRespawnedChild`: enters
through the real `startTurn` admission path — flow child pinned to a
ledger-blocked codex account, auto mode, healthy claude candidate.
`startTurn` refuses with `quota_route_committed`, commits the rotation,
and the replacement child's handoff is the refused prompt — with a stale
`lastPrompt` planted to prove the in-flight prompt wins. Verified RED
when the `PendingPrompt` assignment is removed (replacement got the
stale title).
