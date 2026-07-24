# CA-421: restore preserves flow-progress fields, not just conversation metadata, when local is ahead

## Summary

Investigating the CP-51 companion doc's C6 gap ("no empty-overwrite of newer
local", flagged repeatedly across the BUG-312..317 wave with zero test
evidence) found that BUG-091's own `localAhead` guard was itself incomplete,
not merely untested: it preserved `LastPrompt`/`LastMessage`/`Status`/
`UpdatedAt` from the local session when the Codex rollout file proved local
was ahead, but fields added afterward that also track live flow progress --
`TurnCount` (BUG-315), `LoopState`, `ActiveFlowNodes`/`ActiveFlowEdges`,
`AgentStatus`, `PendingAgentContext`, `FlowCohortID` -- were still
overwritten from the older remote manifest regardless. Full analysis in
[BUG-322](../requirements/09-BugFix/done/BUG-322-Restore-Regresses-Local-Flow-Progress-Fields-When-Codex-Rollout-Is-Ahead.md).

Root cause: BUG-091's `localAhead` branch predates `TurnCount`/`LoopState`/etc.
on `ProviderSessionState`; each was added later by an unrelated change that
set it unconditionally from the manifest, and none revisited the earlier
branch to fold itself in.

## Change

- `restoreChatRunFromDrive`'s existing `if localAhead { ... }` block
  (`chat_session_sync.go`) now also preserves `local.TurnCount` /
  `local.LoopState` (numeric `>` comparison on `TurnCount` and
  `LoopState.Round` -- a second, independent safety net beyond the file-level
  `localAhead` flag) and `local.AgentStatus` / `local.ActiveFlowNodes` /
  `local.ActiveFlowEdges` / `local.PendingAgentContext` / `local.FlowCohortID`
  (non-empty check, mirroring BUG-091's own style for the original 4 fields).
- Scope stays inside the existing Codex-only, file-byte-proven `localAhead`
  gate -- only the SET of fields it protects widens, not WHEN it fires.

## Provider parity

Scoped identically to BUG-091 itself: Codex only. Claude/Grok restores hard-
fail on any local/remote rollout mismatch (`session_file_conflict`) rather
than picking a winner, so this specific regression risk does not apply to
them (their append semantics are unconfirmed, per BUG-091's own design note).

## additive-tests-only compliance

New test file (`bug322_restore_preserves_flow_progress_fields_test.go`, 1 test
function) only. No pre-existing test edited -- the 3 existing BUG-091 tests
stay untouched and green.

R3 matrix coverage: the reported gap's exact reproduction (local rollout file
ahead via byte-prefix AND local `TurnCount`/`LoopState` ahead of the synced
manifest, via a second restore) is the new test; the pre-existing BUG-091
tests continue to cover remote-extends-local, identical-file, and the
original 4-field local-ahead preservation as regression guards for this
change.

## Verification

- Red-first: `TestRestoreChatRunFromDrivePreservesFlowProgressFieldsWhenLocalAhead`
  fails pre-fix (`TurnCount` regressed to the stale manifest value), passes
  after.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Pre-existing BUG-091 + BUG-319 tombstone suite: 37/37 pass, unmodified.
- Full-package sweep vs `git stash` baseline at the same HEAD: fix = 2345
  passed / 17 failed, matching exactly the same 17 pre-existing
  environment-dependent failures already isolated and documented in this
  session's BUG-321 sweep (Codex/Grok provider-home & path, git-commit-guard
  shim, skills-merge, auth-workspace command) -- no changed-area test
  regressed.
- Real `provider-accounts.json` verified unchanged (8 accounts).
- Live: the same session's live Review Loop retest (`run-82214`, Gate-sandbox,
  2 rounds to genuine `done`) exercised the surrounding chat-sync/session
  machinery this change touches without disturbance; the restore-regression
  scenario itself is a two-restore, no-UI-surface case better proven by the
  unit test above.

## Not fixed by recent commits

BUG-091 (2026-06) introduced the `localAhead` branch this widens. BUG-315
(2026-07) added `TurnCount` to the manifest/restore path but did not touch
BUG-091's preservation branch -- this gap predates BUG-315 in spirit (any
field added to the restore path after BUG-091 without revisiting it) and was
first concretely exposed by BUG-315's own field.

## Known limits (documented, out of scope)

- A no-transcript flow hub (BUG-250's read-only restore path) has no
  file-level signal to detect "local ahead" and is not covered by this fix.
  Would need a different detection mechanism; flagged for future
  consideration, not silently accepted.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-322
change_type: bugfix
summary: restoreChatRunFromDrive's BUG-091 localAhead branch now also preserves TurnCount, LoopState, AgentStatus, ActiveFlowNodes, ActiveFlowEdges, PendingAgentContext, and FlowCohortID from the local session when the local Codex rollout file is byte-prefix-ahead of the restored remote snapshot, closing the CP-51 C6 "no empty-overwrite of newer local" gap for flow-runtime fields (BUG-091 already covered rollout-file bytes and 4 conversation-metadata fields); scope stays inside BUG-091's existing Codex-only localAhead gate.
# --->8---
