# CA-411: seed the id counter past persisted dispatch records, not just the session index

## Summary

A brand-new flow chat failed instantly on its first hub turn with
`dispatch record already exists (HTTP 502 / dispatch_prepare_failed)` (live
repro: `run-53890`). Root cause: `seedIDCounter` (BUG-117) advances the id
counter past the largest numeric suffix among persisted runs **in the session
index only**. The CP-51 durable dispatch store is a SECOND durable source of ids
handed out before a restart, keyed by `(run_id, turn_id)` in per-project
`dispatch.ndjson`. Deleting a chat from history removes its session-index entry
and its `run-<id>-turns.ndjson` but NOT its dispatch records, so the session
index max can fall below a suffix a surviving dispatch record still owns. The
counter re-seeds to that lower max, `nextID` re-mints the id, and
`prepareDispatchV2` -> `CreatePrepared` rejects the reused id (existing record,
different envelope hash) with `ErrAlreadyExists`. Full analysis in
[BUG-317](../requirements/09-BugFix/done/BUG-317-New-Flow-Turn-Fails-Dispatch-Record-Already-Exists-ID-Remint.md).

## Change

- `memoryDispatchStore.MaxPersistedIDSuffix` returns the largest numeric id
  suffix among every persisted record (run id + turn id) plus the non-prunable
  per-run activation and stop-state ids.
- `multiProjectDispatchStore.MaxPersistedIDSuffix` aggregates that across every
  open project shard. `newMultiProjectDispatchStore` already loads all shards
  into RAM at construction, so this reflects orphan records left by chats deleted
  from history.
- `dispatchIDCeiling` is an OPTIONAL interface (both the in-memory and
  multi-project stores satisfy it); a store that does not is skipped.
- `SetDispatchStore` now calls `seedIDCounterFromDispatchStore` after wiring the
  store, lifting the id counter to at least that ceiling. It seeds there rather
  than in the constructor's `seedIDCounter` because the durable dispatch store is
  wired AFTER the constructor runs.

Because a running process's id counter only ever increases (atomic add), a
correct boot seed fully closes the collision (no intra-process re-mint is
possible) and makes existing orphan records harmless (the counter simply starts
above them). This complements BUG-117's session-index seed rather than replacing
it.

## Provider parity

Provider-agnostic by construction: the seed keys on numeric id suffix only. The
orphan record and the re-minted run are frequently different providers entirely
(the live repro re-minted a deleted grok run's id for a new claude flow). The
regression test covers grok, claude, and codex.

## additive-tests-only compliance

New test file (`bug317_id_counter_dispatch_seed_test.go`, 2 test functions + 1
helper) only. No existing test touched. `SetDispatchStore`'s signature is
unchanged; the added behavior is a no-op for an empty store (max <= 0), so
existing SetDispatchStore callers are unaffected.

## Verification

- Red-first: `TestSetDispatchStoreSeedsIDCounterPastOrphanDispatchRecords`
  (grok/claude/codex) fails on the pre-fix tree (`nextID after SetDispatchStore =
  suffix 11, want > 53903`) using only existing public API, then passes after the
  fix.
- `TestMaxPersistedIDSuffixReflectsPersistedDispatchRecords`: empty -> 0; memory
  store max over both run and turn ids; multi-project reload FROM DISK (the real
  restart path) reports the cross-shard max.
- `go build ./...`, `go vet ./internal/runner/...`: clean. Targeted sweep of
  dispatch / seed / SetDispatchStore-caller tests: 112 passed.
- Full-package sweep (`go test ./internal/runner/`): fix = 2178 passed / 18
  failed; `git stash` baseline (fix removed) = 2172 passed / 16 failed. The 16
  are the identical pre-existing machine/CLI/env set. The two extra on the fix
  run are proven pre-existing flakes: `TestGeminiAdapterPromptPrepAndEnv` passes
  in isolation, and `TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID`
  flakes identically on the baseline (5 isolated baseline runs: pass, pass, FAIL,
  pass, FAIL — a Windows `t.TempDir` teardown race; it hardcodes its run/turn
  ids, so the seed change cannot affect it).
- Real `provider-accounts.json` verified unchanged (8 accounts, correct active
  flags) after the sweeps.

## Known limits (documented, out of scope)

- Orphan dispatch records left by chats deleted from history are NOT purged; the
  seed fix makes them harmless (the counter starts above them). Purging on delete
  is a possible future defense-in-depth hardening (BUG-317 section 8).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-317
change_type: bugfix
summary: SetDispatchStore now seeds the id counter past every id persisted in the durable dispatch store, so a chat deleted from history can no longer let a re-minted run/turn id collide with a surviving dispatch record (ErrAlreadyExists -> dispatch_prepare_failed) on the next flow start.
# --->8---
