# CA-747 — Task-324: bug-plan-harness (task-harness clone writing BUG.md) + task investigate note + picker dedup

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-324
change_type: feature
summary: new bug-plan-harness flow (investigate-first plan loop routing plan_md to 09-BugFix) with pack tests and inventory bump, optional investigate note in plan-task.md, rag-harness removed from flow picker
# --->8---

## Two-tier bug strategy (operator decision 2026-09-06)

- Clear bug, no plan → 9-step `bug-harness` (unchanged runtime).
- Bug needing investigation + plan → NEW `bug-plan-harness`: task-harness clone whose plan loop writes `requirements/09-BugFix/todo/BUG-*.md`.
- Naming verdict (no re-debate): NO id rename — beside `bug-plan-harness`, `bug-harness` already reads as its light sibling; display labels were already purpose-matched. The irritant was the duplicate `rag-harness` picker entry.

## Change (pack-only, zero runner code)

- `flows/bug-plan-harness.yaml` (new, 12 nodes): id/description/header swapped; `plan_writer` → `prompts/plan-bug.md`, `plan_reviewer` → `prompts/review-bug-plan.md`; both `plan_md` bindings' `pathTemplate` → `requirements/09-BugFix/todo/BUG-{{idx}}-{{slug}}.md`. Same cap:3/policy/contexts/tools/acceptance/edges; freeze still dominates agent.code writers (CP-55); plan_writer stays agent.delegate pre-freeze (Task-305 deviation).
- `prompts/plan-bug.md` (new): investigate-first (symptom → repro → root cause with file:line evidence, hypotheses labeled with disproofs) then BUG doc contract (Metadata / AI Quick View / Evidence / Root Cause / Fix direction per BUG-357 shape); document-writer scope guard mirroring plan-task.md.
- `prompts/review-bug-plan.md` (new): BUG gate (verifiable symptom, evidenced root cause, actionable fix with DeclaredPaths, feature_key, R3 coverage, CA non-contradiction, BUG contract sections, additive tests). No Task-isms.
- `manifest.yaml`: flow entry (`selectableIn:[flow]`, `cloneable:true`) + 2 prompt entries.
- `plan-task.md`: short OPTIONAL "Investigation pre-work" subsection pointing at plan-bug.md for full investigations.
- Picker dedup: `rag-harness.yaml` + manifest `selectableIn` → `[]` (sync per BUG-NOTE-CP42 #22; `chatBaseline:true` kept); `bug-harness.yaml` description polish ("clear bug, no plan needed").
- Desktop `HARNESS_LABELS` += `bug-plan-harness` ("Bug + Plan").

## Verification

- New `TestBugPlanHarnessPack` PASS (loads, validates, picker/cloneable/cap:3, 12 nodes, non-plan nodes + edges + acceptance DeepEqual task-harness, bug prompts + BUG templates pinned).
- `TestLoadBuiltinPack` (10→11, the feature's own assertion), `TestBugHarnessPackClone` PASS; full `agentpack/...` green.
- Runner `TestHarnessDocWriter*` + `TestHarnessTemplated*` PASS after the plan-task.md note.
- Zero pre-existing test edits. BUG-357 recorder covers the new `plan_md` binding with no code change (required OUTPUT paths).
- Live operator items: `/flow` picker visual + Supabase mirror row for bug-plan-harness (auto via EnsureBuiltinFlowMirrorsWithStore on runner start).
