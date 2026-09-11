# CA-837 — Task-334: promptpacker budget packer behind opt-in flag (CP-23 Phase 1)

# ---8<--- flowpilot:change-ledger
feature_key: runtime-intelligence
source_doc_id: Task-334
change_type: feature
summary: add internal/promptpacker (priority-ordered token budget packing, fence-aware dedup, JSONL prompt-context audit, compact skill cards) wired into runTurn behind FLOWPILOT_ENABLE_BUDGET_PACKER (default OFF, byte-identical)
# --->8---

## Why

CP-23 Phase 1: long Vibe/task sessions balloon the prompt with raw history, full skill files, and duplicate context — costing tokens and diluting model attention. The packer enforces the CP-23 priority order (system contract > current task > mandatory docs > memory summaries > raw excerpts > compact skills), prunes lowest-priority content first, and audits every selection/drop.

## Change

- `internal/promptpacker/` (new, stdlib-only): `SectionKind`/`PromptSection`/`SectionBudget`/`PackerOptions`/`DefaultPackerOptions` (8000 total; memory 30% / excerpts 20% / skills 10%), `PackPrompt` (validate → dedup → stable priority sort → per-kind caps → global prune lowest-priority-first; mandatory kinds retained whole with over-budget warning), `DeduplicateContext` (3-line sliding-window fingerprints, normalized, fence-aware, keeps higher-priority copy), `WriteAuditLog` (JSONL with dropped items + reasons), `CompactSkillCard` (`## Always Do`/`## Core Rules`, fallback first bullets, <200 tokens), `EstimateTokens` (~4 chars/token).
- `runner/interactive_service.go` (+190/−0, purely additive): seam `applyBudgetPackerIfEnabled` in `runTurn` after flow-context + feature-history injection, before `logComposedPrompt`; env flag `FLOWPILOT_ENABLE_BUDGET_PACKER` (repo's existing env-flag pattern) default OFF → original prompt returned untouched (byte-identical); flag ON → sectionize → PackPrompt → audit JSONL under `.flowpilot/runs/<project>/<run>/prompt-context-audit-<turn>.jsonl`; pack failure falls back to the original prompt. GitNexus impact `runTurn`: LOW.

## Tests

`packer_test.go`: all 8 Task-334 §10 signatures exact-name + `TestDefaultPackerOptions_CPSliceBudget`. `task334_budget_packer_integration_test.go` (runner): flag-off byte-identity, flag-on prune+audit (369,398 → 6,872 bytes demo, task + canonical head retained), section classification, fence atomicity. `go test ./internal/promptpacker/...` and targeted runner/flowgate/docscan suites all green; gofmt/vet clean.

## Providers

Case 1 agnostic: stdlib-only Go, heuristic estimator, 0 LLM; the seam sits in the provider-agnostic prompt assembly and flag-OFF is a byte-identical passthrough — identical for Claude/Codex/Grok by construction (pinned by test).

## Prior claims intact

CA-833..CA-836, CA-695, CA-442, CA-441 — untouched; new leaf package + additive seam only. Review non-blockings recorded in Task-334 Completion Notes (classifier default for unrecognized blocks, per-section vs aggregate caps, byte-based estimator for Vietnamese text).
