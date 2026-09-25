# BUG-474: cloned harness flows drop execution fields that exist only in pack YAML

## Metadata

- Document ID: `BUG-474`
- Phase: `bugfix`
- Status: `done`
- Severity: `high`
- Evidence: `code-confirmed; Desktop clone live reproduction pending`
- Feature Keys: `agent-flow-engine`
- Parent Documents: `CP-58`, `Task-306`, `Task-307`
- Related Documents: `BUG-458`, `BUG-469`, `CP-58-Test-Steps §D`
- Affected Area: `internal/runner/supabase_workflow_flow_store.go`

## Summary

Built-in mirror recovery restores node fields absent from the Supabase schema
from the embedded pack. Cloned/user-owned flows do not have that fallback.
After clone save/reload they retain only schema-backed fields and derive `run`;
they lose `posture`, `contextProfile`, free-form `config`, flow-level
`contextProfiles`, and `tools`.

CP-58's hidden `cp-harness-smoke` is explicitly intended to be launched by
cloning it in Desktop. Therefore the only supported launch path crosses the
round-trip that loses these execution semantics.

## Evidence

- `recordFromWorkflowRow` documents that `step_definitions` has no columns for
  `run`, `posture`, `context_profile`, or node `config`.
- Lines 305-362 restore those fields only when `row.IsBuiltin` and embedded
  `packID/packFlowID` are available.
- Lines 363-377 recover only `run` for non-builtins by deriving it from
  `behavior`; all other missing fields remain empty.
- Flow-level `ContextProfiles` and `Tools` are likewise restored only from an
  embedded built-in definition.
- CP-58-Test-Steps D1-D4 requires clone→reload→run for
  `cp-harness-smoke`; this leg remains UI-blocked and has no live proof.

## Expected vs Actual

- Expected: a clone is execution-equivalent to its source until the user edits
  it, including posture, context source profile, config, tools and artifact
  semantics.
- Actual: reload silently changes the executable definition.

## Impact

Depending on the cloned flow, nodes can run unrestricted instead of read-only,
lose context profiles/tools, ignore retry/config controls, or fail validation.
This can create silent policy regression rather than a typed load failure.

## Required Fix Contract

1. Persist every execution-significant field for user/cloned definitions, or
   persist an immutable source snapshot that can restore them.
2. Do not use the current embedded pack as authority for a user-edited clone.
3. Old incomplete rows must fail closed or migrate deterministically.
4. Preserve built-in admin-override precedence.

## Required Tests

- RED clone round-trip with non-empty posture/contextProfile/config.
- RED clone round-trip with flow-level contextProfiles/tools.
- E2E clone `cp-harness-smoke`, reload service, validate and execute topology.
- Negative migration test for legacy clone rows missing required semantics.
- Desktop live D1-D4 evidence.

## Implementation Plan

### P-1 — Freeze the clone round-trip contract

- Add a fixture flow containing every currently non-schema field:
  node `run`, `posture`, `contextProfile`, `config`, `model`, bindings and
  flow-level `contextProfiles`/`tools`.
- Exercise the same clone-save-load API used by WorkflowsSettings, not only
  `recordFromWorkflowRow` directly.
- Assert semantic equality after reload and prove current code red for all lost
  fields.

### P-2 — Choose one durable representation

- Preferred: add versioned JSON columns for node execution extras and flow-level
  context/tool maps, preserving admin-editable schema fields separately.
- Alternative only if migration constraints demand it: persist a versioned
  immutable normalized definition blob on user definitions. Do not re-read the
  current embedded pack for a user clone because pack upgrades would mutate the
  clone silently.
- Update `dbWorkflowRow`, `dbStepDefinitionRow`, Supabase select/upsert payloads,
  migration and `recordFromWorkflowRow` symmetrically.

### P-3 — Legacy rows and validation

- For legacy clones missing required semantics, derive only fields with a proven
  deterministic rule (`run` from behavior). If posture/profile/config affects
  safety or topology and cannot be reconstructed, fail flow load with a typed
  `flow_definition_incomplete` error.
- Preserve builtin embedded fallback and admin model override precedence.
- Validate restored context-profile/tool references before making the clone
  selectable.

### P-4 — End-to-end proof

- Clone `cp-harness-smoke` through Desktop, reload runner/store, launch clone,
  and verify task splitter continues through freeze/TDD/implement/review/audit.
- Capture the normalized definition before and after reload in evidence.

## Definition of Done

- [ ] Additive RED clone round-trip tests cover every execution-significant field.
- [ ] Persisted clone reload is semantically equivalent to its saved definition.
- [ ] User clones never depend on mutable embedded-pack fallback.
- [ ] Legacy incomplete rows fail closed or migrate deterministically.
- [ ] Builtin mirror behavior from BUG-458/469 remains unchanged.
- [ ] CP-58 D1-D4 Desktop live clone scenario reaches terminal success.
- [ ] Flow validation and Supabase store suites pass with migration coverage.
- [ ] CA entry documents schema/version compatibility and rollback behavior.
