---
feature: context-regression-engine
change_type: feature
refs: Task-112, CP-35
---

# CA-127: Requirements Scaffold On Bind

## What changed

Added `apps/local-runner/internal/reqscaffold/` — a new package that embeds the five
FORMAT-REFERENCE-*.md files (SS, SD, CP, Task, BugFix) and scaffolds
`requirements/05-09` subfolders in any bound project that does not already have them.

The scaffold runs as a new `req_scaffold` step in `runEngineInit` (engine_setup.go),
immediately after `tooling_check`. It is non-fatal, idempotent, and never overwrites
existing FORMAT-REFERENCE files.

**Files**:
- `apps/local-runner/internal/reqscaffold/scaffold.go` (new)
- `apps/local-runner/internal/reqscaffold/scaffold_test.go` (new)
- `apps/local-runner/internal/reqscaffold/scaffold-pack/**` (5 embedded FORMAT-REFERENCE files)
- `apps/local-runner/internal/runner/engine_setup.go`

## Why

Without this, binding a fresh project left it unable to use the phase-document workflow
(SS → SD → CP → Task → BugFix): the AI would find no FORMAT-REFERENCE files and either
produce free-form documents or fail compliance checks. Embedding them in the runner
(same pattern as the skill pack) keeps the setup self-contained.

## Invariants preserved

- Existing FORMAT-REFERENCE files are never overwritten (install-once semantics).
- The scaffold failure does not abort the rest of the init sequence.
- The skill-pack embed pattern (`//go:embed`) is reused exactly.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: TASK-112
change_type: feature
summary: Requirements Scaffold On Bind
# --->8---
