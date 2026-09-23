# BUG-379: Mux change-detection fingerprint shared across subscribers — every subscriber but the first silently starved

## Metadata

- Document ID: `BUG-379`
- Title: `runUpdateSub change-detection stored per-run (rs.muxFingerprint) — first drainer marks the run "sent" for all subscribers`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `event-plane`
- Parent Documents: CP-84, Task-429 (multiplexed run updates)
- Child Documents: `none`
- Related Documents: CP-84 code-review pass 2026-09-23
- Replaces: `none`
- Tags: `mux, sse, multi-subscriber, fingerprint, starvation`

## AI Quick View

### Summary

- The mux suppression fingerprint lived on `interactiveRun.muxFingerprint` — a
  single per-run field shared by every subscriber.
- `drainRunUpdates` compared `fp == c.rs.muxFingerprint` then wrote it back.
  Once subscriber A drained a dirty run, every other subscriber computed the
  same fingerprint and `continue`d — never receiving the upsert.
- Effect: a second SSE client (second desktop window, CLI, test) got the
  initial snapshot but no lane updates until the next unrelated change — the
  opposite of the multi-lane contract.
- Fix: per-subscriber fingerprint map `runUpdateSub.fps[runID]`, seeded from
  the snapshot, cleared on remove. `rs.muxFingerprint` removed.

### Constraints

- safe-fix-contract: additive regression test; no existing test touched.
- Lock order unchanged (`s.mu` only around `sub.fps`).

## 4. Expected vs Actual

- expected: every subscriber receives upserts for runs marked dirty.
- actual: only the first subscriber to drain received the upsert; others
  silently skipped it (fingerprint equality on shared state).

## 5. Root Cause

`internal/runner/decision_payload.go` — suppression state was scoped to the
run (`rs.muxFingerprint`) instead of the subscriber (`runUpdateSub`). Change
detection answers "did THIS subscriber see this state?", which is inherently
per-subscriber.

## 6. Fix

- `runUpdateSub.fps map[string]string` added; seeded in `subscribeRunUpdates`
  from the snapshot fingerprints.
- `drainRunUpdates` compares/updates `sub.fps[runID]`; `emitRemove` deletes
  the entry so a revived lane upserts fresh.
- `interactiveRun.muxFingerprint` field deleted (single-subscriber bug
  vector removed).

## 7. Regression

- New: `TestRunUpdates_EverySubscriberReceivesUpserts` — red before fix
  (subscriber 1 missed the upsert), green after. `-race` clean.
- All `TestRunUpdates_*` + `TestDecisionPayload*` green.
