# CA-978: BUG-477 — durable knowledge-update ledger with crash replay

## Change

The audit-completion knowledge refresh is no longer fire-and-forget: changed
paths are persisted to a durable per-workspace ledger before the audit hook
acknowledges, replayed under the per-workspace lock, and committed only after
the knowledge files are written.

## Root cause

`updateKnowledgeForAudit` ran `go UpdateAsync(...)` with no durable intent.
A runner killed after the audit hook returned but before the worker wrote
its sections lost the update permanently — the next boot saw a healthy
`index.json` and stayed silent, leaving planner/scout context describing
pre-audit architecture.

## Fix

- New `knowledge_update_ledger.go`: versioned
  `.flowpilot/knowledge/pending-updates.json` ledger — atomic append
  (temp+rename), bounded dirty set (512 paths → folds to a single
  `full:true` rebuild tombstone), per-intent IDs with monotonic `nextId`.
- `replayKnowledgeUpdates` is the leased worker: per-workspace lock,
  coalesces all pending intents, runs `IncrementalUpdate` (or `WriteFull`
  on a `full` tombstone), commits by intent ID only after files land —
  re-reading the ledger so intents appended mid-replay survive.
- Fail-closed semantics: corrupt ledger → full rebuild + clear;
  corrupt/mismatched index → `IncrementalUpdate`'s existing full-rebuild
  fallback; missing index → intents stay pending for the bootstrap path
  (it owns mid-flow rebuild decisions) and a successful bootstrap
  `WriteFull` clears the ledger since a full distill subsumes pending paths.
- `updateKnowledgeForAudit` appends the intent before spawning the worker;
  an append failure falls back to the old best-effort `UpdateAsync`
  (never blocks a flow). The hook stays a strict no-op on unbootstrapped
  workspaces (preserves `TestAuditHookSkipsUnbootstrappedWorkspace`).
- `ensureKnowledgeBaseForWorkspace` replays leftover intents on bind —
  crash recovery without a new audit event.

## Evidence

- RED: `TestBUG477_AuditCompletionPersistsDurableUpdateIntent` failed —
  no durable intent existed before this change.
- GREEN: all 6 BUG-477 tests pass — crash replay, retry-on-failure,
  corrupt-ledger rebuild, multi-intent coalesce, missing-index deferral,
  bootstrap clear.
- Regression found + fixed during verification: the first draft persisted
  intents on unbootstrapped workspaces, tripping
  `TestAuditHookSkipsUnbootstrappedWorkspace`; append is now gated on
  `!knowledge.Missing`.

## Risk

- Ledger writes are tiny (bounded intents) and serialized per workspace.
- `knowledgeUpdateRetryDelay` bounds replay to 3 attempts; leftovers stay
  durable — never silently dropped, never unbounded.
- No signature changes to `knowledge` package APIs.
