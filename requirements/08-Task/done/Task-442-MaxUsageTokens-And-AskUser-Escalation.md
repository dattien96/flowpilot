# Task-442: maxUsageTokens Real-Token Budget + ask_user Escalation

- Document ID: `Task-442`
- Title: `Per-node real usage budget measured on provider-reported tokens; exceed escalates to ask_user after the in-flight turn finishes`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86`, `SD-10`, `SS-22`
- Child Documents: ``
- Related Documents: `Task-441` (est rename — land first), `decision-card-ui` (escalation card path), `flow-gates` (`when: escalate` edge)
- Replaces: ``
- Tags: `token-usage`, `budget`, `escalation`, `context-profiles`

## AI Quick View

### Summary

- `maxEstPromptTokens` (renamed, Task-441) caps estimated prompt input —
  it says nothing about what a node actually burns. A coder node with a 24k
  prompt cap can consume 200k real tokens inside its provider session.
- `maxUsageTokens` is the real-consumption budget per node invocation,
  accumulated from `Total.TotalTokens` on `token_usage_updated` events —
  the only number comparable post-run.
- Exceeding it is a **decision point, not a bug**: the in-flight turn runs
  to completion (never killed mid-turn), then the shared routing gate
  (CP-87) decides: `extend` / `rotate` / `stop` in manual mode; auto mode
  may rotate to a high-confidence candidate but never auto-extends.

### Current Ask

- Parse `maxUsageTokens` on context profiles, accumulate real usage per
  node run, and on post-turn exceed raise `usage_budget_exceeded` through
  the shared gate: `extend` raises the cap once (user-only action),
  `rotate` delegates to the CP-87 resolver, `stop` escalates the node out
  via the existing `when: escalate` path.

### Key Decisions

- `T-1` Measurement = `TokenUsage.Total.TotalTokens` (cumulative) from the
  run's own usage events — never `Last` (per-response), never estimated.
- `T-2` Check fires **post-turn only**, after the turn's terminal event —
  mid-turn kills are destructive and banned by CP-86 constraint.
- `T-3` Escalation emits `EventUserQuestionRequired` with options
  `extend` (cap += profile value, once per exceed — never auto-selected
  because it changes the user's declared budget), `rotate` (delegates to
  the CP-87 `QuotaPreflightResolver`; rotation implicitly applies the same
  one-time extension or the check would re-fire immediately), and `stop`
  (structured `usage_budget_exceeded` → flow `escalate` edge). When the
  CP-87 gate is not yet landed the card degrades to `extend`/`stop` only
  (typed degradation). Usage accounting carries across legs of the same
  node run — rotation never resets the counter.
- `T-4` No profile / no `maxUsageTokens` → uncapped (zero behavior change
  for chat and unprofiled nodes). Provider silent on usage → cap never
  fires (degrade-soft, audit records "no usage data").

### Constraints

- Depends on Task-441 (field naming) and Task-440 (window enrich is not
  required here but the accounting seam is shared — keep helpers in
  `context_usage.go`).
- Usage accounting must be durable-traceable: write per-node usage totals
  into the prompt-context-audit jsonl alongside the est figures
  (est-vs-actual record — input for future calibration; no auto-calibration
  in this task).
- Provider parity: accounting is adapter-agnostic (works off normalized
  `TokenUsageSnapshot`) — parity proven by fake-adapter table tests.

### Open Questions

- Whether `extend` increments by the profile value or a fixed +50% —
  default: += profile value each time, logged.

### Source Refs

- `CP-86 P-3`, `agentpack/pack.go` (`ContextProfile`),
  `provider_event.go` (`TokenUsageSnapshot.Total`),
  `gate_hook.go` (`lastTurnTokensConsumed` — same event-scan pattern),
  `decision_payload.go` (decision card contract), flow YAML `when: escalate`.

## 1. Goal

A node's real token consumption becomes a first-class budget: observable,
comparable, and capped — with the user (not the engine) deciding what
happens at the cap.

## 2. Parent Links

- coding plan: `CP-86` (P-3)
- tech design: `SD-10`
- system spec: `SS-22`
- specific upstream ids: `Task-441`, `CP-62 Task-341`, `decision-card-ui`

## 3. Trigger

Production flows multiply per-invocation cost across review loops
(`cap: 5` + extends). Without a real-usage cap there is no bound on what
one node can spend — and no signal to the user that a node is burning
abnormally.

## 4. Exact Change

- `T-1` `agentpack.ContextProfile` gains `MaxUsageTokens int`; parse
  `maxUsageTokens` (optional, 0/absent = uncapped).
- `T-2` New `context_usage.go`: `nodeUsageTokensLocked(rs)` sums/reads the
  latest `Total.TotalTokens` from `rs.events` (same scan pattern as
  `lastTurnTokensConsumed`); `nodeUsageBudgetFor(rs)` resolves the profile
  cap via the Task-441 profile lookup.
- `T-3` Post-turn hook (same seam class as `recordDriftTelemetry`): after
  the turn's terminal event, if usage > cap and no pending decision, emit
  `EventUserQuestionRequired` with `usage_budget_exceeded` options
  extend/rotate/stop; `extend` raises cap by the profile value; `rotate`
  hands off to the CP-87 routing gate (same one-time extension applied);
  `stop` marks the node outcome escalate. Auto mode (CP-87) may auto-pick
  `rotate` for a high-confidence candidate but never `extend`.
- `T-4` Audit: append `actual_usage_tokens` + `est_prompt_tokens` +
  `usage_budget` fields to the per-turn audit record written by
  `writePromptContextAudit`.

## 5. Touched Areas

- files: `internal/agentpack/pack.go`, `internal/runner/context_usage.go`
  (new), `internal/runner/interactive_service.go` (post-turn hook seam),
  `internal/runner/context_profile.go` (budget resolver sibling),
  `internal/runner/decision_payload.go` (option ids), flow YAMLs
  (`maxUsageTokens` on profiles where wanted)
- modules: `agentpack`, `runner`
- routes: none (card flows through existing event stream)
- tables: none

## 6. Code Guide Signatures

```go
// internal/agentpack/pack.go — extends Task-441 struct
type ContextProfile struct {
    Name               string
    CandidateSources   []string
    MaxEstPromptTokens int
    // MaxUsageTokens caps REAL provider-reported consumption per node
    // invocation (cumulative Total.TotalTokens). 0 = uncapped.
    MaxUsageTokens int
}
```

```go
// internal/runner/context_usage.go (new)
func (s *InteractiveService) nodeUsageTokensLocked(rs *interactiveRun) int64
func (s *InteractiveService) nodeUsageBudgetFor(rs *interactiveRun) int64
func (s *InteractiveService) checkUsageBudgetPostTurn(rs *interactiveRun, turnID string)
func (s *InteractiveService) applyUsageBudgetAnswer(rs *interactiveRun, optionID string) // "extend" | "rotate" | "stop"
```

```go
// internal/runner/interactive_service.go — post-turn seam (mirrors recordDriftTelemetry call site)
s.checkUsageBudgetPostTurn(rs, turnID) // T-3
```

## 7. Test Signatures

- `TestTask442_CumulativeUsageAccumulatesPerNode` — two usage events →
  latest `Total.TotalTokens` read correctly (covers T-1)
- `TestTask442_ExceedFiresAfterTurnNotMidTurn` — usage > cap mid-turn →
  no card; after turn_completed → card emitted (covers T-2)
- `TestTask442_ExceedEmitsAskUserExtendRotateStop` — card has
  `usage_budget_exceeded` + extend/rotate/stop options (covers T-3)
- `TestTask442_RotateDelegatesToRoutingGate` — answer `rotate` →
  resolution handed to quota preflight; one-time extension applied so the
  check does not re-fire immediately (covers T-3 rotate)
- `TestTask442_GateUnavailable_OffersExtendStopOnly` — resolver absent →
  card degrades to extend/stop (covers T-3 typed degradation)
- `TestTask442_AutoModeNeverSelectsExtend` — auto policy may rotate an
  exact candidate but cannot choose `extend` (covers T-3)
- `TestTask442_ExtendChoiceRaisesBudgetOnce` — answer `extend` → cap
  raised by profile value, node continues; second exceed re-asks (covers
  T-3 extend semantics)
- `TestTask442_StopChoiceEscalatesNode` — answer `stop` → node outcome
  `usage_budget_exceeded` routes the flow's escalate edge (covers T-3)
- `TestTask442_NoProfileOrNoCap_Uncapped` — no profile / maxUsageTokens=0
  → zero cards (covers T-4)
- `TestTask442_ProviderSilentOnUsage_CapNeverFires` — no usage events →
  no card, audit notes "no usage data" (covers T-4 degrade-soft)
- `TestTask442_AuditRecordsEstVsActual` — audit jsonl carries both est and
  actual figures (covers T-4 audit)
- `TestTask442_ClaudeCodexGrok_Parity` — fake-adapter table: same
  accounting across all three (covers AGENTS §5)

## 8. Acceptance Check

- A flow node whose usage exceeds `maxUsageTokens` produces a visible
  ask_user card; answering extend resumes work, answering rotate creates
  a new leg via the routing gate, answering stop escalates through the
  flow's existing path — all events durable in ndjson.
- Prompt-context-audit records show est vs actual side by side.

## 9. Out of Scope

- Window-pressure detection (Task-443) and any UI rendering (Task-444).
- Auto-calibration of the est heuristic from actual ratios (data is
  recorded; learning is a later CP).
- Run-level (whole-flow) usage budgets — this task is per-node only.

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [x] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: maxUsageTokens on ContextProfile (0=uncapped); nodeUsageTokensLocked accumulates Total.TotalTokens per provider session across the node's legs; checkUsageBudgetPostTurn runs after the turn terminal event only (never mid-turn); exceed emits durable usage_budget_exceeded question (extend/rotate/stop) persisted before emit; extend raises cap once per exceed; rotate delegates to usageRouter seam (CP-87); stop escalates via applyFlowControl. Audit line records est vs actual per turn. Task tests green incl. parity + gate-unavailable degrade (extend/stop only) + auto-mode never auto-selects extend.
- live-found bug (fixed, CA-975): checkUsageBudgetPostTurn sat below early-return guards in runChildArtifactOutputGateAtEpoch/runFlowGateAtEpoch → profiled children with no artifact bindings skipped the check entirely (live Devin child burned 12384 real tokens vs cap 2000, no card). Hoisted to the top of both gate fns — post-turn invariant of the gate dispatcher. Regression: TestTask442_GateEarlyReturnStillEnforcesBudget. Verified live: card fired with extend/stop (rotate absent — seam unwired, typed degradation correct); answering stop via /questions accepted → run escalated.
- follow-ups: usageRouter implementation lands with CP-87 gate; audit file may append duplicate lines on reprompt-resume re-gate (append-only diagnostic, acceptable)
- upstream docs updated: CP-86, CP-86-Test-Steps
