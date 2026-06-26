# CA-095: Fix Desktop Live Approval Stamped Resolved On Run Switch

## Scope

- `apps/desktop-flowpilot/src/state/store.ts`
- `requirements/09-BugFix/done/BUG-079-Desktop-Live-Approval-Stamped-Resolved-On-Run-Switch.md`

Fixes BUG-079: navigating away from a live `waiting_approval` run and back incorrectly stamps the approval card as `"resolved"` and hangs the AI.

## Completed

### F-1 — `_streamRunSeq` state field

Added `_streamRunSeq: number` to the store state interface and initial state (`0`). Follows the same monotonic-counter pattern as `_historyLoadSeq` (BUG-060 F-3).

### F-2 — Increment on new stream start

`_streamRunSeq` is incremented (`get()._streamRunSeq + 1`) inside the `set()` call that precedes `consumeStream` in:
- `openHistoryRun` — navigating to any run (same or different runId)
- `reconnect` — reconnecting the current run from the Reconnect button

These are the two paths that start a new `consumeStream` for a `runId` that may already have a live stream instance.

### F-3 — `consumeStream` stale guard

`consumeStream` now captures `const mySeq = get()._streamRunSeq` at call time. The per-event guard is replaced with a local `isStale()` predicate:

```typescript
const isStale = () => !shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== mySeq;
```

Both the `for await` loop check and the post-loop check use `isStale()`. The original `shouldApplyRunEvent` check is retained as a secondary guard (still needed for run-switch-to-different-runId).

## Verification

- `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → **TypeScript: No errors found**
- Logic invariant verified by inspection (see BUG-079 V-2).
- End-to-end UI smoke test not run in this session.

## Residual Notes

- `_streamRunSeq` is intentionally NOT reset in `resetRun` or `selectProject` — those actions set `runId: undefined`, which already makes `shouldApplyRunEvent` return false for any pending stream. Keeping the counter monotonic avoids wrap-around edge cases.
- `timelineReducer.ts` (BUG-074 F-1 stale detection) is unchanged — the invariant it relies on is now restored: only one active `consumeStream` per run at any time.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: BUG-079
change_type: fix
summary: Fix Desktop Live Approval Stamped Resolved On Run Switch
# --->8---
