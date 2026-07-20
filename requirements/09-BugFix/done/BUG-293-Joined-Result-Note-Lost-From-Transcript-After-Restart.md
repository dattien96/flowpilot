# BUG-293: Joined Result Note Lost From Transcript After Restart

## Metadata

- Document ID: `BUG-293`
- Title: `Joined result note bubble shown live but dropped on replay after restart`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `agent-flow-engine`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- Child Documents: `-`
- Related Documents: `CA-371-live-turnstarted-hides-system-prompts`, `BUG-272` (restart replay class)
- Replaces: `-`
- Tags: `agent-flow-engine, chat-ui, transcript-replay, restart, regression, A1, severity-medium`

## AI Quick View

### Summary

- During a review-loop run (run-11948), the live transcript showed the `[flow-engine joined result note] … you must call submit_review_outcome` synthesis prompt as a chat bubble.
- After a server restart + reopen, that exact bubble disappeared — the user observed "one message lost" between agent cards and the first tool-call group.
- No data loss on disk: the note is still in the Claude session file and `run-11948-turns.ndjson`. The two render paths simply disagreed.
- Live emitted the prompt on `turn_started`; replay filtered it out as an internal system prompt.

### Current Ask

- Make live and replay render the joined result note (and all internal flow-engine/system prompts) identically. Per product decision: **hide in both** — align live with the already-correct replay behaviour.

### Key Decisions

- `V-1` A live `turn_started` for an `isSystemPrompt` prompt MUST NOT render a user bubble; its display prompt is redacted to empty. The provider still receives the full prompt.
- `V-2` Live and replay MUST classify every prompt identically: `liveTurnStartedDisplayPrompt(p) == ""` iff `isInternalTranscriptEvent(turn_started{p})`.

### Constraints

- Redaction applies only to the DISPLAY copy on the emitted event. `in.Prompt` still flows to `runTurn` (provider call) and the turn log verbatim — no behavioural change to the LLM turn.
- Additive-tests-only: no existing test edited; one new dedicated test file added.

### Open Questions

- None. Direction confirmed by user: hide consistently (match existing `isInternalTranscriptEvent` intent).

### Source Refs

- run-11948 (project db51ec26-1a0f-4b92-8ceb-b03dc8e9b363), screenshots Image 1–3.
- `run-11948-turns.ndjson` line 2 (joined note persisted).

## 1. Issue Summary

In a review-loop chat, after the reviewer cohort joins, the hub is auto-reinvoked with a synthesis prompt that embeds the `[flow-engine joined result note]`. Live, that prompt appeared as a user bubble in the transcript. After restarting the runner and reopening the chat, the bubble was gone while every other message (assistant synthesis text, tool-call groups, final approval message) survived. The user reported exactly one lost message.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- impacted tech design: agent-flow-engine review-loop / transcript replay
- impacted system spec: chat transcript render contract (live SSE vs resume snapshot)

## 3. Environment and Reproduction

- environment: local runner `127.0.0.1:4318`, desktop FlowPilot, Claude provider (claude-sonnet main + reviewers), Review Loop built-in orchestration.
- reproduction steps:
  1. Run a review-loop task (e.g. "fix bug 1+1 != 2") until the reviewer cohort joins and the hub synthesises + submits the review outcome.
  2. Observe the live transcript shows the `[flow-engine joined result note]` bubble between the agent cards and the first tool-call group.
  3. Restart the runner; reopen the same chat from history.
  4. Observe the joined-note bubble is now absent.
- frequency: deterministic for any review-loop run whose hub reinvoke carries a joined note.

## 4. Expected vs Actual

- expected: live and replay show the same transcript. The internal joined-result-note prompt is an orchestration message and should be hidden consistently (it is already hidden on replay by design).
- actual: live rendered the note as a user bubble; replay dropped it → apparent message loss after restart.

## 5. Impact

- users affected: anyone running review-loop / flow-engine chats who restarts the runner and reopens a chat.
- workflows affected: agent-flow-engine review loop (and any flow whose hub reinvoke embeds a system prompt).
- severity: medium — no functional/data loss, but transcript inconsistency erodes trust ("where did my message go?").

## 6. Root Cause

- hypothesis: live and replay use different filters for `turn_started` prompts.
- confirmed cause:
  - Live: `startTurn` emitted `EventTurnStarted{Prompt: in.Prompt}` at `interactive_service.go:6863` with the full prompt; the desktop reducer (`timelineReducer.ts:198`, `if (e.prompt)`) rendered it as a bubble.
  - Replay: `seedTranscriptFromDisk` → `userFacingTranscriptEvents` → `isInternalTranscriptEvent` (`interactive_resume.go:2584`) drops any `turn_started` whose prompt satisfies `isSystemPrompt` (→ `isFlowEnginePrompt`, matching `[flow-engine joined result note]`). So replay hid it.
- evidence: `run-11948-turns.ndjson` line 2 holds the note; `feature_history.go:200-208` matches the marker; the replay filter is keyed on the same predicate.

## 7. Fix Strategy

- `F-1` Add `liveTurnStartedDisplayPrompt(prompt string) string` (`interactive_resume.go`, co-located with its replay twin `isInternalTranscriptEvent`): returns `""` for `isSystemPrompt` prompts, else the prompt unchanged.
- `F-2` At the single live emit site (`interactive_service.go:6863`) pass `liveTurnStartedDisplayPrompt(in.Prompt)` as the event's display `Prompt`. `in.Prompt` still flows to `runTurn` and the turn log (`interactive_service.go:6946`) unchanged.

## 8. Validation

- `V-1` `go test ./internal/runner -run "TestBug293" -count=1` → 9 passed.
- `V-2` Blast-radius battery `go test ./internal/runner -run "TestBug293|TestRun2334|TestHubNotify|TestRun9437|TestRun1618|TestHubParked|Reprompt|Transcript|FeatureBucket|JoinedNote|Synthesis|Reinvoke|Handoff" -count=1` → 107 passed.
- `V-3` Full-package `go test ./internal/runner` failures are all pre-existing environmental cases (Codex/Gemini CLI binaries, machine-specific provider home paths, git-guard shim, Google Drive provider config, skill filesystem merges); none are in the prompt/transcript/hub area.
- Required live retest (pending): rerun a review-loop task to the joined-note synthesis; confirm the note is NOT shown as a bubble live; restart + reopen; confirm the transcript is identical (assistant synthesis text, tool-call groups, and final approval message all present, no note bubble either way).

## 9. Regression Guard

- tests: `bug293_joined_note_live_replay_symmetry_test.go` (additive) pins:
  - the joined result note is hidden live,
  - a real user prompt is preserved,
  - live and replay agree on hidden/shown across joined-note, flow-engine-ready, gate-reprompt, handoff, real-user, and continuation prompts.
- alerts: none.
- audit checks: `CA-371-live-turnstarted-hides-system-prompts` ledger block (feature_key `agent-flow-engine`).

## 10. Follow-Up Document Updates

- upstream docs that must change: append verification note to `CP-51-PhaseAB-Timeline-And-Verification-Log` after live retest.
- notes left unchanged on purpose: replay filter (`isInternalTranscriptEvent`) is already correct and intentional; only the live path was brought into alignment with it.
