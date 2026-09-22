# CA-917: Desktop Manual Scaffold Trigger + Progress Feed Fixes

## Summary

Follow-up to CA-916 with two parts:

**Desktop `/init` parity.** TUI's `/init all` runs engine init and then the AI
scaffold turn; Desktop previously had no way to dispatch a scaffold at all — it
only fired server-side on project creation. `dispatchScaffold` +
`fetchScaffoldStatus` are now exposed in `projectEngine.ts`, and EngineSettings
gains a capability-gated "Run AI Scaffold" button next to "Initialize /
Re-sync Project". The button re-labels to "Re-run AI Scaffold" (force=true)
once a `scaffold-status.json` says done, and the CA-916 `ScaffoldActivity` card
renders the turn live while the POST is in flight.

**Feed fixes found during live validation review.**

- Ring-trim transcript loss: the in-memory hub keeps the last 400 events, so a
  scaffold with >400 events (the live Devin run emitted 102; verbose providers
  can exceed it) lost the transcript head for any client joining with
  `after=0`. `scaffoldProgressSnapshot` now detects trimming
  (`hubFloor > runStartSeq`) and prepends the persisted NDJSON head — the merge
  is bounded to the current run's seq range so a re-run never replays the
  previous run's events.
- TUI `scaffoldPhase` is reset on the terminal message so a stale phase label
  can never linger into the next busy state.

## Tests

- `TestScaffoldProgress_RingTrimMergesPersistedHead` — 460 events through the
  feed, then `after=0` must return seq 1 and the full transcript (merged from
  the persisted log).
- All CA-916 runner/TUI/Desktop tests still green; `go vet` and Desktop `tsc`
  clean (pre-existing `chat-mode-persist` test error unchanged).

## Provider parity

No provider-affecting change: the trigger is the existing POST /scaffold
contract; the merge fix is provider-agnostic buffer logic.

# ---8<--- flowpilot:change-ledger
feature_key: skill-anchored-init
source_doc_id: CP-68
change_type: feature
summary: Desktop manual AI scaffold trigger (capability-gated, force re-run) + scaffold feed ring-trim merge fix and stale phase reset
# --->8---
