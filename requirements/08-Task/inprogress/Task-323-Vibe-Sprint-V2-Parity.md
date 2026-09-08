# Task-323: Vibe-Sprint v2 Parity (Context + Validate + Audit, Auto-Only)

## Metadata

- Document ID: `Task-323`
- Title: `Vibe-sprint v2 — copy task-harness auto nodes for harness-grade accuracy`
- Phase: `task`
- Status: `in_progress` (CA-764 YAML v2 + CA-767 drift inject + tdd-before-coder)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-05`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [CP-58: Bug / Task / CP Harness](../../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md), [Task-321: Vibe CP-Driven Entry](./Task-321-Vibe-Cp-Driven-Entry.md)
- Replaces: `None`
- Tags: `vibe-mode, accuracy-parity, desktop, tui, flow-gate, TDD`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- Slice `CP-60 P-7` only: rewrite `vibe-sprint.yaml` to v2 (`plan → freeze → context → tdd → coder → validate → synthesis → audit`), single `continue/back` `synthesis → coder`, `cap:3`.
- Copies only auto nodes from `task-harness`; plan-review loop, reviewer cohort, and Dev cards are explicitly not copied.
- Vibe stays more automatic than harness (one entry lock, zero per-sprint gates) with accuracy at harness grade or better.

### Current Ask

- **BLOCKED.** Do not rewrite `vibe-sprint.yaml` to v2 yet. v2 `synthesis` + `vibe-requirement-outcome` assume `P-2` (`r-requirement` + TurnResult advisory) and `P-1` (`working_mode`). Unblock after `P-2` lands; Branch C re-demo waits on Task-321.

### Key Decisions

- `T-1` Single code loop only: `context`/`validate`/`audit` are inline `once` nodes; the only `continue/back` remains `synthesis → coder` (no dual-loop engine work, no `CP-58 R-2` crosstalk class).
- `T-2` `acceptance_nodes: [validate, synthesis, audit]`; freeze dominates all writers; `context` stays `DONE` on code-loop re-entry.
- `T-3` Review parity comes from `synthesis` + resolver `vibe-owner-debate` + `r-requirement`, never from a copied reviewer cohort.

### Constraints

- Additive only; no `P-1`..`P-6` behavior change except the v2 node insertions; no engine/adapter change; no Supabase migration.
- Sequencing: `P-7` → `P-2` (requirement face on sprint) and reuses CP-58 node semantics. Current `vibe-sprint.yaml` is still v1 (`plan → freeze → tdd → coder → synthesis`). Pack inventory today is **11 flows**, not `7`.
- No pre-existing test edited; `dev` byte-for-byte; `selectableIn: []` unchanged.

### Open Questions

- `Q-1` `validate` failure back-target: `validate --continue--> implement` analogue is `synthesis → coder` (single edge) — keep single-edge, or mirror harness with an explicit `validate → coder` edge?

### Source Refs

- `CP-60 P-7`, §5–§8; `CP-58` `task-harness` 12-node topology + `R-2`; `SD-24 D-5`; `pack.go:ValidateFlowDefinition`/`ValidateFlowSafetyTopology`.

## 1. Goal

Every `vibe-sprint` — whether reached from Branch V (`sprint_plan`) or Branch C (`task_plan`) — packages deep code context, gates on a machine `validate` step, and closes with an audit ledger, automatically per sprint, so vibe accuracy is never below the Dev harness family.

## 2. Parent Links

- coding plan: `CP-60 P-7` (this task implements exactly `P-7`).
- tech design: `SD-24 D-5` (sprint topology), `D-3` (resolver unchanged).
- system spec: `SS-18 BR-3` (TDD guards coding), `BR-6` (bounded + explicit).
- specific upstream ids: `CP-60 P-7`, `CP-58` task-harness topology.

## 3. Trigger

`vibe-sprint` v1 (`plan → freeze → tdd → coder → synthesis`) is leaner than `task-harness` (12 nodes): no deep `context.produce`, no machine `validate`, no `audit` ledger — accuracy risk the owner explicitly rejects.

## 4. Exact Change

- `T-1` Rewrite `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml` to v2: nodes `preflight_contract_plan (delegate, once) → preflight_contract_freeze (inline, once) → context (inline, once, context.produce) → tdd (delegate, reinvoke, agents/tester.md) → coder (delegate, reinvoke, agent.code, agents/coder.md) → validate (inline, once, command.validate) → synthesis (hub.inline, reinvoke, agents/synthesizer.md) → audit (inline, once, artifact.audit_draft) → done`; forward edges linear on `done`; single `continue/back synthesis → coder`; `escalate → ask_user`; `policy: {cap:3, onCap: escalate}`; `acceptance_nodes: [validate, synthesis, audit]`; `tools: [tools/vibe-requirement-outcome.yaml]` unchanged.
- `T-2` `pack_test.go` topology assertions for v2 (node set/order, single back-edge, acceptance set, freeze-dominates-writers, `done` paths cross acceptance; `tdd` = `agents/tester.md` + signature-only contract, edge `tdd → coder` with no bypass, `synthesis` checks signature↔`AC-*` coverage); `ValidateFlowSafetyTopology` green.
- `T-3` Runner parity probe: code-loop `continue` reuses `coder` session and keeps `context`/`freeze` `DONE` (scoped reset, `forwardReachableNodeIDs` rule); `synthesis` 1:1 check runs after `validate` green; `audit` emits ledger per sprint.
- `T-4` Re-demo: one Branch V sprint + one Branch C sprint on v2 showing `context` packaged → `validate` green → `synthesis done` → `audit` ledger, with zero new user gates (only pre-existing `r-requirement`/cap asks).

## 5. Touched Areas

- files: `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml`, `apps/local-runner/internal/agentpack/pack_test.go`, `apps/local-runner/internal/runner/flow_executor.go` (reset-scope probe only, no behavior change).
- modules: `agentpack` (topology), `runner` (re-entry scope proof).
- routes: none.
- tables: none.

## 6. Acceptance Check

- `LoadBuiltinPack` green (**12 flows** / **8 agents**); v2 topology test green; `go test ./internal/agentpack ./internal/runner` + `go vet` green.
- Manual: both demo sprints show `tdd` signature artifact (use/edge/error per `SS-04 §3.5.8`, written before any `coder` run) → `context DONE` → `validate DONE (green)` → `synthesis done` → `audit DONE` with ledger; a forced `validate` red re-enters `coder` only; missing edge/error cases are caught at `synthesis` as `continue`; no reviewer-cohort nodes spawn; no Dev cards in `vibe`.
- Accuracy claim: same fixture run through `task-harness` vs v2 `vibe-sprint` shows no missing `context`/`validate`/`audit` stage on the vibe side.

## 7. Out of Scope

- Plan-review loop or reviewer cohort copy; second `continue/back` edge; resolver/`r-requirement`/ingest changes (`P-1`..`P-6`); Admin Web; engine changes.

## 8. Completion Notes

- result: `pending implementation`
- follow-ups: `TBD (parity demo evidence)`
- upstream docs updated: `CP-60 P-7` (this task is its slice; no upstream intent change)
