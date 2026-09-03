# CA-712 — Opencode replay recovery for the missing answer after a denied permission (BUG-341 root cause)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-341
change_type: bugfix
summary: root-cause BUG-341 — opencode never streams the answer after a denied permission; recover it via session/load replay so the first turn is no longer blank (421135/424302/430742/433929/437116/440305)
# --->8---

## Root cause (wire-proven, not inferred)

`cli-runner.log` + `~/Library/Application Support/FlowPilot/logs/runner.log` contain full ACP frame
logs for every failing turn. All 6 BUG-341 blanks (12:47, 15:20, 15:23, 15:44, 21:07 run-437116,
21:17 run-440305 — the last one on the CA-711 binary) share one wire shape:

```
send session/prompt
recv tool_call(read) → tool_call(bash) → session/request_permission
send {"outcome":{"optionId":"reject"}}            ← scan posture auto-deny (chat_posture_policy.go)
recv tool_call_update "The user rejected permission..."
recv usage_update
recv {"id":5,"result":{"stopReason":"end_turn","usage":{...,"outputTokens":98}}}   ← NO agent_message_chunk, ever
[opencode] post-result wait lastText_len=0 waited=8.001s
[opencode] WARN turn_completed with EMPTY finalMessage ... lastText_len=0
```

- opencode 1.18.25 completes the turn `end_turn` with outputTokens 92–423 (the model DID write
  text) but emits **zero** `agent_message_chunk` frames — not before the result, not after, not in
  any drain window. 9/9 denied turns in the log end silent; 0 exceptions.
- Counter-proof: turns whose permission is APPROVED stream the answer normally (16 chunks before the
  result, session 08/30 13:03). Turns with no permission request stream normally (08/29 14:13).
- The lost answer exists only in opencode's session store: a later `session/load` replays it
  (13:09:50 reject → `end_turn` outputTokens=423 → 13:10:11 load replays history; "second prompt
  shows first reply" symptom).
- Therefore every prior fix (CA-706 600ms → CA-707 8s → CA-708 wait-on-text-growth → CA-709 array
  mapper → CA-711 15s activity budget) waited for frames that are never sent. run-437116's "8s of
  silence then the answer" reading in CA-711 was wrong — the answer never arrived; turn-2 replay
  surfaced it.

Upstream class: opencode ACP missing-chunk emission ([PR #15614], unmerged as of 2026-09-01;
post-rejection turn abort reported from Zed [#48540]).

## What changed

- `opencode_adapter.go`:
  - `permissionDenied map[string]bool` per-session, set by `handleInbound` on every deny path
    (bridge==nil fallback, RequestApproval error, decision != approve), reset per turn in `SendTurn`,
    deleted with the other per-turn maps.
  - `SendTurn` post-result path: when a permission was denied and `lastText` is empty, the generic
    8s wait collapses to `opencodeDeniedEmptyTextWait` (1.5s grace — evidence says nothing arrives),
    then `recoverOpencodeEmptyAnswer` fires for completed turns.
  - `recoverOpencodeMissingAnswer`: issues `session/load` on the SAME session/process (BUG-329 keeps
    sessions process-local) and collects the replay.
  - `collectOpencodeReplayAnswer`: consumes replay frames locally. History is chronological and the
    just-finished prompt replays as a `user_message_chunk`; the answer is the `agent_message_chunk`
    text after the LAST user frame, so the collector resets on every user frame — old answers are
    dropped, a turn with genuinely no message part recovers `""`. Tool/thought/usage replay frames
    are NOT re-emitted to the bridge (no duplicate timeline events). Quiet window 400ms, hard cap 8s,
    load RPC timeout 4s.
  - `emitTerminal` WARN now carries `permissionDenied=%t` for future correlation.
  - The 8s literal became `opencodeEmptyTextWait` (var, zero behavior change) so tests can shorten it.

## R1 evidence

- New additive tests (`opencode_replay_recovery_test.go`, no legacy edits), all PASS:
  - `TestCollectOpencodeReplayAnswerKeepsOnlyCurrentTurn` — old answer + replay tool noise dropped,
    current answer concatenated across chunks.
  - `TestCollectOpencodeReplayAnswerNoCurrentAnswer` — no message part this turn → `""` (never an
    older turn's answer).
  - `TestOpencodeReplayRecoveryAfterDeniedPermission` — full `SendTurn` over `fakeOpencode`:
    denied permission → `end_turn` + outputTokens=98, zero chunks → exactly ONE `session/load`,
    one delta, `FinalMessage` = replayed answer, zero leaked tool events, old answer absent.
  - `TestOpencodeReplayRecoverySkippedWithoutDenial` — no denial → zero `session/load` calls.
- **Red-before proven by mutation**: short-circuiting `recoverOpencodeEmptyAnswer` →
  `TestOpencodeReplayRecoveryAfterDeniedPermission` FAILs (`session/load recovery calls = 0, want 1`);
  restored → PASS.
- Existing suites: `go test ./internal/runner -run TestOpencode -count=1` PASS (all CA-706..711
  regression locks green, no edits); TUI suite PASS.

## Provider parity

- Opencode-only adapter path: `handleInbound` and the post-result branch exist only in
  `opencodeAdapter` (SendTurn's single production caller is `sendTurnWithRetry`,
  interactive_service.go:7294, via `ProviderRuntimeAdapter` — no signature change). Claude/Grok/
  Codex adapters untouched. TUI + Desktop both benefit (same `turn_completed.FinalMessage` channel
  as CA-706..711).

## Honest gaps

- If a future opencode build replays history WITHOUT the current turn's `user_message_chunk`, the
  collector would return the previous turn's answer; live 1.18.25 always includes it (wire-proven).
- A live late `agent_message_chunk` racing the recovery window (no user-frame boundary) is dropped;
  9/9 wire captures show zero post-result frames on denied turns, so this is theoretical.
- Recovery adds one extra RPC + replay burst (~0.5–1s) on denied-blank turns; the answer now appears
  in ~2s instead of never (or at turn-2 replay).
- Upstream fix ([PR #15614]) is the real long-term cure; this recovery degrades gracefully to the
  old blank behavior if opencode changes the replay shape.

## Prior CA not undone

- CA-708/709/711 wait/mapper contracts remain for the NON-denied paths (late chunk / array shapes /
  activity budget); this CA adds a recovery layer for the denied shape those fixes cannot cover.
- CA-705 TUI `turnLive` guard remains.

[PR #15614]: https://github.com/anomalyco/opencode/pull/15614
[#48540]: https://github.com/zed-industries/zed/issues/48540
