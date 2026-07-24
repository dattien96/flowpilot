# BUG-317: New Flow Turn Fails Immediately — "dispatch record already exists" After Deleting Old Chats Re-Mints Their IDs

## Metadata

- Document ID: `BUG-317`
- Title: `Starting a new flow chat fails instantly with "dispatch record already exists" (HTTP 502 / dispatch_prepare_failed) because the id counter re-mints a run/turn id whose dispatch records survived a delete-from-history`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-23`
- Last Updated: `2026-07-23`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md)
- Child Documents: `none`
- Related Documents: [BUG-315: Restored Flow Chat Reruns Whole Flow On Followup](../done/BUG-315-Restored-Flow-Chat-Reruns-Whole-Flow-On-Followup-Sync-Dropped-TurnCount.md) (same testing session's delete/restore workflow exposed this), [BUG-316: Restored Grok Chat Fails Next Turn](../done/BUG-316-Restored-Grok-Chat-Fails-Next-Turn-Session-Directory-Incomplete.md) (the delete step that left orphan dispatch records), [CA-411](../../change-audit/CA-411-seed-id-counter-past-persisted-dispatch-records.md)
- Replaces: `none`
- Tags: `agent-flow-engine, dispatch, id-counter, cross-provider, latent-bug, severity-critical`

## AI Quick View

### Summary

A brand-new flow chat (any provider) dies on its very first hub turn with
`dispatch record already exists (HTTP 502 / dispatch_prepare_failed)` — hub
Failed, 0/N steps, no children spawn. Root cause: the id counter is seeded only
from the session index (BUG-117). Deleting a chat from history removes its
session-index entry and its `run-<id>-turns.ndjson` but NOT its per-project
`dispatch.ndjson` records. On the next start the counter re-seeds BELOW the
deleted run's numeric suffix, so `nextID` re-mints a run/turn id a surviving
dispatch record still owns; `prepareDispatchV2` -> `CreatePrepared` then rejects
the reused id (existing record, different envelope hash) with `ErrAlreadyExists`.

### Current Ask

Seed the id counter past every id persisted in the durable dispatch store, not
just the session index, so a re-mint can never collide with an orphan record.

### Key Decisions

- Fix at the SEED layer (not by purging dispatch records on delete): within a
  single process the counter only ever increases, so seeding correctly at boot
  fully closes the collision AND heals existing orphans (the counter simply jumps
  past them). Purging an append-only shared log on delete is more invasive and
  would not heal orphans already on disk.
- Seed from the dispatch store in `SetDispatchStore`, not in the constructor's
  `seedIDCounter`: the durable store is wired AFTER the constructor runs, so the
  constructor cannot see it.
- Provider-agnostic by construction: the orphan record and the re-minted run are
  often different providers entirely (a deleted grok run's id re-minted for a new
  claude flow), so the fix keys on numeric id suffix, never on provider.

### Constraints

- Additive tests only; no existing test call site changed.
- No real machine paths in tests (temp chats roots + fake session store only).

### Open Questions

- `none` (delete-side purge of orphan dispatch records is a documented optional
  hardening in section 8, not required by this fix).

### Source Refs

- `apps/local-runner/internal/runner/dispatch_live.go` — `SetDispatchStore`,
  `seedIDCounterFromDispatchStore`, `dispatchIDCeiling`.
- `apps/local-runner/internal/runner/dispatch_store_memory.go` —
  `memoryDispatchStore.MaxPersistedIDSuffix`.
- `apps/local-runner/internal/runner/dispatch_store_open.go` —
  `multiProjectDispatchStore.MaxPersistedIDSuffix`.
- `apps/local-runner/internal/runner/interactive_service.go` — `seedIDCounter`
  (BUG-117, the session-index-only seed this complements).

## 1. Issue Summary

Creating a new chat and running a flow with Claude failed instantly:
`dispatch record already exists (HTTP 502 / dispatch_prepare_failed)` on
`run-53890`. The hub turn never started, all steps stayed pending, no agent
child spawned. Reproduces for any provider — it is not Claude-specific.

## 2. Parent Links

Belongs to CP-51 (durable turn dispatch). The failing surface is
`prepareDispatchV2` -> `DispatchStore.CreatePrepared`, the CP-51 Task-249 prep
barrier. The id-counter seed it collides with is BUG-117.

## 3. Environment and Reproduction

Runner started with `just dev` from `C:\working\flowpilot`; target project
`gate-sandbox` (project id `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`). Dispatch V2
is default-on and Claude is V2-enabled.

Reproduction (matches the operator's live session):

1. Run several flow/chat sessions so the id counter climbs high (e.g. run-53890,
   turn-53903).
2. Delete the highest-numbered chats from history (BUG-315/BUG-316 testing did
   exactly this). This drops them from the session index and deletes their
   `run-<id>-turns.ndjson`, but leaves their `dispatch.ndjson` records.
3. On the next runner start the id counter re-seeds from the (now lower)
   session-index max.
4. Create a new chat and run a flow. As `nextID` climbs back through the deleted
   range it re-mints an id a surviving dispatch record still owns ->
   `dispatch record already exists`.

On-disk evidence at the time of the failure:

- `.flowpilot/chats/db51ec26.../dispatch.ndjson` held 28 lines for `run-53890`,
  including two terminal grok turns `turn-53892` (envelope hash `1d28b435...`)
  and `turn-53903` (`2e17f15b...`).
- No `run-53890-turns.ndjson` existed (the run was deleted from history).
- Runner log: persisted-run count fell 50 -> 40 -> 33 as chats were deleted, and
  `id counter seeded to 53698 from 33 persisted runs` — i.e. below `turn-53903`.

## 4. Expected vs Actual

- Expected: a new flow chat starts its hub turn and spawns its entry nodes.
- Actual: `CreatePrepared` returns `ErrAlreadyExists` (existing record, different
  envelope hash) -> `dispatch_prepare_failed` (HTTP 502); the hub Fails with
  0/N steps.

## 5. Root Cause

`seedIDCounter` (BUG-117, 2026-06-22) advances the id counter past the largest
numeric suffix among persisted RUN IDS **in the session index only**. The CP-51
durable dispatch store is a SECOND durable source of ids handed out before a
restart, keyed by `(run_id, turn_id)` in per-project `dispatch.ndjson`. The two
subsystems do not know about each other.

Deleting a chat from history removes its session-index entry and turns file but
NOT its dispatch records. So the session-index max can drop below a suffix that a
surviving dispatch record still uses. The counter re-seeds to that lower max and
`nextID` re-hands-out the id. `CreatePrepared`'s create-if-absent path treats an
existing record with a **different** envelope hash as `ErrAlreadyExists` (the
envelope hash includes the fresh `CreatedAt` and the new turn's content, so a
re-minted turn never matches the orphan's hash). Result:
`dispatch_prepare_failed`.

This is NOT a regression from BUG-312..BUG-316 — none of them touch id seeding or
dispatch prepare. It is a latent gap between BUG-117 and CP-51, first exposed by
deleting many old hubs from history during the BUG-315/316 testing pass.

## 6. Fix Strategy

Seed the id counter past the dispatch store's persisted ids too, at the point the
store is wired:

- `memoryDispatchStore.MaxPersistedIDSuffix` and
  `multiProjectDispatchStore.MaxPersistedIDSuffix` report the largest numeric
  suffix among all persisted records (run id + turn id) plus the non-prunable
  per-run activation / stop-state ids. `newMultiProjectDispatchStore` already
  loads every shard into RAM at construction, so this reflects orphan records
  from deleted chats.
- `dispatchIDCeiling` is an OPTIONAL interface (both stores satisfy it); a store
  that does not is simply skipped.
- `SetDispatchStore` calls `seedIDCounterFromDispatchStore` after wiring the
  store, lifting the counter to at least that ceiling. It runs there rather than
  in the constructor because the durable store is wired after the constructor's
  `seedIDCounter`.

Because a running process's counter only ever increases (atomic add), a correct
boot seed fully closes the collision — no intra-process re-mint is possible — and
existing orphans become harmless (the counter starts above them).

## 7. Validation

- Red-first: `TestSetDispatchStoreSeedsIDCounterPastOrphanDispatchRecords`
  (grok/claude/codex) fails on the pre-fix tree — `nextID after SetDispatchStore
  = suffix 11, want > 53903` for every provider — using only existing public API
  so it compiles on the baseline. Passes after the fix.
- `TestMaxPersistedIDSuffixReflectsPersistedDispatchRecords`: empty store -> 0;
  memory store max over both run and turn ids; multi-project shard reload FROM
  DISK (the real restart path) reports the cross-shard max.
- `go build ./...`, `go vet ./internal/runner/...`: clean. Targeted sweep of
  every dispatch / seed / SetDispatchStore-caller test: 112 passed.
- Full-package sweep (`go test ./internal/runner/`): fix = 2178 passed / 18
  failed; `git stash` baseline (fix removed) = 2172 passed / 16 failed. The 16
  are the identical pre-existing machine/CLI/env set (Codex CLI absent x6,
  SkillsMerge x3, GitCommitGuard, NextAccountHomeGrok, ParseGoogleDriveMcp,
  RunCompatCheck, DefaultProviderSessionHome, StartInteractiveAuth). The two
  extra on the fix run are proven pre-existing flakes:
  `TestGeminiAdapterPromptPrepAndEnv` passes in isolation (order-dependent), and
  `TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID` flakes IDENTICALLY on
  the baseline (5 isolated baseline runs: pass, pass, FAIL, pass, FAIL — a
  Windows `t.TempDir` teardown race; the test hardcodes its run/turn ids so the
  seed change cannot affect it).
- The real `provider-accounts.json` was verified unchanged (8 accounts, correct
  active flags) after the sweeps.
- Live real-data proof (non-invasive; the operator's running runner was not
  disturbed): `newMultiProjectDispatchStore` opened over a read-only copy of the
  real failing project shard (`.flowpilot/chats/db51ec26.../dispatch.ndjson`) —
  which holds 182 distinct run ids / 312 turn ids versus only 33 runs left in the
  session index, i.e. ~149 orphan runs — reports `MaxPersistedIDSuffix = 54181`
  (turn-54181), well above the orphan turn-53892 / turn-53903 that broke
  run-53890. Wiring that store via `SetDispatchStore` then mints the next run id
  above 54181, so the exact collision cannot recur. To load the fix into the live
  runner, restart it (`just dev`): the boot log will show `id counter seeded to
  54181 from durable dispatch records (BUG-317)`.

## 8. Regression Guard

- `TestSetDispatchStoreSeedsIDCounterPastOrphanDispatchRecords` locks the
  invariant for all three V2 providers.
- Optional future hardening (NOT required by this fix): purge a run's dispatch
  records when its chat is deleted from history, so orphans never accumulate. The
  seed fix makes any orphan harmless regardless, so this is defense-in-depth only.

## 9. Follow-Up Document Updates

- CA-411 records the change.
- CP-51 Phase A/B timeline notes the dispatch-prepare-collision fix.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-317
change_type: bugfix
summary: seed the id counter past every id persisted in the durable dispatch store (not just the session index) when the store is wired, so deleting a chat from history can no longer let a re-minted run/turn id collide with a surviving dispatch record (ErrAlreadyExists -> dispatch_prepare_failed) on the next flow start.
# --->8---
