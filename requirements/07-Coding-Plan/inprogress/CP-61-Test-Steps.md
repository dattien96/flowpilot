# CP-61 Test Steps — Harness Done-Verdict Gate

## Metadata

- Document ID: `CP-61-TEST-STEPS`
- Title: `CP-61 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `active`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-09-08`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-61](./CP-61-Harness-Done-Verdict-Gate.md)
- Child Documents: `<none>`
- Related Documents: [CA-757](../../../change-audit/CA-757-CP-61-P1-Harness-Done-Verdict-Gate.md) (P-1 landed), [CA-755](../../../change-audit/CA-755-Slice-A-Hide-Review-Loop-Dual-Cap-Reset.md), [CP-53-Test-Steps](../done/CP-53-Test-Steps.md) (review-loop only), [CP-58-Test-Steps](../done/CP-58-Test-Steps.md) (harness family smoke), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: CP-53-Test-Steps §2 for **harness hubs** only (review-loop §2 stays on CP-53-Test-Steps)
- Tags: `agent-flow-engine, harness, review-verdict, verification, test-steps, cp-61`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Automated + live checklist for CP-61: `plan_synthesis` / `synthesis` / `cp_synthesis` cannot dispatch their done-successor without inbound-cohort `submit_review_outcome` = approved.
- Manual bed: project **gate-sandbox** (`/Users/tiendat/Desktop/BE/gate-sandbox`; Windows alias `D:\working\gate-sandbox`). Feature `calc-core`.
- P-1 (CA-757) is the live gate. P-2 reviewer asymmetry is **not** in this guide until that slice lands.

### Current Ask

- Run automated P-1. Drive M.* / C.* on gate-sandbox. Tick with `runId` + log line. Do not claim CP-61 DoD until P-2 also ships.

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

Prove harness hub `done` is a machine-checked PASS, not a synthesizer self-grade. Review-loop `synthesis→done` stays on [CP-53-Test-Steps §2](../done/CP-53-Test-Steps.md).

## 2. Automated (P-1 / CA-757) — run first

Working dir: `apps/local-runner`.

```bash
go test ./internal/runner/ -count=1 -run 'TestCP61HubDone' -v
# old contracts, untouched:
go test ./internal/runner/ -count=1 -run 'TestCP53ReviewDoneVerdict|TestFlowRequiresSynthesisMachineVerdict' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 2.1 | `TestCP61HubDone` green (13 subtests × claude/codex/grok) | [ ] |
| 2.2 | plan missing / changes_requested / PASS / wrong-cohort covered | [ ] |
| 2.3 | synthesis missing / PASS (audit starts; not freeze) | [ ] |
| 2.4 | cp missing / PASS (splitter + 1 child); `cp_reviewer` record-only | [ ] |
| 2.5 | normal chat unaffected | [ ] |
| 2.6 | churned PASS parks; churned missing escalates (never `plan_approval`) | [ ] |
| 2.7 | `TestCP53ReviewDoneVerdict*` + `TestFlowRequiresSynthesisMachineVerdict` green, **untouched** | [ ] |

**Provider class:** Case 1 agnostic (CA-757). Matrix is the R2 lock.

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm |
|---|------|-----------|
| P1 | Branch has CA-757 (`61f0d43d` or later) | `git log --oneline --grep CP-61` |
| P2 | Runner builds | `cd apps/local-runner && go build ./internal/runner/` |
| P3 | Bind **gate-sandbox** | cwd `/Users/tiendat/Desktop/BE/gate-sandbox` (Win: `D:\working\gate-sandbox`) |
| P4 | Engine up | `just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` **or** Desktop Flow Mode on that project |
| P5 | Feature | `calc-core`. Additive tests only if the turn edits `calc.go`. |
| P6 | Log file | runner / `cli-runner.log` — grep the two event names in §4 |

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
| M.1 | Start `task-harness` | Timeline: `plan_writer` → `plan_reviewer` → `plan_synthesis`. `review-loop` **not** in picker. | [ ] | |
| M.2 | First plan PASS | `plan_reviewer` calls `submit_review_outcome` approved → `plan_synthesis` `done` → `preflight_contract_freeze` → `test_signatures`. Round reset (CA-755). **Not** `plan_approval` park on a clean first pass. | [ ] | |
| M.3 | Force plan `changes_requested` | Prompt round 1: “plan must include a benchmark section”. Expect `flow_control_hub_done_continue_on_review_verdict` → `plan_writer` re-entry, **no freeze**. Plan nodes except `context`/scout reset; code nodes not started. | [ ] | |
| M.4 | Plan PASS after M.3 | Freeze then code chain. `plan_*` stay DONE. | [ ] | |
| M.5 | Code loop `changes_requested` | Reviewer rejects implement. `synthesis` continue → `implement` re-entry. `plan_*` + freeze stay DONE. Log: same continue event, `hub=synthesis`. | [ ] | |
| M.6 | Code PASS | `reviewer` approved → `synthesis` `done` → `audit` (not freeze). | [ ] | |
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
| C.1 | Start `cp-harness` | `cp_plan_writer` → `cp_reviewer` → `cp_synthesis`. No `implement`. | [ ] | |
| C.2 | `cp_reviewer` reject | `cp_synthesis` continue → writer re-entry. Log continue, `hub=cp_synthesis`. No `task_splitter`. | [ ] | |
| C.3 | `cp_reviewer` approved | `submit_review_outcome` (record-only — CA-757). `cp_synthesis` `done` → `task_splitter` → `audit`. | [ ] | |
| C.4 | Missing cp verdict (optional) | Same escalate event, `hub=cp_synthesis`. No splitter. | [ ] | |

## 6. Manual N — negative / non-harness

| Step | Hành động | Pass khi | Tick | runId |
|------|-----------|----------|------|-------|
| N.1 | Plain chat on gate-sandbox (“what does Add do? do not edit files”) | No hub gate. No `flow_control_rejected_missing_review_verdict`. | [ ] | |
| N.2 | `bug-harness` if used | Own review loop; do not treat as CP-61 3-hub matrix. `bug-plan-harness` follows M.* (`plan_synthesis` + `synthesis`). | [ ] | |

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

- [ ] §2 automated all ticked (P-1).
- [ ] M.1–M.6 ticked on gate-sandbox with `runId` (one provider live is enough; matrix is §2).
- [ ] C.1–C.3 ticked on gate-sandbox **or** explicitly deferred with reason.
- [ ] N.1 ticked.
- [ ] P-2 (asymmetry) still open — do not close CP-61 on this file alone.
- [ ] Fail on M.3/M.5/C.2 (freeze/audit/splitter without PASS) → new BUG, `feature_key: agent-flow-engine`, prior CA-757. Do not edit old tests.
