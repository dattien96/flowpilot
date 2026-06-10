import { describe, expect, it } from "vitest";

import {
  mapArtifactDefinition,
  mapArtifactRun,
  mapStepDefinition,
  mapWorkflow,
  mapWorkflowStep,
  mapWorkflowRun,
  mapWorkflowRunStep,
  mapWorkflowRunLog,
  mapWorkflowRunSession,
} from "./workflow-engine-mappers";

describe("WorkflowEngine mappers", () => {
  it("maps step definitions", () => {
    const row = {
      step_type: "tech_spec",
      name: "Technical Spec",
      description: "Produce technical layout",
      prompt_base: "Produce technical layout for the workflow.",
      required_mcps: ["jira"],
      mcp_access_mode: "read_write",
      required_skills: ["tech_spec_skill"],
      model: "gpt-5.5",
      reasoning_effort: "high",
      yolo_mode: true,
      agent_type: "standard",
      input_artifact_definitions: ["business_summary_artifact"],
      output_artifact_definitions: ["tech_spec_artifact"],
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    };
    const entity = mapStepDefinition(row);
    expect(entity).toEqual({
      stepType: "tech_spec",
      name: "Technical Spec",
      description: "Produce technical layout",
      promptBase: "Produce technical layout for the workflow.",
      requiredMcps: ["jira"],
      mcpAccessMode: "read_write",
      requiredSkills: ["tech_spec_skill"],
      teamRole: null,
      subagent: null,
      model: "gpt-5.5",
      reasoningEffort: "high",
      yoloMode: true,
      agentType: "standard",
      inputArtifactDefinitions: ["business_summary_artifact"],
      outputArtifactDefinitions: ["tech_spec_artifact"],
      createdAt: "2026-05-20T00:00:00Z",
      updatedAt: "2026-05-20T01:00:00Z",
    });
  });

  it("defaults missing artifact bindings to empty arrays", () => {
    const entity = mapStepDefinition({
      step_type: "tech_spec",
      name: "Technical Spec",
      description: "Produce technical layout",
      prompt_base: "Produce technical layout for the workflow.",
      required_mcps: ["jira"],
      required_skills: ["tech_spec_skill"],
      model: "gpt-5.5",
      reasoning_effort: null,
      agent_type: "standard",
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    });

    expect(entity.inputArtifactDefinitions).toEqual([]);
    expect(entity.outputArtifactDefinitions).toEqual([]);
    expect(entity.mcpAccessMode).toBe("read_only");
  });

  it("normalizes legacy Gemini aliases when mapping workflow config rows", () => {
    expect(
      mapStepDefinition({
        step_type: "tech_spec",
        name: "Technical Spec",
        description: "Produce technical layout",
        prompt_base: null,
        required_mcps: [],
        required_skills: [],
        model: "gemini-flash",
        reasoning_effort: "medium",
        agent_type: "standard",
        created_at: "2026-05-20T00:00:00Z",
        updated_at: "2026-05-20T01:00:00Z",
      }).model,
    ).toBe("gemini-2.5-flash");

    expect(
      mapWorkflow({
        id: "wf-gemini",
        project_id: null,
        name: "Gemini Workflow",
        description: "Uses legacy model aliases",
        is_template: false,
        provider_override: "gemini",
        model_override: "gemini-pro",
        reasoning_effort_override: null,
        yolo_mode: true,
        created_by: "dev-1",
        created_at: "2026-05-20T00:00:00Z",
        updated_at: "2026-05-20T01:00:00Z",
      }).modelOverride,
    ).toBe("gemini-2.5-pro");
    expect(
      mapWorkflow({
        id: "wf-yolo",
        project_id: null,
        name: "YOLO Workflow",
        description: "Uses YOLO mode",
        is_template: false,
        provider_override: "codex",
        model_override: "gpt-5.4",
        reasoning_effort_override: null,
        yolo_mode: true,
        created_by: "dev-1",
        created_at: "2026-05-20T00:00:00Z",
        updated_at: "2026-05-20T01:00:00Z",
      }).yoloMode,
    ).toBe(true);
  });

  it("maps artifact definitions", () => {
    const entity = mapArtifactDefinition({
      key: "plan_artifact",
      name: "Plan",
      description: "Planning output",
      local_path_template: ".flowpilot/artifacts/{projectId}/plan.md",
      remote_path_template: "artifacts/{projectId}/plan.md",
      default_file_name: "Plan.md",
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    });

    expect(entity).toEqual({
      key: "plan_artifact",
      name: "Plan",
      description: "Planning output",
      localPathTemplate: ".flowpilot/artifacts/{projectId}/plan.md",
      remotePathTemplate: "artifacts/{projectId}/plan.md",
      defaultFileName: "Plan.md",
      createdAt: "2026-05-20T00:00:00Z",
      updatedAt: "2026-05-20T01:00:00Z",
    });
  });

  it("maps artifact runs", () => {
    const entity = mapArtifactRun({
      id: "art-1",
      artifact_definition_key: "plan_artifact",
      workflow_id: "wf-1",
      workflow_run_id: "run-1",
      workflow_run_step_id: "wrs-1",
      project_id: "p-123",
      title: "Plan.md",
      local_path: ".flowpilot/artifacts/p-123/run-1/plan.md",
      remote_path: "artifacts/p-123/run-1/plan.md",
      remote_url: "https://example.com/plan.md",
      storage_provider: "supabase",
      remote_object_id: "object-1",
      sync_status: "synced",
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    });

    expect(entity).toEqual({
      id: "art-1",
      artifactDefinitionKey: "plan_artifact",
      workflowId: "wf-1",
      workflowRunId: "run-1",
      workflowRunStepId: "wrs-1",
      projectId: "p-123",
      title: "Plan.md",
      localPath: ".flowpilot/artifacts/p-123/run-1/plan.md",
      remotePath: "artifacts/p-123/run-1/plan.md",
      remoteUrl: "https://example.com/plan.md",
      storageProvider: "supabase",
      remoteObjectId: "object-1",
      syncStatus: "synced",
      replicas: [],
      createdAt: "2026-05-20T00:00:00Z",
      updatedAt: "2026-05-20T01:00:00Z",
    });
  });

  it("preserves null artifact definition keys for fallback workflow artifacts", () => {
    const entity = mapArtifactRun({
      id: "art-2",
      artifact_definition_key: null,
      workflow_id: "wf-1",
      workflow_run_id: "run-2",
      workflow_run_step_id: "wrs-2",
      project_id: "p-123",
      title: "Response.md",
      local_path: ".flowpilot/artifacts/p-123/run-2/step-1/.snapshots/art-2/Response.md",
      remote_path: "",
      remote_url: "",
      storage_provider: null,
      remote_object_id: null,
      sync_status: "local_only",
      created_at: "2026-05-20T00:00:00Z",
      updated_at: "2026-05-20T01:00:00Z",
    });

    expect(entity.artifactDefinitionKey).toBeNull();
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
      reasoning_effort_override: "high",
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
      reasoningEffortOverride: "high",
      yoloMode: false,
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
      reasoning_effort_override: null,
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
      yolo_mode: true,
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
      reasoningEffortOverride: null,
      yoloMode: true,
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
      reasoning_effort: "medium",
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
      reasoningEffort: "medium",
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
      artifact_run_id: "run-art-1",
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
      artifactRunId: "run-art-1",
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

  it("maps workflow run sessions", () => {
    const row = {
      id: "sess-1",
      workflow_run_id: "run-1",
      provider: "claude",
      model: "claude-sonnet",
      transport_type: "claude_stream_json",
      provider_session_id: "prov-sess-123",
      process_key: "proc-456",
      process_pid: 9876,
      status: "active",
      metadata_json: { custom: "field" },
      started_at: "2026-05-20T03:05:00Z",
      completed_at: null,
    };
    const entity = mapWorkflowRunSession(row);
    expect(entity).toEqual({
      id: "sess-1",
      workflowRunId: "run-1",
      provider: "claude",
      model: "claude-sonnet",
      transportType: "claude_stream_json",
      providerSessionId: "prov-sess-123",
      processKey: "proc-456",
      processPid: 9876,
      status: "active",
      metadataJson: { custom: "field" },
      startedAt: "2026-05-20T03:05:00Z",
      completedAt: null,
    });
  });
});
