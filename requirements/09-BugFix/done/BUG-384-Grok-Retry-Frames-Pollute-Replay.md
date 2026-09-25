# BUG-384: Grok retry frames pollute replay — phantom `turn_started` with composed prompt, failed turn replays as `turn_completed`, leak into handoff

## Metadata

- Document ID: `BUG-384`
- Title: `Each retry persists a <user_query> frame → surplus frames surface as phantom turns carrying internal composed prompt (timeline + cross-provider handoff)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-46-Grok-Build-Controlled-Adapter-Over-ACP](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- Feature Keys: `chat-history, cross-provider-handoff, ai-providers`

## AI Quick View

### Summary

- Every `SendTurn` retry attempt re-issues `session/load` + `session/prompt`; grok persists each `session/prompt` as a `<user_query>` user frame in `chat_history.jsonl` → 6 frames for 2 logical turns (3 attempts each).
- On restart replay (`seedGrokTranscriptFromDisk` → `overlayRawTurnPrompts`, `interactive_resume.go:4651`), only `len(rawPrompts)` leading `turn_started` slots get overlaid with the typed text; surplus frames survive as extra user turns holding the FULL composed prompt — including internal `ask_user`/`spawn_agent` routing instructions — stamped with the real `providerTurnId`.
- `appendTranscriptReplayEventsOpt` (`interactive_resume.go:3388`) appends a synthetic `turn_completed` whenever the seeded transcript's last event isn't terminal → a FAILED (402) turn replays as completed.
- The same phantoms enter cross-provider handoff: grok→codex envelope reports `includedTurnCount:4` for 2 real prompts and embeds internal routing instructions as `User:` turns in `<previous_conversation>`.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Note: cp46 evidence filed this as medium (user-visible phantom turns + internal-prompt leak + handoff pollution); severity recorded here as low per wave triage — revisit at prioritization.

## Bug report

- **Symptom**: After runner restart, the resumed timeline shows extra `turn_started` events containing the full composed prompt (with `---` reinforcement blocks telling the model to use FlowPilot MCP tools and avoid native pickers/`spawn_subagent`), and the failed turn is closed by a synthetic `turn_completed`.
- **Expected**: Replay shows exactly the 2 typed prompts; internal prompt scaffolding never renders as user turns; a failed turn replays as failed.
- **Actual** (cp46 run-1, grok session `01a0c620-…`): seq1 `turn_started` "Reply with exactly: ok" (typed — correct); seq2/seq3 `turn_started` full composed prompt, `providerTurnId: turn-3` (phantom retries); seq4 synthetic `turn_completed` masking the failed turn. Handoff `includedTurnCount:4`.
- **Impact**: low-medium — phantom user turns in user-visible timeline; internal prompt boilerplate leaks to users AND to other providers via handoff; failed-turn state masked on resume.

## Reproduction

1. Grok chat run; send any prompt while the account is quota-exhausted (or any deterministic prompt failure the runner classifies recoverable) — cp46 run-1 turns turn-3/turn-8.
2. Let the 3 retry attempts run; restart the runner; send a follow-up (3 more attempts).
3. Read admin events / SSE for the run: phantom composed-prompt `turn_started` events appear; provider `chat_history.jsonl` has one extra `<user_query>` frame per attempt (`l46-3-grok-chat-history.jsonl` lines 4-9).
4. `POST …/handoff-context` grok→codex → `l46-6-handoff-context-grok-source.json` shows `includedTurnCount:4` with composed prompts as `User:` turns.

## Root cause

- Retry path reloads the session and resends `session/prompt`; grok persists every prompt as a user frame — the runner has no dedup between "logical turn" and "provider prompt attempts" when replaying from `chat_history.jsonl`.
- `overlayRawTurnPrompts` (`apps/local-runner/internal/runner/interactive_resume.go:4651`) maps raw turn-log prompts onto the first N overlayable slots; surplus identical frames remain as separate `turn_started` events rather than being recognized as duplicates of the same logical turn.
- `appendTranscriptReplayEventsOpt` (`interactive_resume.go:3388`) appends a synthetic `turn_completed` whenever the seeded transcript's last event isn't terminal — for a failed turn this masks the failure state on resume (`turn_failed` is never in the provider transcript).

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-02-grok-retry-frames-pollute-replay.md`, `RESULT.md` (L-46-3, L-46-6).
- `l46-3-grok-chat-history.jsonl` (6 `<user_query>` frames for 2 typed prompts), `l46-3-run1-resume-events.sse` (seq2/3 phantom `turn_started`, seq4 synthetic `turn_completed`), `l46-6-run1-admin-events.json`, `l46-6-handoff-context-grok-source.json` (`includedTurnCount:4`), `l46-run1-turns.ndjson`, `runner.log` L49/83/114, L181/224/266 (each retry = `session/load` + `session/prompt` with fresh MCP token).

## Severity

- low

## Completion Notes (implemented 2026-09-23, CA-917b)

- Fix 1: `dedupeRetryDuplicatedTurnStarts` collapses consecutive
  identical-prompt `turn_started` events in `seedGrokTranscriptFromDisk` —
  retry re-sends of the same composed prompt no longer surface as phantom
  user turns leaking internal MCP scaffolding into timeline/handoff.
- Fix 2: `appendTranscriptReplayEventsOpt` closes a non-terminal seeded
  transcript with `turn_failed` when the run status is failed/cancelled —
  a 402 turn no longer replays as turn_completed.
- Files: `interactive_resume.go`.
- Tests: `bug384_grok_retry_replay_test.go` — 3-attempt retry frames collapse
  to the 2 durable prompts; failed run tails as turn_failed.
