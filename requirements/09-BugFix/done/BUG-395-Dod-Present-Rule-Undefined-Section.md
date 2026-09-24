# BUG-395: r-dod-present requires `## Definition of Done` that writer prompts/format spec never define

## Metadata

- Document ID: `BUG-395`
- Title: `Audit tier-3 r-dod-present demands a DoD section absent from plan-task/task-splitter prompts and FORMAT-REFERENCE-TASK.md`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-61-Test-Steps](../../07-Coding-Plan/done/CP-61-Test-Steps.md)
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Gate rule `r-dod-present` (`flowgate/evaluate.go:257-268`) blocks audit on any AI-written `Task-*.md` lacking a `## Definition of Done` section with ≥1 `- [ ]` checkbox — but no writer prompt or format reference ever defines that section.
- `plan-task.md:41-56` lists required sections (Metadata, AI Quick View, §1 Goal … §8 Completion Notes — no DoD); `task-splitter.md:33` requires `- [ ]` checkboxes under `## 6. Acceptance Check` (not a DoD section); `FORMAT-REFERENCE-TASK.md` defines sections `## 1. Goal` through `## 8. Completion Notes` — no `## Definition of Done`.
- Result: every harness run that reaches `audit` parks at tier-3 — CP-61 run-2688 (task-harness) and run-4464 (cp-harness) both parked `flow_audit_tier3_block` → `flow_control_escalate` → `flow_parked_awaiting_user`; CP-58 observed the same systematically.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** Flows produce spec-conformant Task docs, then `audit` blocks on `r-dod-present` demanding a section the writers were never told to emit.
- **Expected:** The required-section contract is symmetric — either the writer prompts/format spec require `## Definition of Done`, or the gate accepts the `## 6. Acceptance Check` checkbox list the spec does require.
- **Actual:** Schema mismatch between producer spec and gate rule → systematic tier-3 park; unwinnable without operator feedback since the agent already wrote exactly what was specified.
- **Impact:** Every task-harness/cp-harness run stalls at audit pending human action (possibly mistaken for intended human-in-the-loop, but it's a spec bug — CP-58 notes it as effectively systematic). Downstream audit/human gates can never be exercised unattended.

## Reproduction

1. Run `task-harness` (or `cp-harness` → `task_splitter`) end-to-end so Tasks are written by `plan-task`/`task-splitter` prompts.
2. Let the flow reach `audit` tier-3.
3. Observe block: `Task/BUG document(s) without a Definition of Done checklist: …` → escalate → park.
- runIds: `run-2688` (task-harness, opencode), `run-4464` (cp-harness, wrote `CP-921` + `Task-912`/`Task-913`), both CP-61; CP-58 run-2284 needed `agent-loop/continue` past the same audit DoD gate.

## Root cause

- `apps/local-runner/internal/flowgate/evaluate.go:257-268` — `r-dod-present` requires `## Definition of Done` + ≥1 `- [ ]` on AI-written Task/BUG docs.
- `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-task.md:41-56` and `task-splitter.md:33` — required-section lists contain no DoD section; checkboxes are specified under `## 6. Acceptance Check`.
- `requirements/08-Task/FORMAT-REFERENCE-TASK.md` (and scaffold copy `apps/local-runner/internal/reqscaffold/scaffold-pack/08-Task/FORMAT-REFERENCE-TASK.md`) — canonical section list `## 1. Goal` … `## 8. Completion Notes`; no `## Definition of Done`.

## Evidence

- `~/fp-beds/lt-evidence/cp61/RESULT.md` (BUG-LIVE-61-1; both runs parked `audit WAITING_USER_APPROVAL` at tier-3; `l611/`, `l612/` diag + verdict excerpts)
- `~/fp-beds/lt-evidence/cp58/RESULT.md` ("Audit tier-3 gate requires `## Definition of Done` … every task-harness run parks at audit pending operator feedback")

## Severity

- `medium` — systematic audit-tier park on every harness run; recoverable via operator feedback/continue but makes unattended runs impossible.

## Completion Notes (implemented 2026-09-23, CA-920b)

- `ParseDefinitionOfDone` accepts the spec-mandated `## Acceptance Check` section as DoD-equivalent and keeps scanning later headings (union semantics); fenced blocks still ignored.
- Unit: `bug395_dod_acceptance_check_test.go` green; real-repo corpus `TestParseDefinitionOfDone_RecognizesRealRepoDocs` green again.
