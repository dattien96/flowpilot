# CA-987 — BUG-488: chat reattach fails closed when leg enumeration is unreadable

## What changed

`apps/local-runner/internal/runner/chat_ssot.go` +
`interactive_handlers.go`:

- `resolveChatIdentity` now returns `(chatID string, legSeq int, mode string,
  err error)` — previously it swallowed `ListProviderSessionsByChat` errors
  and computed `legSeq = persistedMax+1` from a memory-only scan.
- `createRun` maps the failure to HTTP 502 `chat_identity_unprovable`.

## Why

On reattach, the next leg sequence must be `max(persisted, memory) + 1`.
When the store read failed, the function fell back to the in-memory max
alone — after a restart the in-memory map is empty, so legSeq restarted at
0 or collided with a persisted leg. Duplicate legSeq corrupts the per-chat
leg ordering that manifest v2 and Drive restore depend on.

## Contract

- Caller-supplied `ChatID + LegSeq` remains trusted verbatim.
- Store read failure → 502 `chat_identity_unprovable`; no leg created.
- Persisted max-seq still wins over resident memory.

## Verification

- `bug488_reattach_legseq_test.go`: store fault on reattach → error; healthy
  persisted max seq still allocates correctly.
- Full runner package suite green.
