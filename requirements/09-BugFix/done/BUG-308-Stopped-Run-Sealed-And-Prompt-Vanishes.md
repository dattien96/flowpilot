# BUG-308 — A Stop-ped run is sealed against follow-up (409) and the rejected prompt vanishes on restart

## Metadata

- Document ID: `BUG-308`
- Title: Make a Stop-ped flow run continuable like a "done" one, so a follow-up is admitted (and persisted) instead of 409'd and silently dropped
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: BUG-302 (opened "done", deliberately kept "stopped" sealed — this reverses that scope), BUG-305 (plain-chat carve-out flag), BUG-307 (companion — stops the stranded cohort note from poisoning the now-admitted follow-up), CP-51 A8/A11
- Child Documents: none
- Related Documents: BUG-289 R19-1 (intent-race durable-persist abort — independent of this seal), BUG-226 (escalate fallback that the offerReviewOutcomeTool scoping protects)
- Replaces: none
- Tags: agent-flow-engine, chat-history, resume, admission-gate, regression

## AI Quick View

### Summary

- Found live on run-19845 (Review Loop, `fix bug 1+1 != 2`): coder done, Stop pressed while 2 reviewers in-flight. A follow-up ("turn trước có fix gì chưa") was rejected with `409 flow_stopped` — both live AND after a server restart. Because a 409-rejected turn is never persisted, the optimistically-rendered prompt bubble VANISHED from the transcript on every restart (run-19845 images 2 and 4: 2nd and 3rd prompts gone).
- Root cause: the `startTurn` admission gate seals a `"stopped"` root loop with `409 flow_stopped` (interactive_service.go). BUG-302 had opened `"done"` for follow-ups but deliberately kept `"stopped"` sealed (its V-1 scope decision, guarded by `TestChatRunStillRejectsNewTurnWhenStopped`). The persisted `LoopState.Status="stopped"` is faithfully restored on restart (when `Mode/Cap/Round` are non-zero — the review-loop case), so the seal — and the vanish — reproduce across restarts.
- Distinct-but-adjacent: run-19500 earlier *hung* instead of 409'ing, because ITS loop status was not restored as `"stopped"` (the follow-up was admitted and then poisoned by a stranded cohort note) — that is BUG-307. The inconsistency between the two is `interactive_resume.go`'s conditional `LoopState` restore (only when `Mode/Cap/Round != 0`). BUG-308 makes the outcome consistent and correct either way: the follow-up is admitted (no 409, no vanish) AND clean (no hang, via BUG-307).

### Current Ask

- Let a genuinely Stop-ped run keep accepting normal follow-up chat, exactly like a "done" run (the desktop composer is identical for every run; a user naturally keeps typing after Stop), without reopening any flow-decision machinery and without regressing the intent-race protections.

### Key Decisions

- `V-1` **Deliberately reverse BUG-302's V-1 scope**: treat `"stopped"` the same as `"done"` in the root admission gate. This is a product decision (confirmed with the user) — Stop stops the FLOW's acting, it does not seal the CHAT. `"blocked"` still seals (a live Continue/Stop decision the user must resolve first).
- `V-2` Reuse BUG-305's `turnStartedAfterLoopDone` flag and BUG-302's `offerReviewOutcomeTool`/`loopAlready…AtTurnStart` scoping for `"stopped"` too, so a stopped follow-up is plain chat: no post-turn flow gate, no `submit_review_outcome` offer, and thus no BUG-226 escalate fallback reopening a stale "Needs your decision" card.
- `V-3` **Only the ROOT branch is relaxed.** A CHILD turn whose parent loop is `"stopped"` (or `"done"`) is still rejected — children must not run under a terminated parent flow. Pinned by `TestChildTurnStillRejectedWhenParentStopped`.
- `V-4` Safety: `stopAgentLoop` cancels children + clears durable approval/question/resume intents under `s.mu` before any follow-up admission can run (`s.mu` serializes them); the dispatch stop-fence still fences stale child sends; BUG-307 stops a stranded cohort note from poisoning the admitted turn; and the intent-race guard (BUG-289 R19-1, `TestDurableStartAbortWhenStoppedDuringPersist`) is a separate durable-persist-abort path that never depended on this loop-status seal (its test never sets loop status `"stopped"`). Verified all three still pass.

### Constraints

- additive-tests-only: one existing test is intentionally updated — `TestChatRunStillRejectsNewTurnWhenStopped` → `TestChatRunAllowsNewTurnAfterStopped` — because it guarded the exact behavior being reversed by explicit product decision (updated, not weakened). All other coverage is new.
- cross-provider-parity: the admission gate and `offerReviewOutcomeTool` scoping read only `AgentLoopState.Status`, never `providerKey` (Case 1, same as BUG-302). A Grok-adapter variant test confirms it.
- Must not regress BUG-302 ("done" still open), BUG-305, BUG-307, BUG-289 R19-1, or the child-seal.

### Open Questions

- The `interactive_resume.go` conditional `LoopState` restore (`Mode != "" || Cap > 0 || Round > 0`) does not include `Status`, so a stopped loop with all-zero Mode/Cap/Round would restore as `""` (the run-19500 shape). BUG-308 makes both outcomes correct (admitted + clean), so this is no longer user-visible, but the restore condition itself is arguably still worth tightening to always carry `Status` — flagged, not changed here.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` (`startTurn` admission gate; `runTurn`'s `loopAlreadySealedAtTurnStart` / `offerReviewOutcomeTool` / gate-skip)
- `apps/local-runner/internal/runner/interactive_resume.go:1187` (conditional LoopState restore — root of the run-19500/run-19845 inconsistency)
- `apps/local-runner/internal/runner/bug308_stopped_run_followup_allowed_test.go`, `bug302_chat_followup_after_flow_done_test.go`

## 1. Issue Summary

The admission gate rejected any new turn on a `"stopped"` root run with `409 flow_stopped`, so a user could not continue chatting after pressing Stop, and the optimistically-shown prompt (never persisted for a rejected turn) disappeared on restart.

## 2. Parent Links

- impacted coding plan: CP-51 A8 (found during), A11 (new scenario)
- impacted tech design: BUG-302 admission-gate scope; BUG-305 plain-chat carve-out
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: any; a flow run that was Stop-ped (loop status `"stopped"`), then a follow-up on the same run (live or after restart).
- reproduction (live): run-19845 — Stop with reviewers in-flight → follow-up 409 flow_stopped (both before and after restart); the rejected prompt vanished on reopen.
- reproduction (test): `go test ./internal/runner -run TestChatRunAllowsNewTurnAfterStopped` (and the BUG-308 file) — fail without the fix with `flow_stopped`.

## 4. Expected vs Actual

- expected: a Stop-ped run keeps accepting normal chat turns (like "done"); the turn is admitted and persisted so it survives restart.
- actual: `409 flow_stopped`; the un-persisted optimistic prompt vanished on restart.

## 5. Impact

- users affected: anyone continuing a chat after Stop-ping its flow — across Codex/Claude/Grok.
- workflows affected: post-Stop follow-up chat; restart transcript.
- severity: medium (blocks a natural workflow and loses the user's typed message on restart; no durable data corruption).

## 6. Root Cause

- confirmed cause: `startTurn`'s root admission gate returned `409 flow_stopped` for loop status `"stopped"` (BUG-302's deliberate carve-out excluded `"stopped"`). A rejected turn is never persisted, so the desktop's optimistic prompt bubble had nothing to restore from.
- evidence: run-19845 session file `LoopState={status:"stopped", mode:"explicit", cap:3}`; runner.log shows the two follow-ups logging `[flow-ref-resolve] bailing` with NO subsequent `[prompt]`/`[turn-params]` (rejected at admission), `events` staying at 7.

## 7. Fix Strategy

- `F-1` Admission gate: `rs.turnStartedAfterLoopDone = st == "done" || st == "stopped"`; drop the `st == "stopped" → 409` branch (keep `"blocked" → 409`). Root branch only.
- `F-2` `runTurn`: `loopAlreadySealedAtTurnStart := st == "done" || st == "stopped"` gates `offerReviewOutcomeTool` and the post-turn flow-gate skip, so a stopped follow-up is plain chat and cannot trip BUG-226.
- `F-3` Leave the child admission branch, the `"blocked"` seal, BUG-307, and BUG-289 R19-1 untouched.

## 8. Validation

- `V-1` **git-stash / temp-revert discipline:** with the admission seal restored, the four "allow" tests fail with `flow_stopped`; `TestChildTurnStillRejectedWhenParentStopped` passes in both states (child branch unchanged). With the fix, all pass.
- `V-2` **cross-provider-parity:** admission gate + offerReviewOutcomeTool read only loop status (Case 1); `TestChatFollowUpAllowedAfterFlowLoopStoppedGrok` exercises the Grok adapter.
- `V-3` **additive-tests-only:** only `TestChatRunStillRejectsNewTurnWhenStopped` (renamed `TestChatRunAllowsNewTurnAfterStopped`) is edited — the exact guard whose behavior is deliberately reversed; everything else is new.
- `V-4` **prior-fix invariants preserved:** BUG-302 (`TestChatFollowUpAllowedAfterFlowLoopDone`, `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone`), BUG-305 (`TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState`), BUG-307, and BUG-289 R19-1 (`TestDurableStartAbortWhenStoppedDuringPersist`) all green — 14-test combined battery.
- `V-5` **full-suite regression:** `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1` — 18 failures, all pre-existing environment/external-CLI only (Codex/Gemini CLI, Grok/Drive/skills home paths, git-guard shim); the specific flaky one that trips under parallel load varies (Gemini↔Grok tempdir) but each passes in isolation. No new failure tied to the admission gate or cohort logic.
- `go build ./...` and `go vet` clean.

## 9. Regression Guard

- tests: `TestChatRunAllowsNewTurnAfterStopped`, `TestChatFollowUpAllowedAfterFlowLoopStoppedResumedFromDisk`, `TestTurnStartedAfterLoopDoneFlagTrueForStoppedFollowUp`, `TestChatFollowUpAllowedAfterFlowLoopStoppedGrok`, `TestChildTurnStillRejectedWhenParentStopped`.
- alerts: n/a
- audit checks: none.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-51 A11 updated to reference this fix alongside BUG-307.
- notes left unchanged on purpose: `interactive_resume.go:1187`'s conditional LoopState restore (flagged in Open Questions); the `"blocked"` seal and child-parent-stopped seal (intentionally kept).
