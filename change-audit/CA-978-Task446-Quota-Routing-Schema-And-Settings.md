# CA-978 — Task-446: quota routing schema & settings

## Summary

CP-87's routing contract lands as data: flow nodes declare a workload class,
and a machine-global `quota-routing.json` carries the user-owned rotation
policy (manual by default). The runner snapshots that policy into every new
run so a mid-run settings edit cannot change the rules an active run executes
under. No routing decisions execute yet — Task-447+ consumes this schema.

## What changed

- `agentpack/pack.go` — `FlowNode.WorkloadClass` (YAML `workloadClass`),
  `WorkloadClass` type with exactly three values (`scan`, `high_reasoning`,
  `coding`), `ValidWorkloadClass`, `ProviderBackedBehavior` (agent.delegate /
  agent.code / agent.reproduce / agent.scaffold), and
  `ValidateFlowWorkloadClasses`. `ValidateFlowDefinition` now fails closed on
  an unknown class value or a class attached to a non-provider node;
  `LoadBuiltinPack` additionally requires every provider-backed builtin node
  to be classified (pack-author contract). Stored/user-authored flows are
  additive — a missing class stays legal outside the builtin pack.
- Builtin flow-pack YAMLs — all 52 provider-backed nodes across 13 flows
  annotated: readers/ingesters/scouts → `scan`, planners/reviewers/debaters/
  slicers → `high_reasoning`, coders/scaffolds/reproducers → `coding`.
- `runner/quota_routing.go` (new) — `QuotaRoutingSettings` (mode,
  providerPriority, modelBindings, headroomLowPercent, telemetryTtlSeconds,
  sameProviderCooldownSeconds), `ModelClassBinding` (providerKey +
  agentpack.WorkloadClass + model), `QuotaRoutingSnapshot` (frozen copy +
  policyVersion), `ProviderAccountSummary` + `NormalizeAccountHeadroom`
  (min of declared quota windows; unknown/stale/exhausted/low/healthy;
  confidence exact|none — never token balances), machine-global persistence
  beside chat-posture.json (`FLOWPILOT_QUOTA_ROUTING_FILE` override for
  tests), fail-closed normalization (unknown mode → manual, out-of-range
  thresholds → defaults), and GET/PUT handlers.
- `interactive_handlers.go` — `GET/PUT /client/quota-routing-settings`
  (runner-owned SSOT; Desktop/TUI never write the file).
- `interactive_service.go` / `interactive_resume.go` / `workflow_store.go` —
  `interactiveRun.quotaRouting` + `ProviderSessionState.QuotaRouting`: the
  settings snapshot is taken at run creation, persisted, and restored on
  resume/replay.
- Desktop — `contract.ts` gains `QuotaRotationMode`, `WorkloadClass`,
  `ModelClassBinding`, `QuotaRoutingSettings` and optional client methods;
  `HttpWsRunnerClient` + `MockRunnerClient` implement them; `store.ts` gains
  `quotaRoutingSettings` state + `load`/`save` actions (fail-open on load,
  propagate on save).

## Evidence

- `task446_workload_class_test.go` — valid/invalid classes, non-provider
  misuse rejected, YAML parse, builtin pack fully classified.
- `task446_quota_routing_test.go` — headroom normalization matrix (unknown/
  stale/exhausted/low/healthy, min-window rule), manual default,
  settings file round-trip, run snapshot captured+persisted.
- `quotaRouting.test.ts` — desktop store round-trips mode/priority/bindings
  through the mock client; asserts no token-budget field leaks onto the
  quota settings shape.
- Focused: `go test ./internal/agentpack/ -run 'TestTask446|TestLoadBuiltinPack'`
  and `go test ./internal/runner/ -run 'TestTask446'` green; compiled desktop
  phase1 test green.
- CA-977 follow-up in this commit: five `store.test.ts` quota-surface tests
  still drove the deleted string-classifier path via `turn_failed` text
  (masked because `npm run test:phase1`'s directory args fail under the
  phase1-runtime resolver on this machine, so the suite never executed).
  They now emit the typed `provider_limit_reached` event before
  `turn_failed`, matching production order; assertions unchanged.
- GitNexus: `gitnexus_detect_changes` is an MCP-only tool with no CLI/MCP
  server in this session — equivalent check performed via full-diff review
  plus pre-edit `impact` on the modified symbols (RegisterInteractiveRoutes,
  ValidateFlowDefinition, ProviderSessionState, createRun — all CRITICAL
  blast radius; changes are strictly additive).
- Remaining suite failures are pre-existing/environmental: localStorage-less
  store tests, gitnexus auto-index TempDir races, MCP/env, provider inventory
  4-vs-6, and pre-branch drift (history-replay ordering vs spawn-clamp,
  message_completed `text` nil-deref in timelineReducer, stop-interrupt
  child scope, jira integration fixture, health-payload mapping).

## Follow-ups

- Task-447 consumes `QuotaRoutingSnapshot` + `NormalizeAccountHeadroom` for
  same-provider account preflight and per-leg pinning; Task-448 adds the
  cross-provider candidate resolver; Task-450 surfaces the settings UI.

Note: Task-446 §10 names feature keys `token-usage` + `runtime-intelligence`;
the ledger block carries the dominant key `ai-providers` (CP-87 convention, as
with CP-86's two-key DoD resolving to dominant `token-usage`).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-446
change_type: feature
summary: workload-class schema on FlowNode (scan/high_reasoning/coding) with builtin-pack enforcement; machine-global quota-routing settings (manual default) persisted + GET/PUT API; per-run policy snapshot; normalized account headroom; desktop contract/client/store wiring
# --->8---
