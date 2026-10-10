# BUG-664 — Watch: no progress (hang)

## Metadata

- Document ID: `BUG-664`
- Title: `Watch: no progress (hang)`
- Phase: `bugfix`
- Status: `superseded`
- Superseded By: `BUG-663` (CA-658) — same run-641691 incident: parent timeline frozen at nextSeq=7 while the gate-driven owner-debate churn held the run; no-progress is the debate overlay symptom, not a distinct defect
- Owner: `flowpilot runner`
- Reviewers: `dev`
- Created: `2026-10-09T04:09:36Z`
- Last Updated: `2026-10-09T04:09:36Z`
- Related Documents: `run: run-641691`
- Tags: `watchdog, production-watch`

## AI Quick View

### Summary

- No timeline/steps movement for 300s while status='running'.

### Evidence (watchdog snapshot)

```
run: run-641691
status=running nextSeq=7 idle=300s
steps={"DONE": 4}
pendingApproval=False pendingQuestion=False pendingGate=False
```

### Triage Result

- Attributed to BUG-663: identical run (run-641691), identical frozen
  seq (nextSeq=7), identical shape — owner-debate legs churned on the
  same gate card while the parent timeline held still. CA-658's
  empty-verdict circuit breaker resolves the freeze by escalating
  instead of remounting. No separate defect.
