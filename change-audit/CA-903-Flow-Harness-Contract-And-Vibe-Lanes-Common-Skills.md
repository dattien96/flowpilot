# CA-903: Flow Harness Contract And Vibe Lanes Common Skills

## Summary

- Added `flow-pack/common/flow-harness-contract/SKILL.md`: umbrella harness
  discipline (frozen scope, reproduce-first, contract-first TDD + signature
  lock, machine-checkable done verdict, doc intent ceiling, drift ladder,
  lesson promotion, session checkpoint).
- Added `flow-pack/common/vibe-lanes/SKILL.md`: multi-worktree lane protocol
  (lane registry mutex, shared-package lane, merge order, explicit merge-back,
  checkpoint resume, lost-worktree handling).
- Added `apps/local-runner/AGENTS.md`: contributor-facing invariant card for
  the runner internals (domain-free engine, hub-only routing, durability,
  fail-closed gates, worktree isolation, provider parity).
- Both skills sit in the always-installed `common` group, so `flowpilot init`
  ships them to every project type with no installer change. Shipped content
  contains no CP identifiers — rules are semantic only.

## Files

- `apps/local-runner/internal/skillpack/flow-pack/common/flow-harness-contract/SKILL.md` (new)
- `apps/local-runner/internal/skillpack/flow-pack/common/vibe-lanes/SKILL.md` (new)
- `apps/local-runner/AGENTS.md` (new)
- `requirements/08-Task/done/Task-412-Flow-Harness-Contract-And-Vibe-Lanes-Common-Skills.md` (new)

## Out of Scope

- No Go code changes; `go:embed flow-pack` picks up the new folders.
- CP71 runtime worktree work in the same working tree is untouched.

# ---8<--- flowpilot:change-ledger
feature_key: skill-injection
source_doc_id: Task-412
change_type: feature
summary: add flow-harness-contract + vibe-lanes common skills and local-runner AGENTS.md
# --->8---
