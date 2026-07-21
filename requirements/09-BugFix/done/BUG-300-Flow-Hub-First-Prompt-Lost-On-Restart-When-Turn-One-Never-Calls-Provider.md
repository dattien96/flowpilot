# BUG-300 — Flow-hub first prompt lost on restart when turn 1 never calls the provider

## Metadata

- Document ID: `BUG-300`
- Title: Flow-hub first prompt lost on restart when turn 1 never calls the provider
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: CP-51 (durable replay contract), CA-379 (run-20332 history replay parity)
- Child Documents: none
- Related Documents: BUG-293 (joined-note/live-replay symmetry), BUG-ApprovalReplay-Restart, run-17987, run-18371
- Replaces: none
- Tags: chat-history, restart, replay, claude, codex, regression

## AI Quick View

### Summary

- After a full server restart, a "Review Loop" chat run's very first user message ("fix bug 1+1 != 2") disappeared from the reopened transcript — everything else (agent cards, synthesis text, ordering) restored correctly.
- The hub's turn 1 spawns its entry child (`coder`) directly and never itself makes a real Claude/Codex call — the hub's own first genuine provider turn is the round-0 synthesis turn, after the children join.
- `overlayRawTurnPrompts` (used to stitch the durable raw prompt log back onto the replayed provider transcript) had no `turn_started` slot to overlay the raw prompt onto, since no such event existed in the persisted session file for turn 1.
- Grok's restore path (`seedGrokTranscriptFromDisk`) already guards against exactly this with a `prependMissingPromptOnlyEvents` safety net; the shared Claude/Codex restore path (`seedTranscriptFromDisk`) never had the equivalent call.

### Current Ask

- Restore the original first prompt into the replayed transcript for Claude/Codex flow-hub runs whose turn 1 never produced its own provider `turn_started` event, without touching agent-card ordering or any other restore behavior.

### Key Decisions

- `V-1` Mirror the existing, already-shipped Grok fix (`prependMissingPromptOnlyEvents` immediately after `overlayRawTurnPrompts`) into the shared Claude/Codex path, rather than inventing a new mechanism.

### Constraints

- Must not regress the existing restart/resume/replay test battery (`TestRun1264*`, `TestRun2334*`, `TestRun5695*`, `TestRun20332*`) or agent-lifecycle/sidecar-anchoring ordering.

### Open Questions

- None.

### Source Refs

- run-17987, run-18371 (`.flowpilot/chats/run-17987-turns.ndjson`, `.flowpilot/chats/run-18371-turns.ndjson`, `.flowpilot/chats/sessions.ndjson`)
- Real Claude session file confirming the root cause: `~/.claude/projects/D--working-gate-sandbox/716a901c-158a-4603-a3dc-314a3f66814c.jsonl` (its first line is already the round-0 synthesis turn — no "fix bug 1+1 != 2" user frame exists anywhere in the file)
- `apps/local-runner/internal/runner/interactive_resume.go`

## 1. Issue Summary

A user ran a "Review Loop" built-in-orchestration chat with prompt "fix bug 1+1 != 2". Before a server restart, the live transcript correctly showed the prompt bubble, the spawned `coder`/`reviewer_correctness`/`reviewer_security` cards, and the final synthesis text. After restarting the local-runner and reopening the same run, every part of the transcript reappeared identically **except** the original prompt bubble — the transcript now started directly at the `coder` agent card.

## 2. Parent Links

- impacted coding plan: CP-51 durable replay contract
- impacted tech design: n/a
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: local dev, Claude provider, `chat` run kind, "Review Loop" built-in orchestration launched from Chat mode
- reproduction steps:
  1. Start a chat with a "Review Loop"-style flow where turn 1 spawns an entry child directly (autoOrchestrate path).
  2. Let it run to completion (children join, hub synthesizes, run completes).
  3. Restart the local-runner process.
  4. Reopen the run from history.
- frequency: deterministic for any flow-hub run whose entry node is spawned on turn 1 (the hub's own first real provider call is therefore the round-0 synthesis turn, not turn 1)

## 4. Expected vs Actual

- expected: reopening after restart shows the exact same transcript as before restart, including the original first prompt.
- actual: the first prompt bubble is silently missing; everything after it (agent cards, synthesis) is intact.

## 5. Impact

- users affected: anyone reopening a flow-hub chat run after a runner/server restart
- workflows affected: any built-in-orchestration / flow-hub chat run on Claude or Codex whose entry child is spawned on turn 1
- severity: medium (cosmetic/history-integrity loss, not a functional runtime failure — the run itself already completed correctly)

## 6. Root Cause

- hypothesis: the restart replay path drops the first prompt because it can't be matched back onto the persisted provider transcript.
- confirmed cause: `seedTranscriptFromDisk` (`apps/local-runner/internal/runner/interactive_resume.go:2641`) loads the real Claude/Codex session file and calls `overlayRawTurnPrompts` to replace each replayed `turn_started`'s text with the durable raw prompt recorded in the turn log. When the hub's turn 1 never issues its own provider call — because it spawns its entry child directly instead — the persisted session file has no `turn_started` event at all for turn 1; its first frame is already the round-0 synthesis turn (itself correctly system-flagged and skipped by `isSystemPrompt`). With no slot to overlay onto, `rawPrompts[0]` (the original "fix bug 1+1 != 2") is never consumed, and `userFacingTranscriptEvents` has nothing left to render for it — it is silently dropped.
- evidence:
  - `.flowpilot/chats/run-17987-turns.ndjson` / `run-18371-turns.ndjson`: the durable turn log correctly records `turn-17989`/`turn-18373` with the clean prompt `"fix bug 1+1 != 2"`.
  - The real Claude session file for the hub's own synthetic thread (`716a901c-158a-4603-a3dc-314a3f66814c.jsonl` in `~/.claude/projects/D--working-gate-sandbox/`) starts its very first line directly with the `[FlowPilot system note — sub-agents started ...]` / `[flow-engine joined result note]` synthesis turn — confirming no turn-1 user frame was ever written for this session.
  - `seedGrokTranscriptFromDisk` (`interactive_resume.go:3433`) already has a call to `prependMissingPromptOnlyEvents` immediately after its own `overlayRawGrokTurnPrompts` for exactly this reason; the shared Claude/Codex `seedTranscriptFromDisk` had no equivalent call.

## 7. Fix Strategy

- `F-1` Added `historical = prependMissingPromptOnlyEvents(turnLogPromptTexts(rawPrompts), historical)` in `seedTranscriptFromDisk`, immediately after `overlayRawTurnPrompts` and before `mergeTurnLogAssistantsIntoTranscript` — the same relative position already used by the Grok restore path. Any raw prompt from the turn log that isn't already present as a `turn_started` prompt anywhere in the replayed transcript is now restored as its own synthetic prompt-only turn, instead of being silently dropped.

## 8. Validation

- `V-1` New regression test `TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne` (`apps/local-runner/internal/runner/restart_hub_first_turn_no_provider_call_test.go`) reproduces the real scenario: a Claude session file whose only `user` frame is the round-0 synthesis turn, with the turn log independently recording the original raw prompt. Confirmed the test fails without `F-1` (git-stash verified) and passes with it.
- `V-2` Full restart/resume/replay regression battery (`TestRun1264*`, `TestRun2334*`, `TestRun5695*`, `TestRun20332*`, plus all `Restart|Resume|Replay|Transcript` tests): 191 passed both before and after `F-1`; the 8 pre-existing failures (Codex CLI-binary-dependent tests, plus one unrelated `TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis` failure) reproduce identically with or without this change — confirmed environment/pre-existing, not caused by `F-1`.
- `go build ./...` passes.

## 9. Regression Guard

- tests: `TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne` guards this specific path going forward.
- alerts: n/a
- audit checks: none beyond the existing restart/replay battery.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: agent-card ordering (`appendResumedParentAnnotations`), sidecar anchoring, and all other restore logic were left untouched per explicit scope — this fix only restores the missing prompt-only fallback already present for Grok.
