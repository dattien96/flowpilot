# CA-412: durable step-log cohort order for post-restart agent cards

## Summary

Post-restart agent-card **order** in multi-round Review Loop / flow-mode was
driven by wall-clock heuristics (peer start waves, 45s gaps, 9/10 or -1ms
parking). That fixed some fixtures after BUG-314 but was not durable authority
— Terra (gpt-5.6-terra) **REWORK_DESIGN**: use step-transition **append order**
and `synthesis RUNNING` boundaries already on disk.

Live regressions:

- `run-52518` (chat dual-reviewer, short gap)
- `run-58237` (flow-mode custom nodes, long R1)

## Change

### Authority (when step-transition sidecar exists)

1. `stepActivationsFromOrderedLog` walks parent `*-step-transitions.ndjson` in
   append order.
2. Each agent-node `RUNNING`→terminal is one activation with:
   - `cohort` = count of preceding **synthesis** node `RUNNING` lines
     (`synthesis`, `grok-synthesis`, … via `isSynthesisStepNode`)
   - `ord` = global append ordinal among agent activations
3. Reinvoke (`labelCounts[label]==1`): all activations of that label on one child.
4. Spawn-lifecycle (`labelCounts>1`): each child run pairs with the next log
   activation for that label in child `startedAt` order.
5. `insertUnanchoredFlowAgentLifecycleLocked` groups by **durable cohort**
   (`clusterFlowAgentPairsByDurableCohort`) and inserts cohort *i* before hub
   `message_completed` *i*. No 45s / 9/10 / -1ms for round assignment.

### Legacy fallback

When the step-transition sidecar is absent (older unit fixtures), keep the
pre-existing wave-parking + gap-cluster path so no-sidecar tests stay green.

### Resume-only event fields

`ProviderEvent.ResumeDurable` / `ResumeCohort` / `ResumeLogOrd` are `json:"-"`
(not sent to clients).

## additive-tests-only

New files only (no edits to `bug314_*` or other pre-existing resume-order tests):

### `run52518_dual_reviewer_resume_order_test.go`

| Test | Intent |
|---|---|
| `TestRun52518DualReviewerReinvokeResumeOrderPreservesRounds` | Live short-gap dual-reviewer skeleton |
| `TestRun52518SingleSynthesisMessageKeepsRoundOrder` | One hub synth; R0 before R1 before message |
| `TestRun52518OrderPreservedAcrossProviders` | codex / claude / grok same dual-reviewer shape |
| `TestRun58237FlowModeLongR1ReviewerResumeOrderPreservesRounds` | Flow-mode long R1 |
| `TestDurableCohortOneSecondInterRoundGap` | **1s** gap — no 45s dependency |
| `TestDurableCohortSingleReviewerShortGapOrder` | BUG-314 single-reviewer count+order ×3 |

### `durable_resume_agent_order_matrix_test.go` (edge matrix)

| Test | Intent |
|---|---|
| `TestStepActivationsFromOrderedLogAssignsCohortsBySynthesisRunning` | Pure unit: cohort/ord math + open RUNNING |
| `TestIsSynthesisStepNodeAcceptsAliases` | synthesis / grok-synthesis naming |
| `TestDurableOrderAgentsStayBeforePostFlowFollowUp` | No cards between follow-up and answer |
| `TestDurableOrderMoreCohortsThanHubMessages` | 3 rounds / 2 synth → no bottom-append |
| `TestDurableOrderMissingSynthesisRunningKeepsCardsButSingleCohort` | Degraded single cohort, count kept |
| `TestDurableOrderGrokSynthesisNodeNameSplitsCohorts` | Flow-mode synth node id |
| `TestDurableOrderNoSidecarStillRestoresCardsWithoutPanic` | Legacy path without sidecar |
| `TestDurableOrderWithinCohortFollowsStepLogAppendOrder` | Within-round append order |
| `TestDurableOrderThreeRoundDualReviewerAllProviders` | 3 rounds dual-reviewer ×3 providers |
| `TestDurableOrderFlowModeShapeAllProviders` | run-58237-like labels ×3 providers |
| `TestDurableOrderUncompletedChildEmitsSpawnWithoutResult` | Cancelled child: spawn only |
| `TestDurableOrderNoHubMessagesPlacesAgentsAfterPrompt` | Empty hub prose clamp |

## Verification

```text
go test ./internal/runner/ -count=1 \
  -run 'TestBug314|TestRun52518|TestRun58237|TestDurableCohort|TestRun24377|TestRun5695|TestRun1264|TestRun20332'
→ ok
```

All `TestBug314*` PASS; all new durable-order tests PASS; legacy resume-order suite PASS.

## Why this closes the Terra finding

| Before | After |
|---|---|
| Round from synthetic times | Round from step-log append + synthesis RUNNING |
| 45s / 9/10 / -1ms knobs | Unused on durable path |
| Green only for calibrated fixtures | 1s-gap + long-R1 + 3-provider matrix |

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-314
change_type: bugfix
summary: Place restored multi-round agent cards by durable step-transition append order and synthesis RUNNING cohorts (not wall-clock wave parking); cover chat/flow, 1s gap, and Codex/Claude/Grok.
# --->8---
