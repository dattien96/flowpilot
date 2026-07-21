# CA-360 — run-5296: round-2 reviewers cancelled by stale BUG-226 escalate

## Symptom

Round 1: both reviewers completed → synthesis → **continue** → coder round 2 →
two new reviewers spawn → ~19s later form **"Reviewers reported: …"** (stale
round-0 text) and both new reviewers show **cancelled**.

## Cause

1. Hub `submit_review_outcome(continue)` starts the next coding round correctly.
2. A **separate hub turn** (gate: missing BugFix / rewrite BUG-907) completes
   with prose only — no `submit_review_outcome` on **that** turn id.
3. BUG-226 fallback escalates using `lastCohortNote` from **round 0**.
4. `parkFlowForAwaitingUser` cancels in-flight round-1 reviewers.

## Fix

`hubShouldSkipProseEscalate`: if open cohort or any live child, **do not**
BUG-226 escalate (log and leave loop running so reviewers can finish).

## Tests

- `TestHubShouldSkipProseEscalateWhenOpenCohort`
- `TestHubShouldSkipProseEscalateWhenChildRunning`
- `TestHubShouldNotSkipProseEscalateWhenIdle`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Skip BUG-226 prose escalate while open cohort/children in flight (run-5296 cancel reviewers)
# --->8---
