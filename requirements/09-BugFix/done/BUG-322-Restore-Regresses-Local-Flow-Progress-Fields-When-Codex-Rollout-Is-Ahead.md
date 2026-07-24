# BUG-322: A Second Restore Regresses Local Flow-Progress Fields Even When the Codex Rollout File Is Genuinely Ahead

## Metadata

- Document ID: `BUG-322`
- Title: `restoreChatRunFromDrive's BUG-091 localAhead branch preserved only LastPrompt/LastMessage/Status/UpdatedAt from the local session; fields added later that also track live flow progress -- TurnCount (BUG-315), LoopState, ActiveFlowNodes/ActiveFlowEdges, AgentStatus, PendingAgentContext, FlowCohortID -- were still overwritten from the older remote manifest, so re-restoring an already-locally-progressed chat could resurrect BUG-315's own symptom (flow re-runs from scratch)`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-24`
- Last Updated: `2026-07-24`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: [BUG-091](./BUG-091-Drive-Restore-Rejects-Same-Session-Prefix-Extension-As-Conflict.md) (introduced the `localAhead` preservation branch this bug widens), [BUG-315](../done/BUG-315-Restored-Flow-Chat-Reruns-Whole-Flow-On-Followup-Sync-Dropped-TurnCount.md) (added `TurnCount` to the restore manifest without folding it into BUG-091's preservation branch -- the exact regression risk this bug closes), [CP-51-PhaseAB-Timeline-And-Verification-Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md) C6 ("no empty-overwrite of newer local"), [CA-421](../../change-audit/CA-421-restore-preserves-flow-progress-fields-when-local-ahead.md)
- Replaces: `none`
- Tags: `agent-flow-engine, google-drive, chat-session-sync, restore, conflict, cross-pc, cp-51-c6`

## AI Quick View

### Summary

Investigating the CP-51 companion verification doc's C6 gap ("no
empty-overwrite of newer local" -- flagged repeatedly across the BUG-312..317
wave of sync/restore fixes with zero test evidence) found that BUG-091's own
`localAhead` guard was itself incomplete, not merely untested. BUG-091
(2026-06) added a preservation branch to `restoreChatRunFromDrive`: when the
local Codex rollout file is a byte-prefix-extension of the restored remote
snapshot (i.e. local genuinely progressed since the last sync-up), the local
session's `LastPrompt`/`LastMessage`/`Status`/`UpdatedAt` are kept instead of
being downgraded to the older remote manifest. Fields added to
`ProviderSessionState` afterward that ALSO track live flow progress --
`TurnCount` (BUG-315), `LoopState`, `ActiveFlowNodes`/`ActiveFlowEdges`,
`AgentStatus`, `PendingAgentContext`, `FlowCohortID` -- were never folded into
that same branch, so they were still set unconditionally from the stale
manifest even when `localAhead` was true.

### Current Ask

Widen BUG-091's `localAhead` preservation branch to also protect the
flow-progress fields added since, so a second restore of an
already-locally-progressed chat cannot roll its flow state backward.

### Key Decisions

- `D-1` Only fields that mutate over a run's lifetime are added to the
  preservation branch: `TurnCount`, `LoopState`, `AgentStatus`,
  `ActiveFlowNodes`, `ActiveFlowEdges`, `PendingAgentContext`, `FlowCohortID`.
  Static per-run identity (`ProjectID`, `AgentName`, `ModelName`, `DependsOn`,
  `ChatFlowRef`, `ChatSubMode`, ...) is left as the manifest's -- it does not
  change after the run starts and should already agree between local and
  remote.
- `D-2` `TurnCount` and `LoopState.Round` use a numeric `>` comparison rather
  than an unconditional overwrite (unlike the other fields, which only check
  non-empty): this adds a second, independent safety net beyond the file-level
  `localAhead` flag, so even if the boolean were ever wrong for some future
  case, a strictly lower manifest value can never win over a strictly higher
  local one.
- `D-3` Scope stays inside the existing `if localAhead` gate (Codex-only, per
  BUG-091's own `codexExtend` condition) rather than widening WHEN the branch
  fires. A broader question -- whether a no-transcript flow hub (BUG-250's
  read-only restore, where there is no rollout file to byte-compare at all)
  can also have its flow-progress fields regressed by a stale restore -- is
  noted as a known limit, not fixed here: there is no cheap, proven-safe
  file-level signal to detect "local ahead" for that case, and inventing one
  is a separate, more speculative piece of design work.

### Constraints

- Additive tests only; no pre-existing test edited (the existing BUG-091
  tests -- `TestRestoreChatRunFromDriveOverwritesWhenRemoteExtendsLocal`,
  `TestRestoreChatRunFromDriveKeepsLocalWhenLocalExtendsRemote`,
  `TestRestoreChatRunFromDrivePreservesLocalMetadataWhenLocalAhead` -- stay
  green, unmodified).
- No real machine paths in tests.

### Open Questions

- `none`.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` --
  `restoreChatRunFromDrive`'s `if localAhead` block.

## 1. Issue Summary

`restoreChatRunFromDrive`'s `localAhead` branch (BUG-091) preserved 4 fields.
`TurnCount` (BUG-315), `LoopState`, `ActiveFlowNodes`/`ActiveFlowEdges`,
`AgentStatus`, `PendingAgentContext`, and `FlowCohortID` were all added to
`ProviderSessionState` and to the manifest-driven restore path afterward, but
none of them were added to the `localAhead` branch -- they were set straight
from `manifest.*` regardless of whether local had already progressed past
that manifest.

## 2. Parent Links

`agent-flow-engine` (flow-runtime session fields) and `google-drive`
(restore path) -- this is a gap in BUG-091's own fix, surfaced while auditing
the CP-51 companion doc's C6 checklist item.

## 3. Environment and Reproduction

Any project with a Codex flow chat that has been synced to Drive once, then
progressed further locally (more turns/rounds), then restored again from the
same (now-stale) Drive snapshot -- e.g. the same chat re-clicked from Remote
Chats, or re-swept by a batch "Restore all".

1. Sync a Codex flow run to Drive at `TurnCount=0`.
2. Continue the run locally past that point (`TurnCount=5`, `LoopState.Round=2`
   or similar) -- the local Codex rollout file grows accordingly.
3. Restore the same chat from Drive again (still only `TurnCount=0` in the
   manifest).
4. The rollout FILE correctly stays untouched (BUG-091's own guard covers the
   bytes) -- but the session's `TurnCount`/`LoopState` regress to the stale
   manifest values.

## 4. Expected vs Actual

- Expected: a restore that detects local is ahead (file-level) also keeps the
  local flow-progress fields; `TurnCount`/`LoopState`/etc. are never rolled
  backward by an older manifest.
- Actual: `TurnCount` and the other flow-progress fields were silently reset
  to the older manifest's values, exactly the shape of regression BUG-315
  itself fixed (a wrongly-low `TurnCount` makes `startTurn` and
  `resolveWorkflowFlowRef` treat a continuing chat as turn-0 of a new flow).

## 5. Root Cause

BUG-091's `localAhead` branch was written before `TurnCount` (BUG-315),
`LoopState`, and the other flow-runtime fields existed on
`ProviderSessionState`; each of those was added by a later, unrelated change
that populated `session.<Field> = manifest.<Field>` in the main struct
literal, and none of those later changes revisited the earlier
`localAhead`-gated preservation block to fold themselves in.

## 6. Fix Strategy

- Add to the existing `if localAhead { ... }` block in
  `restoreChatRunFromDrive`: preserve `local.TurnCount` /
  `local.LoopState` when strictly greater than the manifest-derived value
  (numeric comparison, not just a non-empty check); preserve
  `local.AgentStatus`, `local.ActiveFlowNodes`, `local.ActiveFlowEdges`,
  `local.PendingAgentContext`, `local.FlowCohortID` when non-empty (mirroring
  BUG-091's own existing style for `LastPrompt`/`LastMessage`/`Status`/
  `UpdatedAt`).
- Nothing else changes: the branch still only fires under BUG-091's own
  `codexExtend && bytes.HasPrefix(existing, providerBytes)` condition: Codex
  only, and only when the local rollout file bytes prove local is ahead.

## 7. Validation

- Red-first: `TestRestoreChatRunFromDrivePreservesFlowProgressFieldsWhenLocalAhead`
  reproduces BUG-315's own scenario via a SECOND stale restore instead of a
  fresh one -- seeds a local chat, syncs it, then advances the local rollout
  file (localAhead) and the session's `TurnCount`/`LoopState` past what the
  synced manifest has, then restores again. Fails pre-fix
  (`TurnCount` regressed to the stale manifest value), passes after.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Pre-existing BUG-091 tests (`TestRestoreChatRunFromDriveOverwritesWhenRemoteExtendsLocal`,
  `TestRestoreChatRunFromDriveKeepsLocalWhenLocalExtendsRemote`,
  `TestRestoreChatRunFromDrivePreservesLocalMetadataWhenLocalAhead`) and the
  BUG-319 tombstone-contract suite: all green, unmodified (37/37 in the
  targeted run).
- Full-package sweep vs `git stash` baseline (same HEAD): fix = 2345 passed /
  17 failed; baseline pattern (from the same-session BUG-321 sweep at the
  same HEAD) = the identical 17 pre-existing environment-dependent failures
  (Codex/Grok provider-home & path, git-commit-guard shim, skills-merge,
  auth-workspace command). No changed-area test regressed; the single
  order-dependent flake seen in the BUG-321 baseline
  (`TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID`) happened to pass
  this run, consistent with its already-documented flakiness.
- Real `provider-accounts.json` verified unchanged (8 accounts) across the
  sweep.
- Live end-to-end: the same live session's BUG-321/BUG-322 verification also
  ran a full real Review Loop (`run-82214`, Grok hub + Codex coder/reviewers,
  Gate-sandbox project) through 2 rounds to a genuine `done` outcome via the
  restore-adjacent CP-51 dispatch path -- confirms the surrounding
  chat-sync/session-state machinery this fix touches was not disturbed by the
  change (not a direct repro of the restore-regression scenario itself, which
  is inherently a two-restore, no-live-UI-surface scenario better suited to
  the unit proof above).

## 8. Regression Guard

- `TestRestoreChatRunFromDrivePreservesFlowProgressFieldsWhenLocalAhead` locks
  the fix for `TurnCount` and `LoopState`.
- The pre-existing BUG-091 tests stay untouched and green, locking the
  original 4-field preservation and the remote-extends-local / identical /
  genuinely-divergent branches this fix does not touch.

## 9. Follow-Up Document Updates

- CA-421 records the change.
- CP-51 companion verification doc's C6 row updated to reflect this fix as
  the closure of its "no empty-overwrite of newer local" flow-progress gap
  (the file-content half of that property was already closed by BUG-091
  itself; the property was never fully closed for the newer flow-runtime
  fields until now).

## Known limits (documented, out of scope)

- Scoped to the existing `codexExtend`-gated `localAhead` branch: Claude/Grok
  restores still hard-fail on any local/remote rollout mismatch
  (`session_file_conflict`) rather than silently picking a winner, so this
  specific regression risk does not apply to them today (per BUG-091's own
  design note on non-Codex append semantics being unconfirmed).
- A no-transcript flow hub (BUG-250's read-only restore path, no rollout file
  to byte-compare) has no file-level signal to detect "local ahead" at all,
  so it is not covered by this fix either. Not addressed here -- would need a
  different detection mechanism (e.g. timestamp or `TurnCount` comparison
  without a file-bytes anchor), which is a separate, more speculative design
  question flagged for future consideration, not a silently-accepted gap.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-322
change_type: bugfix
summary: restoreChatRunFromDrive's BUG-091 localAhead branch now also preserves TurnCount, LoopState, AgentStatus, ActiveFlowNodes, ActiveFlowEdges, PendingAgentContext, and FlowCohortID from the local session (numeric-greater-than for TurnCount/LoopState.Round, non-empty check for the rest) when the local Codex rollout file is byte-prefix-ahead of the restored remote snapshot, so a second restore of an already-locally-progressed chat cannot roll its flow state backward and resurrect BUG-315's own "flow re-runs from scratch" symptom; scope stays inside BUG-091's existing Codex-only localAhead gate.
# --->8---
