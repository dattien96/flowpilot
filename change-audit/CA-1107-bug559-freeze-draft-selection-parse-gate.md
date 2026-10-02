# CA-1107 — BUG-559: freeze draft selection is parse-gated, scoped, and cohort-safe

## What
- `findPlannerResultForFreeze` now parse-gates every candidate: it scans the
  matching planner child(ren)'s `EventTurnCompleted` messages newest-first
  (spawn order) and returns only messages `changecontract.ParsePreflightDraft`
  accepts — a later verdict/reasoning turn can no longer mask an earlier valid
  draft. Drafts whose `source_doc_id` resolves to a different `Task-NNN` than
  the run's current vibe sprint are rejected, and a failed scout re-run sets
  a durable `preflightDraftStale` marker so superseded turn-history drafts
  cannot resurrect (incl. across restart). The durable stash remains the last
  fallback; both scans are deterministic (spawn-order + createdAt tiebreak).
- `cachePreflightDraftLocked` treats a verdict turn — an engine-issued
  missing-verdict reprompt (`verdictRepromptInFlight`) or a completion
  carrying a buffered `submit_review_outcome` — as "no draft attempted": it
  never clears the stashed draft.
- `reinvokeMatchingFlowChild` scans children newest-first, so a retried
  delegate re-drives the current sprint's leg, and clears the verdict marker.
- The BUG-505 delegate retry predicate re-arms a drained one-member cohort
  (`rearmCohortIfDrainedLocked`), so a retried scout's completion rejoins the
  barrier and re-dispatches `contract.freeze` instead of being dropped.
- `PreflightDraftStale` round-trips through `ProviderSessionState`, the local
  file session record, and the Supabase runtime blob.

## Why
Live run-60899 parked `preflight_contract_freeze` on
`invalid planner proposal: ... invalid character 'l' in literal false`:
the planner leg's valid draft (turn-62800) was masked by a later verdict turn
carrying a malformed `{…flase…}` detail, and the ungated "latest message wins"
lookup returned the prose. Symmetric evidence on run-68834 (Task-024). The
verdict reprompt also wiped the durable stash, and the retried scout's cohort
had already drained, so its rejoin was dropped — the flow stalled until manual
intervention.

## Guarantees kept
- Fail-closed unchanged: no parseable candidate anywhere → escalate, exactly
  as BUG-505 established (operator Continue remains the reprompt trigger).
- Unscoped/`Task-*`-unrelated `source_doc_id` values are accepted as before —
  the gate only rejects provable cross-task mismatches.
- A live (non-drained) cohort is never inflated by the rearm helper.
- Verdict-reprompt semantics are unchanged; only the draft cache observes the
  marker.

## Tests
`bug559_freeze_draft_selection_test.go` (10 cases): earlier-draft-over-verdict-
prose selection, cross-sprint scoping, verdict-turn cache preservation, stale
marker blocking resurrection, newest-first reinvoke, drained-cohort rearm,
no-inflate guard, `vibeTaskDocID` table, fixture parse sanity. Full
`internal/runner` suite: only pre-existing environmental failures remain
(TestBug334, TestBug377, gate/session/provider set, gitnexus tempdir flake).
