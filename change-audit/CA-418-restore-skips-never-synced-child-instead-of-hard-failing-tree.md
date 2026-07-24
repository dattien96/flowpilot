# CA-418: restore tombstones a never-synced child instead of hard-failing the whole chat tree

## Summary

A Claude flow-claude chat stopped right as its reviewer child's turn started
(MCP connection timeout, `turn_failed`, 0 real events) synced up fine (the
reviewer, having no real transcript, was correctly skipped per BUG-311) but
failed to restore at all -- the hub and the fully-synced coder child, both
with valid transcripts on Drive, were lost along with the reviewer that was
never syncable to begin with (live repro `run-55348`/`run-55467`, confirmed
via `runner.log` 2026-07-24). Full analysis in
[BUG-319](../requirements/09-BugFix/done/BUG-319-Restore-Aborts-Whole-Chat-Tree-When-One-Child-Was-Never-Synced.md).

Root cause: `syncChatRunToDrive`'s per-child loop already treats a child sync
failure as best-effort (log + `continue`), so a child whose turn never wrote a
real transcript gets no index row and no manifest blob at all. But the
parent's manifest still lists it in `ChildAgents` (`listAgentRunSummaries`
returns every known child regardless of its own sync outcome), and
`restoreChatRunTreeFromDrive`'s child loop recursed into every entry
unconditionally -- any child restore error, including "never synced," aborted
the whole parent restore.

An earlier cut of this fix fully DROPPED the never-synced child (no persisted
record at all). Caught in review before commit: the step-timeline
reconstruction (`resumedFlowStepRows`) can only tell a node's real per-run
outcome from that child's OWN persisted session record -- with none at all it
silently defaulted the node to DONE. Live-verified as
[BUG-320](../requirements/09-BugFix/done/BUG-320-Restored-Stopped-Flow-Shows-Wrong-Step-Timeline-Because-Dropped-Child-Had-No-Evidence.md)
/ [CA-419](CA-419-stopped-flow-step-timeline-respects-tombstone-evidence.md).
The shipped design instead gives the child a metadata-only TOMBSTONE record.

## Change

- `chatSessionRemoteRunKnown(accessToken, rootFolderID, indexRecords,
  machineID, runID) (bool, error)`: true if the run has an index row, OR
  (CA-404: an index row can be lost to a concurrent-sync race while the
  manifest blob survives) a manifest blob still exists at its expected path.
  Reuses the index bytes already downloaded once at the top of
  `restoreChatRunTreeFromDrive` -- no extra Drive round trip for the common
  case. Returns an error (not `false`) for anything other than a confirmed
  `os.ErrNotExist` -- a transient Drive lookup failure must hard-fail the
  restore, not be silently treated as "never synced" (which could tombstone a
  child that still has real, recoverable data -- the exact case the CA-404
  guard exists to catch).
- The child-restore loop checks this BEFORE recursing: an error hard-fails
  immediately; not known (confirmed absent) -> stage a tombstone
  `ProviderSessionState` in memory (collision-safe `RunID` via
  `resolveRestoredRunID`, `ParentRunID`, `Label`/`AgentName`/`Role`/
  `ModelName`/`ProviderKey` from the manifest, a terminalized `Status`,
  `SyncStatus: "unsyncable"`, no transcript) and `continue`; known ->
  unchanged recursive restore call, and any error from it still hard-fails
  the whole parent restore exactly as before.
- `remapped` is built via `append`, holding both real restores and staged
  tombstones. The per-child metadata pass folds a tombstone's final
  `ParentRunID` + remapped `DependsOn` directly into the still-unpersisted
  struct (a real child still goes through `updateLocalSessionSyncStatus`,
  now also patching `Label`, previously omitted). Every staged tombstone is
  persisted once, AFTER all children are validated and BEFORE the parent --
  never inside the loop, so a later sibling's hard failure never leaves an
  orphan tombstone on disk, and a tombstone is never double-upserted.
- `terminalizeTombstoneStatus`: unlike `normalizeResumedStatus` (which
  deliberately preserves `waiting_approval`/`waiting_question` for real
  restart gate-rehydration), a tombstone has no synced approval/question
  record to rehydrate from -- any non-terminal status collapses to
  `Cancelled`.
- `resolveRestoredRunID` now loops (`firstFreeRestoredRunID`) with an
  incrementing suffix until a genuinely free derived id is found, instead of
  returning the first `sync-<machine>-<id>` candidate unchecked (which could
  silently overwrite an unrelated local run on a second collision).

Design notes: the tombstone decision is deliberately keyed off index/
manifest-blob *presence*, not off the child restore call's error *code* --
`sync_remote_not_found` is reused for three different situations (never
indexed at all; manifest found but its provider file object missing; Grok
sidecar missing) and only the first is safe to treat as "never synced."
Keying off the code alone would have silently broken the pre-existing
`TestRestoreParentChatDoesNotPublishMainHistoryWhenChildRestoreFails` guard (a
child that WAS synced, then had its provider file deleted from Drive, must
still hard-fail). A sibling's `DependsOn` reference to a tombstoned child is
remapped to the tombstone's actual local id (not left as a dangling raw
remote id, and not silently rewritten to something wrong either).

## Provider parity

Provider-agnostic by construction: the tombstone decision runs entirely off
`SourceMachineID`+`SourceRunID` (index rows) and `chatSessionManifestPath`
(no provider segment in the path), before any provider-specific restore code
executes. Verified with a Codex/Claude/Grok table test reusing BUG-313's
3-account harness.

## additive-tests-only compliance

New/amended test file (`bug319_restore_skips_never_synced_child_test.go`, 8
test functions) only -- 4 of the 5 originally-drop-contract tests were
revised to the tombstone contract before this work was ever committed (no
committed test was edited; see BUG-319's own doc for the full list). The
CA-404 guard test and the cross-provider matrix test were kept intact. The
full pre-existing chat-sync/restore/flow-resume suite re-run unmodified and
green.

R3 matrix coverage: reported repro (single never-synced reviewer tombstoned,
hub + coder restore for real); degraded inputs (every child never-synced --
hub restores alone, both tombstoned with exact snapshot statuses preserved; a
non-terminal snapshot terminalizes to Cancelled); id-collision (a restored
sibling's `DependsOn` remaps to the tombstone's derived local id when the raw
id collides with an unrelated local run, which survives untouched); transient
Drive error (hard-fails instead of tombstoning); durability (a tombstone
survives a full service restart); CA-404 regression guard (a child manifest
surviving without an index row still hard-fails, never silently tombstoned);
cross-provider (Codex/Claude/Grok table).

## Verification

- Red-first, reported repro: `TestRestoreParentChatTombstonesNeverSyncedChildRestoresRest`
  fails pre-fix (`sync_remote_not_found`) then passes.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Existing chat-sync/restore/flow-resume suite (`TestRestoreChatRunFromDrive*`,
  `TestRestoreParentChat*`, `TestSyncAndRestoreFlowRunRoundTrip*`,
  `TestSyncChatRunToDrive*`, `TestListRemoteChatSessions*`, `TestBug31[2-6]*`,
  `TestReconstruct*`, `TestStepTransitionReplay*`, BUG-308's stopped-status
  tests): all green, unmodified.
- Full-package sweep vs `git stash` baseline (all production + test files for
  BUG-319+320 combined stashed together) at the same HEAD: fix = 2649 passed
  / 19 failed; baseline = 2636 passed / 17 failed. The 2 differing FAIL names
  (`TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`,
  `TestRun75035_HubResumeStillLoadsOwnCodexSession`), both fix-only, are
  pre-existing order-dependent flakes unrelated to the changed files -- both
  pass 3/3 in isolation on the fixed tree. No changed-area test regressed.
- Real `provider-accounts.json` verified after every sweep. One unrelated,
  pre-existing test-isolation gap was found (a full-suite run leaked a
  temp-dir Codex account row via `TestRun20332FlowHubHistoryParityForEveryProvider`,
  confirmed via `git diff` to be untouched by this change) and cleaned up;
  flagged separately for its own fix, out of scope here.

## Not fixed by recent commits

BUG-312/313/316 (this same feature) hardened Grok-specific restore cases and
the turn-log/sidecar round trip, but none of them touch the child-restore
loop's error-propagation behavior itself -- this gap predates all three and
is orthogonal to what they fixed.

## Known limits (documented, out of scope)

- None remaining. Live end-to-end re-verification is done (see BUG-319's own
  doc) -- the user reproduced the exact sequence after a rebuild/restart and
  the hub + coder restored successfully. The step-timeline display defect
  that live retest surfaced is tracked and fixed separately as BUG-320/CA-419.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-319
change_type: bugfix
summary: restoreChatRunTreeFromDrive now gives a child that was genuinely never synced (no index row and no manifest blob anywhere on Drive) a metadata-only, terminal-status tombstone record instead of hard-failing the ENTIRE parent restore or dropping the child with no trace, while still hard-failing on any child that WAS synced but is now broken for a real reason (including a transient Drive error); resolveRestoredRunID's derived-id collision handling was also hardened.
# --->8---
