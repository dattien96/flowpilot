# BUG-489 — provider-switch leg-close persist failure silently dropped → dual-active legs after restart

- Status: `todo`
- Severity: **high** — durability contract violation: "every mutation must
  survive restart". A failed leg-close write leaves the old leg `active`
  on disk while memory says `closed` → after restart both legs are live.
- Found: 2026-09-25, deep audit pass 2.

## Root cause

`chat_switch.go` Phase C (~line 326):

```go
src.legState = LegStateClosed
src.legClosedReason = LegClosedReasonProviderSwitch
_ = s.persistProviderSession(sessionStateOf(src))
```

The error is dropped with no log and no degraded flag. (The SD26-X-6
`setDegraded` mechanism at `chat_ssot.go:217` only covers
`chatTranscripts.append` failures — `persistProviderSession` has no such
surface.) If the upsert fails (disk full, store corruption, IO error), the
persisted record keeps `status/legState` as it was — typically an
in-flight/active leg.

## Blast radius

1. Restart/reconstruct: the old leg reloads as a still-active leg alongside
   the new leg → two "active" legs for one chat → provider-call routing and
   leg lineage ambiguous (which leg owns the chat?).
2. Boot recovery sees a non-terminal record it believes is live → may
   resume/claim a leg that was superseded.
3. Silent: operator sees a successful switch response; nothing indicates the
   close was never committed.

## Fix contract

1. Do not silently drop the persist error in `switchChatProvider` Phase C:
   - Log loudly (`[chat-switch] leg-close persist failed run=... err=...`).
   - Mark the run/chat degraded via the existing
     `chatRuns.setDegraded(src.id, true)` surface so the failure is visible
     in timeline/timeline responses instead of only in stdout.
   - Attempt **one immediate retry** of the upsert before degrading
     (transient IO should not mark state degraded).
2. Keep the in-memory close (the switch is already committed — the new leg
   exists); the fix is about *surfacing* the durable-write failure, not
   rolling back.
3. Document in the response/log that the leg-close is uncertain when the
   retry also fails — the operator knows a restart may resurrect the leg.

## Required tests (RED first)

- `TestBUG489_LegClosePersistFailureMarksDegraded`: workflow store whose
  `UpsertProviderSession` fails → `switchChatProvider` completes the switch
  but `chatRuns.isDegraded(src.id)` is true and a retry was attempted.
- `TestBUG489_LegCloseTransientPersistRetrySucceeds`: store failing once
  then succeeding → no degraded flag, disk shows closed.
- `TestBUG489_LegClosePersistSuccessUnchanged`: healthy store → no degraded
  flag (positive control).

## Definition of Done

- A leg-close persist failure is never invisible: retry → degraded flag +
  log on final failure.
- Tests green, CA entry, commit.
