# CA-1025 — BUG-513 round 4: query-scoped legs blind to compaction; BUG-517 admission-path test

## Context

Grok review round 4 raised two residual issues after round 3:

1. **BUG-513**: the round-3 near-window gate is still unsound. Claude's
   `snap.Last` is the single query's token aggregate, not session-window
   occupancy — a long query (prev ≥80% of the window) followed by a
   short one satisfies "near-full prev + >30% drop" without the provider
   compacting anything. No drop-based heuristic over per-query usage can
   distinguish "provider compacted" from "operator sent less context".
   Dormant today (FLOWPILOT_CONTEXT_PRESSURE default-off) but a real
   mis-fire when enabled.
2. **BUG-517**: the fix was correct but the tests bypassed admission —
   `respawnChildOnRoute` was called directly with the pending prompt, so
   deleting `res.PendingPrompt = in.Prompt` left every test green.

## Changes

- `context_pressure.go` `evalContextPressureLocked`: legs marked
  query-scoped (present in `rs.legUsageTotals`, set by the accumulation
  seam) never emit `provider_compacted` and are never marked
  `context_degraded`. Compaction detection stays limited to cumulative
  providers (Devin/Grok/Codex) whose `Total` genuinely tracks session
  usage. Query-scoped `Total` accumulation for cap accounting is
  untouched. The pressure ladder (aware/ask tiers off `Last`/window) is
  unchanged — it asks the operator rather than claiming compaction.
- `bug517_respawn_inflight_prompt_test.go`: added
  `TestBug517_AdmissionCarriesInflightPromptToRespawnedChild` — enters
  through `startTurn` with a child pinned to a ledger-blocked codex
  account, auto mode, healthy claude candidate; asserts the refused
  turn's prompt (not the stale `lastPrompt`) reaches the replacement
  child's handoff. Verified RED without the `PendingPrompt` assignment.

## Tests

- BUG-513 file rewritten: `StaysBlindToPerQueryDrop` (Grok's exact
  900→400 on window-1000 shape → no flag, no degrade, Total still
  accumulates to 1300), growth/short-turn/unknown-window cases kept,
  `CumulativeLegStillTracksTotalDrop` unchanged.
- Full BUG-513..517 + Task-442/443/449 + switch/quota/reprompt sweep:
  green; only failure is the known `TestBug414` GitNexus auto-index
  TempDir cleanup race (identical signature to the documented flake).

## Residual

- Claude-side compaction is genuinely undetected — correct posture until
  a real session-fullness signal exists. Unit-only; no live Claude
  account on this machine.
- BUG-517 organic child cross-provider route remains a live gap; the
  admission path is now unit-locked end-to-end.
