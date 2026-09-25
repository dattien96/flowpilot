# CA-992 — BUG-493: history/timeline endpoints surface store errors instead of partial views

## What changed

`apps/local-runner/internal/runner/`:

- `projectRunHistory` → `([]runHistoryItem, error)`; the handler returns
  502 `run_history_unavailable`. Previously both
  `ListProviderSessionsByProject` calls (syncStatus map + non-resident
  merge) folded errors into an in-memory-only history.
- `handleChatTimeline` (chat_timeline.go): `ListProviderSessionsByChat`
  failure → 502 `chat_legs_unavailable` instead of a leg list missing
  every non-resident leg.
- `handleGetRunTimeline` (run_timeline.go): the not-resident fallback's
  `GetProviderSession` failure → 502 `run_lookup_unavailable` instead of a
  misleading `run_not_found` 404.
- `listAgentRunSummaries` → `([]AgentRunSummary, error)` (shared with
  BUG-491): `BuildChatSessionSyncManifest` aborts with 502;
  `handleListAgentRuns` returns 502 `session_index_unavailable`.

## Why

A 200 response with a partial view is worse than an error — the desktop
renders it as complete truth (missing synced runs, missing legs, phantom
"not found"). BUG-475 set the precedent: an unreadable authority returns a
typed retryable error.

## Verification

- `bug493_history_view_swallow_test.go`: history propagates the fault;
  chat timeline and run-timeline fallback return non-200 on fault; healthy
  paths verified unchanged.
- Runner package suite green.
