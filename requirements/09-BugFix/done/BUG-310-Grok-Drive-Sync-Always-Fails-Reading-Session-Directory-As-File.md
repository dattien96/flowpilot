# BUG-310: Grok Drive Sync Always Fails Reading Session Directory As File

## Metadata

- Document ID: `BUG-310`
- Title: `Grok Drive Sync Always Fails Reading Session Directory As File`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-22`
- Last Updated: `2026-07-22`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md)
- Child Documents: `none`
- Related Documents: [BUG-309: Project History Under-Reports Sync Status For In-Memory Runs](../done/BUG-309-Project-History-Under-Reports-Sync-Status-For-In-Memory-Runs.md) (found investigating the same live symptom, different root cause), [BUG-311: Session Unavailable Sync Never Persists Terminal Status](../done/BUG-311-Session-Unavailable-Sync-Never-Persists-Terminal-Status.md) (found and fixed alongside this bug), [CA-399](../../change-audit/CA-399-grok-drive-sync-directory-read.md)
- Replaces: `none`
- Tags: `google-drive, chat-sync, grok, windows, regression, severity-high`

## AI Quick View

### Summary

- Every Drive sync attempt for a Grok chat run failed, always, on every machine that has ever tried it.
- `resolveChatSessionTranscript` called `os.ReadFile` directly on whatever `LocateSessionFile` returned, assuming it was always a single transcript file.
- For Grok specifically, `LocateSessionFile` intentionally returns the session **directory** (Grok stores `chat_history.jsonl` plus sidecar files there, unlike Codex/Claude's single rollout/transcript file) — reading a directory as a file fails, surfacing on Windows as the cryptic `"Incorrect function"` (`ERROR_INVALID_FUNCTION`).

### Current Ask

- Fixed. `resolveChatSessionTranscript` now reads `chat_history.jsonl` inside the returned directory specifically for the Grok provider; Codex/Claude are unaffected (their `LocateSessionFile` result was already a file path).

### Key Decisions

- `V-1` Sync must read the actual transcript **file**, not attempt to open the session directory as a file — the fix is scoped to computing the correct read path for Grok only, not to changing `LocateSessionFile`'s documented directory-return contract (other callers, e.g. `interactive_resume.go`'s relocate/restore paths, already correctly treat it as a directory).
- `V-2` The uploaded/hashed provider bytes for Grok are the raw, unmodified `chat_history.jsonl` contents — not a transformed/canonicalized form (e.g. `grok_transcript_loader.go`'s "Claude-shaped" replay format), since the manifest's purpose is to back up the provider's own native session artifact for a genuine restore, not FlowPilot's internal context-reading representation.

### Constraints

- Scoped to the sync/upload (read) side only. Grok's **restore** side has a separate, pre-existing, larger gap: `restoreTargetPath` (`session_file_locator.go:609`) only recognizes Codex and Claude — any provider outside that `switch` (including Grok) returns `"unsupported provider session relocation"`. This bug does not add Grok restore support; see Follow-Up.
- No change to `LocateSessionFile`, `grokSessionDirPath`, or any other Grok cross-account/resume code path (`interactive_resume.go`, `grok_cross_account_resume_test.go` all still pass unmodified).

### Open Questions

- None for the sync-upload defect itself. Whether/when to add Grok restore support (the pre-existing, separate gap above) is an open scoping question, not a defect of this fix.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` — `resolveChatSessionTranscript`.
- `apps/local-runner/internal/runner/session_file_locator.go` — `LocateSessionFile` (Grok case, `session_file_locator.go:56-70`, returns a directory by design), `restoreTargetPath` (`session_file_locator.go:609`, Grok not yet supported for restore).
- `apps/local-runner/internal/runner/bug310_test.go` — `TestResolveChatSessionTranscriptReadsGrokChatHistoryFile`.
- Live evidence: direct `POST /client/workflow-runs/{runId}/sync-chat` against the running dev server (project "Gate-sandbox") returned, pre-fix: `{"error":{"code":"workflow_state_unavailable","message":"read C:\\Users\\dat.nguyen\\.grok\\sessions\\D%3A%5Cworking%5Cgate-sandbox\\<id>: Incorrect function."}}`; post-fix (same runs, same live server, restarted): `syncStatus:"synced"` for all 5 affected Grok runs in that project (`run-22023`, `run-23556`, `run-28116`, `run-24345`, `run-22637`).

## 1. Issue Summary

Clicking "Sync" (per-chat or "Sync all") on any chat run whose provider is Grok always failed. The Navigator's unsynced-count badge could never reach zero for a project containing Grok chats, since every sync attempt for them failed identically on every retry.

## 2. Parent Links

- impacted coding plan: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) (discovered during CP-51 C6 Drive-sync live verification, same session as BUG-309)
- impacted tech design: [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md) (Option B directory-per-session design this bug's fix must respect; its own follow-ups already note "Drive restore packaging for Grok session directories" as out of scope, consistent with this bug's own Follow-Up section)
- impacted system spec: `none known`

## 3. Environment and Reproduction

- environment: local-runner (any OS -- see Impact for why Windows surfaced the clearest symptom), any project with at least one Grok chat run.
- reproduction steps:
  1. Have a completed Grok chat run with a real (non-synthetic) provider session id.
  2. `POST /client/workflow-runs/{runId}/sync-chat` (or click "Sync" in the Navigator).
  3. The request fails with `workflow_state_unavailable`, wrapping a raw filesystem read error on the session directory path.
- frequency: 100% -- every Grok chat, every attempt, every machine.

## 4. Expected vs Actual

- expected: the Grok chat's transcript uploads to Drive and `syncStatus` becomes `"synced"`, same as Codex/Claude.
- actual: the sync attempt always failed with `workflow_state_unavailable`; `syncStatus` never advanced past unset for any Grok chat.

## 5. Impact

- users affected: any user with Grok chats who tries to sync chat history to Drive -- likely every FlowPilot user who has used the Grok provider, on every machine, since this code path has no working case.
- workflows affected: Drive sync (`syncChatRunToDrive`), the Navigator "Sync all" batch, CP-51 C6 live-verification evidence quality.
- severity: high -- a whole provider's chats could never be backed up to Drive or moved to another machine via the sync pipeline; no data corruption, but a complete feature gap for one of three supported providers.

## 6. Root Cause

- hypothesis: initially suspected the percent-encoded directory name itself (`D%3A%5Cworking%5Cgate-sandbox`) was a FlowPilot path-construction bug (encoding a cwd that should have stayed a normal path).
- confirmed cause: the percent-encoding is **correct and intentional** -- it exactly replicates how the real Grok CLI names its own session directories on disk (documented and verified in `grok_transcript_loader.go:181-183`). The actual defect is in `resolveChatSessionTranscript` (`chat_session_sync.go:401`, pre-fix): it called `os.ReadFile(sessionPath)` unconditionally on whatever `LocateSessionFile` returned. For Codex/Claude this is a file path and works. For Grok, `LocateSessionFile` (`session_file_locator.go:70`) deliberately returns the **session directory** (Grok stores `chat_history.jsonl` plus sidecar files inside it, per Task-210 Option B) -- `os.ReadFile` on a directory path fails. On Windows this raises `ERROR_INVALID_FUNCTION` ("Incorrect function"); other platforms would instead see `EISDIR` ("is a directory"), so the defect was never OS-specific, only its error text was.
- evidence: `TestResolveChatSessionTranscriptReadsGrokChatHistoryFile` fails on the pre-fix baseline reproducing the exact live error text (`read .../<id>: Incorrect function.`), confirmed via `git stash` on the sole modified file. Live re-test against the running dev server (5 real Grok runs in project "Gate-sandbox") failed identically pre-fix and succeeded (`syncStatus: "synced"`) post-fix.

## 7. Fix Strategy

- `F-1` In `resolveChatSessionTranscript`, when `session.ProviderKey == ProviderKeyGrok`, join `"chat_history.jsonl"` onto the directory `LocateSessionFile` returned before calling `os.ReadFile`. Codex/Claude paths are unchanged (still read directly).
- `F-2` `relativePath` (used both for the uploaded filename and the manifest's `ProviderFile.RelativePath`) is now computed against the actual file path, so it correctly ends in `chat_history.jsonl` rather than the bare directory name.

## 8. Validation

- `V-1` New test `TestResolveChatSessionTranscriptReadsGrokChatHistoryFile` (`bug310_test.go`) -- PASS on the fix; confirmed FAIL on the pre-fix baseline via `git stash`, reproducing the exact live error text.
- `V-2` `go build ./...` and `go vet ./internal/runner/` -- clean.
- `V-3` Existing Grok test suites (`grok_cross_account_resume_test.go`, `grok_transcript_loader_test.go`, `TestSyncCodex*`, etc.) -- unaffected; `LocateSessionFile`/`grokSessionDirPath` were not touched.
- `V-4` Regression sweep (`TestDispatch*`, `TestCrashMatrix*`, `TestRecoveryScanner*`, `TestSync*`, `TestChatSession*`, `TestRestoreChatRun*`, `TestDeleteRun*`, `TestTurn*`, `TestInteractive*`, `TestBug*`, `TestRunHistory*`, `TestProjectHistory*`, `TestGrok*`, `TestCrossAccount*`): only 2 pre-existing failures unrelated to this change, both confirmed present on baseline via `git stash` -- `TestRestoreChatRunFromDriveMissingActiveAccountHome` (see CA-397) and `TestNextAccountHomePathGrokUsesGrokHomePrefix` (depends on real machine-local provider-account slot state, unrelated to session-file reading).
- `V-5` Live re-verification: restarted the dev stack (`just dev`) to load the fix, then called `POST /client/workflow-runs/{runId}/sync-chat` directly against all 5 Grok runs in project "Gate-sandbox" (`run-22023`, `run-23556`, `run-28116`, `run-24345`, `run-22637`) that had failed pre-fix -- all 5 now return `syncStatus: "synced"`.

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/bug310_test.go` (`TestResolveChatSessionTranscriptReadsGrokChatHistoryFile`).
- alerts: none.
- audit checks: [CA-399](../../change-audit/CA-399-grok-drive-sync-directory-read.md).

## 10. Follow-Up Document Updates

- upstream docs that must change: none for this fix -- Task-210's directory-per-session design for Grok is unchanged and correctly respected.
- notes left unchanged on purpose: Grok's **restore** side (`restoreTargetPath` rejecting all providers except Codex/Claude) remains unsupported -- this is a separate, larger gap (multi-file directory restore, not a single-file write) that this fix does not address. Worth a dedicated follow-up task if cross-machine restore of Grok chats is needed; not required for the confirmed sync-upload defect this bug fixes.
