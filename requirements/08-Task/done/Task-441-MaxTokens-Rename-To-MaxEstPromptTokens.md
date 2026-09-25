# Task-441: Rename maxTokens → maxEstPromptTokens

- Document ID: `Task-441`
- Title: `Hard rename the context-profile prompt budget key so "tokens" unambiguously means provider-reported usage`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86`, `SD-10`, `CP-62`
- Child Documents: ``
- Related Documents: `CP-62 Task-341` (context profiles origin), `CP-23 Task-334` (packer that consumes the budget)
- Replaces: ``
- Tags: `context`, `naming`, `schema`, `context-profiles`

## AI Quick View

### Summary

- `contextProfiles.*.maxTokens` caps the **estimated prompt length**
  (bytes/4 heuristic, pre-send) — it is not and never was provider-reported
  token usage. The name actively misleads: Task-442 introduces
  `maxUsageTokens` which IS real usage, so the est field must be renamed
  first or the pair is unreadable.
- Hard rename, no alias: all builtin flows are repo-owned and updated in the
  same pass; the legacy key fails flow load loudly instead of being silently
  ignored.

### Current Ask

- Rename YAML key `maxTokens` → `maxEstPromptTokens` and Go field
  `ContextProfile.MaxTokens` → `MaxEstPromptTokens` across pack parsing,
  the budget seam, all builtin flow YAMLs, tests, and doc references — in
  one atomic pass, using `gitnexus_rename` for the Go symbol.

### Key Decisions

- `T-1` Hard rename with fail-closed parse: `maxTokens` in a flow YAML
  errors at load (unknown key surfaces immediately), never silently
  ignored — a misnamed cap is worse than a loud failure.
- `T-2` `gitnexus_rename` for `ContextProfile.MaxTokens` — repo rule, never
  find-and-replace a symbol.
- `T-3` Semantics unchanged: still the pre-send estimate cap consumed by
  `flowNodeProfileBudgetFor` → packer `TotalMaxTokens`. Only the name moves.

### Constraints

- Mechanical fixture updates to existing tests are allowed (key renames,
  struct field renames) — weakening assertions is not (oracle-rule).
- Every builtin flow YAML using `maxTokens` must be updated in the same
  commit — a flow left on the old key must fail load.
- Doc references (CP-62/SD-10/Task-341 comments mentioning `maxTokens`)
  updated where they describe the live key.

### Open Questions

- None — user picked `maxEstPromptTokens` over `maxPromptTokens` /
  `maxPromptLength`.

### Source Refs

- `CP-86 P-2`, `agentpack/pack.go` (`ContextProfile`, parse at `maxTokens`),
  `context_profile.go` (`flowNodeProfileBudgetFor` reads `.MaxTokens`),
  `agentpack/flow-pack/flows/task-harness.yaml` + `vibe-sprint.yaml` +
  `bug-plan-harness.yaml` (all declare `maxTokens`).

## 1. Goal

One unambiguous vocabulary: `*EstPrompt*` = pre-send estimate caps;
`*Usage*` (Task-442) = provider-reported consumption. No field named
"tokens" may describe a prompt-length estimate.

## 2. Parent Links

- coding plan: `CP-86` (P-2)
- tech design: `SD-10`
- system spec: `SS-22`
- specific upstream ids: `CP-62 Task-341`, `CP-23 Task-334`

## 3. Trigger

`maxUsageTokens` (Task-442) cannot land next to a field called `maxTokens`
that measures something else — operator and AI readers will conflate the
two budgets.

## 4. Exact Change

- `T-1` `agentpack/pack.go`: `ContextProfile.MaxTokens` →
  `MaxEstPromptTokens`; parse `intField(pm, "maxEstPromptTokens")`; a map
  containing legacy `maxTokens` (and no new key) fails with a descriptive
  error naming the renamed key.
- `T-2` `runner/context_profile.go`: read `profile.MaxEstPromptTokens`.
- `T-3` All builtin flow YAMLs: `maxTokens:` → `maxEstPromptTokens:`.
- `T-4` Comments/docs naming the live key updated
  (`context_profile.go` doc block, pack.go ContextProfile doc, flow YAML
  comments, Task-341 references where they describe the key literally).

## 5. Touched Areas

- files: `internal/agentpack/pack.go`,
  `internal/runner/context_profile.go`,
  `internal/agentpack/flow-pack/flows/*.yaml` (all with `maxTokens`),
  `internal/runner/context_profile_test.go`,
  `internal/runner/knowledge_flow_profile_test.go`,
  `internal/runner/task334_budget_packer_integration_test.go` (fixture keys
  if present), related doc references
- modules: `agentpack`, `runner`
- routes: none
- tables: none

## 6. Code Guide Signatures

```go
// internal/agentpack/pack.go
type ContextProfile struct {
    Name             string
    CandidateSources []string
    // MaxEstPromptTokens caps the ESTIMATED prompt length (bytes/4
    // heuristic, pre-send). It is not provider-reported usage — see
    // MaxUsageTokens (Task-442) for the real-token budget.
    MaxEstPromptTokens int
}
```

```go
// internal/agentpack/pack.go — parse block
def.ContextProfiles[name] = ContextProfile{
    Name:               name,
    CandidateSources:   stringSliceField(pm, "candidateSources"),
    MaxEstPromptTokens: intField(pm, "maxEstPromptTokens"),
}
// + explicit check: map has "maxTokens" && !has "maxEstPromptTokens" → error
```

```go
// internal/runner/context_profile.go — unchanged signature, renamed field read
return profile.MaxEstPromptTokens
```

## 7. Test Signatures

- `TestTask441_MaxEstPromptTokens_Parses` — YAML with new key → profile
  carries value (covers AC: new key works)
- `TestTask441_LegacyMaxTokens_FailsFlowLoad` — YAML with only `maxTokens`
  → load error naming `maxEstPromptTokens` (covers T-1 fail-closed)
- `TestTask441_ProfileBudget_UsesRenamedField` —
  `flowNodeProfileBudgetFor` returns the new field's value (covers T-3
  semantics preserved)
- `TestTask441_AllBuiltinFlows_ParseClean` — every flow in
  `LoadBuiltinPack` parses with zero `maxTokens` keys remaining (covers
  atomic rename)

## 8. Acceptance Check

- `grep -rn "maxTokens" apps/local-runner` returns only the legacy-detection
  error path + comments explaining the rename.
- All existing flows load; `flowNodeProfileBudgetFor` returns the same
  values as before (6000/16000/12000/24000 in task-harness).

## 9. Out of Scope

- Changing budget values or packer semantics.
- `maxUsageTokens` implementation (Task-442).
- Any alias/back-compat parse for `maxTokens`.

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (existing tests updated only
      mechanically for the rename — documented, not weakened)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [x] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: hard rename: ContextProfile.MaxTokens -> MaxEstPromptTokens, YAML key maxTokens -> maxEstPromptTokens; legacy key fails flow load closed (non-strict decode needed explicit rejection). promptpacker TotalMaxTokens intentionally untouched (different concept). gitnexus_rename returned 0 edits (struct-field refs untracked) — manual rename after impact analysis + exhaustive grep. 4/4 task tests green.
- follow-ups: consumers: flowNodeProfileBudgetFor + flow pack YAML
- upstream docs updated: CP-86, CP-86-Test-Steps
