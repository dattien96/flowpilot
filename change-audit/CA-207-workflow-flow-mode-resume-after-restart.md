# CA-207: Workflow/Flow-Mode Resume After Restart

## Summary

Fixed `BUG-170`: after a runner restart, only `normal_chat` history runs could be reopened — workflow/flow-mode runs hit a blanket `409 resume_unsupported`, an intentional MVP-era limitation (`TestResumeRunRestoredWorkflowRunUnsupportedForMVP`). The reconstruction path (`reconstructRun`, `ensureResumeReady`) turned out to already be generic over `runKind`, so removing the guard was sufficient for full resume rather than requiring new persisted state.

## What Changed

- `apps/local-runner/internal/runner/interactive_resume.go`: removed the `st.RunKind != "chat"` → `409 resume_unsupported` rejection in `loadPersistedRun`. Gated `reconstructRun`'s synthetic chat-step seed call on `st.RunKind == "chat"` (mirroring `createRun`'s own branch) so a resumed workflow run's real step-runtime list isn't overwritten with a fake single "chat" step.
- `apps/local-runner/internal/runner/cross_account_resume_test.go`: replaced `TestResumeRunRestoredWorkflowRunUnsupportedForMVP` with `TestResumeRunReconstructsWorkflowRunFromDisk`, proving a persisted workflow-kind session resumes successfully and its step-runtime isn't corrupted by the reconstruction seed call.
- `apps/desktop-flowpilot/src/state/store.ts`: `openHistoryRun` now derives `chatMode` (and, when a `workflowId` is present, `launchMode`/`selectedWorkflowId`) from the reopened history item's `runKind`, so a reopened workflow run re-mounts its Flow Mode surfaces (step-timeline sidebar, Agents panel) instead of staying in whatever mode the UI was previously in.

## Verification

- `TestResumeRunReconstructsWorkflowRunFromDisk` — passes.
- Full runner `go test ./...` — no new failures (two pre-existing, unrelated failures confirmed present without this change: `TestSkillsMerge*WithPrecedence`, `TestStartInteractiveAuthLaunchesFromWorkspace`); all 10 `TestResumeRun*` tests and the untouched Drive-sync `TestBuildChatSessionSyncManifestRejectsNonChatRun` guard pass.
- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (no runner + provider account in this environment) — flagged in `BUG-170` (`V-5`).

## Notes

- Investigated alongside `BUG-168` and `BUG-169` from the same user report.
- `chat_session_sync.go`'s own, separate `RunKind != "chat"` guard (Google Drive chat sync) is a different feature (which runs can sync to Drive) and was intentionally left untouched; its test still passes unmodified.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-170
change_type: bugfix
summary: Allow workflow/flow-mode runs to resume after a runner restart (previously rejected outright), and restore the desktop's chat/launch mode on reopen
# --->8---
