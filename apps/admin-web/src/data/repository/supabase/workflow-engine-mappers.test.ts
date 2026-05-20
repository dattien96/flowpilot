import { describe, expect, it } from "vitest";

import {
  mapStepDefinition,
  mapWorkflow,
  mapWorkflowStep,
  mapWorkflowRun,
  mapWorkflowRunStep,
  mapWorkflowRunLog,
} from "./workflow-engine-mappers";

describe("WorkflowEngine mappers", () => {
  it("maps step definitions", () => {
    const row = {
      step_type: "tech_spec",
      name: "Technical Spec",
      description: "Produce technical layout",
      required_mcps: ["jira"],
      required_skills: ["tech_spec_skill"],
      agent_type: "standard",
    };
    const entity = mapStepDefinition(row);
    expect(entity).toEqual({
      stepType: "tech_spec",
      name: "Technical Spec",
      description: "Produce technical layout",
      requiredMcps: ["jira"],
      requiredSkills: ["tech_spec_skill"],
      agentType: "standard",
    });
  });

  it("maps workflows", () => {
    const row = {
      id: "wf-1",
      project_id: null,
      name: "Main Pipeline",
      description: "Main workflow",
      is_template: false,
      provider_override: "claude",
      model_override: "sonnet-3.7",
      created_by: "dev-1",
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    };
    const entity = mapWorkflow(row);
    expect(entity).toEqual({
      id: "wf-1",
      projectId: null,
      name: "Main Pipeline",
      description: "Main workflow",
      isTemplate: false,
      providerOverride: "claude",
      modelOverride: "sonnet-3.7",
      createdBy: "dev-1",
      createdAt: "2026-05-20T00:00:00Z",
      updatedAt: "2026-05-20T01:00:00Z",
    });
  });

  it("maps private workflow ownership", () => {
    const row = {
      id: "wf-2",
      project_id: "project-123",
      name: "Private Pipeline",
      description: "Project-only workflow",
      is_template: false,
      provider_override: null,
      model_override: null,
      created_by: "dev-2",
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    };

    expect(mapWorkflow(row).projectId).toBe("project-123");
  });

  it("maps workflow steps", () => {
    const row = {
      id: "wfs-1",
      workflow_id: "wf-1",
      step_type: "tech_spec",
      order_index: 2,
      is_enabled: true,
      provider_override: null,
      model_override: null,
      requires_approval: true,
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T00:00:00Z",
    };
    const entity = mapWorkflowStep(row);
    expect(entity).toEqual({
      id: "wfs-1",
      workflowId: "wf-1",
      stepType: "tech_spec",
      orderIndex: 2,
      isEnabled: true,
      providerOverride: null,
      modelOverride: null,
      requiresApproval: true,
      createdAt: "2026-05-20T00:00:00Z",
      updatedAt: "2026-05-20T00:00:00Z",
    });
  });

  it("maps workflow runs", () => {
    const row = {
      id: "run-1",
      workflow_id: "wf-1",
      project_id: "p-123",
      status: "RUNNING",
      provider: "claude",
      model: "sonnet",
      yolo_mode: true,
      started_by: "user-1",
      started_at: "2026-05-20T03:00:00Z",
      finished_at: null,
      error_message: null,
    };
    const entity = mapWorkflowRun(row);
    expect(entity).toEqual({
      id: "run-1",
      workflowId: "wf-1",
      projectId: "p-123",
      status: "RUNNING",
      provider: "claude",
      model: "sonnet",
      yoloMode: true,
      startedBy: "user-1",
      startedAt: "2026-05-20T03:00:00Z",
      finishedAt: null,
      errorMessage: null,
    });
  });

  it("maps workflow run steps", () => {
    const row = {
      id: "wrs-1",
      workflow_run_id: "run-1",
      workflow_step_id: "wfs-1",
      execution_order_index: 5,
      step_type: "tech_spec",
      status: "WAITING_USER_APPROVAL",
      artifact_id: "art-1",
      prompt_cache_id: "cache-1",
      rejection_note: "Fix design patterns",
      retry_count: 1,
      started_at: "2026-05-20T03:05:00Z",
      finished_at: null,
      error_message: null,
    };
    const entity = mapWorkflowRunStep(row);
    expect(entity).toEqual({
      id: "wrs-1",
      workflowRunId: "run-1",
      workflowStepId: "wfs-1",
      executionOrderIndex: 5,
      stepType: "tech_spec",
      status: "WAITING_USER_APPROVAL",
      artifactId: "art-1",
      promptCacheId: "cache-1",
      rejectionNote: "Fix design patterns",
      retryCount: 1,
      startedAt: "2026-05-20T03:05:00Z",
      finishedAt: null,
      errorMessage: null,
    });
  });

  it("maps workflow run logs", () => {
    const row = {
      id: "log-1",
      workflow_run_step_id: "wrs-1",
      log_level: "info",
      message: "Started assembling prompt",
      created_at: "2026-05-20T03:05:01Z",
    };
    const entity = mapWorkflowRunLog(row);
    expect(entity).toEqual({
      id: "log-1",
      workflowRunStepId: "wrs-1",
      logLevel: "info",
      message: "Started assembling prompt",
      createdAt: "2026-05-20T03:05:01Z",
    });
  });
});
