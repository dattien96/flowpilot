# CA-099 - Fix Desktop Legacy Default Account Chat Resume

## Scope

Fix the local-runner chat resume path for legacy desktop history rows whose `provider_account_id` was persisted as `default` before provider accounts were represented by durable ids. The affected workflow is clicking a completed/cancelled chat in the desktop left history list after provider account sync has created a generated active account id for the same physical provider home.

## Completed

- Confirmed the desktop symptom maps to `openHistoryRun` -> `resumeRun` -> `ensureResumeReady` -> `prepareCrossAccountResume`.
- Confirmed local history rows used `provider_account_id: "default"` while the current provider account registry uses generated active ids.
- Added same-home handling in `RelocateSessionFile`: when the computed destination path is the same physical path as the source session file, relocation succeeds as a no-op instead of failing as an overwrite.
- Added regression coverage for Codex and Claude same-home no-op relocation and for rebinding a resumed run from an old account id to the current active account id.

## Verification

- GitNexus index refreshed with `npx gitnexus analyze`.
- GitNexus impact for `RelocateSessionFile`: LOW risk, 4 direct upstream callers, runner module only, 0 affected processes.
- GitNexus impact for `prepareCrossAccountResume` and `ensureResumeReady`: LOW risk, 0 upstream callers/affected processes in the refreshed index.
- Passed: `go test ./internal/runner -run 'Test(RelocateSessionFileSameHomeCodexIsNoop|RelocateSessionFileSameHomeClaudeIsNoop|ResumeRunSameHomeAccountRebindsProviderAccountID|PrepareCrossAccountResumeRelocatesAndRepointsRun|PrepareCrossAccountResumeRelocationFailureKeepsHistoryVisible)' -count=1 -v`.

## Residual Notes

- A broader targeted batch also including `TestRestoredCodexRunUsesCLIResumePath` was not fully green on Windows. That test failed because its shell stub did not create the expected `codex-home.txt` capture file; the same-home resume regression tests passed.
- The greyout behavior remains correct for real missing provider files, missing auth, or true destination collisions. This change only handles the false collision where source and destination are the same file.
