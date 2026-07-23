# CA-407: chat-sync manifest carries the turn log; restore rebuilds the sidecar

## Summary

Operator restored a Grok flow-hub chat (run-24345, Gate-sandbox) and the
timeline opened broken: their prompts gone, agent cards dumped below the
prose, response text suspect. Root-caused by studying how a timeline is
rebuilt after a SAME-machine restart (which works): the entire reconstruction
(`seedTranscriptFromDisk` / `seedGrokTranscriptFromDisk` /
`preferFlowHubTurnLogTranscript`) is driven by the per-run turn-log sidecar
`<runID>-turns.ndjson` -- raw prompts (sole prompt source for flow hubs),
per-turn provider session-id chains, durable transcript frames, and the
empty-OccurredAt clustering that keeps agent cards beside their turns. The
Drive sync manifest carried everything EXCEPT that sidecar, and restore never
wrote one -- confirmed live: the broken restored run had a session record but
no sidecar in the store. Full analysis in
[BUG-313](../requirements/09-BugFix/done/BUG-313-Restored-Chat-Timeline-Broken-Sync-Never-Carried-Turn-Log.md).

## Change

- `ChatSessionSyncManifest` += `TurnLog []turnLogLine` (`turnLog,omitempty`;
  no schema-version bump -- old manifests restore as before, old runners
  ignore the field).
- `BuildChatSessionSyncManifest` attaches the run's sidecar verbatim
  (children automatically covered -- the child sync loop builds each child's
  manifest through the same function).
- `restoreChatRunTreeFromDrive` rewrites the sidecar for the resolved local
  run id, only when the local sidecar has no entries (original machine's log
  stays authoritative; repeated restores never duplicate).

Provider parity: no `providerKey` branch anywhere in the change; verified by
an identical cross-provider round-trip test for Codex, Claude, and Grok.

## additive-tests-only compliance

New test functions + 3 small new helpers only, appended to
`chat_session_sync_test.go`. No existing test touched.

## Verification

- Red-first TDD: all 3 tests written and run BEFORE the fix; each failed on
  unfixed HEAD for the exact defect (`manifest turnLog = <nil>`, `restored
  turn log = null` on codex AND claude AND grok), then passed after. Tests
  assert via raw manifest JSON / ReadTurnLog behavior so they compile on the
  baseline.
- `go build ./...`, `go vet ./internal/runner/`: clean. Targeted chat-sync/
  restore/grok/cross-account sweep: fully green. Full-package sweep: same
  pre-existing failure set as before the change (catalogued in BUG-312 V-5).
- Live end-to-end (running dev runner, Gate-sandbox): real Grok chat
  run-44695 (sidecar = prompt + grok_session + transcript_turn) synced up,
  local sidecar deleted (backed up), restored from Drive -- sidecar rebuilt
  **byte-identical** (`diff` clean); second restore added nothing (still 3
  lines).

## Known limits (documented, out of scope)

- Chats synced before this fix restore without prompts until re-synced from
  a machine that still holds the original sidecar.
- Only the latest per-turn provider file is synced; `transcript_turn` frames
  cover earlier turns' visible text on restore.
- Each restore re-downloads the full per-project dispatch.ndjson (~3.4MB) --
  restore-side mirror of CA-397, separate follow-up.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-313
change_type: bugfix
summary: chat-sync manifest now carries the per-run turn-log sidecar and restore rebuilds it, so restored chats reconstruct prompts, hub prose, and agent-card placement identically to a same-machine restart.
# --->8---
