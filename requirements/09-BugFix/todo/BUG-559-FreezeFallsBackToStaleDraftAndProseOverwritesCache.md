---
# BugFix Format Reference compliance: metadata block
document_id: BUG-559
title: Freeze picks latest FinalMessage (verdict prose masks draft) + verdict turns overwrite the cached draft
phase: bugfix
status: done
owner: flowpilot-team
reviewers: flowpilot-team
created: 2025-01-01
last_updated: 2025-01-01
parent_documents: CP-02
child_documents: ""
related_documents: BUG-505, CA-1105, run-60899
replaces: ""
tags: agent-flow-engine, contract.freeze, preflight, vibe-tasks, regression
---

## AI Quick View

### Summary

- `contract.freeze` resolves the planner draft via `findPlannerResultForFreeze`,
  which returns the **latest** `EventTurnCompleted.FinalMessage` from the
  `preflight_contract_plan` child **without parse-gating**. A later verdict /
  reasoning turn on the same child masks an earlier valid JSON draft — freeze
  then tries to parse prose (live failure: `invalid character 'l' in literal
  false` when the verdict detail embedded `{...flase...}`), escalates, parks.
- `cachePreflightDraftLocked` unconditionally clears `preflightDraftResult`
  whenever the child's newest final message isn't a parseable draft — a
  **verdict-reprompt turn** (engine-issued, never asked for a draft) wipes the
  durable stash even though no real re-draft was attempted.
- When several same-label planner legs exist (one per vibe sprint), candidate
  selection isn't ordered newest-first and isn't scoped by `source_doc_id`, so
  a prior task's draft can be silently picked for the current task.
- When a cohort-member scout is retried via the BUG-505 continue path, its
  already-drained cohort is never re-armed, so the retried completion is
  dropped (deferred join note) and the join target (freeze) is never
  re-dispatched — the flow stalls.

### Current Ask

- Parse-gate every candidate in `findPlannerResultForFreeze`: scan newest-first
  across the matching planner child(ren)'s turn events, accept only
  `changecontract.ParsePreflightDraft`-parseable messages, scope by
  `source_doc_id` vs the current `vibeTaskName` when both are Task references,
  then fall back to non-matching children, then the durable cache.
- In `cachePreflightDraftLocked`, treat a verdict-reprompt completion
  (`verdictRepromptInFlight` flag set when scheduling the reprompt, or a
  buffered `pendingReviewVerdictByLabel` entry) as "no draft attempted": never
  let it clear a stashed draft.
- When a genuine scout re-run completes with no parseable draft, mark the stash
  stale (`preflightDraftStale`, durable) so event-scan can't resurrect an
  already-superseded draft.
- Iterate `reinvokeMatchingFlowChild` newest-first so a re-drive targets the
  current sprint's leg, and clear the verdict-reprompt marker when a real
  re-drive begins.
- Re-arm a drained one-member cohort (`preRegisterCohort(parent, id, 1)`) when
  the BUG-505 retry re-drives a cohort-member delegate whose barrier drained —
  the completion must rejoin and re-dispatch the join target.

### Key Decisions

- Fail-closed stays: with no parseable candidate anywhere, freeze still
  escalates (BUG-505 contract unchanged — operator Continue is the reprompt
  trigger; it is bounded by the loop cap and now actually rejoins the cohort).
- `source_doc_id` gate only rejects Task-*-shaped mismatches; non-Task values
  can't be scope-proven and are accepted (same as today, minus the ordering fix).
- `preflightDraftStale` is persisted in `ProviderSessionState` so a restart
  cannot resurrect a superseded draft.

## 1. Issue Report

- Observed on run-60899: `preflight_contract_freeze` parked with
  `invalid planner proposal: ... invalid character 'l' in literal false`.
  Planner child run-62795 had emitted a valid draft at turn-62800; a later
  verdict turn (turn-62977/63236) carried prose + a malformed `{...flase...}`
  verdict detail. The ungated "latest FinalMessage wins" lookup returned the
  prose; strict parse failed; escalate parked the flow.
- Symmetric live evidence on run-68834 (Task-024 sprint planner): valid draft
  at an early turn, verdict prose in the last turn.

## 2. Reproduction

Regression tests in `bug559_freeze_draft_selection_test.go`:

- earlier valid draft + later prose FinalMessage → lookup must return the draft.
- two same-label planner children across sprints → the newest child's draft for
  the *current* task wins; a draft scoped to a different Task is never frozen.
- verdict-reprompt completion must not clear `preflightDraftResult`.
- failed scout re-run sets the stale marker; event-scan must not resurrect.
- retrying a cohort-member scout re-arms the drained cohort.

## 3. Diagnosis

- `findPlannerResultForFreeze` loop 1 returns the first non-empty
  `FinalMessage` walking newest→oldest, no parse check, no child ordering,
  no scope check.
- `cachePreflightDraftLocked` clears the stash on any empty/unparseable newest
  message, including engine-issued verdict reprompt turns.
- `reinvokeMatchingFlowChild` walks `listChildren` (spawn order, oldest first),
  so a same-label leg from an earlier vibe sprint is re-driven instead of the
  current one.
- The BUG-505 retry re-drives the failed delegate but a drained cohort can
  never rejoin, so the new draft's completion is dropped.

## 4. Fix Plan

- `flow_validate_audit_dispatch.go`: rewrite candidate selection; add
  `vibeTaskDocID` + `rearmCohortIfDrainedLocked` helpers.
- `plan_approval_park.go`: verdict-turn exemption + stale marker in
  `cachePreflightDraftLocked`.
- `interactive_service.go`: `verdictRepromptInFlight` / `preflightDraftStale`
  fields; set/consume at the verdict-reprompt and settle sites; rearm in the
  BUG-505 retry predicate; snapshot the stale flag.
- `interactive_resume.go`: restore `preflightDraftStale`.
- `flow_executor.go`: `reinvokeMatchingFlowChild` scans newest-first and clears
  `verdictRepromptInFlight` on the re-driven child.
- `workflow_store.go`, `local_file_session_store.go`,
  `supabase_workflow_store.go`: persist `PreflightDraftStale`.

## 5. Implementation

- `findPlannerResultForFreeze`: parse-gated candidate selection, newest-first
  (spawn order), source-node scan then any-child scan, `source_doc_id` Task
  scope gate vs `vibeTaskPlan[vibeSprintCurrentPlanIndex]`, `preflightDraftStale`
  skips event scans, durable cache last.
- `cachePreflightDraftLocked`: verdict-turn exemption (flag + buffered verdict),
  stale marker set on genuine failed scout re-runs, cleared on every parseable
  write.
- `reinvokeMatchingFlowChild`: newest-first child scan; clears
  `verdictRepromptInFlight` on the re-driven child.
- Verdict reprompt sites (settle path + `resumeVerdictDeficientMembers`) set
  the marker; settle consumes it after the cache consult.
- BUG-505 continue-retry predicate re-arms drained one-member cohorts.
- `PreflightDraftStale` persisted via ProviderSessionState, local file session
  record, and Supabase session_runtime blob; restored on resume.

## 6. Acceptance Criteria

- [x] New regression tests red → green (`bug559_freeze_draft_selection_test.go`).
- [x] Existing BUG-505 / freeze / cohort-join suites stay green (no edits).
- [x] `go test -count=1 ./internal/runner/...` — only pre-existing
  environmental failures remain (verified identical on stash).
- [x] Restart mid-sprint cannot resurrect a superseded draft
  (`preflightDraftStale` is durable).
- [ ] CA note + detect_changes clean before commit.

## 7. Human Confirmation

- pending
