# Task-448: Cross-Provider Workload-Class Routing

- Document ID: `Task-448`
- Title: `Dynamically evaluate other providers using workload class, capability, context-window, quota, and configured priority`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-87`, `Task-446`, `Task-447`
- Child Documents: ``
- Related Documents: `Task-320`, provider registry/model catalog
- Replaces: ``
- Tags: `quota`, `provider-routing`, `workload-class`, `capabilities`

## AI Quick View

### Summary

- If no same-provider account qualifies, enumerate all registered providers
  except current; no hardcoded Claude→Codex chain.
- A model is equivalent through explicit workload binding (`scan`,
  `high_reasoning`, `coding`) plus required capabilities/context window — not
  pairwise names.
- Deterministic priority + headroom ranks eligible candidates; unknown/stale
  quota or missing bindings are manual-only.

### Current Ask

- Implement pure candidate generation/ranking with complete rejection reasons,
  new-provider extensibility, and no execution side effects.

### Key Decisions

- `T-1` Filter before score: connected account, explicit class binding,
  provider/model availability, required capabilities, sufficient context window.
- `T-2` Rank: provider priority → healthy/fresh headroom → account slot/model ID
  deterministic tie-break. Current provider is already exhausted in this phase.
- `T-3` Auto eligibility requires exact class binding + known healthy account;
  all uncertainty still appears in manual table with reason.

### Constraints

- Router never contains provider-specific model names or provider lists.
- Quality mapping is user/catalog data; adding provider/model requires config,
  not Go changes.
- Pure resolver: no account switch, leg creation, UI, or DB mutation.

### Open Questions

- Capability set initially reuses `ProviderCapabilities`; add only missing
  normalized capabilities proven necessary by tests.

### Source Refs

- `CP-87 P-6/P-8`, Task-320 `ModelProviderKey`, provider registry/catalog,
  Task-446 model bindings, Task-447 demand/candidates.

## 1. Goal

Produce a deterministic, explainable cross-provider candidate set matching the
node's real workload and runtime requirements.

## 2. Parent Links

- coding plan: `CP-87 P-4`
- tech design: `SD-07`
- system spec: `SS-06`
- specific upstream ids: `Task-320`, `Task-446`, `Task-447`

## 3. Trigger

Same-provider account exhaustion needs provider-neutral fallback without an
unmaintainable pairwise model-equivalence matrix.

## 4. Exact Change

- `T-1` Enumerate provider registry entries where key != current provider.
- `T-2` Resolve preferred model from `(provider, workloadClass)` binding.
- `T-3` Filter model/account by availability, capability, context window,
  headroom and telemetry evidence.
- `T-4` Stable rank and emit accepted/rejected candidates with reason codes.
- `T-5` Expose one result contract consumed identically by manual/auto modes.

## 5. Touched Areas

- files: new `quota_candidates.go`, provider registry/model catalog accessors,
  settings model-binding types
- modules: `runner`
- routes: none
- tables: none

## 6. Code Guide Signatures

```go
type RouteCandidate struct {
    ProviderKey ProviderKey; Model string; AccountID string
    WorkloadClass WorkloadClass; Headroom AccountHeadroom
    AutoEligible bool; Score int; RejectionReasons []string
}
type CandidateSet struct { Eligible []RouteCandidate; ManualOnly []RouteCandidate; Rejected []RouteCandidate }
func (s *InteractiveService) CrossProviderCandidates(ctx context.Context, demand ExecutionDemand, settings QuotaRoutingSettings) (CandidateSet, error)
func RankRouteCandidates(candidates []RouteCandidate, priority []ProviderKey) []RouteCandidate
```

## 7. Test Signatures

- `TestTask448_EnumeratesRegistryExcludingCurrent`
- `TestTask448_WorkloadBindingSelectsModel`
- `TestTask448_MissingBindingGates`
- `TestTask448_CapabilityMismatchRejected`
- `TestTask448_ContextWindowTooSmallRejected`
- `TestTask448_UnknownQuotaManualOnly`
- `TestTask448_DeterministicPriorityTieBreak`
- `TestTask448_NewProviderNeedsNoRouterCodeChange`

## 8. Acceptance Check

- Registering a fake sixth provider plus settings binding makes it eligible
  without editing router source; every rejection is explainable in UI/audit.

## 9. Out of Scope

- Rotation execution and gates (Task-449).
- Pairwise model equality.

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 additive tests green
- [ ] Resolver pure/deterministic/provider-neutral
- [ ] Candidate rejection reasons complete
- [ ] CA ledger + feature key entries complete
- [ ] GitNexus detect_changes reviewed before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
