# BUG-312: Grok Drive Restore Rejects Every Session — Missing Provider Case

## Metadata

- Document ID: `BUG-312`
- Title: `Grok Drive Restore Rejects Every Session — Missing Provider Case`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-23`
- Last Updated: `2026-07-23`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md)
- Child Documents: `none`
- Related Documents: [BUG-310: Grok Drive Sync Always Fails Reading Session Directory As File](../done/BUG-310-Grok-Drive-Sync-Always-Fails-Reading-Session-Directory-As-File.md) (fixed the sync-UP side same day; its own §10 Follow-Up explicitly named this restore-side gap as separate and unaddressed), [CA-312](../../change-audit/CA-312-grok-cross-account-session-relocate.md) (added Grok to `relocationTargetPath` for local cross-account relocation only, never to `restoreTargetPath` for Drive restore), [CA-404](../../change-audit/CA-404-remote-chats-index-repair.md) (fixed REMOTE CHATS under-listing the same day — made Grok chats visible/attempt-restorable for the first time, which is why this pre-existing gap was only just reached), [CA-406](../../change-audit/CA-406-grok-drive-restore-missing-provider-case.md)
- Replaces: `none`
- Tags: `google-drive, chat-sync, grok, desktop, regression, severity-medium`

## AI Quick View

### Summary

- Clicking "Restore" on any Grok chat in REMOTE CHATS always failed with `sync_integrity_failed` ("remote provider session file path is invalid") and permanently disabled that row.
- `restoreTargetPath` (the function that maps a synced provider file back onto local disk) only had cases for `ProviderKeyCodex` and `ProviderKeyClaude`. Every other provider — Grok included — fell into the `default` branch and returned `"unsupported provider session relocation"`.
- This is not a new regression: the case was simply never added. It only became reachable today because two same-day fixes (BUG-310 — Grok sync-up never worked at all; CA-404 — REMOTE CHATS under-listed rows) made Grok chats sync up and list correctly for the first time, so a user could finally reach the "click Restore" step and hit this pre-existing gap.

### Current Ask

- Fixed. `restoreTargetPath` now has a `ProviderKeyGrok` case that recomputes the destination session directory from **this machine's own `cwd`** (via the same `grokSessionDirPath` helper `relocationTargetPath`'s existing Grok case already uses), instead of either rejecting the provider outright or naively reusing the source machine's embedded path segment.

### Key Decisions

- `V-1` The destination directory must be **recomputed from the local `cwd`**, not parsed out of the synced `relativePath`. Grok's synced provider file's relative path embeds a percent-encoded segment of the *source* machine's cwd (`grokSessionsCwdDirName`, `resolveChatSessionTranscript`). Reusing it verbatim on a different machine (or even the same machine restoring under a different project path) would "succeed" but write into a directory `LocateSessionFile` can never look up again there, since that function always re-derives the expected path from the *caller's own* cwd at lookup time. This exact reasoning is why `relocationTargetPath`'s pre-existing Grok case (local cross-account relocation, CA-312) already recomputes from `cwd` instead of reusing `srcPath`'s structure — this fix mirrors that shape for the cross-machine case.
- `V-2` Validated inputs mirror the other providers' cases: the relative path must end in `/chat_history.jsonl` (the only file Grok's sync-up ever uploads, per BUG-310), the session id must pass `isGrokRealSessionID` (rejects synthetic `thread-*` placeholders and path-traversal payloads), `cwd` must be non-empty (restore already guarantees this upstream — `cwd_remap_required` fires first otherwise), and the computed destination must stay under `targetHome` (`pathUnderRoot`).
- `V-3` Scope stays exactly what BUG-310's own Follow-Up section named: this restores the single synced `chat_history.jsonl` file, not the full Grok session directory (sidecars like `events.jsonl`, `summary.json`, etc. were never part of the sync manifest to begin with — restoring less than what was never uploaded is correct, not a gap in this fix).

### Constraints

- Change is scoped to `restoreTargetPath`'s `switch` statement (one new `case`) in `session_file_locator.go`. No change to `resolveChatSessionTranscript`, `LocateSessionFile`, `relocationTargetPath`, or any other Grok cross-account/resume code path.
- A pre-existing, unrelated test-isolation gap was found and fixed in service of writing this bug's own end-to-end test — see §6 "Incidental finding" and §8 `V-6`.

### Open Questions

- None for the restore-path defect itself. Whether a *fresh* Grok session (one that has never been synced before, so `chat_history.jsonl` doesn't exist locally yet on the restoring machine) needs anything beyond this fix was not in scope here — this fix's own tests only exercise the "chat already existed on the source, restore onto a machine with no prior copy" and "restore under a different cwd than the source" cases, which is exactly the C6 restore-side scenario the CP-51 doc has been tracking.

### Source Refs

- `apps/local-runner/internal/runner/session_file_locator.go` — `restoreTargetPath` (new `ProviderKeyGrok` case), `relocationTargetPath` (pre-existing Grok case this fix mirrors), `grokSessionDirPath`, `isGrokRealSessionID`, `pathUnderRoot` (all reused, unmodified).
- `apps/local-runner/internal/runner/chat_session_sync.go` — `restoreChatRunTreeFromDrive` (the caller; unmodified, already passed the right `cwd`/`targetHome` — the bug was entirely inside `restoreTargetPath`).
- `apps/local-runner/internal/runner/grok_cross_account_resume_test.go` — `TestRestoreTargetPathGrokAcceptsWellFormedPathAndRejectsBadInput`, `TestRestoreSessionFileGrokRoundTripsAcrossDifferentCwdEncoding`.
- `apps/local-runner/internal/runner/chat_session_sync_test.go` — `TestRestoreChatRunFromDriveGrokRecomputesPathForDifferentCwd`; `newChatSyncService` (incidental test-isolation fix, §6).
- Live evidence: direct `POST /client/chat-sessions/restore` against the running dev server (project "Gate-sandbox", `sourceRunId: "run-22023"`) returned, pre-fix: `409 {"code":"sync_integrity_failed","message":"remote provider session file path is invalid"}`; post-fix (same call, runner restarted): `200 {"restoreStatus":"restored"}`. Confirmed on disk: the restored `chat_history.jsonl` landed under `<GROK_HOME>/sessions/D%3A%5Cworking%5Cgate-sandbox/019f8722-.../` — the percent-encoding of this machine's own `D:\working\gate-sandbox`, not any foreign/source encoding.

## 1. Issue Summary

REMOTE CHATS lists Grok chats correctly (as of BUG-310 + CA-404, same day), but clicking "Restore" on any of them always failed and permanently disabled that row in the Navigator, with no working recovery short of never trying again.

## 2. Parent Links

- impacted coding plan: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) / [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — closes the last open half of the C6 "Drive sync round-trip" scenario (sync-up was already closed same day via BUG-310/CA-397/CA-404; this closes Grok's slice of the restore-down side).
- impacted tech design: [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md) (its own §7 Out of Scope / follow-ups line explicitly deferred "Drive restore packaging for Grok session directories" to CA-312 — this bug is that deferred work).
- impacted system spec: `none known`

## 3. Environment and Reproduction

- environment: desktop + local-runner, any project with at least one synced Grok chat run.
- reproduction steps:
  1. Have a Grok chat run that has successfully synced to Drive (post-BUG-310).
  2. Open REMOTE CHATS in the Navigator, click "Restore" on that Grok row (or `POST /client/chat-sessions/restore` directly with its `sourceMachineId`/`sourceRunId`/a valid `cwd`).
  3. The request fails with `409 sync_integrity_failed`; the Navigator disables that row (`unavailableReason` set, matches the store's known-permanent-error-code list) until the remote list is refreshed — and retrying fails identically every time, since the underlying code path never changes.
- frequency: 100% — every Grok chat, every restore attempt, every machine.

## 4. Expected vs Actual

- expected: restoring a Grok chat writes its `chat_history.jsonl` onto the local machine (same behavior class as Codex/Claude), and the chat becomes resumable there.
- actual: every restore attempt failed immediately with a path-validation error; the row became permanently unusable until a full remote-list refresh, at which point retrying it failed again.

## 5. Impact

- users affected: any user restoring a Grok chat onto a different machine (or a clean local state) — i.e. exactly the CP-51 C6 "Drive sync round-trip" scenario, for one of three supported providers.
- workflows affected: Drive restore (`restoreChatRunFromDrive`), Navigator per-row and bulk restore, CP-51 C6 live-verification evidence quality.
- severity: medium — no data loss (the source chat remains intact and re-syncable), but a complete, 100%-reproducing feature gap for Grok restore, discovered live by the operator while validating today's other Drive-sync fixes.

## 6. Root Cause

- hypothesis: user asked whether this was related to the CA-404 index-repair fix landed earlier the same day (i.e. a newly-introduced wrong path on the Drive side).
- confirmed cause: unrelated to CA-404. `chatSessionManifestPath`/`manifestToDriveIndexRecord` (the functions CA-404's repair path also uses) compute the *Drive-side* manifest path identically for every provider — verified by reading both the discovery-repair code path and the normal sync path, which call the exact same helper. The actual defect is entirely on the *local* restore-target side: `restoreTargetPath` (`session_file_locator.go`, pre-fix) has explicit `case`s only for `ProviderKeyCodex` and `ProviderKeyClaude`; every other provider falls into `default: return "", errors.New("unsupported provider session relocation")`. This is the exact gap BUG-310's own §10 Follow-Up section and Task-210's §7 follow-ups line both already named as a separate, deferred, out-of-scope item.
- why it surfaced only now: before today, Grok chats could not sync up at all (BUG-310), and even after fixing that, REMOTE CHATS under-listed rows (CA-404's own root cause). Both fixed the same day, so a user could reach "click Restore on a Grok row" for the first time today — immediately hitting this separate, pre-existing, never-implemented case.
- evidence: live-reproduced via direct `POST /client/chat-sessions/restore` against the running dev server (project "Gate-sandbox", a real synced Grok run) — `409 sync_integrity_failed`, "remote provider session file path is invalid", matching the operator's live report exactly. Confirmed Claude/Codex restores succeed unaffected on the same live server, same call shape — isolating the failure to the Grok provider case specifically, not a general restore regression.
- **incidental finding while writing this bug's own end-to-end test**: the shared test helper `newChatSyncService` (`chat_session_sync_test.go`, used by 40+ tests in this file) only overrides the `HOME` environment variable, not `USERPROFILE`/`APPDATA`/`FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`. On Windows, `getPossibleHomeDirs()` (provider-account auto-discovery) and `providerAccountsConfigPath()` (where accounts get persisted) both read `USERPROFILE`/`APPDATA` directly, bypassing the `HOME` override entirely. Every test using this helper was therefore reading and overwriting the **real, machine-wide** `provider-accounts.json` — confirmed live on this exact development machine, twice, while working on this bug's own tests: once corrupting a real Grok account's `id` field, once (during a deliberate git-stash comparison against the pre-fix baseline) flipping the `is_active` flag on two real, pre-existing accounts. Both incidents were caught immediately, the real file was hand-repaired back to its confirmed-correct state each time (recomputing the affected account's deterministic id via the actual `deterministicProviderAccountID` function rather than guessing), and the helper itself was fixed to fully sandbox `USERPROFILE`/`APPDATA`/`FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH` — matching the isolation pattern every other provider-account test file already uses (`provider_accounts_test.go`, `grok_process_test.go`, `grok_registry_test.go`, `cross_account_resume_test.go`, `bug083_test.go`).

## 7. Fix Strategy

- `F-1` Add a `ProviderKeyGrok` case to `restoreTargetPath` (`session_file_locator.go`) that: validates the relative path ends in `chat_history.jsonl`, validates the session id via `isGrokRealSessionID`, requires a non-empty `cwd`, computes the destination as `grokSessionDirPath(targetHome, cwd, sessionID) + "/chat_history.jsonl"`, and validates the result stays under `targetHome` via `pathUnderRoot` — mirroring `relocationTargetPath`'s existing Grok case exactly, but keyed off `cwd` (always available at restore time) instead of a local `srcPath`.
- `F-2` (incidental, test-only) `newChatSyncService` now also sets `USERPROFILE`, `APPDATA`, and `FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH` to sandboxed paths under its own `t.TempDir()`, so no test using it can read or write the real machine's provider-accounts config.

## 8. Validation

- `V-1` New test `TestRestoreTargetPathGrokAcceptsWellFormedPathAndRejectsBadInput` (`grok_cross_account_resume_test.go`) — accepts a well-formed Grok path and asserts the destination is recomputed from the **target** cwd (not the source-embedded segment); rejects a synthetic `thread-*` id, a missing `cwd`, a wrong file suffix, and a path-traversal session id.
- `V-2` New test `TestRestoreSessionFileGrokRoundTripsAcrossDifferentCwdEncoding` (`grok_cross_account_resume_test.go`) — the decisive proof: writes a Grok session under a Mac-shaped source cwd, restores it under a Windows-shaped target cwd, and asserts `LocateSessionFile` can find the restored file again **under the target's own cwd** (not merely that `RestoreSessionFile` returned no error) — and explicitly asserts the destination is NOT the source's verbatim cwd-encoded directory.
- `V-3` New test `TestRestoreChatRunFromDriveGrokRecomputesPathForDifferentCwd` (`chat_session_sync_test.go`) — full `syncChatRunToDrive` → `restoreChatRunFromDrive` round trip for a real Grok manifest, restored under a different cwd than the source; asserts the restored transcript is discoverable and byte-correct.
- `V-4` All 3 new tests confirmed to **fail on the pre-fix baseline** via `git stash` (isolating just the `session_file_locator.go` fix), reproducing the identical `sync_integrity_failed` / `"unsupported provider session relocation"` errors as the live report.
- `V-5` `go build ./...`, `go vet ./internal/runner/` — clean. Targeted sweep (`TestGrok*`, `TestRestore*`, `TestChatSession*`, `TestCrossAccount*`, `TestSync*`, `TestListRemoteChatSessions*`, `TestLocateSessionFile*`, `TestRelocateSessionFile*`) — clean except one pre-existing intermittent flake (`TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`, a CA-404 concurrency test, confirmed to fail at the same rate on the clean baseline via `git stash`). Full `go test ./...` — 15 failures, all 15 confirmed pre-existing: 14 reproduce identically on the clean baseline (Windows HOME/USERPROFILE env gaps, real `codex`/`git` CLI dependencies, a Grok account-slot machine-state test), and the 15th (`TestFinalizerHookSurfacesArtifacts`) is flaky even run alone, repeatedly, in isolation — unrelated to this change either way.
- `V-6` Real `provider-accounts.json` checksum monitored across the entire verification process (including the full-package sweep) and confirmed stable once `F-2` landed — no further real-file writes from any test in this package.
- `V-7` Live re-verification: restarted the dev stack, re-issued the exact `POST /client/chat-sessions/restore` call that previously 409'd for `run-22023` (Grok, project "Gate-sandbox") — now `200 {"restoreStatus":"restored"}`. Restored a second Grok run (`run-28116`) the same way. Confirmed on disk both landed under `<GROK_HOME>/sessions/D%3A%5Cworking%5Cgate-sandbox/<session-id>/chat_history.jsonl` — this machine's own cwd encoding, byte-identical transcript content.

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/grok_cross_account_resume_test.go` (`TestRestoreTargetPathGrokAcceptsWellFormedPathAndRejectsBadInput`, `TestRestoreSessionFileGrokRoundTripsAcrossDifferentCwdEncoding`), `apps/local-runner/internal/runner/chat_session_sync_test.go` (`TestRestoreChatRunFromDriveGrokRecomputesPathForDifferentCwd`).
- alerts: none.
- audit checks: [CA-406](../../change-audit/CA-406-grok-drive-restore-missing-provider-case.md).

## 10. Follow-Up Document Updates

- upstream docs updated: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) §6 "Gaps known" — the "Grok restore specifically is a known, pre-existing, larger gap" bullet is superseded by this fix; updated in the same pass as this doc.
- notes left unchanged on purpose: Grok restore still only restores the single `chat_history.jsonl` transcript, not the full session directory's sidecar files (`events.jsonl`, `summary.json`, etc.) — because sync-up never uploads those either (BUG-310's own documented scope). Restoring exactly what was synced is correct; a *fuller* Grok directory sync/restore (if ever wanted) is new scope, not a gap in this fix.
