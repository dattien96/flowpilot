# CA-971 — Task-441: maxTokens → maxEstPromptTokens hard rename

## Summary

`ContextProfile.maxTokens` was ambiguous: it bounded the *estimated* prompt
length fed to the packer, while CP-86 introduces `maxUsageTokens` for *real*
provider-reported usage. Two budgets with one vague name was a bug farm —
renamed so the schema says what it means.

## What changed

- `internal/agentpack/pack.go` — `ContextProfile.MaxTokens` →
  `MaxEstPromptTokens` (yaml `maxEstPromptTokens`); legacy `maxTokens` key
  in a context profile now fails flow load **closed** (decode is non-strict,
  so an explicit unknown-key check was added — silent acceptance would have
  dropped the cap entirely).
- `internal/agentpack/flow-pack/flows/task-harness.yaml` — key renamed in the
  built-in flow's context profiles.
- `internal/runner/context_profile.go` — `flowNodeProfileBudgetFor` reads the
  renamed field.
- Intentionally NOT renamed: `promptpacker.SectionBudget.TotalMaxTokens` —
  a different budget (section packing), per task scope.
- New tests: `internal/agentpack/task441_maxest_rename_test.go` (parses /
  legacy fails closed / all built-in flows load) +
  `internal/runner/task441_maxest_rename_test.go` (budget resolver reads the
  new field).

## Tests

- `go test ./internal/agentpack/ -run TestTask441` — green.
- `go test ./internal/runner/ -run TestTask441` — green.
- `gitnexus_rename` returned 0 edits — the index does not track struct-field
  references; rename done by hand after impact analysis + exhaustive grep
  (4 reference sites). LOW blast radius.

# ---8<--- flowpilot:change-ledger
feature_key: token-usage
source_doc_id: Task-441
change_type: refactor
summary: ContextProfile.maxTokens → maxEstPromptTokens hard rename; legacy key fails flow load closed; promptpacker TotalMaxTokens untouched (different budget)
# --->8---
