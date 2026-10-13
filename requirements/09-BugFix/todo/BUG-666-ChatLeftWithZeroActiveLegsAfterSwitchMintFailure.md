# BUG-666 — Chat Left With Zero Active Legs After Switch Successor Mint Failure

## Metadata

- Document ID: `BUG-666`
- Title: `Chat left with zero active legs after provider-switch successor mint failure`
- Phase: `bugfix`
- Status: `FIXED — CA-1237`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Last Updated: `2025-01-24`
- Parent Documents: `SD-26 (chat provider switch)`, `CP-89 (flowArm latch)`
- Related Documents: `BUG-489`, `BUG-516`, `BUG-541`, `BUG-1187`, `BUG-667`
- Tags: `chat-legs, provider-switch, durability, recovery, severity:high`

## AI Quick View

### Summary

- A committed provider switch closes the source leg in Phase C; when the successor leg mint (Phase B `createRun`) fails after the durable intent is stamped, the chat is left with **zero active legs**.
- `healChatLegsLocked` has no row for "closed leg + no successor": its orphan-intent arm only clears `switchFromRunID` on a lone **active** leg — a chat whose legs are all closed/terminal heals nothing.
- Every subsequent `startTurn` on the closed leg scans `activeChatLegsLocked`, finds no `LegStateActive` leg, and fails `leg_closed` — permanently. Hub reinvokes on the flow-owner leg hit the same wall (see BUG-667 for the accounting defect that turns this into a silent permanent stall).

### Current Ask

- Add a heal row: chat with a closed leg carrying `switchFromRunID` intent (or a switch record gap) and **no active successor** must either (a) re-arm the intent for a retry surface, or (b) surface an explicit `leg_recovery_required` state — never silently dead.

### Key Decisions

- `V-1` The durable record is the single source of truth: zero-active-leg is a reachable durable state today, so it must be a first-class recoverable state, not an invisible one.

### Constraints

- Never auto-mint a successor leg in the heal path (provider pinning + user routing decision must stay explicit).
- Heal must be idempotent and crash-safe (SD26-S-2 matrix).

### Source Refs

- live `run-752742` leg `leg_state=closed (provider_switch)` with **no successor minted**; all `hub_reinvoke_start_failed` events carried `"this run's leg was superseded; resubmit on the chat's active leg"` while no active leg existed.
- `chat_switch.go` `healChatLegsLocked` (two-active / closed-no-record / orphan-intent rows only).

## 1. Issue Summary

During live run `run-752742` (PrivateVault vibe-sprint), the chat's leg was closed with `legClosedReason=provider_switch` but the successor leg was never minted (mint failure / dropped intent after Phase-A durable stamp). The chat then had **no active leg at all**: every hub turn and user turn failed `leg_closed`, and there was no surface to recover from other than manually minting a reattach leg via `startRun` with `chatId+switchFromRunId`.

## 2. Parent Links

- impacted tech design: SD-26 §7.1 (three-phase switch), §8 (crash-window heal matrix)

## 3. Environment and Reproduction

- environment: local-runner, chat with provider switch where successor mint fails after intent stamp
- reproduction: stamp `switchFromRunID` on leg A (Phase A) → crash/fail before `createRun` returns (Phase B) → restart. Leg A is closed or active-with-intent; no leg B exists.
- frequency: rare (crash window) but observed live

## 4. Expected vs Actual

- expected: heal matrix covers "no successor" — the chat can be re-driven or surfaces a repair state
- actual: chat is permanently dead; every turn fails `leg_closed` with no recovery surface

## 5. Impact

- users affected: any chat/flow whose provider switch hits the Phase-B failure window
- workflows affected: entire vibe-sprint runs (hub cannot dispatch a single turn)
- severity: high — silent total deadlock, requires manual API surgery

## 6. Root Cause

- confirmed cause: `healChatLegsLocked` handles only (1) two-active, (2) closed-no-record, (3) orphan-intent-on-active-leg. A **closed** source leg with `switchFromRunID` stamped and zero active legs matches no row.
- evidence: `chat_switch.go:107-140`; live run-752742 leg rows.

## 7. Fix Strategy

- `F-1` Add heal row: `closedSrc` exists (provider_switch/context_reset close), **no** `newLeg`, **no** active legs → surface a typed `leg_recovery_required` state on the most recent closed leg (queryable via run detail + events), and re-arm the durable intent marker so an explicit retry (reattach/`startRun`) can proceed without stale-state conflicts.
- `F-2` Do NOT auto-mint (pin/routing are user decisions). F-1 surfaces + enables; it never decides.
