# CP-61 Test Steps — Harness Done-Verdict Gate

## Metadata

- Document ID: `CP-61-TEST-STEPS`
- Title: `CP-61 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-09-08`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-61](../inprogress/CP-61-Harness-Done-Verdict-Gate.md)
- Child Documents: `<none>`
- Related Documents: [CA-757](../../../change-audit/CA-757-CP-61-P1-Harness-Done-Verdict-Gate.md) (P-1 landed), [CA-755](../../../change-audit/CA-755-Slice-A-Hide-Review-Loop-Dual-Cap-Reset.md), [CP-53-Test-Steps](./CP-53-Test-Steps.md) (review-loop only), [CP-58-Test-Steps](./CP-58-Test-Steps.md) (harness family smoke), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: CP-53-Test-Steps §2 for **harness hubs** only (review-loop §2 stays on CP-53-Test-Steps)
- Tags: `agent-flow-engine, harness, review-verdict, verification, test-steps, cp-61`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Automated + live checklist for CP-61: `plan_synthesis` / `synthesis` / `cp_synthesis` cannot dispatch their done-successor without inbound-cohort `submit_review_outcome` = approved.
- Manual bed: project **gate-sandbox** (`/Users/tiendat/Desktop/BE/gate-sandbox`; Windows alias `D:\working\gate-sandbox`). Feature `calc-core`.
- P-1 (CA-757) is the live gate. P-2 reviewer asymmetry is **not** in this guide until that slice lands.

### Current Ask

- P-1 live checklist **closed** 2026-09-08. Residuals M.3/M.4/C.2 (live reject-then-PASS) deferred — unit `TestCP61HubDone` covers those legs. Do not claim CP-61 DoD (P-2 still open).

### Key Decisions

- `V-1` Missing / not-approved verdict → escalate (`flow_control_rejected_missing_review_verdict`). `changes_requested` → continue writer re-entry (`flow_control_hub_done_continue_on_review_verdict`). PASS → freeze / audit / splitter unchanged.
- `V-2` Cohort is inbound to the hub: plan hubs need `plan_reviewer`/`cp_reviewer`, code hub needs `reviewer`. Code-reviewer PASS is never required to freeze.
- `V-3` Live missing-verdict is rare (reviewer usually calls the tool). Prefer `changes_requested` continue for manual; missing/escalate is locked by `TestCP61HubDone`.

### Constraints

- `feature_key: agent-flow-engine`. R1: do not edit `cp53_review_done_verdict_test.go` / `TestFlowRequiresSynthesisMachineVerdict`. R2: automated matrix Claude+Codex+Grok. R3: missing / changes / PASS / wrong-cohort / chat-unaffected already in `TestCP61HubDone`.
- Will-not-undo: CA-755 Round reset on PASS, CA-749/752 park on churned PASS, Task-274 review-loop gate.

### Open Questions

- P-2 (reviewer model/effort defaults) — no live steps until that Task exists.
- Forcing a live missing verdict without a test double: optional; log grep is enough if automated is green.

### Source Refs

- CP-61 D-1…D-4, P-1, DoD. CA-757. `advanceHubDoneThroughEdge`. `hubDoneVerdictError`.

## 1. Goal

Prove harness hub `done` is a machine-checked PASS, not a synthesizer self-grade. Review-loop `synthesis→done` stays on [CP-53-Test-Steps §2](./CP-53-Test-Steps.md).

## 2. Automated (P-1 / CA-757) — run first

Working dir: `apps/local-runner`.

```bash
go test ./internal/runner/ -count=1 -run 'TestCP61HubDone' -v
# old contracts, untouched:
go test ./internal/runner/ -count=1 -run 'TestCP53ReviewDoneVerdict|TestFlowRequiresSynthesisMachineVerdict' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 2.1 | `TestCP61HubDone` green (13 subtests × claude/codex/grok) | [x] 2026-09-08 `go test ./internal/runner/ -count=1 -timeout 180s -run 'TestCP61HubDone\|TestCP53ReviewDoneVerdict\|TestFlowRequiresSynthesisMachineVerdict'` → `ok flowpilot-runner/internal/runner 4.204s`. `cp61Providers()` = Claude+Codex+Grok. |
| 2.2 | plan missing / changes_requested / PASS / wrong-cohort covered | [x] `plan_synthesis_missing_blocks`, `plan_synthesis_changes_requested_continues`, `plan_synthesis_approved_dispatches_freeze`, `plan_synthesis_wrong_cohort_blocks` |
| 2.3 | synthesis missing / PASS (audit starts; not freeze) | [x] `synthesis_missing_blocks`, `synthesis_approved_dispatches_audit` |
| 2.4 | cp missing / PASS (splitter + 1 child); `cp_reviewer` record-only | [x] `cp_synthesis_missing_blocks`, `cp_synthesis_approved_dispatches_splitter`, `cp_reviewer_records_verdict` |
| 2.5 | normal chat unaffected | [x] `normal_chat_unaffected` |
| 2.6 | churned PASS parks; churned missing escalates (never `plan_approval`) | [x] `churned_plan_pass_parks_missing_escalates` (+ `apply_err_*` fail-closed) |
| 2.7 | `TestCP53ReviewDoneVerdict*` + `TestFlowRequiresSynthesisMachineVerdict` green, **untouched** | [x] same `go test` PASS; `git diff --name-only` empty on `cp53_review_done_verdict_test.go` |

**Provider class:** Case 1 agnostic (CA-757). Matrix is the R2 lock.

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm |
|---|------|-----------|
| P1 | Branch has CA-757 (`61f0d43d` or later) | [x] `git merge-base --is-ancestor 61f0d43d HEAD` PASS. `git log --grep CP-61` → `61f0d43d Gate harness hub done on reviewer PASS` |
| P2 | Runner builds | [x] `cd apps/local-runner && go build ./internal/runner/` PASS (2026-09-08) |
| P3 | Bind **gate-sandbox** | [x] `/Users/tiendat/Desktop/BE/gate-sandbox` exists; `.flowpilot/engine-init.json` present |
| P4 | Engine up | [x] `just chat-dev` recipe in repo justfile. Project already bound. Start TUI at M.*: `just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` |
| P5 | Feature | [x] `change-audit/FEATURE-KEYS.md` has `calc-core`; `calc.go` present |
| P6 | Log file | [x] events in `interactive_service.go`: `flow_control_hub_done_continue_on_review_verdict`, `flow_control_rejected_missing_review_verdict`. Live grep at M.* |

Do **not** run these on the FlowPilot repo itself (self-dogfood). Bed is gate-sandbox.

## 4. Manual M — `task-harness` on gate-sandbox

Picker: TUI `/flow` → `task-harness`, or Desktop Flow Mode → Task Harness.

**Prompt (copy):**

```text
Add integer GCD to the calc package, feature_key: calc-core.
Scope: calc.go (new GCD) + a NEW test file only (do not edit existing tests).
AC: GCD(48,18)=6, GCD(0,5)=5, GCD(5,0)=5, GCD(-48,18)=6, GCD(0,0)=0 (no panic).
go test ./... green.
```

| Step | Hành động | Pass khi | Tick | runId |
|------|-----------|----------|------|-------|
| M.1 | Start `task-harness` | Timeline: `plan_writer` → `plan_reviewer` → `plan_synthesis`. `review-loop` **not** in picker. | [x] | run-621371 |
| M.2 | First plan PASS | `plan_reviewer` calls `submit_review_outcome` approved → `plan_synthesis` `done` → `preflight_contract_freeze` → `test_signatures`. Round reset (CA-755). **Not** `plan_approval` park on a clean first pass. | [x] freeze `flow_contract_frozen` then `plan_phase_round_reset`; no `plan_approval` | run-621371 |
| M.3 | Force plan `changes_requested` | Prompt round 1: “plan must include a benchmark section”. Expect `flow_control_hub_done_continue_on_review_verdict` → `plan_writer` re-entry, **no freeze**. Plan nodes except `context`/scout reset; code nodes not started. | [ ] **not this run** — plan approved lần 1, `plan_writer` RUNNING 1 lần | |
| M.4 | Plan PASS after M.3 | Freeze then code chain. `plan_*` stay DONE. | [ ] blocked on M.3. Freeze+code chain trên run này thuộc **M.2**, không phải “after M.3” | |
| M.5 | Code loop `changes_requested` | Reviewer rejects implement. `synthesis` continue → `implement` re-entry. `plan_*` + freeze stay DONE. Log: same continue event, `hub=synthesis`. | [x] grok `changes_requested` → `flow_control_received` continue → implement RUNNING; `plan_*` không reset | run-621371 |
| M.6 | Code PASS | `reviewer` approved → `synthesis` `done` → `audit` (not freeze). | [x] 2nd review approved → `audit` READY → `flow_control_done` | run-621371 |
| M.7 | Missing verdict (optional) | If synthesizer calls `done` with no cohort PASS: `flow_control_rejected_missing_review_verdict` + escalate. Never freeze/audit. Skip if you cannot stall the reviewer tool; 2.1 covers it. | [ ] | |
| M.8 | Churned plan (Task-325) | After a later plan PASS that is a churn: `plan_approval` park **only** when cohort is approved. Missing verdict must **not** park. | [ ] | |

## 5. Manual C — `cp-harness` on gate-sandbox

Picker: `/flow` → `cp-harness`. Slice-only: no `implement`.

**Prompt (copy):**

```text
Write a tiny coding plan CP-9xx to add GCD to calc-core (2 phases: P-1 function, P-2 tests).
feature_key: calc-core. Do not implement code.
```

| Step | Hành động | Pass khi | Tick | runId |
|------|-----------|----------|------|-------|
| C.1 | Start `cp-harness` | `cp_plan_writer` → `cp_reviewer` → `cp_synthesis`. No `implement`. | [x] no `implement` in step-transitions | run-623294 |
| C.2 | `cp_reviewer` reject | `cp_synthesis` continue → writer re-entry. Log continue, `hub=cp_synthesis`. No `task_splitter`. | [ ] **not this run** — approved lần 1; `cp_plan_writer` RUNNING 1 lần | |
| C.3 | `cp_reviewer` approved | `submit_review_outcome` (record-only — CA-757). `cp_synthesis` `done` → `task_splitter` → `audit`. | [x] “Approved CP-910” → splitter → audit READY → `flow_control_done` | run-623294 |
| C.4 | Missing cp verdict (optional) | Same escalate event, `hub=cp_synthesis`. No splitter. | [ ] skip — 2.4 covers | |

## 6. Manual N — negative / non-harness

| Step | Hành động | Pass khi | Tick | runId |
|------|-----------|----------|------|-------|
| N.1 | Plain chat on gate-sandbox (“what does Add do? do not edit files”) | No hub gate. No `flow_control_rejected_missing_review_verdict`. | [x] `run_kind: chat`; no `run-623625.ndjson` flow log; no hub events | run-623625 |
| N.2 | `bug-harness` if used | Own review loop; do not treat as CP-61 3-hub matrix. `bug-plan-harness` follows M.* (`plan_synthesis` + `synthesis`). | [x] skip — operator; not in CP-61 3-hub DoD | |

## 7. Log grep (evidence)

From the runner log for the parent `runId`:

```text
flow_control_hub_done_continue_on_review_verdict
flow_control_rejected_missing_review_verdict
```

| Hub | Inbound cohort | PASS successor | FAIL/missing |
|-----|----------------|----------------|--------------|
| `plan_synthesis` | `plan` (`plan_reviewer`) | freeze | continue / escalate |
| `synthesis` | `review` (`reviewer`) | audit | continue / escalate |
| `cp_synthesis` | `plan` (`cp_reviewer`) | `task_splitter` | continue / escalate |

Wrong-cohort: `reviewer=approved` must **not** unlock `plan_synthesis`. Automated 2.2; live only if you can inspect snapshot.

## 8. CP-61 verification-complete when

- [x] §2 automated all ticked (P-1). 2026-09-08.
- [x] M.* live: M.1/M.2/M.5/M.6 `run-621371`. M.3/M.4 deferred (no plan-reject live; unit 2.2).
- [x] C.* live: C.1/C.3 `run-623294`. C.2 deferred (no cp-reject live; unit 2.4).
- [x] N.1 ticked. run-623625. N.2 skipped.
- [ ] P-2 (asymmetry) still open — this file does **not** close CP-61.
- [x] Fail on M.3/M.5/C.2 (freeze/audit/splitter without PASS) → new BUG, `feature_key: agent-flow-engine`, prior CA-757. Do not edit old tests.
