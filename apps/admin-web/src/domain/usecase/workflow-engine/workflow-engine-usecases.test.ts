import { describe, expect, it, vi } from "vitest";

import { ListStepDefinitionsUseCase } from "./list-step-definitions-usecase";
import { ListWorkflowsUseCase } from "./list-workflows-usecase";
import { GetWorkflowDetailUseCase } from "./get-workflow-detail-usecase";
import { SaveWorkflowUseCase } from "./save-workflow-usecase";
import { ListWorkflowRunsUseCase } from "./list-workflow-runs-usecase";
import { GetWorkflowRunDetailUseCase } from "./get-workflow-run-detail-usecase";
import { StartWorkflowRunUseCase } from "./start-workflow-run-usecase";
import { ToggleYoloModeUseCase } from "./toggle-yolo-mode-usecase";
import { SubmitStepApprovalDecisionUseCase } from "./submit-step-approval-decision-usecase";
import { SaveStepDefinitionUseCase } from "./save-step-definition-usecase";

describe("WorkflowEngine UseCases", () => {
  const mockGateway = {
    listStepDefinitions: vi.fn(),
    saveStepDefinition: vi.fn(),
    listWorkflows: vi.fn(),
    getWorkflowDetail: vi.fn(),
    saveWorkflow: vi.fn(),
    listWorkflowRuns: vi.fn(),
    getWorkflowRunDetail: vi.fn(),
    startWorkflowRun: vi.fn(),
    toggleYoloMode: vi.fn(),
    submitStepApproval: vi.fn(),
  };

  it("ListStepDefinitionsUseCase calls gateway", async () => {
    mockGateway.listStepDefinitions.mockResolvedValueOnce([{ stepType: "tdd" }]);
    const usecase = new ListStepDefinitionsUseCase(mockGateway as any);
    const result = await usecase.execute();
    expect(mockGateway.listStepDefinitions).toHaveBeenCalled();
    expect(result).toEqual([{ stepType: "tdd" }]);
  });

  it("ListWorkflowsUseCase calls gateway", async () => {
    mockGateway.listWorkflows.mockResolvedValueOnce([{ id: "wf-1" }]);
    const usecase = new ListWorkflowsUseCase(mockGateway as any);
    const result = await usecase.execute("proj-1");
    expect(mockGateway.listWorkflows).toHaveBeenCalledWith("proj-1");
    expect(result).toEqual([{ id: "wf-1" }]);
  });

  it("SaveStepDefinitionUseCase calls gateway with step payload", async () => {
    const payload = {
      stepType: "custom_step" as const,
      name: "Custom Step",
      description: "Do custom work",
      requiredMcps: [],
      requiredSkills: [],
      agentType: "standard" as const,
    };
    mockGateway.saveStepDefinition.mockResolvedValueOnce(payload);
    const usecase = new SaveStepDefinitionUseCase(mockGateway as any);
    const result = await usecase.execute(payload);
    expect(mockGateway.saveStepDefinition).toHaveBeenCalledWith(payload);
    expect(result.name).toBe("Custom Step");
  });

  it("GetWorkflowDetailUseCase calls gateway with workflowId", async () => {
    mockGateway.getWorkflowDetail.mockResolvedValueOnce({ id: "wf-1", name: "Build" });
    const usecase = new GetWorkflowDetailUseCase(mockGateway as any);
    const result = await usecase.execute("wf-1");
    expect(mockGateway.getWorkflowDetail).toHaveBeenCalledWith("wf-1");
    expect(result).toEqual({ id: "wf-1", name: "Build" });
  });

  it("SaveWorkflowUseCase calls gateway with payload", async () => {
    const payload = { name: "W", steps: [] };
    mockGateway.saveWorkflow.mockResolvedValueOnce({ id: "wf-1", ...payload });
    const usecase = new SaveWorkflowUseCase(mockGateway as any);
    const result = await usecase.execute(payload);
    expect(mockGateway.saveWorkflow).toHaveBeenCalledWith(payload);
    expect(result.id).toBe("wf-1");
  });

  it("ListWorkflowRunsUseCase calls gateway with projectId", async () => {
    mockGateway.listWorkflowRuns.mockResolvedValueOnce([{ id: "run-1" }]);
    const usecase = new ListWorkflowRunsUseCase(mockGateway as any);
    const result = await usecase.execute("proj-1");
    expect(mockGateway.listWorkflowRuns).toHaveBeenCalledWith("proj-1");
    expect(result).toEqual([{ id: "run-1" }]);
  });

  it("GetWorkflowRunDetailUseCase calls gateway with runId", async () => {
    const detail = { run: { id: "run-1" }, steps: [], logs: [] };
    mockGateway.getWorkflowRunDetail.mockResolvedValueOnce(detail);
    const usecase = new GetWorkflowRunDetailUseCase(mockGateway as any);
    const result = await usecase.execute("run-1");
    expect(mockGateway.getWorkflowRunDetail).toHaveBeenCalledWith("run-1");
    expect(result).toEqual(detail);
  });

  it("StartWorkflowRunUseCase calls gateway with workflowId & projectId", async () => {
    mockGateway.startWorkflowRun.mockResolvedValueOnce({ id: "run-1", status: "PENDING" });
    const usecase = new StartWorkflowRunUseCase(mockGateway as any);
    const result = await usecase.execute("wf-1", "proj-1");
    expect(mockGateway.startWorkflowRun).toHaveBeenCalledWith("wf-1", "proj-1");
    expect(result.status).toBe("PENDING");
  });

  it("ToggleYoloModeUseCase calls gateway with runId & value", async () => {
    mockGateway.toggleYoloMode.mockResolvedValueOnce({ id: "run-1", yoloMode: true });
    const usecase = new ToggleYoloModeUseCase(mockGateway as any);
    const result = await usecase.execute("run-1", true);
    expect(mockGateway.toggleYoloMode).toHaveBeenCalledWith("run-1", true);
    expect(result.yoloMode).toBe(true);
  });

  it("SubmitStepApprovalDecisionUseCase calls gateway with details", async () => {
    mockGateway.submitStepApproval.mockResolvedValueOnce({ id: "wrs-1", status: "DONE" });
    const usecase = new SubmitStepApprovalDecisionUseCase(mockGateway as any);
    const result = await usecase.execute("wrs-1", true, "Looks good");
    expect(mockGateway.submitStepApproval).toHaveBeenCalledWith("wrs-1", true, "Looks good");
    expect(result.status).toBe("DONE");
  });
});
