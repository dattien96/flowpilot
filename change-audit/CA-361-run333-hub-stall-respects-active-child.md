# CA-361 - run-333: hub stall respects active child

## Symptom

Live A2 `run-333` showed `my-reviewer` still running when the hub/root
watchdog fired `hub_stalled`. The watchdog parked the flow and cancelled the
reviewer, so the cohort only joined after a synthetic failure and synthesis was
blocked instead of running.

## Cause

`checkAndBlockStalledHub` only counted root-run activity as busy: hub turn,
hub reinvoke, root approval/question, gate reprompt, or resume prompt. It did
not count active child/sub-agent work, so a healthy reviewer turn could be
mistaken for hub idleness.

## Fix

Add `hasActiveFlowChild` to treat any child/sub-agent that is running,
starting, waiting for approval/question, in-flight, queued, or gate-settling as
busy for the hub watchdog. Child/member-specific stall handling remains the
owner for stalled children; hub_stalled is now reserved for true root idleness.

## Tests

- `TestRun333HubStallDoesNotCancelRunningChild`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Prevent hub_stalled from cancelling active child/sub-agent work (run-333 A2)
# --->8---
