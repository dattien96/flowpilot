# BUG-493 — history/timeline endpoints swallow store errors → silently partial views

- Status: `todo`
- Severity: **medium** — display-level, but same class as BUG-475: an
  unreadable authority must not masquerade as a smaller-but-valid answer.
- Found: 2026-09-25, deep audit pass 2.

## Root cause

| Site | Feed | On store error |
|------|------|----------------|
| `chat_timeline.go:~125` | chat timeline legs | legs from memory only — persisted legs silently absent from the chat view |
| `run_timeline.go:~53` | run timeline session enrich | session fields dropped silently |
| `interactive_handlers.go:~1398` | history `persistedSyncByRunID` | every item reports `syncStatus=""` → desktop "unsynced" badge re-targets already-synced chats |

Each site uses `if …; err == nil { … }` — the handler returns 200 with a
view that is wrong in a way the client cannot detect (vs BUG-475's fix:
typed error → 502 so the client resyncs).

## Fix contract

Surface the error to the handler → typed retryable HTTP error (502-class,
`history_load_failed`-style), matching the BUG-475 `attention_list_failed`
precedent. Clients already handle 5xx as retryable; a wrong-looking 200 is
not recoverable.

Per-site semantics check before choosing propagate-vs-degrade:
- timeline legs: propagate (a timeline missing legs is a wrong answer).
- syncStatus enrichment: propagate at the list level OR mark items
  `syncStatus:"unknown"` — choose whichever the handler contract supports;
  the key is the client can tell "unknown" from "definitely unsynced".

## Required tests (RED first)

- `TestBUG493_ChatTimelineStoreErrorReturns502`-class: reader error →
  handler error, not partial legs.
- `TestBUG493_HistorySyncStatusStoreErrorNotEmpty`: reader error → error
  surfaced or items carry explicit unknown marker, not `""`.
- Positive controls with healthy reader.

## Definition of Done

- No history/timeline endpoint returns a silently-partial view on store
  fault.
- Tests green, CA entry, commit.
