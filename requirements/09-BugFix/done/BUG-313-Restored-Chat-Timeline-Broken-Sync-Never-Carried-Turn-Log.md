# BUG-313: Restored Chat Timeline Broken — Sync Never Carried The Turn Log

## Metadata

- Document ID: `BUG-313`
- Title: `Restored Chat Timeline Broken — Sync Never Carried The Turn Log`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-23`
- Last Updated: `2026-07-23`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md)
- Child Documents: `none`
- Related Documents: [BUG-312: Grok Drive Restore Rejects Every Session](../done/BUG-312-Grok-Drive-Restore-Rejects-Every-Session-Missing-Provider-Case.md) (unblocked Grok restore the same day; the first successfully-restored Grok chat is what exposed this bug), [BUG-083](../done/BUG-083-Resume-Replays-Composed-Prompt-And-Drops-Later-Codex-Turns.md) (introduced the turn-log sidecar this bug teaches sync/restore to carry), [BUG-306](../done/BUG-306-Post-Flow-Followup-Transcript-Misordered-On-Restart.md) (transcript_turn ordering the sidecar preserves), [CA-407](../../change-audit/CA-407-sync-manifest-carries-turn-log.md)
- Replaces: `none`
- Tags: `google-drive, chat-sync, chat-history, cross-provider, regression, severity-high`

## AI Quick View

### Summary

- A chat restored from Drive opened with a broken timeline: the user's own prompts were gone entirely, agent cards were dumped at the bottom instead of beside their turns, and (for a flow hub) the prose itself could mix in frames from OTHER runs sharing the same cwd.
- Root cause: the entire post-restart timeline reconstruction is driven by the per-run **turn-log sidecar** (`<runID>-turns.ndjson` — raw prompts, per-turn provider session-id chains, durable transcript frames), and the Drive sync manifest **never carried it**. Restore rebuilt the session record, provider file, children, and dispatch log — everything except the one artifact the timeline is actually built from.
- Same-machine restart worked fine (the sidecar was still on disk), which is exactly why the gap stayed invisible until a real restore was attempted (unblocked same-day by BUG-310/BUG-312/CA-404).

### Current Ask

- Fixed. `ChatSessionSyncManifest` now carries `TurnLog []turnLogLine` verbatim; `BuildChatSessionSyncManifest` attaches it (children automatically included — the child sync loop builds each child's manifest through the same function); `restoreChatRunTreeFromDrive` rewrites the local sidecar, skipping when a local sidecar already has entries (the original machine's log stays authoritative; repeated restores never duplicate).

### Key Decisions

- `V-1` Sync the sidecar **verbatim** (all kinds: `prompt`, `codex_session`, `grok_session`, `assistant`, `transcript_turn`), not a filtered subset — the consumers (`seedTranscriptFromDisk`, `seedGrokTranscriptFromDisk`, `preferFlowHubTurnLogTranscript`/`seedFlowHubTranscriptFromTurnLog`, `mergeTurnLogAssistantsIntoTranscript`) each read different kinds, and the log is tiny (KB range).
- `V-2` Restore-side idempotency = "skip when the local sidecar is non-empty", not per-line dedup. On the original machine the local log IS the source of truth (possibly newer than the synced copy); on a restoring machine the sidecar starts absent, so the first restore writes it once and later restores are no-ops.
- `V-3` No manifest schema-version bump: the field is `omitempty`, old manifests simply restore with no turn log (pre-fix behavior), old runners ignore the unknown field. **Consequence: chats synced BEFORE this fix still restore without prompts** — healing them requires one re-sync from a machine that still has the run's local sidecar.
- `V-4` Provider-agnostic by construction: no `providerKey` branch anywhere in the change; the cross-provider regression test runs the identical round trip for Codex, Claude, AND Grok (per the project's cross-provider parity rule).
- `V-5` Out of scope (documented trade-off): sync still uploads only the LATEST per-turn provider file (Grok session dirs / Codex rollouts rotate per turn). With the turn log restored, `prependMissingPromptOnlyEvents` + `mergeTurnLogAssistantsIntoTranscript` reconstruct earlier turns' prompt+assistant text from `transcript_turn` frames, so the visible timeline reaches restart parity without those files; syncing every per-turn file is a separate enhancement if raw-file parity is ever needed.

### Constraints

- Backend-only; no desktop change. No change to how/when the live turn path writes the sidecar — only to carrying it across machines.

### Open Questions

- None for the defect. Whether to add a UI affordance for "this remote chat predates turn-log sync (will restore without prompts)" is UX polish, not required.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` — `ChatSessionSyncManifest.TurnLog`, `BuildChatSessionSyncManifest` (attach), `restoreChatRunTreeFromDrive` (rewrite sidecar).
- `apps/local-runner/internal/runner/turn_log.go` — `turnLogLine` kinds; `local_file_session_store.go` — sidecar file layout (`<runID>-turns.ndjson`).
- `apps/local-runner/internal/runner/interactive_resume.go` — the reconstruction consumers: `seedTranscriptFromDisk` (F-1 raw prompts, Codex chain), `seedGrokTranscriptFromDisk` (Grok chain + cwd-wide fallback), `preferFlowHubTurnLogTranscript`/`seedFlowHubTranscriptFromTurnLog` (hub prose + empty-OccurredAt card clustering).
- `apps/local-runner/internal/runner/chat_session_sync_test.go` — the 3 new tests (§8).
- Live evidence: run-24345 (Gate-sandbox, restored 2026-07-23 by the operator) opened with prompts missing + agent cards at the bottom; its store record existed but `run-24345-turns.ndjson` did not — while every same-machine run that reopened correctly had its sidecar present.

## 1. Issue Summary

The first Grok chat successfully restored from Drive (a flow-hub run) opened with visibly wrong content: the operator's prompts were gone, every agent card had moved below the prose, and the response text was suspect. Sync-up and restore both reported success — the corruption was purely in what the restored machine could rebuild.

## 2. Parent Links

- impacted coding plan: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — this closes the "restored content parity" half of C6's restore side (the anti-overwrite property was never the issue; content parity was).
- impacted tech design: `none directly` — extends the BUG-083 turn-log design across machines rather than changing it.
- impacted system spec: `none known`

## 3. Environment and Reproduction

- environment: any two-machine (or delete-then-restore) Drive chat-sync setup; any provider; most visible on flow-hub runs.
- reproduction steps:
  1. Sync any chat with at least one turn to Drive (pre-fix manifest).
  2. On a machine without the run's local store (or after deleting the local run, which removes the sidecar), restore it from REMOTE CHATS.
  3. Open the restored chat.
- expected: timeline identical to reopening after a same-machine restart.
- actual: no user prompts (flow hubs: guaranteed — their prompt exists ONLY in the turn log since CP-42 suppresses the hub's own provider turn); agent cards dumped at the bottom (provider-file replay stamps `createdAt`, so time-sort pushes all children after the prose — the "run-24377 bottom agent dump" failure mode the empty-OccurredAt clustering exists to prevent); for Grok additionally the cwd-wide `discoverGrokSessionDirs` fallback loads EVERY session dir under that cwd, mixing other runs' frames into the transcript.
- frequency: 100% of restores onto a machine lacking the run's sidecar.

## 4. Expected vs Actual

- expected: "restore từ Drive" reaches parity with "restart cùng máy" — same prompts, same prose, same card placement.
- actual: parity held only for artifacts the manifest carried; the timeline's actual driving input (turn log) was never synced, so reconstruction fell into its degraded fallbacks.

## 5. Impact

- users affected: everyone using cross-machine chat restore; also anyone who deletes a local chat and re-restores it.
- workflows affected: REMOTE CHATS restore, CP-51 C6 restore-side verification.
- severity: high — restored chats looked corrupted (missing the user's own words), undermining trust in the whole sync feature, even though no source data was ever lost.

## 6. Root Cause

- hypothesis: operator asked whether restore content was wrong because of the earlier remote-path bug class (BUG-312) or index repair (CA-404).
- confirmed cause: neither. The timeline reconstruction contract, established by BUG-083 and extended since (flow-hub turn-log transcripts, Grok/Codex per-turn session chains), reads the per-run sidecar `<runID>-turns.ndjson` for: raw user prompts (`prompt` kind — replaces composed CLI prompts; sole prompt source for flow hubs), per-turn provider session-id chains (`codex_session`/`grok_session` — which files to replay), and durable prose frames (`transcript_turn`/`assistant` — sole hub-prose source via `preferFlowHubTurnLogTranscript`, gap-filler for rotated segments otherwise). `ChatSessionSyncManifest` carried session metadata, ONE provider file, children, and flow state — but no turn log; `restoreChatRunTreeFromDrive` wrote everything back except the sidecar. Same-machine restart never noticed because the sidecar was still on disk.
- evidence: live store inspection — the operator's broken restored run (`run-24345`) had a session record but no `run-24345-turns.ndjson`, while every correctly-reopening local run had one; all 3 new tests fail on the pre-fix HEAD with `turnLog = <nil>` / `restored turn log = null` (red-first TDD, see §8).

## 7. Fix Strategy

- `F-1` `ChatSessionSyncManifest` gains `TurnLog []turnLogLine` (`json:"turnLog,omitempty"`).
- `F-2` `BuildChatSessionSyncManifest` attaches the run's sidecar verbatim via `TurnLogStore.ReadTurnLog` (ok-pattern; runs with no sidecar sync exactly as before). Children inherit automatically (child manifests come from the same builder).
- `F-3` `restoreChatRunTreeFromDrive` rewrites the sidecar for the resolved local run id via `AppendTurnLog`, only when the local sidecar has no entries; placed before the parent session is published (BUG-123 ordering pattern).

## 8. Validation

- `V-1` Red-first TDD: all 3 new tests written and run BEFORE the fix — each failed on unfixed HEAD for the exact defect (`manifest turnLog = <nil>`; `restored turn log = null` for codex AND claude AND grok) — then pass after. The tests inspect raw manifest JSON / `ReadTurnLog` behavior only (no new-field references), so they compile and genuinely fail on the baseline.
- `V-2` `TestSyncChatRunToDriveManifestCarriesTurnLog` — uploaded manifest.json embeds the 3-entry log verbatim.
- `V-3` `TestRestoreChatRunFromDriveRebuildsTurnLogSidecarAllProviders` — cross-provider parity: identical flow-hub round trip (sync → wipe sidecar → restore → sidecar JSON-equal to original) for Codex, Claude, Grok.
- `V-4` `TestRestoreChatRunFromDriveDoesNotDuplicateTurnLogSidecar` — restore over an existing sidecar adds nothing; wipe-then-restore rebuilds exactly once.
- `V-5` `go build ./...`, `go vet ./internal/runner/` clean; targeted sweep (`TestSyncChat*`, `TestRestoreChatRun*`, `TestBuildChatSessionSyncManifest*`, `TestListRemoteChatSessions*`, `TestChatSession*`, `TestGrok*`, `TestCrossAccount*`, `TestRestoreTargetPath*`, `TestRestoreSessionFile*`) fully green. Full `go test ./...`: same pre-existing failure set as before the change (Windows env/CLI-dependent, catalogued in BUG-312 V-5), none in chat-sync.
- `V-6` Live end-to-end on the running dev runner (project Gate-sandbox): created a real Grok chat (`run-44695`, one turn, sidecar = prompt + grok_session + transcript_turn), synced up, deleted the local sidecar (backed up first), restored from Drive → **sidecar rebuilt byte-identical** (`diff` clean vs backup); restored a second time → still exactly 3 lines (no duplication).

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/chat_session_sync_test.go` (`TestSyncChatRunToDriveManifestCarriesTurnLog`, `TestRestoreChatRunFromDriveRebuildsTurnLogSidecarAllProviders`, `TestRestoreChatRunFromDriveDoesNotDuplicateTurnLogSidecar`).
- alerts: none.
- audit checks: [CA-407](../../change-audit/CA-407-sync-manifest-carries-turn-log.md).

## 10. Follow-Up Document Updates

- upstream docs updated: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) §6 — C6 restore-side note updated in the same pass.
- notes left unchanged on purpose: (a) chats synced pre-fix restore without prompts until re-synced from a machine holding the original sidecar — runs whose source store was deleted (e.g. run-24345's) are not healable; (b) per-turn provider files beyond the latest remain unsynced (V-5 trade-off); (c) each restore call re-downloads the full per-project dispatch.ndjson (~3.4MB, observed once per restore in the live log) — the restore-side mirror of CA-397's upload-side fix, worth its own follow-up.
