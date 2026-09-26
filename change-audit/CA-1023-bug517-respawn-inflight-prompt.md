# CA-1023 — BUG-517: cross-provider child respawn seeds the in-flight prompt

## Context

Grok review round 3: `respawnChildOnRoute` seeded the replacement child
from `child.lastPrompt` — the 100-char display-truncated title of a
previous turn. The refused turn's `in.Prompt` never reached any field, so
the replacement started on a stale, truncated instruction.

## Changes

- `quota_gate.go` `QuotaResolution`: new in-memory `PendingPrompt`
  carrier (`json:"-"`).
- `interactive_service.go` `startTurn`: sets `res.PendingPrompt =
  in.Prompt` before `CommitQuotaResolution` on the rotate path.
- `quota_gate.go` `respawnChildOnRoute`: takes the pending prompt;
  prefers pending → `lastFullPrompt` → `lastPrompt`, never seeding the
  100-char title when a fuller prompt exists.

## Tests

`bug517_respawn_inflight_prompt_test.go` — replacement's handoff prompt
equals the refused turn's prompt (asserted via the parent agent-bus
handoff message); fallback uses the full prior prompt, not the truncated
title; old leg still closes.
