# CA-395: Flow-done follow-up no longer bounces on 409 turn_in_progress (run-24345)

## Summary

Live on `run-24345` (Grok Review Loop, Flow mode). The moment the flow *looked*
done — every agent chip green, hub "submitting the consolidated result" — typing a
follow-up ("done rồi hả") and pressing Send returned a raw
`a turn is already in flight for this session (HTTP 409 / turn_in_progress)` error
and **dropped the typed message**, forcing a re-type.

## Root cause

A desync between the desktop's run status and the runner's per-session turn guard:

1. The hub marks its loop `done` from **inside** its own final turn (after
   `submit_review_outcome`), while that turn's provider stream is still open — so the
   runner still holds `turnInFlight = true`.
2. On the desktop, `deriveOrchestrationRunStatus` treats `loopState.status === "done"`
   as authoritative and flips the run to `"completed"`
   ([store.ts:2655](../apps/desktop-flowpilot/src/state/store.ts)). That clears
   `blocked`, so `canSend` goes true and the composer unblocks
   ([ChatInput.tsx:578](../apps/desktop-flowpilot/src/components/ChatInput.tsx),
   [:596](../apps/desktop-flowpilot/src/components/ChatInput.tsx)).
3. The follow-up `POST /turns` hits the hard one-turn-per-session guard
   `if rs.turnInFlight` ([interactive_service.go:6867](../apps/local-runner/internal/runner/interactive_service.go))
   → `409 turn_in_progress`.
4. `sendPrompt`'s catch surfaced the raw `RunnerApiError` and dropped the message
   ([store.ts:1247](../apps/desktop-flowpilot/src/state/store.ts)).

The BUG-302/305/307/308 `turnStartedAfterLoopDone` admission relaxes the
loop-*status* and post-turn-*gate* guards for a plain-chat follow-up, but it is set
**after** the `turnInFlight` guard, so it never covered the "loop done but hub turn
still streaming" window. Same bug family, different (turnInFlight) dimension.

## Fix

Desktop-only. In `sendPrompt`, wrap the send+consume in a bounded retry (6 × 700ms)
for the three **pre-mint** transient rejections — `turn_in_progress`,
`gate_in_progress` (post-turn gate settling), `hub_parked` (children still active).
The optimistic prompt + thinking bubbles are kept ("Waiting for the current step to
finish…") and the turn is auto-resent once the run goes idle, instead of dropping
the message with a raw error. On a non-transient error, an exhausted window, or a
run switch, it falls through to the existing error path unchanged.

Safe because all three codes are returned **before** a turn is minted — the guards
at `interactive_service.go` 6836 (`hub_parked`) / 6847 (`gate_in_progress`) /
6867 (`turn_in_progress`) all sit above `turnID := s.nextID("turn")` at 6926, and
`sendTurn` rejects on the `POST /turns` call before opening any stream
([HttpWsRunnerClient.ts:357](../apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts)).
So re-POSTing can never duplicate a turn.

## Cross-provider parity

Classification: **provider-agnostic.** The transient codes are runner-level
(session turn/gate/park guards, identical for Claude/Codex/Grok) and the fix lives
in the shared desktop composer path (`sendPrompt`). No per-provider branch touched.
The live repro was Grok; the code path is the same for all three.

## additive-tests-only compliance

One new test added to `store.test.ts`
("sendPrompt retries a transient turn_in_progress instead of dropping the message");
no existing test modified.

## Verification

- **Impact (gitnexus CLI 1.6.2, strict CLAUDE.md gate):**
  `gitnexus impact sendPrompt --repo flowpilot --direction upstream` → **risk LOW**,
  1 direct caller (`confirmProviderSwitch`), 0 processes affected, 1 module (State).
  Index up-to-date (commit d07bd21).
- **Change scope:** only `store.ts` (+35/−1) and `store.test.ts` (+41); **0 Go files**
  (`git diff --numstat -- '*.go'` empty) → runner and all three providers byte-identical.
- `npm run typecheck` (tsc --noEmit): clean.
- **No-regression, harness-independent (same esbuild+node --test bundle, same
  localStorage shim, baseline via git-stash vs fix):**
  baseline **84 pass / 4 fail / 88**; fix **85 pass / 4 fail / 89** — delta exactly
  **+1 test / +1 pass / +0 fail**. The 4 failures are the SAME tests on both
  (`openHistoryRun…`, `sendPrompt aborts an open-ended history replay stream`,
  `workflow handoff turn settles`, `stop uses loop stop…`) — ad-hoc harness lacks
  the jsdom/vite-env/fake-timers of the real runner, unrelated to this change.
  Note `sendPrompt aborts …replay stream` fails identically on baseline AND fix,
  confirming the retry loop does not perturb the send path it lives in.
- New test **passes** (`sendPrompt retries a transient turn_in_progress instead of
  dropping the message`).
- **Logic:** the retry alters behavior ONLY for the 3 pre-mint transient codes;
  success, non-transient error, and run-switch paths are byte-identical to before.
  Bounded (6×700ms) so it cannot hang; pre-mint guarantee means no duplicate turn;
  optimistic bubbles are added once (before the loop) so no duplicate prompt.

**Residual (not in scope):** the retry window is ~4.2s. If a follow-up still 409s
after that, it falls to the existing error path — which would indicate a separate
*backend* `turnInFlight` leak on the hub finalize path (reinvoke/synthesis), to be
diagnosed independently.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: run-24345
change_type: bugfix
summary: Retry pre-mint transient turn rejections (turn_in_progress/gate_in_progress/hub_parked) in the desktop composer so a follow-up sent as a flow completes is delivered instead of dropped with a raw 409 (run-24345)
# --->8---
