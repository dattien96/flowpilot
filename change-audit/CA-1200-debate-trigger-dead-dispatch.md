# CA-1200 — dead debate_trigger dispatch re-driven (live-039)

## Evidence

Live run `run-225691` round-4: the owner-debate mount swapped the overlay in
(`owner_1`/`owner_2` → PENDING, `debate_trigger` → RUNNING at 04:03:48) but
the trigger's hub turn never dispatched owner legs — the mount goroutine
exited, `vibeDebateMountInFlight` cleared, and the shape matched no heal:

- `maybeSettleVibeOwnerDebate` `ownersStarved` required `stTrig != RUNNING`
  (BUG-624 mount-window guard).
- `maybeResolveZombieVibeDebate` requires owners DONE/RUNNING or synthesis
  DONE.
- `hub_stalled` watchdog is shielded while a debate overlay is mounted.

Ledger: `live-039|debate-trigger-dispatch-dead-end`. Fixed live by operator
hub nudge (turn → debate_trigger DONE 04:25:08).

## Root cause

`debate_trigger` is a `hub.inline` entry node: only its own hub turn's
flow_control verdict can stamp it DONE and fan out owner_1/owner_2. When the
mount's dispatch died mid-window, no turn, gate eval, settle, or reinvoke was
in flight and nothing would ever stamp the node — while the RUNNING status
itself excluded the starved-owner detector.

## Fix

`vibe_debate.go`:

- New detector `vibeDebateTriggerWedge`: debate_trigger RUNNING **and**
  `StartedAt` older than `vibeDebateTriggerWedgeBound` (45s — far past any
  legit dispatch/settle window, far below stall timeouts) **and** the parent
  has no `turnInFlight`, no `pendingFlowGateSettle`, no `pendingHubReinvoke`,
  no `reinvokeInFlight`, no live post-turn gate window, and no mount in
  flight. The age bound + in-flight fields preserve the BUG-624/CA-1088
  mount-window contract — a fresh RUNNING stamp is still skipped.
- `maybeSettleVibeOwnerDebate` treats the wedge as a third trigger shape on
  the existing bounded ladder: same `vibeOwnerFailRetries` budget, same cap →
  park/escalate. The heal is `maybeAutoReinvokeHubWithPrompt` with a
  dedicated `vibeDebateTriggerWedgePrompt` (not a full `startResolvedFlow`
  re-mount — the overlay is already swapped in).

## Tests

`live039_debate_trigger_wedge_test.go`:
- aged RUNNING trigger + no in-flight work + zero owner children → re-drive
  arms a hub reinvoke.
- aged trigger + `turnInFlight` → skipped (live work wins).
- fresh RUNNING trigger → skipped (dispatch window still open).
- retries at cap → blocked/cap surfaced (tournament seam may lift status).

Regression: CA-1088, BUG-624/589/594/595/596/597/567/568/570/578/1182,
CA-769/792/796/805/1095, vibe/owner/cohort/hub families — all green.
(`TestBug557` TempDir cleanup flake passes solo; unrelated.)
