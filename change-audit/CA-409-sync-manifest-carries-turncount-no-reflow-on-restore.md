# CA-409: chat-sync manifest carries TurnCount; restored flow chats don't re-run their flow

## Summary

Operator deleted 3 flow-hub chats (Codex/Claude/Grok), restored them from
Drive, and sent each a plain follow-up — all three re-spawned their entire
flow (coder + reviewers + synthesis) instead of the follow-up reaching the
hub. Root cause: the flow's entry nodes start only on a run's genuine first
turn, gated on `turnCount==0` in both `startTurn` and `resolveWorkflowFlowRef`
(which re-resolves the flowRef from the still-restored `workflowID`). The sync
manifest carried every other flow-runtime field but not `TurnCount`, so a
restored hub came back with `turnCount=0` and both guards misfired. Same-machine
restart was never affected — it reads `TurnCount` from the local session store,
which persists and restores it. Full analysis in
[BUG-315](../requirements/09-BugFix/done/BUG-315-Restored-Flow-Chat-Reruns-Whole-Flow-On-Followup-Sync-Dropped-TurnCount.md).

## Change

- `ChatSessionSyncManifest` += `TurnCount int` (`json:"turnCount,omitempty"`;
  no schema bump — old manifests restore as 0, old runners ignore it).
- `BuildChatSessionSyncManifest` attaches `session.TurnCount`;
  `restoreChatRunTreeFromDrive`'s restored session build writes
  `manifest.TurnCount` back.
- `startTurn`'s first-turn flow-start condition gains `&& rs.restoredFrom == ""`;
  `resolveWorkflowFlowRef` bails early when `rs.restoredFrom != ""`. A restored
  run is a continuation, never a genuine first-turn flow start — this guard also
  heals chats synced by a pre-fix manifest (still `turnCount=0` on restore).

Carrying `TurnCount` is the correct data fix, not only a re-trigger patch: a
restored run with `turnCount=0` also mis-ran `offerReviewOutcomeTool`
(`turnCount>1`) and the turn mode-prefix (`prependModePrefix(…, turnCount, …)`).

Provider parity: no `providerKey` branch anywhere; verified by an identical
cross-provider round-trip test for Codex, Claude, and Grok.

## additive-tests-only compliance

New test file (`bug315_restore_turncount_no_reflow_test.go`, 2 test functions)
only. No existing test touched.

## Verification

- Red-first TDD: both tests fail on pre-fix HEAD for the exact defect
  (`manifest turnCount = <nil>, want 4` on codex/claude/grok; restored run
  resolves its flowRef), proven via `git stash` isolating the 3 fix files, then
  pass after. Assertions read raw manifest JSON / `resolveWorkflowFlowRef`
  return so they compile on the baseline.
- `TestSyncRestoreCarriesTurnCountAllProviders` — cross-provider: manifest
  carries `turnCount:4` and the restored session's `TurnCount==4` for all three.
- `TestResolveWorkflowFlowRefSkipsRestoredRun` — restored run does not resolve
  its flowRef; a non-restored control with the identical setup still does.
- `go build ./...`, `go vet ./internal/runner/...`: clean. Full `go test ./...`:
  **18 failures byte-identical to the pre-fix baseline** (`git stash`
  comparison — all machine/env/CLI/live-dependent), new tests flip red→green.
  `TestRun20332…/grok` is a known order-dependent flake (passes isolated; absent
  from both full-sweep runs), not caused by this change.
- Live end-to-end (running dev runner, project Gate-sandbox): re-synced flow hub
  `run-46797`, deleted local, restored → `turn_count=1` (**not 0** — round-tripped
  through the real Drive), resumed, sent a plain follow-up → **no new flow child
  spawned** (children stayed 8), hub took its own turn (`turn_count` 1→2, status
  completed). Pre-fix this restored to 0 and re-spawned a coder.

## Known limits (documented, out of scope)

- Chats synced by a pre-fix manifest still restore with `turnCount=0`, but the
  `restoredFrom` guard prevents the re-trigger for them too — they just carry
  the less-precise count until re-synced from a machine holding the run.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-315
change_type: bugfix
summary: chat-sync manifest now carries TurnCount and a restoredFrom guard blocks first-turn flow start on restored runs, so a Drive-restored flow chat's plain follow-up reaches the hub instead of re-spawning the entire flow.
# --->8---
