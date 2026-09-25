# BUG-488 — chat reattach swallows session-store read error → duplicate legSeq → corrupted leg ordering

- Status: `todo`
- Severity: **high** — a single store read error silently mints a leg with a
  legSeq already used by a persisted leg; manifest v2 ordering, Drive
  restore ordering and leg lineage all key on legSeq.
- Found: 2026-09-25, deep audit pass 2.

## Root cause

`chat_ssot.go` `resolveChatIdentity` reattach path (~line 114):

```go
if reader, ok := s.workflowStore.(ChatSessionReader); ok {
    if rows, err := reader.ListProviderSessionsByChat(ctx, in.ChatID); err == nil {
        for _, row := range rows { if row.LegSeq > maxSeq { maxSeq = row.LegSeq } }
    }
}
return in.ChatID, maxSeq + 1, in.SwitchFromRunID
```

After BUG-485, `ListProviderSessionsByChat` **returns an error** when the
session store loaded partially (fat line, torn tail). Here that error is
swallowed: `maxSeq` is derived from in-memory runs only, so a persisted leg
that failed to load is invisible and the new leg is assigned a legSeq that
**already exists on disk**.

## Blast radius

- Two legs with the same `(chatID, legSeq)` → manifest v2 ordering
  ambiguous, Drive restore may order/overlay legs wrongly, timeline leg
  iteration loses a leg silently.
- Downstream consumers (chat_sync_manifest, timeline, BUG-476 sync) treat
  legSeq as a unique ordering key — the duplicate violates that assumption
  at mint time.
- Direction of failure: on a store fault we mint corrupt state instead of
  refusing — exactly the "unreadable authority is not empty authority"
  contract violation.

## Fix contract

`resolveChatIdentity` gains an error return. On `ListProviderSessionsByChat`
error: propagate → `handleStartRun` fails the run start with a typed,
retryable error (`502 chat_identity_unprovable`-class, matching the
BUG-475 `attention_list_failed` precedent). A store that cannot prove the
max legSeq cannot safely mint a new leg. In-memory scan alone is sufficient
only when the store itself is healthy.

## Required tests (RED first)

- `TestBUG488_ReattachStoreErrorRejectsRunStart`: fake `ChatSessionReader`
  returning error on `ListProviderSessionsByChat` + `in.ChatID` set →
  `resolveChatIdentity` returns error (pre-fix: returns legSeq=1).
- `TestBUG488_ReattachHealthyStoreKeepsMaxSeq`: store returns leg with
  legSeq=3 not in memory → new leg gets legSeq=4 (positive control).

## Definition of Done

- `resolveChatIdentity` cannot produce a legSeq when the session store is
  unreadable.
- HTTP surface returns a typed retryable error, not a minted corrupt leg.
- Tests green, CA entry, commit.
