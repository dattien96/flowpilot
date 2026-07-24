# BUG-316: Restored Grok Chat Fails Its Next Turn — Sync Only Carried One Of The Session Directory's Files

## Metadata

- Document ID: `BUG-316`
- Title: `A Grok chat restored from Drive opens fine but fails its very next turn with "Path not found." — the sync manifest only ever carried chat_history.jsonl, not the rest of the session directory Grok's own session/load reads`
- Phase: `bugfix`
- Status: `done`
- Owner: `google-drive`
- Reviewers: `TBD`
- Created: `2026-07-23`
- Last Updated: `2026-07-23`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md)
- Child Documents: `none`
- Related Documents: [BUG-310: Grok Drive Sync Always Fails Reading Session Directory As File](../done/BUG-310-Grok-Drive-Sync-Always-Fails-Reading-Session-Directory-As-File.md) (first found that Grok's session path is a directory, not a file — fixed the READ side; this bug is the same fact biting the SYNC/RESTORE completeness side), [BUG-312: Grok Drive Restore Rejects Every Session](../done/BUG-312-Grok-Drive-Restore-Rejects-Every-Session-Missing-Provider-Case.md) (fixed the PATH the file restores to; this bug fixes WHICH files restore), [BUG-313: Restored Chat Timeline Broken](../done/BUG-313-Restored-Chat-Timeline-Broken-Sync-Never-Carried-Turn-Log.md) (same class: sync silently dropped a field the restored run needs to behave like a same-machine resume), [CA-410](../../change-audit/CA-410-sync-carries-full-grok-session-directory.md)
- Replaces: `none`
- Tags: `google-drive, chat-sync, grok, restore, cross-provider, regression, severity-high`

## AI Quick View

### Summary

- Operator restored a deleted Grok chat from Drive. Opening it worked; sending it any follow-up (even "say hi" a second time) triggered `[recovering: re-sending turn after a recoverable error]` three times, then a hard failure: `"Path not found."`.
- Root cause: unlike Codex/Claude (FlowPilot itself replays a single transcript/rollout file it owns), Grok is **stateful on the provider side** — its own `session/load` call reads the *entire* session directory (`chat_history.jsonl`, `events.jsonl`, `updates.jsonl`, `prompt_context.json`, `system_prompt.txt`, `summary.json`, `signals.json`, `resources_state.json`, `rewind_points.jsonl`, `announcement_state.json`). The sync manifest (and restore) only ever carried `chat_history.jsonl` (established by BUG-310's fix, which taught the *read* side that Grok's path is a directory, but never extended to *carrying the whole directory*). Deleting a chat deletes its provider session directory outright, so every restored Grok chat's directory came back with exactly one file — enough to *display* history, not enough for Grok's own resume.
- Confirmed live by diff: a session directory that was never deleted has 14 entries; the same session directory rebuilt by restore had 1.

### Current Ask

- Fixed. The manifest carries every other regular file in a Grok session directory (new `ProviderFiles` field, Grok-only); restore rewrites all of them into the same directory as `chat_history.jsonl`.

### Key Decisions

- `V-1` Skip `*.lock` files. They are zero-byte runtime lock markers Grok recreates itself; carrying a stale one risks the restored machine believing a lock is already held for no reason.
- `V-2` Carry sidecar bytes through an **unexported** manifest field (`grokSidecarBodies map[string][]byte`) rather than widening `BuildChatSessionSyncManifest`'s public return signature. `encoding/json` silently skips unexported fields (never reaches `manifest.json`), and 8 pre-existing tests call `BuildChatSessionSyncManifest` with its current 3-value signature — this keeps every one of them compiling and passing unchanged, per the additive-tests-only rule.
- `V-3` Restore-side strictness for sidecars **mirrors the primary file's own rules** exactly (hard `sync_remote_not_found` / `sync_integrity_failed` / `session_file_conflict` on missing/corrupt/conflicting, not a silent best-effort skip) — a silently-skipped sidecar would just reproduce this exact bug on the next turn, so failing loudly is the correct behavior, not overly strict.
- `V-4` Sidecar restore reuses the primary file's own already-validated target directory (`filepath.Dir(targetPath)`, computed once via the existing `restoreTargetPath` — BUG-312's cwd-recompute fix) rather than adding a second path-validation path — every sidecar lives alongside `chat_history.jsonl` by construction, so this is both simpler and doesn't touch BUG-312's careful path-traversal guard at all.
- `V-5` No manifest schema-version bump: `ProviderFiles` is `omitempty`; old manifests restore exactly as before (chat_history.jsonl only — the pre-fix, still-broken-on-next-turn behavior) until re-synced.
- `V-6` Provider-scoped by construction, not by an explicit branch that could silently widen later: `resolveGrokSessionSidecarFiles` is only ever called when `session.ProviderKey == ProviderKeyGrok`; a dedicated cross-provider test asserts Codex/Claude manifests never populate `ProviderFiles`.

### Constraints

- Backend-only; no desktop change.
- Does not change Codex/Claude sync/restore at all (verified — see V-6 and §8).

### Open Questions

- None for the defect. Whether `.lock` files should ever be synced (e.g., to signal "this session was mid-write on the source machine") is speculative UX polish, not required.

### Source Refs

- `apps/local-runner/internal/runner/chat_session_sync.go` — `ChatSessionSyncManifest.ProviderFiles` / `grokSidecarBodies` (unexported), new `resolveGrokSessionSidecarFiles`, `BuildChatSessionSyncManifest` (attach), `uploadChatSessionRunFiles` (upload loop), `restoreChatRunTreeFromDrive` (restore loop).
- `apps/local-runner/internal/runner/grok_transcript_loader.go` — `grokSessionDirPath`/`grokSessionsCwdDirName` (existing, reused verbatim — no change).
- `apps/local-runner/internal/runner/bug316_grok_restore_session_dir_test.go` — the 4 new tests (§8).
- Live evidence: hub `run-45881` (Gate-sandbox, Grok) — `initialize`/`session/load` RPC frames captured directly from the runner's own log: `session/load` for the restored session id returned `{"code":"FS_NOT_FOUND","detail":"The system cannot find the file specified. (os error 2)"}` / `"Path not found."` on 3 consecutive attempts, both under the run's own registered account (`.grokHome1`, file confirmed present at the exact resolved path by `LocateSessionFile`) and, separately, under a different account (`.grok`) after an unrelated account-file mix-up — ruling out account/cwd as the cause and pointing at the directory's own completeness. A live, never-deleted Grok session directory for the same project was found to hold 14 files; the restored directory held 1.

## 1. Issue Summary

A Grok chat restored from Google Drive opens and displays its history correctly, but the very next turn sent to it fails outright — a 3x "recoverable error" retry loop, then a hard `"Path not found."` failure, regardless of which account or working directory it runs under.

## 2. Parent Links

- impacted coding plan: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — the C6 Drive-restore-down property; this is the second live defect found on that path today (after BUG-315), and the reason C6's Grok coverage specifically was not yet fully closed.
- impacted tech design: `none directly` — extends the same-machine-resume parity contract BUG-310/BUG-312/BUG-313 already established for other Grok/manifest gaps.
- impacted system spec: `none known`.

## 3. Environment and Reproduction

- environment: any Grok chat synced to Drive, then deleted locally (or restored onto a machine that never had it), then restored again.
- reproduction steps:
  1. Have any Grok chat with at least one completed turn. Sync it to Drive.
  2. Delete the chat locally (this deletes the provider's own session directory, not just FlowPilot's own records).
  3. Restore it from REMOTE CHATS.
  4. Open it (works — history displays).
  5. Send any follow-up message.
  6. expected: the follow-up gets a normal reply, same as continuing any other restored chat (Codex/Claude both work this way).
  7. actual: `[recovering: re-sending turn after a recoverable error (attempt 2/3)]`, `(attempt 3/3)`, then `"Path not found."` in red — every time, deterministically.
- frequency: 100% of the first follow-up after restoring any Grok chat whose session directory was deleted (which is every restore following a chat delete — the common case).

## 4. Expected vs Actual

- expected: a restored Grok chat's next turn is a normal continuation, same as the always-worked Codex/Claude case.
- actual: parity held for *display* (BUG-310/BUG-313 already fixed prompts/timeline/agent-cards) but not for *resumability* — Grok's own provider-side session state was never fully carried across the sync/restore round trip.

## 5. Root Cause

- confirmed cause: Grok's ACP `session/load` is a **provider-owned stateful resume**, unlike Codex (FlowPilot resumes a rollout file it fully owns and replays itself) or Claude (same). It needs every file in its own session directory to reconstruct its state — not only the human-readable transcript (`chat_history.jsonl`). `resolveChatSessionTranscript` (BUG-310) correctly taught FlowPilot that Grok's `LocateSessionFile` result is a *directory*, and to read `chat_history.jsonl` specifically inside it for the manifest's transcript field — but nothing was ever added to carry the OTHER files in that directory. `uploadChatSessionRunFiles` uploaded exactly one file per run; `restoreChatRunTreeFromDrive` wrote back exactly one file. A chat delete (confirmed live) removes the provider's own session directory outright, so any subsequent restore rebuilds a directory containing only that one file — everything Grok's own `session/load` needs besides the transcript is simply gone.
- evidence: live RPC capture from the running dev runner — `session/load({"cwd":"...","sessionId":"019f8cc3-..."})` against a restored, byte-verified-present `chat_history.jsonl` still returned `FS_NOT_FOUND` / `"Path not found."` three times in a row, both under the run's correctly-registered account (`.grokHome1`) and, in an earlier diagnostic pass, under a different account — ruling out account resolution or working-directory mismatch (an earlier hypothesis this session, retracted once the operator pointed out the same topology had worked live minutes earlier). Direct filesystem comparison: a Grok session directory that was never deleted holds 14 files (`chat_history.jsonl`, `chat_history.jsonl.lock`, `events.jsonl`, `updates.jsonl`, `prompt_context.json`, `resources_state.json`, `rewind_points.jsonl`, `rewind_points.jsonl.lock`, `signals.json`, `summary.json`, `summary.json.lock`, `system_prompt.txt`, `announcement_state.json`); the same session directory rebuilt by restore held only `chat_history.jsonl`.

## 6. Fix Strategy

- `F-1` `ChatSessionSyncManifest` gains `ProviderFiles []ChatSessionFile` (`json:"providerFiles,omitempty"`) plus an unexported `grokSidecarBodies map[string][]byte` carrying their bytes (never reaches `manifest.json` — `encoding/json` skips unexported fields by construction).
- `F-2` New `resolveGrokSessionSidecarFiles(accountHome, cwd, sessionID string)` lists the Grok session directory (`grokSessionDirPath`, unchanged, reused verbatim), reads every regular file except `chat_history.jsonl` (already handled) and `*.lock` markers (excluded, V-1), and returns their `ChatSessionFile` metadata + bytes.
- `F-3` `BuildChatSessionSyncManifest` calls it — only when `session.ProviderKey == ProviderKeyGrok` and the resolved session id is a real (non-`thread-`) ACP id — and attaches the result.
- `F-4` `uploadChatSessionRunFiles` uploads each sidecar into the same per-run provider folder as `chat_history.jsonl`, recording each one's Drive object id back onto the manifest.
- `F-5` `restoreChatRunTreeFromDrive` downloads and writes each sidecar into the same directory as the primary file (`filepath.Dir(targetPath)`), with the exact same strictness as the primary file's own restore path (missing → `sync_remote_not_found`; hash mismatch → `sync_integrity_failed`; different local copy already present → `session_file_conflict`; identical local copy → no-op).

## 7. Validation

- `V-1` Red-first TDD: `TestSyncChatRunToDriveManifestCarriesGrokSessionSidecarFiles`, `TestRestoreChatRunFromDriveRebuildsGrokSessionSidecarFiles`, and `TestRestoreChatRunFromDriveGrokSidecarConflictsWithDifferentLocalCopy` all fail on pre-fix HEAD (`git stash` isolating only `chat_session_sync.go`, new test file kept in place) — `manifest providerFiles = <nil>`, `restored session dir missing "events.jsonl"`, and "restore must conflict" respectively — then pass after popping the stash back. Assertions read raw manifest JSON / on-disk file bytes, not new Go symbols, so they compile cleanly on the baseline.
- `V-2` `TestSyncChatRunToDriveNeverCarriesSidecarFilesForCodexOrClaude` — cross-provider guard: Codex and Claude manifests never populate `providerFiles` (passes on both baseline and fix, since it locks in a property this fix must not disturb, not a repro of the defect itself).
- `V-3` `TestRestoreChatRunFromDriveRebuildsGrokSessionSidecarFiles` proves the full round trip: sync a 10-file session directory, delete it entirely (matching a real chat delete), restore under a different target cwd, and every sidecar file is byte-identical to the original at the newly-recomputed location.
- `V-4` `go build ./...`, `go vet ./internal/runner/...`: clean. Targeted sweep (every `TestBug31[2-6]*`, `TestSyncChat*`, `TestRestoreChat*`, `TestBuildChatSessionSyncManifest*`, `TestRestoreTargetPath*`, `TestRestoreSessionFile*`): fully green, including the pre-existing chat-sync/BUG-310/BUG-311 suite (39/39) re-run unmodified after this change.
- `V-5` Live end-to-end re-verification on the running dev runner (fixed binary, project Gate-sandbox): created a fresh real Grok chat (`run-53890`), sent one turn to completion, synced it to Drive, **deleted it** (confirmed the entire 12-file session directory was gone from disk, not just `chat_history.jsonl` — matching the exact real-world trigger), restored it from Drive (all 9 non-lock files rebuilt, sizes byte-matching the originals), resumed it, and sent a second live turn — Grok streamed a real reply (`"bug316 followup ok"`, echoing the requested exact text) and the turn completed normally. Before this fix, this exact sequence reproduced `"Path not found."` 100% of the time (confirmed earlier the same day on live hub `run-45881`).
- `V-6` Full-package regression sweep (`go test ./...`, `git stash` baseline comparison): fix = 2496 passed / **17** failed; baseline (fix stashed) = 2494 passed / **16** failed. The extra failure on the fix run, `TestFinalizerHookSurfacesArtifacts`, is NOT caused by this change — it passed 3/3 when re-run in isolation on the fixed tree immediately after, confirming it as the same order-dependent flake already catalogued in this session's regression notes (alongside `TestRun20332FlowHubHistoryParityForEveryProvider/grok`); it simply didn't trigger on the particular baseline run. Every other failure on both runs is the identical, previously-catalogued machine/CLI/env-dependent set (real `codex` CLI resume, live Grok account-slot state, skills-merge fixtures, Windows paths, git-shim). Net: zero new failures attributable to BUG-316.

## 8. Regression Guard

- tests: `apps/local-runner/internal/runner/bug316_grok_restore_session_dir_test.go` (`TestSyncChatRunToDriveManifestCarriesGrokSessionSidecarFiles`, `TestRestoreChatRunFromDriveRebuildsGrokSessionSidecarFiles`, `TestRestoreChatRunFromDriveGrokSidecarConflictsWithDifferentLocalCopy`, `TestSyncChatRunToDriveNeverCarriesSidecarFilesForCodexOrClaude`).
- alerts: none.
- audit checks: [CA-410](../../change-audit/CA-410-sync-carries-full-grok-session-directory.md).

## 9. Follow-Up Document Updates

- upstream docs updated: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — C6 restore-side note updated in the same pass.
- notes left unchanged on purpose: chats synced by a pre-fix manifest still restore with only `chat_history.jsonl` until re-synced from a machine that still holds the full session directory (mirrors BUG-313's identical caveat for the turn log).

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-316
change_type: bugfix
summary: chat-sync manifest now carries every file in a Grok session directory (not just chat_history.jsonl) and restore rewrites them all, so a restored Grok chat's next turn resumes instead of failing FS_NOT_FOUND ("Path not found.").
# --->8---
