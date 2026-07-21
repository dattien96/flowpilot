# CA-382 — Chat follow-up allowed after flow loop done

## Summary

Fixed BUG-302: any run (Chat mode **or** Workflow mode) whose flow finished
successfully (`loop_state.Status == "done"`) rejected every subsequent message
with `409 flow_stopped`, identical to a genuinely `Stop`-ped run —
contradicting CP-36's own design (the flow stops acting; the chat does not)
and confirmed live on run-18997 (CP-51 A10). The desktop's composer
(`ChatInput`) is a single component rendered unconditionally for every run
regardless of mode, with no distinct "closed" state either way, so the fix
applies uniformly rather than special-casing chat (an initial chat-only scope
was corrected after user review — see BUG-302 §Key Decisions `V-1`).

Two changes, both required together (a partial fix trades one bug for
another):

1. `startTurn`'s root-run admission gate now only seals a run on `"stopped"`
   — `"done"` never seals a run anymore, for any `run_kind`. The
   flow-launch-from-chat-flowRef path was already independently gated to
   `turnCount == 0`, so this cannot reopen a blind flow-relaunch risk.
2. `runTurn` now captures whether the loop was already `done` **before** this
   turn started, and folds that into `offerReviewOutcomeTool`'s computation —
   a follow-up turn beginning after the flow already settled never
   offers/requires `submit_review_outcome` and never trips BUG-226's "hub
   completed without calling submit_review_outcome → escalate" fallback
   (which is itself gated on `offerReviewOutcomeTool`).

## Cross-provider parity

Classification: **Case 1, provider-agnostic** — confirmed by reading both
touched sites, not assumed. The admission-gate check reads
`agentOrchestrator.loopStateFor(...).Status`; the `offerReviewOutcomeTool`
computation reads `rs.autoOrchestrate`, `rs.parentRunID`, `rs.turnCount`, and
the same loop status. Neither takes nor branches on `providerKey`. Per the
skill's Case-1 allowance, one representative-provider test (Codex) is
sufficient; this is stated explicitly rather than left implicit.

## additive-tests-only compliance

New file only: `apps/local-runner/internal/runner/bug302_chat_followup_after_flow_done_test.go`
(3 tests). No pre-existing test file was edited, renamed, or had an assertion
changed. (The file's own tests were revised in place mid-session as the fix's
scope was corrected from chat-only to chat+workflow — this is not covered by
additive-tests-only, since that rule protects the legacy suite that predates
this change, not this same change's own not-yet-committed tests.)

## Verification

- New tests confirmed via git-stash to fail against the unfixed code
  (`TestChatFollowUpAllowedAfterFlowLoopDone`, `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone`)
  and pass with the fix; the regression-guard test
  (`TestChatRunStillRejectsNewTurnWhenStopped`) passes identically either way,
  proving the carve-out is scoped correctly (only `"stopped"` still seals) and
  nothing else was weakened.
- Full-suite baseline captured **before** any change: `go test
  ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1
  -timeout 10m` → 2199 passed / 20 failed. Each of the 20 individually
  investigated and classified (via a dedicated research pass, not assumed):
  - 16 environment-dependent (missing `codex`/`git` binaries; Windows'
    `USERPROFILE`/`HOMEDRIVE`+`HOMEPATH` fallback chain resolving to this
    machine's real profile instead of a sandboxed temp dir; POSIX-shell
    process mocking that doesn't work the same way on native Windows;
    NTFS having no meaningful executable-bit; 8.3 short-path artifacts) —
    would plausibly pass in CI or on a clean machine.
  - 1 stale assertion: `TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`
    hardcodes `len(statuses) != 3`, predating Grok's addition as a 4th
    supported provider — not environment-dependent, would fail identically
    anywhere; needs the assertion updated to 4 (flagged for the user, not
    edited here per additive-tests-only).
  - 1 real, unrelated bug: `TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis`
    — `mergeTurnLogAssistantsIntoTranscript` only falls back to text-based
    dedup when the incoming entry's `TurnID == ""`; here a turn carries both a
    `TurnID` and text matching a historical event with no `ProviderTurnID`,
    so neither the ID-based nor the text-based dedup path matches, and a
    duplicate message is inserted. Pure in-memory logic, no OS/network
    dependency — genuinely unrelated to this fix, flagged separately.
  - None of the 20 touch this fix's code paths (`startTurn`'s loop-status
    gate, `offerReviewOutcomeTool`).
  After the fix + 3 new tests, across repeated runs: 2201–2203 passed,
  19–21 failed, always the baseline set (or a subset of it) plus already-known
  flakes. Two tests surfaced as occasional EXTRA failures in different runs —
  `TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID` (Windows
  temp-dir-cleanup race) and `TestNonCohortPreflightStartTurnFailReinvokesHub`
  (a genuine pre-existing async race: a synchronous `pendingAgentContext` read
  racing a `go`-spawned reinvoke goroutine that can drain it first) — both
  independently run 8× in isolation against **both** the fixed and unfixed
  code: Grok's temp-dir test failed at the same rate (consistently) either
  way; NonCohortPreflight failed at a statistically indistinguishable rate
  (5/8 pass unfixed, 6/8 pass fixed) either way. Confirmed pre-existing
  flakiness, not caused by this change.
- Targeted 93-test gate/flow-control/reinvoke/settle battery
  (`TestCohort*`, `TestApplyFlowControl*`, `TestResumePendingFlowGate*`,
  `TestChildGate*`, `TestBug288*`, `TestBug298*`, `TestRun1264*`,
  `TestRun1618*`, `TestRun9437*`, `TestRun2383*`, `TestGateSettle*`,
  `TestFinishTurn*`, `TestStartTurn*`, `TestEscalate*`, `TestRun2047*`,
  `TestRun5296*`) — all pass unchanged.
- `go build ./...` passes.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-302
change_type: bugfix
summary: Let any run (chat or workflow) keep accepting follow-up messages after its flow loop reaches done, without reopening review-decision machinery for those follow-ups, while a genuinely stopped/blocked run's seal stays unchanged.
# --->8---
