# CA-1091: last-child-settle never drains parent's armed pendingHubReinvoke — silent ~10m wedge

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — owner-debate
mounted on Task-015's failing contract-test gate.

## Symptom (live)

- 19:46:57 — cohort join stamped `debate_synthesis` RUNNING and scheduled the
  hub reinvoke; `startTurn` refused `hub_parked` (owner children still
  settling) → `hub_reinvoke_start_failed` re-armed `pendingHubReinvoke`.
- 19:47:52 — last owner child settled. `notifyTurnIdle(child)` ran the
  child-settle parent branch: `flushDurableTurnIntents(parent)` — which has no
  `pendingHubReinvoke` leg — so the armed reinvoke was stranded.
- 19:47:52 → 19:57:26 — ~10 minutes of silence: loop "running",
  `debate_synthesis` RUNNING, zero hub turns. The F-0 stall watchdog could not
  surface it: `pendingHubReinvoke` is deliberately NOT busy (BUG-289 H1), and
  CA-1088's `debateMounted` guard re-arms the tick while a debate is mounted —
  so the loop neither blocked nor drained. Only surfaced once the debate
  happened to discharge, letting a later tick block `hub_stalled`.

## Root cause

Two missing drains for an armed-but-undrained `pendingHubReinvoke`:

1. `notifyTurnIdle`'s child-settle parent branch only called
   `flushDurableTurnIntents(parent)` (gate-reprompt/resume/restart intents) —
   `pendingHubReinvoke` is drained only by `notifyTurnIdle(parent)` itself,
   which nothing invoked on the last child's settle.
2. Inside the `debateMounted` watchdog guard, an armed pending reinvoke was
   re-armed forever — the one topology where `hub_stalled` must never fire is
   exactly where a stranded reinvoke had no other escape.

## Fix (`internal/runner` only)

- `notifyTurnIdle` (interactive_resume.go): on last-child-settle, also
  `notifyTurnIdle(parentID)` — drains armed `pendingHubReinvoke` (and, when
  armed, `pendingHubReinvokePrompt` verbatim) through the existing drain leg.
- `checkAndBlockStalledHub` (hub_stall.go): inside the `debateMounted` guard,
  consume+refire an armed `pendingHubReinvoke` (single-shot under `s.mu`,
  `pendingHubReinvokePrompt` preserved) instead of blind re-arm. BUG-289 F-0's
  non-debate contract is untouched: a stranded pending on a plain topology
  still surfaces `hub_stalled` as an actionable card.

## Verification

- Red→green: `ca1091_child_settle_drains_pending_hub_reinvoke_test.go` —
  (a) `TestCA1091_LastChildSettleDrainsParentPendingHubReinvoke`: armed
      pending + settled owner child → before fix, no adapter send + flag stays
      armed (3s timeout = the live wedge); after fix, the verbatim synthesis
      prompt reaches the adapter and the flag clears;
  (b) `TestCA1091_StallTickDrainsPendingHubReinvoke`: mounted-debate topology,
      stall aged out → tick drains the armed reinvoke to the adapter, loop
      stays running (no hub_stalled);
  (c) `TestCA1091_NoDrainWhileFlowChildActive`: live child turn still parks
      the armed reinvoke — drain waits for settle.
- `TestBug289_F0_HubStallBlocksWhenIdleTooLong` still passes — non-debate
  stranded pending still blocks `hub_stalled` by design.
- Regression sweep: HubStall/StalledHub/CA1087/CA1088/CA1090/NotifyTurnIdle/
  Reinvoke/ResumePending/CohortJoin/OwnerDebate/VibeOwner/Bug28x — all green.
