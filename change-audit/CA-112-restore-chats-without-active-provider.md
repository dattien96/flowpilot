# CA-112: Restore Chats Without Active Provider

## Scope

- Google Drive chat-session restore in the local runner
- Read-only history resume without provider authentication
- Desktop chat-controller provider availability
- Task-073 and Task-075 restore requirements

## Completed

- Removed active-account authentication as a precondition for restoring synced chat files.
- Added provider-compatible default storage homes for Codex and Claude when no provider account is registered.
- Allowed chat history resume to replay transcripts when provider auth or an active account is unavailable.
- Kept new-turn execution behind the existing provider readiness checks.
- Disabled unavailable provider cards and chat sending until at least one connected account exists.
- Updated automated coverage for providerless restore, unauthenticated read-only resume, and Claude session storage without installation.

## GitNexus Impact

- `restoreChatRunFromDrive`: HIGH, 15 direct dependents including the HTTP handler and restore regression tests.
- `listRemoteChatSessions`: MEDIUM, 5 direct dependents; inspected but not modified.
- `resumeRun`: LOW, 1 direct handler caller.
- `ensureResumeReady`: LOW, 2 direct runtime callers.
- `seedTranscriptFromDisk`: LOW, 4 direct callers/tests.
- `ChatInput`: LOW, no indexed upstream dependents.
- No HIGH or CRITICAL warning was ignored; the user approved the HIGH-risk restore change before implementation.
- `gitnexus_detect_changes` was unavailable in this environment. Scope was checked with `git diff --name-only`, `git diff --check`, focused tests, and the full runner package suite.

## Verification

- `GOCACHE=/private/tmp/flowpilot-go-cache go test ./internal/runner -count=1` — pass.
- `npm run typecheck` in `apps/desktop-flowpilot` — pass.
- `git diff --check` — pass.
- Phase-document compliance review — pass after adding direct implementation-guide traceability.

## Residual Notes

- Manual validation on PC B with Claude not installed remains recommended.
- Existing user edits in `AGENTS.md` and `CLAUDE.md` were not modified.
