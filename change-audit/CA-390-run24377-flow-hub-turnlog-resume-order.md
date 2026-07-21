# CA-390 — run-24377 flow hub resume order (shared Grok session + turn-log prose)

## Summary

Live Grok Review Loop **run-24377**: after server restart the main timeline was
scrambled — agent cards piled at the bottom, and the post-flow follow-up
("done rồi hả…") sat mid-chat. Durable hub turn log order was correct.

Root cause (not Claude≠Grok adapter logic for overlay alone):

1. **Shared provider session:** hub `run-24377` and coder `run-24382` both
   persisted `provider_session_id=019f8526-…`. Grok `chat_history.jsonl` for
   that id contains **child** turns + hub join notes + follow-up.
2. **Hub seed loaded that whole file**, so child prompts/answers polluted main
   chat; overlay/agent placement then looked like BUG-306-class misorder.
3. Stamping every hub frame with `createdAt` forced
   `reorderResumedTimelineLocked` to dump later-timestamped agent cards after
   all hub prose.

Claude often looked fine because it does not share hub/child session ids the
same way; the **resume prefer path is shared** (Case 1) so Claude/Codex hubs
with durable assistants also use the safe turn-log rebuild.

## Fix

- When `parentRunID=="" && flowEngineDriven && turn log has assistant frames`,
  rebuild hub prose from the **per-run turn log** only
  (`buildFlowHubTranscriptEventsFromTurnLog` +
  `seedFlowHubTranscriptFromTurnLog`). Wired in both
  `seedGrokTranscriptFromDisk` and Claude/Codex `seedTranscriptFromDisk`.
- Do **not** default-fill `OccurredAt` on that path so agent lifecycle stays
  cluster-before-message (run-9034) instead of pure time-sort bottom dump.
- If the hub turn log has no assistants yet, keep the provider-history path
  (run-12613 / run-20332).
- **Follow-up (same CA):** cluster placement must **clamp** insert index to after
  the first user `turn_started` so agent cards cannot stack above the original
  "fix bug …" prompt (live residual: cards moved from bottom dump to top dump).
- **Follow-up 2:** multi-round fixture with real wall-clock times still collapsed
  to one mega-cluster (all agents then all synths). Fix: synthesis anchors =
  `message_completed` **before** the first post-flow follow-up (not the last
  user prompt — a second follow-up made "ok" a fake anchor).
- **Follow-up 3 (desktop):** `orderHistoryReplayEvents` preferred **timed** events
  over **untimed**. Hub turn-log prose was untimed; agent cards had wall-clock
  starts → UI showed all agents, then "fix bug". Fix: (server) strip mixed
  OccurredAt after cluster placement so Seq wins; (desktop) if any event lacks
  finite time, sort by Seq only. New test
  `store.history-replay-mixed-timestamps.test.ts` (Claude/Codex/Grok).
- **Follow-up 4 (Image 1 + Image 2 residuals on real disk run-24377):**
  1. **Image 1** — R2 reviewers between "done rồi hả" and "ok": synthesis
     anchors used `lastUserPromptIdx`, so a second follow-up ("turn cũ…")
     still treated "**ok**" as an anchor. Fix: anchors stop at
     `firstPostFlowFollowUpIdx` (second user-facing prompt); clamp inserts
     never `>=` that index.
  2. **Image 2** — two consecutive coder cards: coder `turn_count=4` with
     even-split timestamps + `partitionFlowAgentPairsEvenly` into 4 buckets.
     Fix: cap multi-activation count to peer start-wave count (3 reviewer
     waves → 3 coder cards); park activations at wave boundaries; **remove**
     even partition. Locked by Image1/Image2 assertions in
     `TestRun24377LiveFixture*` / `TestRun24377RealDisk*`.

## Cross-provider parity

**Case 1, provider-agnostic prefer gate** (`preferFlowHubTurnLogTranscript` —
no `providerKey` branch). Live reporter Grok; Claude/Codex enter via the same
gate in `seedTranscriptFromDisk`. Unit coverage: Grok integration fixture +
pure unit tests on the builder/gate.

## additive-tests-only

New file only:
`apps/local-runner/internal/runner/run24377_flow_hub_shared_session_resume_test.go`

No pre-existing tests edited. Green: `TestRun12613*`, `TestPostFlowFollowUp*`,
`TestRun9034*`, `TestRun5695*`, `TestRun20332*`, `TestRun23820*`.

## Verification

```bash
go test ./internal/runner/ -count=1 -timeout 3m \
  -run 'TestRun24377|TestBuildFlowHub|TestPreferFlowHub|TestRun12613|TestPostFlowFollowUp|TestRun9034|TestRun5695|TestRun20332|TestRun23820'
```

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-306
change_type: bugfix
summary: Flow hubs with durable turn-log assistants rebuild main-chat prose from the per-run turn log on resume (not a Grok session file shared with children), fixing run-24377 post-restart timeline order and agent-card bottom dump while keeping provider-history fallback when assistants are absent.
# --->8---
