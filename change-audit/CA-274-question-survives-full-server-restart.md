# CA-274 — Resolved Question Survives A Full Server Restart

## Scope

Implemented [BUG-271](../requirements/09-BugFix/done/BUG-271-Resolved-Question-Vanishes-Or-Reappears-Interactive-After-Server-Restart.md): CA-271 fixed a resolved question replaying read-only on SSE reconnect while the runner stays alive. Live-testing that fix against a **full process restart** (not just reconnect) showed the question vanished entirely — `reconstructRun` rebuilds `rs.events` from the provider's own transcript (Claude/Codex), which has no concept of FlowPilot's own `user_question_required` gate at all.

## Changes

- `local_file_session_store.go`: `isFlowSidecarEventType` now includes `EventUserQuestionRequired`, so the raw "asked" event auto-persists to (and reloads from) the existing per-run CP-41 flow-events sidecar. New `questionsFilePath()`/`loadQuestionsFromDisk()` (called unconditionally at the top of `loadFromDisk()`, before the sessions.ndjson-missing early return) + an `UpsertQuestion` override that write-throughs to a new `questions.ndjson` (mirrors `sessions.ndjson`/BUG-080) + `ListQuestionsByRun`.
- `workflow_store.go`: new `QuestionHistoryReader` interface (`ListQuestionsByRun`), mirroring the existing `SessionHistoryReader`/`FlowEventStore` optional-extension pattern — zero blast radius on the base `WorkflowStore` interface or `SupabaseWorkflowStore` (which, like `FlowEventStore`, does not implement it — same accepted asymmetry). `fakeWorkflowStore.ListQuestionsByRun` added.
- `interactive_resume.go` — `reconstructRun`: after the existing `LoadFlowEvents` restore, merges persisted question state onto restored `user_question_required` events — stamps `Answer` for `resolved` questions (read-only render, same as CA-271), drops `expired` questions entirely, leaves anything with no persisted state (still-pending) untouched.
- Tests: `interactive_service_test.go` (+3: full-restart-survives-with-answer, full-restart-drops-expired, storage-layer questions.ndjson round-trip). The storage-layer test caught a real ordering bug in this fix itself (see below) before it shipped.

## Verification

- `go build ./...`, `go vet ./internal/runner/` — clean.
- `go test ./internal/runner/ -count=1` — 1243 passed, 15 failed, all pre-existing environment-dependent flakes already tracked this session (Codex CLI resume, account-home, skills-merge, auth-workspace) — zero new failures.
- Caught-and-fixed during development: `loadQuestionsFromDisk` was initially called only at the end of `loadFromDisk`, after an early return that fires when `sessions.ndjson` doesn't exist yet — a fresh install with a question but no session yet would silently skip loading. The new `TestLocalFileSessionStoreQuestionsSurviveRestart` caught this (0 states instead of 2 on first run); fixed by calling it unconditionally before the early return.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-271
change_type: bugfix
summary: a resolved/expired workflow question now durably survives a full process restart (not just an SSE reconnect) — read-only with the recorded answer if resolved, dropped if expired — via a new questions.ndjson sidecar and reusing the existing CP-41 flow-events sidecar for the raw question event
# --->8---
