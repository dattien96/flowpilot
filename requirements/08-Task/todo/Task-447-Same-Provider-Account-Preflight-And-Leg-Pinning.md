# Task-447: Same-Provider Account Preflight & Leg Pinning

- Document ID: `Task-447`
- Title: `Resolve hub/child demand, select healthy same-provider accounts safely, and pin account per leg`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-87`, `Task-445`, `Task-446`
- Child Documents: ``
- Related Documents: `Task-320`, `SetActiveAccount`, provider process scopes
- Replaces: ``
- Tags: `quota`, `account-pool`, `leg`, `cooldown`, `concurrency`

## AI Quick View

### Summary

- Preflight must resolve the actual target: main hub reads pinned run/leg;
  child reads the effective model from existing node precedence, derives
  provider, then chooses an account.
- Existing `SetActiveAccount` is global and cancels all in-flight turns — it is
  forbidden for automatic routing. Selected account is pinned to a new leg.
- Same-provider account switches are bounded/cooldown-protected to avoid rapid
  cycling multiple accounts on one IP: minimum **20s** and maximum two automatic
  switches per run, then user gate. The backend exposes `cooldownStartedAt`,
  `cooldownUntil`, and the stable safety reason so Desktop/TUI can render a
  synchronized countdown bar instead of an unexplained delay.

### Current Ask

- Implement effective demand + same-provider resolver, durable account claim/
  reservation, cooldown/cap, and per-leg account scoping without interrupting
  unrelated runs.

### Key Decisions

- `T-1` Same-provider healthy account always precedes cross-provider search.
- `T-2` Unknown quota is not auto-eligible; it remains a manual candidate.
- `T-3` Recheck account state immediately before provider call after claim.
- `T-4` Changing account creates a new provider leg/session; no in-place session
  mutation and no global active-account switch.

### Constraints

- Selection and claim must be durable/restart-safe and serialized per provider
  account; never overbook through concurrent preflights.
- Respect reset time and typed limit kind; billing-required accounts are blocked.
- In-flight turns are never killed for proactive routing.

### Open Questions

- Account reservation weight can initially be workload-class units rather than
  estimated token quantities; exact quota capacity is unavailable.

### Source Refs

- `CP-87 P-3/P-4`, Task-320 resolution precedence,
  `interactive_service.go:SetActiveAccount`, AGENTS §2 pinned provider legs.

## 1. Goal

Safely use available accounts within the requested provider before changing
provider, without global side effects or rapid account cycling.

## 2. Parent Links

- coding plan: `CP-87 P-3`
- tech design: `SD-07`
- system spec: `SS-06`
- specific upstream ids: `Task-320`, AGENTS §2

## 3. Trigger

Current candidate selection lives in Desktop and activates a machine-global
account, which is incompatible with parallel Flow/Vibe runs.

## 4. Exact Change

- `T-1` Build `ExecutionDemand` for hub and child node.
- `T-2` Rank same-provider connected accounts by headroom/freshness/slot;
  return auto-eligible vs manual-only candidates.
- `T-3` Add durable account claim + immediate pre-call recheck.
- `T-4` Enforce exactly 20s cooldown + max two automatic same-provider
  switches/run. Persist server-authoritative `cooldownStartedAt` /
  `cooldownUntil` and reason code `same_provider_ip_safety`; publish them in
  the gate/rotation projection so clients render one synchronized countdown.
- `T-5` Pin selected account in new leg/process scope; never call global
  `SetActiveAccount` from resolver.

## 5. Touched Areas

- files: new `quota_preflight.go`, `quota_claim.go`, flow spawn/turn admission,
  provider process/session scope keys, durable run/leg store
- modules: `runner`
- routes: none
- tables: existing durable local run records; no Supabase mutation

## 6. Code Guide Signatures

```go
type ExecutionDemand struct {
    RunID string; NodeID string; IsMainHub bool
    RequestedProvider ProviderKey; RequestedModel string
    RequestedAccountID string; WorkloadClass WorkloadClass
    MaxUsageTokens int64
}
type AccountCandidate struct {
    AccountID string; ProviderKey ProviderKey; Headroom AccountHeadroom
    AutoEligible bool; RejectionReasons []string
    CooldownStartedAt string; CooldownUntil string
    CooldownReason string // same_provider_ip_safety
}
func (s *InteractiveService) ResolveExecutionDemand(ctx context.Context, parentRunID string, node *agentpack.FlowNode) (ExecutionDemand, error)
func (s *InteractiveService) SameProviderCandidates(ctx context.Context, demand ExecutionDemand) ([]AccountCandidate, error)
func (s *InteractiveService) ClaimAccountForLeg(ctx context.Context, demand ExecutionDemand, candidate AccountCandidate) (LegBinding, error)
```

## 7. Test Signatures

- `TestTask447_HubDemandUsesPinnedLeg`
- `TestTask447_ChildDemandUsesResolvedNodeModel`
- `TestTask447_HealthySameProviderAccountWins`
- `TestTask447_UnknownQuotaManualOnly`
- `TestTask447_CooldownBlocksRapidSwitch` — no provider call before exactly
  20s has elapsed
- `TestTask447_CooldownPublishesStableTimestamps` — start/until/reason survive
  projection, refresh, and restart without resetting the deadline
- `TestTask447_MaxTwoAutoSwitchesThenGate`
- `TestTask447_NoGlobalSetActiveAccount`
- `TestTask447_ConcurrentClaimsDoNotOverbook`
- `TestTask447_RestartPreservesClaimAndCooldown`

## 8. Acceptance Check

- Two parallel runs can use different accounts of one provider without one
  switch cancelling the other; rapid third switch gates the user.

## 9. Out of Scope

- Cross-provider model candidate selection (Task-448).
- Candidate table UI (Task-450).

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 additive + race tests green
- [ ] No automatic path calls SetActiveAccount
- [ ] Cooldown/cap/claims durable across restart
- [ ] CA ledger + feature key entries complete
- [ ] GitNexus detect_changes reviewed before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
