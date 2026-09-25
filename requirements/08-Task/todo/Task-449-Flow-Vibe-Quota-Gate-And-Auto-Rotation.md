# Task-449: Flow / Vibe Quota Gate & Auto Rotation

- Document ID: `Task-449`
- Title: `Integrate quota preflight into hub and child admission; manual gate by default, bounded automatic new-leg rotation when enabled`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-87`, `Task-445`, `Task-446`, `Task-447`, `Task-448`
- Child Documents: ``
- Related Documents: `CP-84 decision cards`, `vibe-mode`, `provider-switch leg/handoff`, `Task-442`, `Task-443` (CP-86 gate signal producers)
- Replaces: ``
- Tags: `quota`, `flow`, `vibe`, `gate`, `rotation`, `recovery`

## AI Quick View

### Summary

- One admission path applies before every provider-backed root/hub turn and
  child spawn. Flow and Vibe share it.
- Manual mode always parks with candidate decision; Vibe's normal 100% auto
  lifecycle does not bypass quota choice.
- Auto mode rotates only an exact/high-confidence candidate. Unknown/stale/
  ambiguous/no candidate parks at the same gate.
- Every account/provider/model change creates a durable new leg and compact
  handoff; no live session mutation or silent step-default update.

### Current Ask

- Wire preflight decisions to execution, existing ask_user/attention surfaces,
  durable CAS/lease, new-leg handoff, and restart-safe continue/stop semantics.

### Key Decisions

- `T-1` Resolver outcomes: `proceed | gate | rotate | blocked`; execution acts
  only after durable resolution commit.
- `T-2` Manual table options: use once, use for run, stop; optional explicit
  save-default is Task-450. No selection = no provider call.
- `T-3` Auto rotation records requested→resolved route and displays a notice;
  user may stop future work but the committed node starts without confirmation.
- `T-4` Gate triggers: `provider_limit` (typed event — transient
  Retry-After may wait/retry, exhausted/billing routes candidates and
  excludes the failed account) and `usage_budget_exceeded` (CP-86 —
  candidates plus a manual-only `extend` action; auto mode may rotate
  but never extends). Context-pressure leg resets arrive here only via
  escalation: the leg-lifecycle path first tries a same-binding new leg
  (headroom check on the pinned account for the re-seed cost); only when
  the pinned account cannot afford the re-seed does the request enter
  candidate routing.

### Constraints

- No mid-turn proactive switch. A failed turn settles before retry/rotation.
- Recovery: crash between decision commit and provider call yields exactly one
  safely retryable call; never claim exactly-once without durable CAS.
- Non-terminal quota gates survive restart/chat/device switch.

### Open Questions

- None blocking; Task-450 may defer Save-as-default without affecting execution.

### Source Refs

- `CP-87 P-5/P-7`, Task-445..448, AGENTS §§2-3,
  `flow_executor.go` spawn seams, decision-card/attention infrastructure.

## 1. Goal

Apply one safe quota policy to every Flow/Vibe provider action, preserving user
control in manual mode and deterministic bounded automation in auto mode.

## 2. Parent Links

- coding plan: `CP-87 P-5`
- tech design: `SD-07`, `SD-17`
- system spec: `SS-06`, `SS-22`
- specific upstream ids: Task-445..448, AGENTS §2

## 3. Trigger

The current quota-switch path handles focused normal chat in Desktop after
failure; provider-backed Flow/Vibe nodes and unattended execution need the same
policy before dispatch and after typed limit events.

## 4. Exact Change

- `T-1` Call quota preflight at root/hub turn admission and all child spawn
  seams after effective model resolution.
- `T-2` Manual mode emits durable `quota_route_required` decision card and
  parks Flow/Vibe identically.
- `T-3` Auto mode commits best exact candidate, creates new leg/handoff, then
  dispatches; uncertain candidates gate.
- `T-4` Answers use once/run/stop; stop yields structured `quota_route_stopped`.
- `T-5` Persist claim, choice, leg and delivery state with CAS/lease/recovery.

## 5. Touched Areas

- files: `interactive_service.go` admission, `flow_executor.go` spawn seams,
  flow decision payload/projector, provider-switch/handoff/leg store,
  recovery code
- modules: `runner`, flow engine
- routes: existing decision answer route
- tables: existing durable run/leg storage

## 6. Code Guide Signatures

```go
type QuotaPreflightOutcome string
const ( QuotaProceed QuotaPreflightOutcome = "proceed"; QuotaGate QuotaPreflightOutcome = "gate"; QuotaRotate QuotaPreflightOutcome = "rotate"; QuotaBlocked QuotaPreflightOutcome = "blocked" )
type QuotaResolution struct { Outcome QuotaPreflightOutcome; Demand ExecutionDemand; Candidates CandidateSet; Selected *RouteCandidate; Reason string; PolicyVersion string }
func (s *InteractiveService) ResolveQuotaPreflight(ctx context.Context, demand ExecutionDemand) (QuotaResolution, error)
func (s *InteractiveService) CommitQuotaResolution(ctx context.Context, resolution QuotaResolution) error
func (s *InteractiveService) ResumeQuotaGate(ctx context.Context, runID, decisionID, optionID string) error
```

## 7. Test Signatures

- `TestTask449_ManualFlowGates`
- `TestTask449_ManualVibeGates`
- `TestTask449_AutoFlowRotatesExactCandidate`
- `TestTask449_AutoVibeRotatesExactCandidate`
- `TestTask449_AutoUnknownQuotaGates`
- `TestTask449_RotationCreatesNewLeg`
- `TestTask449_NoSelectionNoProviderCall`
- `TestTask449_RestartRestoresPendingGate`
- `TestTask449_RestartDoesNotReplayCommittedRotation`
- `TestTask449_StopEndsStructured`
- `TestTask449_PressureReset_HeadroomFail_EscalatesToRouting`
- `TestTask449_PostCompactionLegOffersRotate`
- `TestTask449_BudgetTrigger_ExtendNeverAuto`

## 8. Acceptance Check

- The same exhausted fixture parks Flow and Vibe in manual mode; auto mode
  rotates an exact candidate into one new leg; restart never duplicates call.

## 9. Out of Scope

- UI layout/settings implementation (Task-450).
- Automatic base step-definition mutation.

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 additive + recovery tests green
- [ ] Manual Flow/Vibe gate parity proven
- [ ] Auto uncertain inputs always gate
- [ ] New-leg/CAS/restart invariants proven
- [ ] CA ledger + feature key entries complete
- [ ] GitNexus detect_changes reviewed before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
