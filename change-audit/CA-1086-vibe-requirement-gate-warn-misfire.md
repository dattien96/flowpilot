# CA-1086: vibe chat-surface runs blocked by spurious r-requirement under warn gate

Date: 2026-10-01
Refs: user-reported — armed vibe-tasks chat ("xin chào", no code) blocked
with a requirement decision card; Start flow then bounced 409
`flow_awaiting_user`. Expected: normal chat turns gate warn-only.

## Symptom (live run-2280, 15:17)

```
[gate] violations=1 gateMode="warn" hasCode=false hasCA=false
[gate-metric] action=block step="chat-run-2280" mode="warn" rules=[r-requirement]
```

The run parked `waiting_user_apr` → the Start-flow forward turn hit the
`flow_awaiting_user` fence → 409.

## Root cause — two stacked defects

1. **`injectVibeSSDrift` treats `rs.sourceDocID` as the locked-SS
   fallback.** CP-sourced vibe runs pin a `CP-*.md` (and task/bugfix chats
   pin `Task-*`/`BUG-*` docs). The signature check extracts `AC-*` tokens
   from that doc and compares them against the test sources written this
   turn — and `tr.Tests.Ran` is true whenever the baseline oracle ran, even
   on a turn that wrote nothing. Zero test writes → every AC reads
   "uncovered" → `RequirementDrift=true` → `r-requirement` violation on a
   plain chat turn.

2. **The requirement precedence route ignored warn mode.**
   `classifyVibeGateWithDrift` downgraded `owner_debate`-routed results to
   passthrough under `result.Action == "warn"` (Task-352) but left
   `PrecedenceRouteUser` hard-blocking — contradicting Task-455's
   chat-surface contract ("enforce/reprompt actions only apply to
   flow-context runs"). The block parked a run whose flow had not even
   launched yet, and the awaiting fence then rejected the user's forward.

## Fix

`internal/runner/vibe_gate.go` only:

- `injectVibeSSDrift`: the `sourceDocID` fallback (own + parent's) now
  applies only when the pinned doc is actually an SS (`SS-*.md`
  basename). CP/Task/BUG source docs can no longer seed the check.
  `vibeLockedSS` stays authoritative and unchanged.
- Same function: a turn that wrote no test sources skips the comparison —
  "green suite + signature drift" requires signatures to have been
  exercised; an empty test-source set is not evidence.
- `classifyVibeGateWithDrift`: the warn downgrade now covers every
  non-drift-routed route (`!res.DriftRouted && result.Action == "warn"`
  → passthrough). Requirement violations still emit the warn event — the
  user sees them (BR-4 routing to the user preserved) — they just cannot
  hard-block a warn-gated surface. Enforce-mode runs (flow-context) keep
  the user-only block unchanged.

## Tests

`ca1086_vibe_requirement_warn_misfire_test.go` (reproduce-first red →
green; the red run printed the exact live card text):

- CP doc as `sourceDocID` + green baseline + no writes → no drift.
- Locked SS + green baseline + zero test writes → no drift.
- SS doc as `sourceDocID` + written test lacking AC → drift still fires
  (fallback preserved).
- warn-mode `r-requirement` violation → passthrough; enforce-mode → still
  `vibeGateRequirement`.

Existing `TestInjectVibeSSDrift_SetsRequirementDrift`, all
`TestClassifyVibeGate*`/`TestVibeGatePrecedence*`/Task-455/Task-326 tests
unchanged and green. `go vet` clean. Focused vibe/gate sweep: only the
known pre-existing baseline failures (Bug425, Bug514, GateBlind×2,
Run200816, StartSession-process-key, GateHook_DodComplete×2).
