# CA-918: Scaffold Dispatch Survives Client Disconnect

## Summary

Live-test investigation of the reported "AI scaffold hangs during project init"
on `test-sandbox` surfaced a real bug plus one UX gap.

**Root cause (runner).** `handleDispatchScaffold` ran the whole dispatch —
30-minute AI turn + compiler gate + heal loop — on `r.Context()`. Go's http
server cancels that context the moment the client connection dies (TUI quit,
laptop sleep, proxy idle timeout, network blip), and `exec.CommandContext` then
SIGKILLed the provider child mid-write. The user sees "hang" → silent failure
(`signal: killed`), and a partially-written workspace.

**Reproduced live (pre-fix):** POST /scaffold on `test-sandbox` with
`devin/swe-2-high`, then `kill -9` on the curl client → `devin -p` child died
within ~2s, feed terminal event `scaffold: AI scaffold turn failed: exit code
-1: signal: killed`. Reproduced twice.

**Fix.** The dispatch now runs detached — `context.WithTimeout(context.
Background(), scaffoldAPITimeout)` — identical to the create_project
auto-trigger path, which has always been detached for exactly this reason.
Cancellation still reaches the turn via the CP-81 lifecycle drain
(`armScaffoldCancel` unchanged). Connected clients see no behavior change: the
POST still returns the final result synchronously.

**Verified live (post-fix):** same dispatch, `kill -9` on the client at ~15s →
Devin kept running, turn completed (exit 0), compiler gate PASS, feed terminal
`result=done`. Runner then self-drained via the CP-81 idle path.

**TUI follow-through.** With a detached dispatch, a transport-level POST error
no longer means the turn failed — the feed is now authoritative. TUI changes:

- `handleEngineScaffoldMsg`: on a non-`APIError` (transport/ctx) failure the
  model enters `scaffoldPostLost` — busy state + feed polling continue, with a
  "connection lost … watching progress" notice instead of a false failure. A
  real HTTP error response (4xx/5xx `APIError`) still fails fast and re-enables
  the composer, since the server answered definitively.
- `handleEngineScaffoldProgressMsg`: when `scaffoldPostLost` is set, a terminal
  `snap.Result` finalizes the turn exactly like the POST path (shared
  `scaffoldResultLine` helper). A bounded error counter (10 consecutive poll
  failures ≈ 7s) covers runner death — the busy latch can never hang forever.
- New model fields: `scaffoldPostLost`, `scaffoldPollErrs`.

Desktop needs no change: `ScaffoldActivity` already polls the feed
independently of the POST promise, so it renders the detached turn's terminal
state correctly.

## Tests

- `TestScaffoldDispatch_ClientDisconnectKeepsTurnAlive` (runner, RED before
  fix): cancel the HTTP request ctx mid-turn → executor ctx must stay alive,
  dispatch completes, feed result=done.
- `TestScaffoldProgress_PostConnLostStillFinishesViaFeed` (TUI): POST transport
  error keeps busy; terminal feed result finalizes the UI.
- `TestScaffoldProgress_PostLostRunnerDeadGivesUp` (TUI): feed unreachable
  after post-loss → bounded retries, busy clears with an unreachable notice.
- Pre-existing `TestScaffoldBusy_ErrorReEnablesComposer` preserved untouched —
  HTTP 500 still fails fast.

## Provider parity

Provider-agnostic: the change is ctx plumbing in the HTTP handler; every
provider goes through the same `ExecutePrompt` + detached ctx. Live-verified
with Devin SWE-2; Claude/Codex/Grok share the identical code path.

## Notes for future work

- `devin -p` one-shot has no answer path: a `waiting_question` mid-scaffold
  would stall until the 30-min turn budget. Worth a prompt-level "never ask"
  enforcement check or a silence detector in the feed if it recurs.
