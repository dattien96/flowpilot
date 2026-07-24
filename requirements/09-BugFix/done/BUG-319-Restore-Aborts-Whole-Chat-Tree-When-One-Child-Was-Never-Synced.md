# BUG-319: Restore Aborts Whole Chat Tree When One Child Was Never Synced (Interrupted Before Any Transcript)

## Metadata

- Document ID: `BUG-319`
- Title: `restoreChatRunTreeFromDrive hard-fails the ENTIRE parent restore when a single child agent's turn was interrupted before it ever wrote a real transcript -- that child correctly has no index row or manifest blob on Drive (BUG-311's own sync-up skip), but restore did not tolerate a genuinely-never-synced child the way sync-up already does`
- Phase: `bugfix`
- Status: `done`
- Owner: `google-drive`
- Reviewers: `TBD`
- Created: `2026-07-24`
- Last Updated: `2026-07-24`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: [BUG-312](../done/BUG-312-Grok-Drive-Restore-Rejects-Every-Session-Missing-Provider-Case.md), [BUG-313](../done/BUG-313-Restored-Chat-Timeline-Broken-Sync-Never-Carried-Turn-Log.md), [BUG-316](../done/BUG-316-Restored-Grok-Chat-Fails-Next-Turn-Session-Directory-Incomplete.md), [BUG-320](../done/BUG-320-Restored-Stopped-Flow-Shows-Wrong-Step-Timeline-Because-Dropped-Child-Had-No-Evidence.md) (live-verify follow-up: this fix's own child-tombstone record is what BUG-320's step-timeline fix needs to detect a failed/canceled node instead of defaulting it to DONE), [CA-418](../../change-audit/CA-418-restore-skips-never-synced-child-instead-of-hard-failing-tree.md)
- Replaces: `none`
- Tags: `google-drive, chat-sync, restore, agent-tree, cross-provider, severity-medium`

## AI Quick View

### Summary

A Claude flow-claude review-loop chat was stopped right as its reviewer child's
turn started (Claude MCP connection never became ready in time, `turn_failed`
with 0 real events). Syncing both a completed chat and this stopped chat up to
Drive worked fine. Deleting both chats locally and syncing back down restored
the completed one, but the stopped one failed with a remote-not-found error --
even though the hub and the (separately, fully) synced coder child both had
perfectly good transcripts on Drive. Root cause: the reviewer never got a real
transcript, so `syncChatRunToDrive`'s own per-child loop correctly skipped
uploading it (BUG-311's `session_unavailable` path) -- it has no index row and
no manifest blob anywhere on Drive. But the parent hub's manifest still lists
it in `ChildAgents` (built from `listAgentRunSummaries`, which returns every
known child regardless of whether that child's own sync succeeded), and
`restoreChatRunTreeFromDrive` recursed into every `ChildAgents` entry
unconditionally -- any child restore error, including "this child was never
synced," aborted the WHOLE parent restore.

### Current Ask

Skip a child during restore only when it was truly never synced (no index row
AND no manifest blob anywhere on Drive for its `SourceMachineID`+`SourceRunID`)
instead of hard-failing the entire tree -- while still hard-failing on any
child that WAS synced but is now broken for some other reason (missing
provider file, integrity mismatch, index row lost to a concurrent-sync race
while its manifest blob survives -- CA-404).

### Key Decisions

- Skip decision is made from **index/manifest-blob membership**, not from the
  child restore call's error *code* -- `sync_remote_not_found` is reused by
  three different situations (never indexed at all; manifest found but its
  provider file object missing; Grok sidecar missing), and only the first one
  is safe to treat as "never synced." Keying off the error code alone would
  have silently broken the pre-existing
  `TestRestoreParentChatDoesNotPublishMainHistoryWhenChildRestoreFails` guard
  (a child that WAS synced, then had its provider file deleted from Drive,
  must still hard-fail the whole restore).
- A skipped child gets a **metadata-only TOMBSTONE record** (a
  `ProviderSessionState` with a collision-safe local `RunID`, `ParentRunID`,
  `Label`/`AgentName`/`Role`, a terminalized `Status`, `SyncStatus:
  "unsyncable"`, and NO transcript/`ProviderSessionID`) instead of being
  dropped from the restored tree entirely. An earlier cut of this fix fully
  dropped the child (no persisted record at all) -- caught in review before
  commit: `resumedFlowStepRows` (interactive_resume.go) can only tell a flow
  node's real per-run outcome from that child's OWN persisted session record;
  with none at all it silently defaulted the node to DONE, which is exactly
  what [BUG-320](../done/BUG-320-Restored-Stopped-Flow-Shows-Wrong-Step-Timeline-Because-Dropped-Child-Had-No-Evidence.md)'s
  live repro caught (a reviewer that was actually FAILED, and a synthesis node
  that never ran, both showed DONE after restore). The tombstone's `RunID` is
  resolved via `resolveRestoredRunID` exactly like a real restored child, so it
  never collides with an unrelated local run, and a sibling's `DependsOn`
  reference to it is remapped to that same local id (not left as a dangling
  raw remote id).
- Tombstones are **staged in memory during the child loop and persisted once,
  together, immediately before the parent** -- never inside the loop itself.
  Persisting eagerly would leave an orphan tombstone on disk if a LATER
  sibling child hard-fails and the whole restore call returns an error, and
  would double-upsert if the tombstone were also run through the per-child
  metadata-patch pass that follows (that pass patches an already-persisted
  real child in place; a tombstone has no separate persisted record to patch).
- A transient Drive lookup failure (network blip, API error) while checking
  whether a child was ever synced is NOT treated as "confirmed never synced" --
  `chatSessionRemoteRunKnown` now returns `(bool, error)` and only
  `errors.Is(err, os.ErrNotExist)` counts as confirmed-absent; any other error
  hard-fails the whole restore. Treating a transient failure as "never synced"
  could tombstone a child that still has real, recoverable data -- exactly
  what the CA-404 guard exists to prevent.
- `resolveRestoredRunID`'s derived-id collision handling was hardened
  alongside this: it previously returned the first `sync-<machine>-<id>`
  candidate unchecked, which could silently overwrite an unrelated local run
  if that candidate was ALSO already taken (e.g. two source machines whose
  8-char `shortMachineID` prefixes collide). It now loops with an incrementing
  suffix until a genuinely free id is found.
- Provider-agnostic by construction: the skip/tombstone decision runs entirely
  off `SourceMachineID`+`SourceRunID` (index rows) and
  `chatSessionManifestPath` (no provider segment) -- before any
  provider-specific restore code executes.

### Constraints

- Additive tests only; no pre-existing test edited.
- No real machine paths in tests.

### Open Questions

- `none`.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` --
  `restoreChatRunTreeFromDrive`'s child-restore loop (tombstone staging +
  persist), `resolveRestoredRunID` (collision loop), `chatSessionRemoteRunKnown`
  (fail-closed on transient errors), `terminalizeTombstoneStatus`.
- `apps/local-runner/internal/runner/chat_session_sync.go:920` --
  `syncChatRunToDrive`'s pre-existing per-child "skip on error, log, continue"
  precedent this fix mirrors on the restore side.
- `apps/local-runner/internal/runner/interactive_service.go` --
  `listAgentRunSummaries`'s disk-fallback branch now preserves `Label` so a
  tombstone (or any restored child) matches its exact flow node.

## 1. Issue Summary

Live repro: Claude flow-claude chat, hub `run-55348`, coder `run-55353`
(completed, synced fine), reviewer `run-55467` (Claude MCP connection never
became ready, `turn_failed`, 0 real events -- user then stopped the flow).
Syncing the parent up to Drive correctly skipped the reviewer
(`[chat-sync] skip child run_id="run-55467" parent="run-55348"
code="session_unavailable" msg="session data not found on this machine"`).
Deleting the chat locally and restoring it back failed:
`[chat-sync] child restore failed run_id="run-55467" parent="run-55348"
code="sync_remote_not_found" msg="remote chat session was not found"` --
aborting the restore of the hub and the coder too, both of which had complete,
valid transcripts on Drive. A sibling chat with no interrupted child (both
coder and reviewer completed a full round) restored with no problem.

## 2. Parent Links

`google-drive` feature: same restore path BUG-312/313/316 already hardened for
Grok-specific and turn-log gaps; this closes a gap in the child-tree restore
loop itself, independent of provider.

## 3. Environment and Reproduction

Runner started with `just dev`; project Gate-sandbox. Custom flow
`flow-claude`.

1. Start a flow-claude review-loop chat; stop it while the reviewer child's
   turn is still starting (before it produces any real output).
2. Sync the chat up to Drive (`chat-sync` correctly skips the reviewer;
   uploads hub + coder).
3. Delete the chat locally.
4. Sync the chat back down from Drive.
5. Restore fails entirely -- the hub and coder, both fully synced, are lost
   along with the reviewer that was never syncable in the first place.

## 4. Expected vs Actual

- Expected: restoring a chat whose only broken/incomplete part is a child that
  was never itself synced should restore everything that WAS synced (hub +
  every other child), simply omitting the never-synced child.
- Actual: the entire restore failed with `sync_remote_not_found`, and nothing
  was restored -- not even the hub or the fully-synced coder.

## 5. Root Cause

`syncChatRunToDrive`'s per-child loop (chat_session_sync.go:914-932) already
treats a child sync failure as best-effort: `BuildChatSessionSyncManifest`
returning an error for a child (e.g. `session_unavailable` -- BUG-311, no
resumable session file was ever written) is logged and the loop simply
`continue`s, so that child gets no index row and no manifest blob uploaded,
while the parent and every other child still upload normally. But the parent's
own manifest still lists that child in `ChildAgents`, because
`listAgentRunSummaries(runID)` (used to build `manifest.ChildAgents`) returns
every child FlowPilot knows about locally, regardless of whether that child's
own sync succeeded.

`restoreChatRunTreeFromDrive`'s child-restore loop
(chat_session_sync.go:1369-1399, pre-fix) recursed into `s.
restoreChatRunTreeFromDrive` for every entry in `manifest.ChildAgents`
unconditionally. Any error from that recursive call --
`return ChatSessionRestoreResult{}, childErr` -- aborted the WHOLE parent
restore, including the hub and every other child that DID restore fine. A
child that was correctly never synced (no index row, no manifest blob
anywhere) always fails this recursive call with `sync_remote_not_found` (the
top of `restoreChatRunTreeFromDrive` looks the run up by `SourceMachineID`+
`SourceRunID` in the Drive index and returns this error the moment no match is
found) -- so the restore-side loop had no tolerance for the exact situation
the sync-up-side loop already treats as routine and recoverable.

## 6. Fix Strategy

- `chatSessionRemoteRunKnown(accessToken, rootFolderID, indexRecords,
  machineID, runID) (bool, error)`: true if the run has an index row, OR
  (CA-404: index rows can be lost to a concurrent-sync last-write-wins race
  while the manifest blob itself survives) a manifest blob still exists at its
  expected path. Returns an error (not `false`) for anything other than a
  confirmed `os.ErrNotExist` -- a transient Drive failure must hard-fail, not
  be treated as "never synced". Deliberately does NOT inspect
  `hasProviderFile`/integrity/etc. -- any of those problems on a run that DOES
  have index/manifest presence must still hard-fail, unchanged.
- In the child-restore loop, before recursing into a child, check
  `chatSessionRemoteRunKnown` for it (reusing the SAME already-downloaded
  index bytes parsed once at the top of the function -- zero extra Drive round
  trips for the common found-it-in-the-index case). An error from the check
  hard-fails immediately. Not known (confirmed absent) -> stage a tombstone
  `ProviderSessionState` in a local map (RunID via `resolveRestoredRunID`,
  `ParentRunID`, `Label`/`AgentName`/`Role`/`ModelName`/`ProviderKey` from the
  manifest's `ChildAgents` entry, `Status`/`AgentStatus` via
  `terminalizeTombstoneStatus`, `SyncStatus: "unsyncable"`, no
  `ProviderSessionID`) and `continue` -- do NOT persist it yet. Known -> run
  the ORIGINAL recursive restore call, and any error from it still hard-fails
  the whole parent restore exactly as before.
- `remapped` (the restored/tombstoned children list persisted via
  `setHistoricalChildren` and used to update each child's
  `ParentRunID`/metadata) is built with `append`, adding an entry for both
  real restores and tombstones. The per-child metadata-patch loop that follows
  it now branches: a tombstone gets its final `ParentRunID` + remapped
  `DependsOn` folded directly into the staged (not-yet-persisted) struct; a
  real child gets `updateLocalSessionSyncStatus` as before (now also patching
  `Label`, previously omitted). After that loop, every staged tombstone is
  persisted once, followed by the parent -- so `DependsOn` remapping and
  cross-child validation are always complete before anything hits disk.
- `terminalizeTombstoneStatus(status)`: unlike `normalizeResumedStatus` (which
  deliberately preserves `waiting_approval`/`waiting_question` so a real
  restart can rehydrate the actionable gate card), a tombstone has no synced
  approval/question record to rehydrate from -- any non-terminal status
  collapses to `Cancelled` instead of leaving a permanently "waiting" card.
- `resolveRestoredRunID` now loops (`firstFreeRestoredRunID`) appending an
  incrementing numeric suffix until a genuinely unused derived id is found,
  instead of returning the first `sync-<machine>-<id>` candidate unchecked.

## 7. Validation

- Red-first, reported repro: `TestRestoreParentChatTombstonesNeverSyncedChildRestoresRest`
  fails pre-fix (`restoreChatRunFromDrive() failed` with `sync_remote_not_found`)
  then passes -- hub + coder restore for real, reviewer gets a terminal
  (`Failed`) tombstone, persisted with `SyncStatus: "unsyncable"` and no
  provider file/transcript.
- Degraded-input matrix (R3):
  `TestRestoreParentChatTombstonesAllNeverSyncedChildren` (every child
  never-synced -- hub restores alone, both children tombstoned with their
  exact snapshot statuses preserved);
  `TestRestoreParentChatTerminalizesNonTerminalTombstoneStatus` (a `Running`
  snapshot collapses to `Cancelled`, not left in-flight);
  `TestRestoreParentChatRemapsDependsOnToTombstoneID` (a restored sibling's
  `DependsOn` reference to the tombstoned child is remapped to the
  tombstone's actual local id, including when that id had to be derived
  because the raw source id collided with an unrelated local run -- and that
  unrelated run is verified untouched);
  `TestRestoreParentChatHardFailsOnTransientDriveErrorInsteadOfTombstoning`
  (a simulated transient Drive error on the child's manifest-path lookup
  hard-fails the whole restore instead of silently tombstoning it);
  `TestRestoreParentChatTombstoneSurvivesServiceRestart` (a THIRD, fresh
  `InteractiveService` instance over the same on-disk store still sees the
  durable tombstone).
- Regression guard, CA-404 (R1 -- must NOT loosen real-corruption handling):
  `TestRestoreParentChatStillFailsWhenChildManifestSurvivesWithoutIndexRow`
  plants a child manifest blob with no index row and asserts the parent
  restore still hard-fails with `sync_remote_not_found` -- proving a child with
  surviving real data is never silently tombstoned. The pre-existing
  `TestRestoreParentChatDoesNotPublishMainHistoryWhenChildRestoreFails`
  (child synced, then its provider file deleted from Drive) was re-run
  UNCHANGED and stays green -- this is the scenario a naive
  "skip on `sync_remote_not_found` code" fix would have silently broken.
- Cross-provider (R2): `TestRestoreParentChatSkipsNeverSyncedChildAcrossProviders`
  -- Codex, Claude, and Grok subtests, same 3-account harness as BUG-313's
  `TestRestoreChatRunFromDriveRebuildsTurnLogSidecarAllProviders`. All three
  pass identically, confirming the tombstone decision runs entirely off
  `SourceMachineID`+`SourceRunID`/manifest-path membership, with no
  provider-specific branch anywhere in the decision.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Existing chat-sync/restore suite (every `TestRestoreChatRunFromDrive*`,
  `TestRestoreParentChat*`, `TestSyncAndRestoreFlowRunRoundTrip*`,
  `TestSyncChatRunToDrive*`, `TestListRemoteChatSessions*`, `TestBug31[2-6]*`,
  `TestReconstruct*`, `TestStepTransitionReplay*`, BUG-308's stopped-status
  tests): all green, unmodified, unedited.
- Full-package sweep vs `git stash` baseline at the same HEAD (BUG-319+320
  combined): fix = 2649 passed / 19 failed; baseline (all production +
  new/amended test files stashed together) = 2636 passed / 17 failed. The
  FAIL-name sets differ by exactly 2 tests -- both fix-only extras
  (`TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`,
  `TestRun75035_HubResumeStillLoadsOwnCodexSession`), both pre-existing,
  order-dependent flakes (unrelated to this change's files) confirmed passing
  3/3 in isolation on the fixed tree. No baseline-only extras. No test in the
  changed area (chat sync/restore/flow-resume) is among the differing set.
- Real `provider-accounts.json` verified after every sweep; one PRE-EXISTING,
  unrelated test-isolation gap was found and cleaned up (see BUG-320's Known
  Limits) -- not caused by this change (confirmed via `git diff` the polluting
  test file is untouched).
- Live end-to-end: CONFIRMED (2026-07-24). User rebuilt/restarted and
  reproduced the exact sequence (stop a flow mid reviewer-start, sync up,
  delete locally, sync down) -- the hub and coder restored successfully. The
  follow-up step-timeline display defect this surfaced is BUG-320.

## 8. Regression Guard

- `TestRestoreParentChatTombstonesNeverSyncedChildRestoresRest`,
  `TestRestoreParentChatTombstonesAllNeverSyncedChildren`,
  `TestRestoreParentChatTerminalizesNonTerminalTombstoneStatus`,
  `TestRestoreParentChatRemapsDependsOnToTombstoneID`,
  `TestRestoreParentChatHardFailsOnTransientDriveErrorInsteadOfTombstoning`,
  `TestRestoreParentChatTombstoneSurvivesServiceRestart`, and
  `TestRestoreParentChatSkipsNeverSyncedChildAcrossProviders` lock the
  tombstone contract for single/all/mixed never-synced children, id
  collisions, transient-error fail-closed behavior, durability across a
  restart, and all three providers.
- `TestRestoreParentChatStillFailsWhenChildManifestSurvivesWithoutIndexRow`
  locks the CA-404 boundary: a child with surviving real data on Drive is
  never silently tombstoned, even without an index row.
- The pre-existing `TestRestoreParentChatDoesNotPublishMainHistoryWhenChildRestoreFails`
  stays untouched and green, locking that a child which WAS synced but is now
  missing its provider file still hard-fails the whole restore.
- BUG-320's own test matrix (`bug320_stopped_flow_step_timeline_test.go`)
  guards the step-timeline consumer of these tombstones -- see that document.

## 9. Follow-Up Document Updates

- CA-418 records the change.
- BUG-320 + CA-419 record the follow-up step-timeline fix this tombstone
  design enables, found during the user's live retest.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-319
change_type: bugfix
summary: restoreChatRunTreeFromDrive now gives a child that was genuinely never synced (no index row and no manifest blob anywhere on Drive -- the sync-up side already tolerates this per BUG-311) a metadata-only, terminal-status tombstone record instead of hard-failing the ENTIRE parent restore or dropping the child with no trace at all, while still hard-failing on any child that WAS synced but is now broken for a real reason (missing provider file, integrity mismatch, a transient Drive error, or an index row lost to a CA-404-style concurrent-sync race while its manifest blob survives); resolveRestoredRunID's derived-id collision handling was also hardened so a tombstone (or any restored child) can never overwrite an unrelated local run.
# --->8---
