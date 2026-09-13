# CA-853 — CP-62 P-5: per-node context profiles + catalog tier layered on the Budget Packer

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: Task-341
change_type: task
summary: flows declare contextProfiles (candidate source set + maxTokens); nodes reference them by name — new source tier between artifact bindings (still highest, CP-45) and node sources; profile maxTokens refines the Task-334 packer budget when packing is active (never turns the packer on); catalog tier appends one line per pruned section so the model keeps a metadata index of dropped bodies; both flow families covered (task-harness/bug-plan-harness + vibe-sprint), everything else falls back byte-equal
# --->8---

## Why

Every node ate the same main_context package (canonical head + history + contract + dependence + chat summary + excerpts) — reviewers paid for dependence chains they never read, scouts paid for excerpts. CP-23's Budget Packer had one global priority list; per-node candidate sets were the missing delta (CP-62 D-5).

## Change

- `agentpack/pack.go`: `ContextProfile{Name, CandidateSources, MaxTokens}`; `FlowDefinition.ContextProfiles` (fail-closed parse of `contextProfiles:`); `FlowNode.ContextProfile` (`contextProfile:`).
- `runner/context_sources_builtin.go`: profile tier inside `resolveEnabledContextSourceIDs` (artifact-bound > profile > node sources > default — all existing call sites inherit); `ValidateFlowContextSources` fail-fast on unknown profile refs and unknown profile sources.
- `runner/context_profile.go` (new): `flowNodeProfileBudgetFor` (run → parent node → builtin pack profile); `buildCatalogSummary` (one line per dropped item, never the bodies).
- `runner/interactive_service.go`: `applyBudgetPackerIfEnabled` — profile budget refines `budget.TotalMaxTokens` (narrower wins, drift-narrow may halve further) + appends the catalog after a successful pack. Packing stays flag-gated byte-identical.
- Flow YAMLs: profiles + node refs on task-harness, bug-plan-harness (scout 6k / plan_writer 16k / reviewer 12k / coder 24k) and vibe-sprint (scout/coder/tdd).

## Tests

`runner/context_profile_test.go` (7): candidate-set resolution, no-profile fallback (default + node-sources), artifact-binding-wins (CP-45 untouched), unknown profile ref/source fail-flow-load ×2, token cap through PackPrompt (mandatory kinds whole per CP-23 R-1 — small task, big prunables), catalog one-line-per-item + empty case, run→profile budget resolution incl. root-run zero. agentpack + promptpacker suites green.

## Providers

Case 1 provider-agnostic — profiles are pack data consumed by the runner's prompt assembly; no adapter logic.

## Prior claims intact

CA-837/CA-841 (Task-334 flag-OFF byte-identity) — the profile never enables packing; CP-45 artifact-binding precedence preserved and pinned; CA-852 postures untouched; BUG-344/BUG-365 lines untouched. Note: profiles live per-flow (YAML `contextProfiles:`) instead of inside the shared flow-context-package artifact template — that file is a common artifact, not per-flow data; the CP-62 P-5 contract is unchanged.
