# Task-446: Quota Routing Schema & Settings

- Document ID: `Task-446`
- Title: `Workload classes, normalized headroom, provider priority/model bindings, and manual-or-auto rotation setting`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-87`, `Task-320`, `CP-86`
- Child Documents: ``
- Related Documents: `Task-445`, builtin Flow/Vibe packs
- Replaces: ``
- Tags: `quota`, `settings`, `workload-class`, `model-catalog`

## AI Quick View

### Summary

- Replace fragile pairwise model equivalence with three explicit workload
  classes: `scan`, `high_reasoning` (plan/review/TDD), `coding`.
- Machine-global settings bind provider+class to a preferred model, order
  provider priority, normalize quota headroom, and choose `manual|auto`.
- Account quota percentages/credits are provider-specific and never exposed as
  token balances; CP-86 `maxUsageTokens` informs cost class only.

### Current Ask

- Add schema + validation + persistence, annotate every provider-backed builtin
  Flow/Vibe node, and expose a pure headroom normalizer consumed later by the
  routing engine.

### Key Decisions

- `T-1` Provider-backed nodes require one workload class; inline/control nodes
  must omit it. Validation fails closed.
- `T-2` Rotation mode defaults `manual`, is machine-global (accounts are
  machine-local), and is snapshotted into each run at creation.
- `T-3` Auto selection requires an explicit provider/class model binding;
  missing binding means user gate, never inferred model quality.

### Constraints

- Existing model precedence from Task-320 remains unchanged; workload class
  selects fallback candidates, not the requested model.
- New providers are settings/catalog data, not switch statements in router.
- Quota telemetry includes freshness/source/confidence.

### Open Questions

- Confirm default freshness TTL: proposed 2 minutes.

### Source Refs

- `CP-87 P-2/P-5/P-6`, `agentpack.FlowNode`, Task-320,
  provider-account summary quota fields, Engine/AI Provider settings.

## 1. Goal

Provide the validated data model the quota resolver needs without pretending
cross-provider quota or model quality are directly comparable.

## 2. Parent Links

- coding plan: `CP-87 P-2`
- tech design: `SD-07`, `SD-17`
- system spec: `SS-06`, `SS-22`
- specific upstream ids: `Task-320`, `CP-86 maxUsageTokens`

## 3. Trigger

Cross-provider auto-routing cannot be safe from model names alone and account
percent cannot be compared directly to token demand.

## 4. Exact Change

- `T-1` Add `FlowNode.WorkloadClass`; parse/validate three values; annotate
  provider-backed nodes in all builtin Flow/Vibe packs.
- `T-2` Add machine-global `QuotaRoutingSettings`: mode, provider priority,
  class model bindings, thresholds, TTL, same-provider cooldown.
- `T-3` Snapshot settings/policy version into each new run.
- `T-4` Normalize provider account telemetry into headroom state with evidence.
- `T-5` Settings API round-trip + default migration (`manual`).

## 5. Touched Areas

- files: `agentpack/pack.go`, builtin flow YAMLs, runner config/settings API,
  provider account metadata mapping, Desktop/TUI settings clients
- modules: `agentpack`, `runner`, Desktop settings, TUI client
- routes: settings GET/PUT
- tables: none; machine-local config

## 6. Code Guide Signatures

```go
type WorkloadClass string
const (
    WorkloadScan WorkloadClass = "scan"
    WorkloadHighReasoning WorkloadClass = "high_reasoning"
    WorkloadCoding WorkloadClass = "coding"
)
type ModelClassBinding struct { ProviderKey ProviderKey; WorkloadClass WorkloadClass; Model string }
type QuotaRoutingSettings struct {
    Mode string // manual | auto
    ProviderPriority []ProviderKey
    ModelBindings []ModelClassBinding
    HeadroomLowPercent int
    TelemetryTTLSeconds int
    SameProviderCooldownSeconds int // default 20; persisted setting
}
type AccountHeadroom struct {
    State string // healthy | low | exhausted | unknown | stale
    RemainingPercent *int
    ResetAt string
    Source string
    FreshnessSeconds int64
    Confidence string
}
func NormalizeAccountHeadroom(summary ProviderAccountSummary, now time.Time, settings QuotaRoutingSettings) AccountHeadroom
```

## 7. Test Signatures

- `TestTask446_WorkloadClassValidation`
- `TestTask446_AllProviderNodesClassified`
- `TestTask446_HeadroomNormalization`
- `TestTask446_RotationModeDefaultManual`
- `TestTask446_RunSnapshotsSettings`
- `test("settings edits mode priority and class model bindings")`

## 8. Acceptance Check

- Every provider-backed Flow/Vibe node validates with one class; settings
  restart round-trip preserves values; UI never labels quota percentage tokens.

## 9. Out of Scope

- Account/candidate selection and execution.
- Pairwise model mapping.

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 additive tests green; old tests remain regression guards
- [ ] All builtin packs pass validation
- [ ] Default manual mode migration/restart verified
- [ ] CA ledger + feature key entries complete
- [ ] GitNexus detect_changes reviewed before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
