import { describe, expect, it, vi } from "vitest";
import * as edgeClient from "@/data/datasource/supabase/edge-function-client";

import { SupabaseWorkflowEngineGateway } from "./supabase-workflow-engine-gateway";

describe("SupabaseWorkflowEngineGateway", () => {
  it("lists step definitions ordered by name", async () => {
    const order = vi.fn().mockResolvedValue({
      data: [
        {
          step_type: "tech_spec",
          name: "Technical Spec",
          description: "Generate specs",
          required_mcps: [],
          required_skills: [],
          agent_type: "standard",
        },
      ],
      error: null,
    });
    const from = vi.fn(() => ({
      select: () => ({ order }),
    }));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.listStepDefinitions();

    expect(from).toHaveBeenCalledWith("step_definitions");
    expect(order).toHaveBeenCalledWith("name", { ascending: true });
    expect(result[0].stepType).toBe("tech_spec");
  });

  it("lists global and project-private workflows", async () => {
    const orderSpy = vi.fn().mockResolvedValue({
      data: [
        {
          id: "w-1",
          project_id: null,
          name: "Plan Features",
          description: "Details",
          is_template: false,
          provider_override: null,
          model_override: null,
          created_by: "seed",
          created_at: "2026-05-20T00:00:00Z",
          updated_at: "2026-05-20T00:00:00Z",
        },
      ],
      error: null,
    });
    const orSpy = vi.fn(() => ({ order: orderSpy }));
    const from = vi.fn(() => ({
      select: () => ({ or: orSpy }),
    }));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.listWorkflows("p-1");

    expect(from).toHaveBeenCalledWith("workflows");
    expect(orSpy).toHaveBeenCalledWith("project_id.is.null,project_id.eq.p-1");
    expect(result[0].projectId).toBe(null);
  });

  it("throws a clear error when workflow update returns no row", async () => {
    const maybeSingle = vi.fn().mockResolvedValue({
      data: null,
      error: null,
    });
    const select = vi.fn(() => ({ maybeSingle }));
    const eq = vi.fn(() => ({ select }));
    const update = vi.fn(() => ({ eq }));
    const from = vi.fn((table: string) => {
      if (table === "workflows") {
        return { update };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);

    await expect(
      gateway.saveWorkflow({
        id: "wf-1",
        projectId: null,
        name: "Global workflow",
        description: "Reusable",
        isTemplate: false,
        providerOverride: null,
        modelOverride: null,
        steps: [],
      })
    ).rejects.toThrow("Unable to update workflow: no row was returned. Check workflow write policies.");
  });

  it("toggles YOLO mode via edge function", async () => {
    vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction").mockResolvedValueOnce(
      {
        id: "run-123",
        workflow_id: "w-1",
        project_id: "p-1",
        status: "RUNNING",
        yolo_mode: true,
        started_by: "dev",
        started_at: "2026-05-20T00:00:00Z",
      } as any
    );
    const from = vi.fn(() => ({}));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.toggleYoloMode("run-123", true);

    expect(edgeClient.invokeSupabaseEdgeFunction).toHaveBeenCalledWith(
      "workflow-engine-toggle-yolo-mode",
      { runId: "run-123", yoloMode: true }
    );
    expect(result.yoloMode).toBe(true);
  });

  it("starts workflow runs via edge function", async () => {
    vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction").mockResolvedValueOnce(
      {
        id: "run-999",
        workflow_id: "w-1",
        project_id: "p-1",
        status: "RUNNING",
        yolo_mode: false,
        started_by: "dev",
        started_at: "2026-05-20T00:00:00Z",
      } as any
    );
    const from = vi.fn(() => ({}));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.startWorkflowRun("w-1", "p-1");

    expect(edgeClient.invokeSupabaseEdgeFunction).toHaveBeenCalledWith(
      "workflow-engine-start-run",
      { workflowId: "w-1", projectId: "p-1" }
    );
    expect(result.status).toBe("RUNNING");
  });

  it("submits approval/rejection decision via edge function", async () => {
    vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction").mockResolvedValueOnce(
      {
        id: "wrs-1",
        workflow_run_id: "run-1",
        step_type: "tech_spec",
        status: "PENDING",
        retry_count: 3,
        rejection_note: "Improve detail",
      } as any
    );
    const from = vi.fn(() => ({}));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.submitStepApproval("wrs-1", false, "Improve detail");

    expect(edgeClient.invokeSupabaseEdgeFunction).toHaveBeenCalledWith(
      "workflow-engine-submit-step-approval",
      { stepId: "wrs-1", approve: false, comment: "Improve detail" }
    );
    expect(result.status).toBe("PENDING");
    expect(result.retryCount).toBe(3);
    expect(result.rejectionNote).toBe("Improve detail");
  });
});
