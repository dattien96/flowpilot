# Logic Sync Report — CP-07 Workflow Engine UI

## 1. Source Discovery
The business logic sources used for this analysis are:
- [SS-04-Workflow.md](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/05-System-Specs/SS-04-Workflow.md) (System Specification)
- [SD-05-Workflow-Engine.md](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/06-System-Tech-Design/SD-05-Workflow-Engine.md) (System Tech Design)
- [CP-07-Workflow-Engine-UI.md](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/CP-07-Workflow-Engine-UI.md) (Coding Plan)

---

## 2. Requirement Extraction & Alignment Check

We audited the existing test files in `apps/admin-web/src/` to check how they align with the new canonical specifications.

| Requirement ID | Core Business Rule | Target Component | Status | Action Required |
|:---|:---|:---|:---:|:---|
| **REQ-01** | Support 17 seeded MVP step definitions with MCP/skill requirements | `step_definitions` repository & seed mapper | **[MISSING]** | Flag for signature and mock catalog creation |
| **REQ-02** | Support 10 built-in templates across 4 personas | `workflows.is_template` template loaders | **[MISSING]** | Flag for template loader test coverage |
| **REQ-03** | Use new uppercase run statuses (`PENDING`, `RUNNING`, `DONE`, `FAILED`, `CANCELED`) and step statuses (`PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`) | `WorkflowEngine` domain entities & type mappers | **[MISSING]** | Flag for mapper unit tests to prevent status confusion with legacy lowercases |
| **REQ-04** | Execution: skip disabled steps consistently (mark `SKIPPED`) | `StartWorkflowRunUseCase` / `WorkflowGateway` | **[MISSING]** | Flag for run step materialization signatures |
| **REQ-05** | Realtime monitoring: refresh dashboards on `workflow_run_steps` updates | `useWorkflowRunRealtime` presentation hook | **[MISSING]** | Flag for hook invalidation event testing |
| **REQ-06** | YOLO mode per run: bypass approval gates | `workflow_runs.yolo_mode` DDL & execution | **[MISSING]** | Flag for launch state and status badge checks |
| **REQ-07** | Secure YOLO toggle and approval decisions via Edge Functions | Edge Function handlers / gateway methods | **[MISSING]** | Flag for Edge Function API POST method assertions |
| **REQ-08** | Reject & Retry: increment retry count, store rejection note, reset status to PENDING | `SubmitRunStepApprovalDecisionUseCase` | **[MISSING]** | Flag for retry mutation logic |
| **REQ-09** | Inspect prompt cache hashes and run logs per step | Logs panel & prompt-cache badge presentation | **[MISSING]** | Flag for query loading and prompt badge display |

---

## 3. Discrepancy & Logic Drift Analysis

1. **Legacy Status Drift**: 
   The current legacy codebase (`apps/admin-web/src/domain/constant/status.ts`) defines lowercase statuses (e.g., `waiting_approval`, `completed`, `rejected`). CP-07 and SD-05 explicitly require new uppercase statuses (e.g., `WAITING_USER_APPROVAL`, `DONE`, `SKIPPED`). 
   - **Risk**: High risk of logic confusion if legacy and canonical models overlap.
   - **Mitigation**: Standardized as **Option B** (isolated parallel slice `workflow-engine-*`). The new tests must enforce that the mapping functions correctly parse canonical uppercase states without throwing errors or fallback failures.

2. **Legacy Approvals vs Canonical Run-Step Approvals**:
   - Existing usecase `SubmitApprovalDecisionUseCase` (in `apps/admin-web/src/domain/usecase/approvals/`) targets the legacy `approvals` and `ai_outputs` tables.
   - The new workflow design consolidates the approval decision to mutate the `workflow_run_steps` execution table directly.
   - **Mitigation**: We are creating a brand new usecase `SubmitRunStepApprovalDecisionUseCase` that writes straight to `workflow_run_steps` via the Edge Function wrapper. The legacy test is marked `[DEPRECATED]` (to be decommissioned after fully phasing out old routes) and the new tests are flagged for complete creation in the TDD phase.

---

## 4. Sync Verdict

The main codebase is cleanly separated. There is zero risk of breaking existing screens since the new features live in a separate route hierarchy and domain slice.

All 9 critical canonical requirements are flagged as **[MISSING]** in the test coverage and have been successfully modeled in the `tdd_signatures.md` file during Phase 1-3. The next phase will implement these test files in their target locations as pure commented signatures.
