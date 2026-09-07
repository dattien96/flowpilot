# Task-324: Bug-Plan-Harness (Task-Harness Clone Writing BUG.md) + Task Investigate Note + Picker Dedup

## Metadata

- Document ID: `Task-324`
- Title: `Bug-Plan-Harness (Task-Harness Clone Writing BUG.md) + Task Investigate Note + Picker Dedup`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-06`
- Last Updated: `2026-09-06`
- Parent Documents: [CP-58: Bug / Task / CP Harness](../../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `none`
- Related Documents: [Task-305](../../08-Task/done/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md), [BUG-357](../../09-BugFix/done/BUG-357-Flow-Doc-Writer-Khong-Ghi-Artifact-Instance.md)
- Replaces: `none`
- Tags: `bug-harness, task-harness, plan-loop, investigate, picker-dedup`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Two-tier bug strategy (operator decision 2026-09-06): clear bug, no plan → 9-step `bug-harness`; bug needing investigation + plan → NEW `bug-plan-harness`, a task-harness clone whose plan loop writes `requirements/09-BugFix/todo/BUG-*.md` instead of `08-Task/todo/Task-*.md`.
- Finding that shaped scope: `plan-task.md` has ZERO investigate/root-cause/repro guidance (task-harness plans from given context) — so the new flow needs its own `plan-bug.md` prompt with investigate-first sections, plus a small optional investigate note backported to `plan-task.md` (T-B).
- Naming verdict (recorded, no re-debate): NO id rename. Beside `bug-plan-harness`, `bug-harness` already reads as its light sibling; display labels already purpose-matched ("Bug / Hotfix"). The real irritant is the duplicate `rag-harness` entry in the `/flow` picker → T-C removes `flow` from its `selectableIn` (keeps `chatBaseline` role).

### Current Ask

- T-A: ship `bug-plan-harness` (flow yaml + 2 prompts + manifest + Desktop label + pack tests).
- T-B: optional investigate note in `plan-task.md`, guarded by prompt-contract tests (revert to doc-note if red).
- T-C: `rag-harness` out of the `/flow` picker (flow yaml + manifest kept in sync per BUG-NOTE-CP42 #22).

### Key Decisions

- D-1: Clone, don't parametrize — engine has no conditional edges/nodes (CP-58 deferred); same CA-727 option (a) precedent as bug-harness.
- D-2: New prompts (`plan-bug.md`, `review-bug-plan.md`), not reuse — `review-plan.md` is Task-specific (feature_key, T-*/P-*, SS-13 §5.1); BUG docs follow the BUG-357 contract (Evidence / Root Cause / Fix direction).
- D-3: No id rename for the 9-step (`bug-harness` stays; `rag-harness` id stays for mirror/chatBaseline) — id churn buys nothing.
- D-4: `cap: 3`, dual back-edges, `acceptance_nodes` mirror task-harness (plan_synthesis/validate/synthesis/audit).

### Constraints

- Additive tests only; zero pre-existing test edits (pack inventory 10→11 IS the expected edit — it is the feature's own assertion, same as CA-727 9→10).
- Manifest ↔ flow-file `builtin.*` kept in sync (TestLoadPackFSRejectsManifestFlowMetadataDrift).
- CP-55 safety topology: freeze dominates agent.code writers; plan_writer stays agent.delegate before freeze (Task-305 deviation note copied).
- BUG-357 recorder picks up the new `plan_md` binding automatically (required OUTPUT paths) — no runner change.

### Open Questions

- None blocking. Live operator items after merge: `/flow` picker visual (bug-plan-harness entry, single 9-step entry) + Supabase mirror row for bug-plan-harness.

### Source Refs

- `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (clone source)
- `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-task.md`, `review-plan.md` (prompt sources)
- `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (registry)
- `apps/local-runner/internal/agentpack/bug_harness_pack_test.go`, `task_harness_pack_test.go` (test patterns)
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:39` (HARNESS_LABELS)

## 1. Goal

A bug needing investigation gets the full plan treatment (investigate → BUG doc → plan review → freeze → TDD → code review → audit) without misfiling a Task doc; a clear bug keeps the 9-step path with exactly one picker entry; tasks gain an optional investigate step.

## 2. Parent Links

- CP-58 §P-1/P-5 (three harnesses, selectableIn flow, cloneable) + Task-305 T-5 clone precedent (CA-727).
- BUG-357 (per-run artifact recording covers the new plan_md binding with no code change).

## 3. Trigger

- Operator: bug-harness must "follow task-harness" for non-trivial bugs, but the two must not be identical-except-name — the BUG.md vs Task.md output routing is the real difference; plus picker shows two identical 9-step entries.

## 4. Exact Change

- `T-A1` `flows/bug-plan-harness.yaml` (new, 12 nodes): task-harness clone with id/description/header swapped, `plan_writer` prompt `prompts/plan-bug.md`, `plan_reviewer` prompt `prompts/review-bug-plan.md`, both `plan_md` bindings' `pathTemplate` → `requirements/09-BugFix/todo/BUG-{{idx}}-{{slug}}.md`; same cap/policy/contexts/tools/acceptance_nodes/edges.
- `T-A2` `prompts/plan-bug.md` (new): investigate-first (Symptom → Repro steps → Root-cause analysis with tools → Evidence) then BUG doc contract (Metadata / AI Quick View / Evidence / Root Cause / Fix direction per BUG-357 shape); next-free BUG number; document-writer scope guard (same as plan-task.md).
- `T-A3` `prompts/review-bug-plan.md` (new): BUG gate — repro present, root cause evidenced (not guessed), fix direction concrete with DeclaredPaths, no Task-isms (no feature_key/T-*/P-* demands), additive-tests-only.
- `T-A4` `manifest.yaml`: `bug-plan-harness` entry (`selectableIn:[flow]`, `cloneable:true`, `mirror required:true source:builtin` — same as siblings).
- `T-A5` `bug_plan_harness_pack_test.go` (new): loads, validates, selectableIn flow, cloneable, cap 3, 12 nodes, plan prompts are the bug files, pathTemplates route to 09-BugFix, acceptance_nodes match task-harness.
- `T-A6` `pack_test.go`: inventory 10→11 + want-list += `bug-plan-harness` (feature's own assertion).
- `T-A7` Desktop `HARNESS_LABELS` += `bug-plan-harness` ("Bug + Plan", "12-node: Investigate + BUG Plan + Review + freeze + TDD + Code Review").
- `T-B` `plan-task.md` += short OPTIONAL "Investigation pre-work" subsection (only when cause unclear: record Symptom/Repro/Hypothesis in AI Quick View before planning). Run `harness_doc_writer` + `harness_artifact` prompt tests after; revert on red.
- `T-C1` `flows/rag-harness.yaml` builtin: drop `flow` from `selectableIn` (keep `chatBaseline:true`, `cloneable:true`).
- `T-C2` `manifest.yaml` rag-harness entry: same drop (sync per BUG-NOTE-CP42 #22).
- `T-C3` `bug-harness.yaml` description polish: "clear bug, no plan needed" (display clarity vs bug-plan-harness sibling).

## 5. Touched Areas

- `apps/local-runner/internal/agentpack/flow-pack/flows/` (1 new + 1 polish + rag-harness metadata)
- `apps/local-runner/internal/agentpack/flow-pack/prompts/` (2 new + 1 note)
- `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`
- `apps/local-runner/internal/agentpack/` (1 new test + inventory bump)
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (1 label)

## 6. Acceptance Check

- Unit: `go test ./internal/agentpack/ -run 'TestBugPlanHarness|TestLoadBuiltinPack' -count=1` PASS; full `go test ./internal/agentpack/...` PASS, zero old-test edits (except the inventory count bump).
- Prompt: `go test ./internal/runner/ -run 'TestHarnessDocWriter|TestHarnessTemplated' -count=1` PASS after T-B.
- Manual (operator): `/flow` picker shows bug-plan-harness + exactly one 9-step (bug-harness); Supabase mirror row for bug-plan-harness after runner start.

## 7. Out of Scope

- Id renames (`bug-harness`, `rag-harness` ids frozen — D-3).
- Engine conditional-skip / parameterized bindings (future CP if merge ever wanted).
- Runner code changes (BUG-357 recorder already generic).
- Main-chat step cards (backlog observation).

## 8. Completion Notes

- Implemented 2026-09-06 (CA-747). T-A: `flows/bug-plan-harness.yaml` (12 nodes, cap 3, dual back-edges, acceptance plan_synthesis/validate/synthesis/audit — edges + non-plan nodes byte-identical to task-harness per pack test), `prompts/plan-bug.md` (investigate-first + BUG contract), `prompts/review-bug-plan.md` (BUG gate, no Task-isms), manifest flow+prompts entries, Desktop HARNESS_LABELS "Bug + Plan", `bug_plan_harness_pack_test.go` (11 assertions incl. BUG pathTemplates), inventory 10→11. T-B: optional "Investigation pre-work" in plan-task.md — prompt tests green. T-C: rag-harness `selectableIn: []` (yaml + manifest sync; chatBaseline kept) + bug-harness description polish ("clear bug, no plan needed").
- Verification: `TestBugPlanHarnessPack`, `TestLoadBuiltinPack`, `TestBugHarnessPackClone` PASS; full `agentpack/...` green; runner prompt tests green; zero old-test edits (inventory bump is the feature's own assertion).
- Live operator items: `/flow` picker visual (bug-plan-harness entry, single 9-step) + Supabase mirror row for bug-plan-harness.
- Follow-up 2026-09-06 (CA-748): `cp-harness-smoke` hidden from the Desktop Settings workflows list (`HIDDEN_BUILTIN_PACK_FLOWS` in WorkflowsSettings.tsx; direct flowRef use unaffected). TS change mirrors existing patterns; no Desktop test covers this file; `tsc` not run (node_modules absent).
