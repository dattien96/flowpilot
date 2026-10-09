# BUG-663 — Gate-driven owner-debate churn bypasses the BUG-652 breaker

## Metadata

- Document ID: `BUG-663`
- Title: `Gate-driven owner-debate churn bypasses the BUG-652 breaker`
- Phase: `bugfix`
- Status: `FIXED — empty-verdict circuit breaker in restoreVibeFlowAfterDebate (this session)`
- Fixed By: `CA-658` (consecutive empty resolutions escalate like identical repeats; sentinel `EMPTY` in the existing durable per-entity ledger — no new persisted fields)
- Owner: `flowpilot runner`
- Reviewers: `dev`
- Created: `2026-10-09`
- Last Updated: `2026-10-09`
- Related Documents: `run-641691` (PrivateVault vibe-tasks CP-12 live run)
- Tags: `watchdog, production-watch, owner-debate, verdict-only, gate-remount`

## AI Quick View

### Summary

- Live CP-12 run spawned 7 owner legs with the same gate card
  ("Tests failed: suite_regressed; governing spec changed since the
  Canonical Head was last computed — reconcile the Head..."), one after
  another, while parent steps/timeline look idle (owner_1/owner_2 DONE
  once, debate_synthesis RUNNING, timeline frozen at 7 records).
- BUG-652 (fixed by CA-1236) covers identical-verdict repeats via
  `VibeDebateVerdictSigs` in `restoreVibeFlowAfterDebate` — but this
  churn was NOT parked. Either the remounts bypass that seam or the
  per-round signatures never match (one card is even reworded in
  Vietnamese, same meaning).
- Aggravating shape: the card demands RECONCILIATION (a writer's job —
  fix drift, recompute the Head), but owner legs run under `verdict_only`
  posture (reads + verdict tool, no Bash). Two legs show the failure
  directly: one aborted right after a tool permission denial, the other
  opens "reading the gate failure detail" (self-diagnosing the same
  card). Reviewer-only actors cannot clear a writer-demanding gate, so
  every round re-parks on the same card and re-spawns.
- Open sub-question: one card body discusses CP-11 drift on a CP-12 run
  — possibly the gate is bound to a stale governing spec, in which case
  no owner round could ever clear it.

### Evidence (watchdog snapshot)

```
run: run-641691
owner children: run-642231, run-642240, run-642498, run-642670, run-642679, run-642987, run-642996 (all completed; 2 spawned within ~64s)
agent-bus: 7 owner handoffs, identical suite_regressed card (1 Vietnamese variant)
owner_1/owner_2 node rows: single DONE windows (loop invisible at parent step level)
parent timeline: frozen at nextSeq=7 while legs churn
attention items: none (no conflict surfaced)
denial trace: run-642987 transcript: "opencode aborted the turn right after a tool permission was denied"
```

### Current Ask

- Triage: find which seam remounts the debate without consulting
  `VibeDebateVerdictSigs` (or why signatures never match), and whether
  the governing spec is stale (CP-11 card on a CP-12 run). Consider: park
  after N identical gate cards regardless of verdict text, and never
  assign reconcile-work to verdict_only legs. The watchdog only
  observes; it never repairs code. Operator action (pause/stop the run
  to stop quota burn) stays human.
