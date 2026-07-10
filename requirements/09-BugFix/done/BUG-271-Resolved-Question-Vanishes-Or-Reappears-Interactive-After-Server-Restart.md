# BUG-271: Resolved Question Vanishes Or Reappears Interactive After Server Restart

## Metadata

- Document ID: `BUG-271`
- Title: `Resolved Question Vanishes Or Reappears Interactive After Server Restart`
- Phase: `bugfix`
- Status: `done` — fixed and verified 2026-07-10, found live re-testing CA-271's own reconnect-while-alive fix against a full app restart.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [Task-204: Wire Production MCP Driver Adapter](../../08-Task/done/Task-204-Wire-Production-MCP-Driver-Adapter.md)
- Child Documents: `none`
- Related Documents: [CA-271: Resolved Question Replays Read-Only On Reconnect](../../../change-audit/CA-271-stale-question-replay-on-reconnect.md) (the sibling fix for the SSE reconnect-while-alive case; this bug is the full-process-restart counterpart), [CA-272: Stale Cancelled Status On App Start](../../../change-audit/CA-272-stale-cancelled-status-on-app-start.md) (a different restart-resume defect found the same day, same live-test session)
- Replaces: `none`
- Tags: `agent-flow-engine`, `flow-mode`, `resume`, `question`, `regression`

## AI Quick View

### Summary

- CA-271 fixed `subscribe()` so a resolved/skipped question replays read-only (with the recorded answer) when a client reconnects **while the runner process stays alive**.
- Live-testing that exact fix against a **full server restart** (not just an SSE reconnect) surfaced a worse, distinct defect: the question disappeared from the transcript entirely.
- Root cause: after a restart, `reconstructRun`/`seedTranscriptFromDisk` rebuilds `rs.events` purely from the **provider's own transcript** (Claude/Codex conversation JSONL). That transcript has no concept of FlowPilot's own `user_question_required` gate at all — it is a FlowPilot-only annotation layered on top of the provider conversation. So after a restart the event simply never existed in the reconstructed timeline, regardless of whether it had been answered.
- The existing `AppendEvent`/`UpsertQuestion` persistence calls (already fired live, via `emitLocked`/`persistQuestion`) were write-only for the local-file backend — `localFileSessionStore` kept both in a process-scoped RAM map, never written to disk, and nothing ever read them back on resume.

### Current Ask

- Make a resolved question survive a full process restart, rendering the same read-only "answered" state CA-271 already established for the reconnect-while-alive case — without weakening any of the many prior status/resume fixes in this area (BUG-060, BUG-074, BUG-080, BUG-118, BUG-121, BUG-157, BUG-174, BUG-233, BUG-242, BUG-248).

### Key Decisions

- `F-1` Reuse the **existing CP-41 flow-events sidecar** mechanism (`FlowEventStore`/`isFlowSidecarEventType`, originally built for `FlowContextPackage`/`ValidationResult`/etc.) rather than inventing a new persistence format: add `EventUserQuestionRequired` to `isFlowSidecarEventType`'s allowlist so the raw "asked" event — already flowing through `emitLocked → persistEvent → store.AppendEvent` on every ask — is now also written to the per-run `<runID>-flow-events.ndjson` sidecar, and reloaded automatically by the already-wired `LoadFlowEvents` call in `reconstructRun`.
- `F-2` Add durable persistence for `ProviderQuestionState` on `localFileSessionStore`, mirroring `sessions.ndjson` (BUG-080) — a new `questions.ndjson`, last-write-wins per `QuestionID`, loaded into the existing in-memory `fakeWorkflowStore.questions` map at startup. `AnswerQuestion`/`expireQuestion`/`clearPendingQuestion` already call `persistQuestion` on every transition; no changes were needed to those call sites.
- `F-3` Add a new optional interface `QuestionHistoryReader.ListQuestionsByRun`, following the exact same "optional extension of `InteractiveStateStore`" pattern as `SessionHistoryReader`/`FlowEventStore` — so `SupabaseWorkflowStore` (which does not implement it, matching the existing precedent that it also does not implement `FlowEventStore`) is unaffected and the base interface's blast radius stays zero.
- `F-4` In `reconstructRun`, after the sidecar reload populates `rs.events`, merge in the persisted question states: a `resolved` question gets its `Answer` stamped onto the restored event (same read-only render CA-271 established); an `expired` question's event is dropped entirely (no answer to show, and replaying it interactive would let the user submit into an already-expired question and hit the existing `question_expired` 409); a question with no persisted state at all (still genuinely `pending` — e.g. the process crashed mid-question) is left as-is, unchanged from today's behavior.

### Constraints

- Must not change `SupabaseWorkflowStore`'s behavior — it already durably persists `ProviderQuestionState`/events via real DB rows but, matching the existing `FlowEventStore` asymmetry, does not yet implement question read-back on resume. Out of scope for this fix (same boundary the CP-41 flow-events sidecar already draws).
- Must not touch `AnswerQuestion`, `expireQuestion`, or `clearPendingQuestion` — all three already call `persistQuestion` on every transition; the gap was purely on the read/durability side.
- Must not regress any of the many prior resume/status fixes in this exact area (BUG-060, BUG-074, BUG-080, BUG-118, BUG-121, BUG-157, BUG-174, BUG-233, BUG-242, BUG-248) — verified via full-suite comparison, not just the new tests.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/local_file_session_store.go` (`isFlowSidecarEventType` allowlist; new `questionsFilePath`/`loadQuestionsFromDisk`/`UpsertQuestion` override/`ListQuestionsByRun`).
- `apps/local-runner/internal/runner/workflow_store.go` (`QuestionHistoryReader` interface; `fakeWorkflowStore.ListQuestionsByRun`).
- `apps/local-runner/internal/runner/interactive_resume.go` (`reconstructRun`'s new question-state merge block, right after the existing `LoadFlowEvents` restore).
- Live re-test (2026-07-10): user reported "back lại đã show card ở dạng answered, NHƯNG nếu restart server là mất" — confirming CA-271's reconnect-while-alive fix worked, but a full restart lost the question entirely.

## 1. Issue Summary

A Google Drive workflow question that had already been answered or skipped disappeared entirely from the run's transcript after the desktop app / runner process was restarted — worse than CA-271's pre-fix symptom (which at least re-showed the form, just incorrectly interactive).

## 2. Parent Links

- impacted coding plan: `CP-44` (§11.5/§11.6 MCP question live-testing is where this was found)
- impacted tech design: none identified
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: any workflow-driven question (`AskWorkflowQuestion`, the Google Drive `mcp.driver` question path) that is answered or skipped, followed by a **full process restart** (not merely switching chat views while the runner keeps running — that is CA-271's already-fixed case).
- reproduction steps:
  1. Run a flow that asks a Drive workflow question; answer or skip it.
  2. Restart the local-runner process (or the desktop app, which restarts it).
  3. Reopen the same chat.
- frequency: deterministic — every full restart, for every previously-answered question, on the local-file (non-Supabase) backend.

## 4. Expected vs Actual

- expected: the question card still appears in the transcript, read-only, showing the recorded answer — the same rendering CA-271 established for the reconnect-while-alive case.
- actual: the question was completely absent from the reconstructed timeline after restart.

## 5. Impact

- users affected: anyone using workflow-driven (Drive) questions on the local-file backend (the default, non-Supabase desktop setup) across a runner/app restart.
- workflows affected: CP-44 §11.5/§11.6 Drive-context question flow; any future workflow-driven `ask_user` question.
- severity: medium — no data loss (the answer was always durably recorded via `UpsertQuestion`/DB row for Supabase; this was a read-back gap on the local-file path), but confusing/incomplete transcript history.

## 6. Root Cause

- hypothesis: `reconstructRun`'s restart-time timeline rebuild (`seedTranscriptFromDisk`) only knows how to replay the underlying provider's own transcript — it was never designed to know about FlowPilot's own synthetic gate events.
- confirmed cause: `transcript_loader.go`'s `loadClaudeTranscriptEvents`/`loadCodexTranscriptEvents` have zero references to `EventUserQuestionRequired` or any question concept — confirmed via direct source inspection. Separately, `localFileSessionStore.UpsertQuestion`/`AppendEvent` (for the `EventUserQuestionRequired` type) were, before this fix, RAM-only in the embedded `fakeWorkflowStore` — never write-through to disk — so even a from-scratch read-back attempt would have found nothing durable to read.
- evidence: see Source Refs; also empirically reproduced in `TestReconstructRunStampsAnswerOnRestoredQuestionEventAfterFullRestart`, which failed before this fix (question absent from `rs.events` after `resumeRun`) and passes after.

## 7. Fix Strategy

- `F-1` `isFlowSidecarEventType` (`local_file_session_store.go`) now includes `EventUserQuestionRequired`, so the raw ask event auto-persists to the existing per-run flow-events sidecar and auto-reloads via the existing `LoadFlowEvents` call already wired into `reconstructRun`.
- `F-2` `localFileSessionStore` gained `questionsFilePath()`, `loadQuestionsFromDisk()` (called unconditionally at the top of `loadFromDisk()`, before the sessions.ndjson-missing early return — a fresh data dir with a question but no session yet must still load it), an `UpsertQuestion` override that write-throughs to `questions.ndjson` (mirrors `UpsertProviderSession`/BUG-080), and `ListQuestionsByRun` (delegates to the now-populated in-memory map).
- `F-3` New `QuestionHistoryReader` interface (`workflow_store.go`) + `fakeWorkflowStore.ListQuestionsByRun`, mirroring `SessionHistoryReader`/`FlowEventStore`'s existing optional-extension pattern — zero blast radius on `WorkflowStore`'s base interface or `SupabaseWorkflowStore`.
- `F-4` `reconstructRun` (`interactive_resume.go`), immediately after the existing `LoadFlowEvents` restore block: type-asserts `QuestionHistoryReader`, lists persisted question states for the run, and rewrites `rs.events` — stamping `Answer` on `resolved` questions, dropping `expired` ones, leaving anything with no persisted state untouched.

## 8. Validation

- `V-1` **New unit tests** — done:
  - `TestReconstructRunStampsAnswerOnRestoredQuestionEventAfterFullRestart` (`interactive_service_test.go`) — end-to-end: seeds a session + raw question event + resolved `ProviderQuestionState` via a `localFileSessionStore`, constructs a *brand-new* `InteractiveService` against the same directory (simulating a restart), calls `resumeRun`, asserts the restored event carries `Answer == ["__skip__"]`.
  - `TestReconstructRunDropsExpiredQuestionEventAfterFullRestart` — same shape, `Status: "expired"`, asserts the event is absent after restart.
  - `TestLocalFileSessionStoreQuestionsSurviveRestart` — storage-layer unit test: two `UpsertQuestion` calls (resolved + expired) on one store instance, a second instance on the same directory reads both back correctly via `ListQuestionsByRun`.
- `V-2` **Load-ordering bug caught by the tests themselves** — `loadQuestionsFromDisk` was initially called at the *end* of `loadFromDisk`, after an early `return` that fires whenever `sessions.ndjson` doesn't exist yet — meaning a fresh install with a question but no session yet would silently skip loading questions. `TestLocalFileSessionStoreQuestionsSurviveRestart` caught this immediately (0 states instead of 2); fixed by calling `loadQuestionsFromDisk()` unconditionally at the top of `loadFromDisk()`, before the early return.
- `V-3` **Full suite** — `go build ./...`, `go vet ./internal/runner/` clean; `go test ./internal/runner/ -count=1` — 1243 passed, 15 failed, all 15 the same pre-existing environment-dependent flakes already tracked across this session (Codex CLI resume, account-home detection, skills-merge, auth-workspace) — zero new failures.
- `V-4` **Existing resume/status regression tests re-confirmed passing**, matching every prior fix's own guard in this area: BUG-060 (`TestProjectRunHistory*`), BUG-074/BUG-157 (approval/question staleness in `timelineReducer.test.ts`), BUG-118 (`updatedAt` preservation), BUG-174/BUG-233/BUG-242 (step-timeline-before-emit ordering), BUG-248 (parent/child cancelled-on-resume).
- `V-5` **Live re-verification** — done 2026-07-10: before restarting, inspected the on-disk data directly for the reproduction run — `run-6098-flow-events.ndjson` had the raw `user_question_required` event (seq 3, `questionId: q-6103`), and the newly-added `questions.ndjson` had the resolved record (`Status: resolved`, `Choice: ["file:1bKK97T-GMd_PkMXb7Iwjk3JdV-WdGEB063c_e6ceE_M"]`) matching the desktop app's own "answer:" display before restart. Killed the 3 stale pre-fix `flowpilot.exe` processes so the user's next launch used the rebuilt binary. User restarted the runner and reopened the same chat: confirmed via direct feedback ("à ok có show r") that the question card now renders read-only/answered after a full restart, not just after an SSE reconnect (CA-271's already-covered case).

## 9. Regression Guard

- tests: `interactive_service_test.go` (+3 new tests, one of which caught a real bug in this very fix before it shipped).
- alerts: none.
- audit checks: this note is the closing record; no separate `change-audit/CA-*` note filed since CA-271/CA-272 already cover this same live-test session's other two findings and this BugFix document itself carries the full change record per SS-13.

## 10. Follow-Up Document Updates

- upstream docs: none required — CP-44 §11.5/§11.6's live-testing notes already reference the reconnect-while-alive fix (CA-271); a restart-survival mention can be added there in a future pass but is not required for this bug's closure.
- notes left unchanged on purpose: `SupabaseWorkflowStore` was deliberately not extended to implement `QuestionHistoryReader`/`FlowEventStore` — matches the existing, already-accepted asymmetry that CP-41 flow events also don't survive restart on the Supabase backend today.
