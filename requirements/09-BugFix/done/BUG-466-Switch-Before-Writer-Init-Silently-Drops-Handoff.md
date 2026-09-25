# BUG-466: Provider switch before lazy transcript-writer init silently drops handoff context

- status: done
- found: live cht_10a27db90766 / leg run-49071 (B-59-1 switch-back drill, codex→devin)
- fixed_by: CA-963
- tests: internal/runner/bug466_switch_lazy_writer_test.go

## Symptom (live)

After a runner restart, the first switch-provider call on a restored chat
returned `handoffMode: fresh_start`, `includedTurnCount: 0` — despite 12
transcript records in the store. Consequences:

- `env.Prompt` empty → no seed turn fired → the new devin leg (run-49071)
  started with **zero conversation context** (would have been `raw` with
  the full `<previous_conversation>` envelope).
- The E-9 `chat_provider_switch` record durably logged
  `handoffMode=fresh_start / includedTurnCount=0` — wrong stats, not a
  transient view artifact.
- No error, no warning — the operator saw a "successful" switch that
  silently discarded all context. Exactly the "never silently lose"
  violation class.

## Root cause

The transcript stack is lazily built by `ensureChatTranscriptWriter()`
behind `chatOnce`. `createRun` calls it for chat runs, so legs *created*
in-process always have it — but legs **restored** from sessions.ndjson
after a restart never pass through createRun. `switchChatProvider`
nil-checked `s.chatTranscripts` (via `buildChatHandoffContext`) without
initializing it first, so a switch that was the first transcript-touching
call on a fresh process hit `chatTranscripts == nil` → `fresh_start`.

Heal paths (`appendChatSwitchRecordOnce`, `hasSwitchRecord`,
`markSwitchSeedFailed`) also nil-check the writer — same exposure.

## Fix

`switchChatTranscriptWriter` is now initialized at the top of
`switchChatProvider` (`s.ensureChatTranscriptWriter()` — idempotent via
`sync.Once`), before Phase A so heal paths can append as well.

## Provider parity

Provider-agnostic: the bug is in the shared switch path — any
source/target pair on a restarted runner reproduced it. Live repro was
codex→devin; claude/grok legs hit the identical code path.

## Notes

- Pre-fix E-9 record for run-49071 remains in the live bed with the
  wrong stats — durable record, left as evidence.
- Regression window: only switches that precede any other
  transcript-touching call (turn, timeline, sync) on a given process.
