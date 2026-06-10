import { afterEach, describe, expect, it, vi } from "vitest";
import * as edgeClient from "@/data/datasource/supabase/edge-function-client";

import { SupabaseWorkflowEngineGateway } from "./supabase-workflow-engine-gateway";

describe("SupabaseWorkflowEngineGateway", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("lists step definitions ordered by name", async () => {
    const definitionsOrder = vi.fn().mockResolvedValue({
      data: [
        {
          step_type: "tech_spec",
          name: "Technical Spec",
          description: "Generate specs",
          prompt_base: "Generate specs carefully.",
          required_mcps: [],
          required_skills: [],
          model: "gpt-5.5",
          reasoning_effort: "medium",
          agent_type: "standard",
        },
      ],
      error: null,
    });
    const inputOrder = vi.fn().mockResolvedValue({
      data: [
        {
          step_type: "tech_spec",
          artifact_definition_key: "business_summary_artifact",
          order_index: 0,
        },
      ],
      error: null,
    });
    const outputOrder = vi.fn().mockResolvedValue({
      data: [
        {
          step_type: "tech_spec",
          artifact_definition_key: "tech_spec_artifact",
          order_index: 0,
        },
      ],
      error: null,
    });
    const from = vi.fn((table: string) => {
      if (table === "step_definitions") {
        return { select: () => ({ order: definitionsOrder }) };
      }
      if (table === "step_input_artifact_definitions") {
        return { select: () => ({ order: inputOrder }) };
      }
      if (table === "step_output_artifact_definitions") {
        return { select: () => ({ order: outputOrder }) };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.listStepDefinitions();

    expect(from).toHaveBeenCalledWith("step_definitions");
    expect(definitionsOrder).toHaveBeenCalledWith("name", { ascending: true });
    expect(result[0].stepType).toBe("tech_spec");
    expect(result[0].reasoningEffort).toBe("medium");
    expect(result[0].inputArtifactDefinitions).toEqual(["business_summary_artifact"]);
    expect(result[0].outputArtifactDefinitions).toEqual(["tech_spec_artifact"]);
  });

  it("saves step definition artifact bindings", async () => {
    const maybeSingle = vi.fn().mockResolvedValue({
      data: {
        step_type: "tech_spec",
        name: "Technical Spec",
        description: "Generate specs",
        prompt_base: "Generate specs carefully.",
        required_mcps: [],
        required_skills: [],
        model: "gpt-5.5",
        reasoning_effort: "high",
        agent_type: "standard",
      },
      error: null,
    });
    const select = vi.fn(() => ({ maybeSingle }));
    const upsert = vi.fn(() => ({ select }));
    const deleteInputEq = vi.fn().mockResolvedValue({ error: null });
    const deleteOutputEq = vi.fn().mockResolvedValue({ error: null });
    const inputInsert = vi.fn().mockResolvedValue({ error: null });
    const outputInsert = vi.fn().mockResolvedValue({ error: null });
    const from = vi.fn((table: string) => {
      if (table === "step_definitions") {
        return { upsert };
      }
      if (table === "step_input_artifact_definitions") {
        return {
          delete: () => ({ eq: deleteInputEq }),
          insert: inputInsert,
          select: () => ({ order: vi.fn() }),
        };
      }
      if (table === "step_output_artifact_definitions") {
        return {
          delete: () => ({ eq: deleteOutputEq }),
          insert: outputInsert,
          select: () => ({ order: vi.fn() }),
        };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    vi.spyOn(gateway, "listStepDefinitions").mockResolvedValueOnce([
      {
        stepType: "tech_spec",
        name: "Technical Spec",
        description: "Generate specs",
        promptBase: "Generate specs carefully.",
        requiredMcps: [],
        requiredSkills: [],
        model: "gpt-5.5",
        reasoningEffort: "high",
        agentType: "standard",
        inputArtifactDefinitions: ["business_summary_artifact"],
        outputArtifactDefinitions: ["tech_spec_artifact"],
        createdAt: "2026-05-20T00:00:00Z",
        updatedAt: "2026-05-20T00:00:00Z",
      },
    ] as any);
    const result = await gateway.saveStepDefinition({
      stepType: "tech_spec",
      name: "Technical Spec",
      description: "Generate specs",
      promptBase: "Generate specs carefully.",
      requiredMcps: ["google_drive"],
      mcpAccessMode: "read_write",
      requiredSkills: [],
      model: "gpt-5.5",
      reasoningEffort: "high",
      agentType: "standard",
      inputArtifactDefinitions: ["business_summary_artifact"],
      outputArtifactDefinitions: ["tech_spec_artifact"],
      createdAt: "2026-05-20T00:00:00Z",
      updatedAt: "2026-05-20T01:00:00Z",
    });

    expect(upsert).toHaveBeenCalledWith(
      expect.objectContaining({
        step_type: "tech_spec",
        name: "Technical Spec",
        description: "Generate specs",
        prompt_base: "Generate specs carefully.",
        required_mcps: ["google_drive"],
        mcp_access_mode: "read_write",
        required_skills: [],
        model: "gpt-5.5",
        reasoning_effort: "high",
        agent_type: "standard",
      }),
      { onConflict: "step_type" }
    );
    expect(deleteInputEq).toHaveBeenCalledWith("step_type", "tech_spec");
    expect(deleteOutputEq).toHaveBeenCalledWith("step_type", "tech_spec");
    expect(inputInsert).toHaveBeenCalled();
    expect(outputInsert).toHaveBeenCalled();
    expect(result.inputArtifactDefinitions).toEqual(["business_summary_artifact"]);
    expect(result.outputArtifactDefinitions).toEqual(["tech_spec_artifact"]);
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
    const neqSpy = vi.fn(() => ({ or: orSpy, order: orderSpy }));
    const from = vi.fn(() => ({
      select: () => ({ neq: neqSpy }),
    }));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.listWorkflows("p-1");

    expect(from).toHaveBeenCalledWith("workflows");
    expect(neqSpy).toHaveBeenCalledWith("created_by", "flowpilot-runtime");
    expect(orSpy).toHaveBeenCalledWith("project_id.is.null,project_id.eq.p-1");
    expect(result[0].projectId).toBe(null);
  });

  it("recovers synced artifacts from Supabase Storage when shared rows are missing", async () => {
    const order = vi.fn().mockResolvedValue({
      data: [],
      error: null,
    });
    const list = vi
      .fn()
      .mockResolvedValueOnce({
        data: [{ id: null, name: "project-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "runs" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "run-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "steps" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "codex_test" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "artifacts" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "artifact-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [
          {
            id: "object-1",
            name: "Response.md",
            created_at: "2026-06-03T00:00:00.000Z",
            updated_at: "2026-06-03T00:01:00.000Z",
          },
        ],
        error: null,
      });
    const from = vi.fn((table: string) => {
      if (table === "artifact_runs") {
        return {
          select: () => ({
            eq: () => ({ order }),
            order,
          }),
        };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({
      from,
      storage: {
        from: vi.fn(() => ({ list })),
      },
    } as any);
    const result = await gateway.listArtifactRuns();

    expect(result).toEqual([
      expect.objectContaining({
        id: "remote:projects/project-1/runs/run-1/steps/codex_test/artifacts/artifact-1/Response.md",
        projectId: "project-1",
        workflowRunId: "run-1",
        workflowRunStepId: "codex_test",
        title: "Response.md",
        remotePath: "projects/project-1/runs/run-1/steps/codex_test/artifacts/artifact-1/Response.md",
        remoteObjectId: "object-1",
        storageProvider: "supabase",
        syncStatus: "synced",
      }),
    ]);
  });

  it("merges artifact-scoped storage recovery into the matching shared artifact row", async () => {
    const order = vi.fn().mockResolvedValue({
      data: [
        {
          id: "artifact-1",
          artifact_definition_key: null,
          project_id: "project-1",
          workflow_id: "workflow-1",
          workflow_run_id: "run-1",
          workflow_run_step_id: "D2DAF71C-625E-403D-B0E0-D418387A4E77",
          title: "Response.md",
          local_path: "",
          remote_path: "",
          remote_url: "",
          storage_provider: null,
          remote_object_id: null,
          sync_status: "local_only",
          created_at: "2026-06-03T00:00:00.000Z",
          updated_at: "2026-06-03T00:01:00.000Z",
        },
      ],
      error: null,
    });
    const replicaIn = vi.fn().mockResolvedValue({ data: [], error: null });
    const list = vi
      .fn()
      .mockResolvedValueOnce({
        data: [{ id: null, name: "project-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "runs" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "run-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "steps" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "codex_test" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "artifacts" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "artifact-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [
          {
            id: "object-1",
            name: "Response.md",
            created_at: "2026-06-03T00:00:00.000Z",
            updated_at: "2026-06-03T00:01:00.000Z",
          },
        ],
        error: null,
      });
    const from = vi.fn((table: string) => {
      if (table === "artifact_runs") {
        return {
          select: () => ({
            eq: () => ({ order }),
            order,
          }),
        };
      }
      if (table === "artifact_run_replicas") {
        return {
          select: () => ({
            in: replicaIn,
          }),
        };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({
      from,
      storage: {
        from: vi.fn(() => ({ list })),
      },
    } as any);
    const result = await gateway.listArtifactRuns();

    expect(replicaIn).toHaveBeenCalledWith("artifact_run_id", ["artifact-1"]);
    expect(result).toHaveLength(1);
    expect(result[0]).toEqual(
      expect.objectContaining({
        id: "artifact-1",
        workflowRunStepId: "D2DAF71C-625E-403D-B0E0-D418387A4E77",
        remotePath: "",
      }),
    );
    expect(result[0]?.replicas).toEqual([
      expect.objectContaining({
        artifactRunId: "artifact-1",
        provider: "supabase",
        remotePath: "projects/project-1/runs/run-1/steps/codex_test/artifacts/artifact-1/Response.md",
        remoteObjectId: "object-1",
        syncStatus: "synced",
      }),
    ]);
  });

  it("parses legacy storage-backed artifact paths without artifact folders", async () => {
    const order = vi.fn().mockResolvedValue({ data: [], error: null });
    const list = vi
      .fn()
      .mockResolvedValueOnce({
        data: [{ id: null, name: "project-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "runs" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "run-1" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "steps" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [{ id: null, name: "codex_test" }],
        error: null,
      })
      .mockResolvedValueOnce({
        data: [
          {
            id: "object-1",
            name: "Response.md",
            created_at: "2026-06-03T00:00:00.000Z",
            updated_at: "2026-06-03T00:01:00.000Z",
          },
        ],
        error: null,
      });
    const from = vi.fn((table: string) => {
      if (table === "artifact_runs") {
        return {
          select: () => ({
            eq: () => ({ order }),
            order,
          }),
        };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({
      from,
      storage: {
        from: vi.fn(() => ({ list })),
      },
    } as any);
    const result = await gateway.listArtifactRuns();

    expect(result).toEqual([
      expect.objectContaining({
        id: "remote:projects/project-1/runs/run-1/steps/codex_test/Response.md",
        remotePath: "projects/project-1/runs/run-1/steps/codex_test/Response.md",
        title: "Response.md",
      }),
    ]);
  });

  it("deletes workflow runs by id list", async () => {
    const inSpy = vi.fn().mockResolvedValue({ error: null });
    const deleteSpy = vi.fn(() => ({ in: inSpy }));
    const from = vi.fn((table: string) => {
      if (table === "workflow_runs") {
        return { delete: deleteSpy };
      }
      throw new Error(`Unexpected table ${table}`);
    });

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    await gateway.deleteWorkflowRuns(["run-1", "run-2"]);

    expect(from).toHaveBeenCalledWith("workflow_runs");
    expect(inSpy).toHaveBeenCalledWith("id", ["run-1", "run-2"]);
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
      if (table === "ai_supported_models") {
        return {
          select: vi.fn(() => ({
            eq: vi.fn(() => ({
              maybeSingle: vi.fn().mockResolvedValue({ data: null, error: null }),
            })),
          })),
        };
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
          reasoningEffortOverride: null,
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

  it("starts workflow runs via the local runtime API when running in the browser", async () => {
    const edgeSpy = vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction");
    vi.stubGlobal("window", {} as Window & typeof globalThis);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            id: "run-999",
            workflow_id: "w-1",
            project_id: "p-1",
            status: "RUNNING",
            yolo_mode: false,
            started_by: "dev",
            started_at: "2026-05-20T00:00:00Z",
          }),
          { status: 200 },
        ),
      ),
    );
    const from = vi.fn(() => ({}));
    const auth = {
      getSession: vi.fn().mockResolvedValue({
        data: { session: { access_token: "token-123" } },
        error: null,
      }),
    };

    const gateway = new SupabaseWorkflowEngineGateway({ from, auth } as any);
    const result = await gateway.startWorkflowRun({
      workflowId: "w-1",
      projectId: "p-1",
      startMode: "workflow-definition",
      beginPrompt: "Build the result.",
    });

    expect(fetch).toHaveBeenCalledWith("/api/workflow-engine/start-run", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer token-123",
      },
      body: JSON.stringify({
        workflowId: "w-1",
        projectId: "p-1",
        startMode: "workflow-definition",
        beginPrompt: "Build the result.",
      }),
    });
    expect(auth.getSession).toHaveBeenCalled();
    expect(edgeSpy).not.toHaveBeenCalled();
    expect(result.status).toBe("RUNNING");
  });

  it("starts single-step workflow runs without a workflow id", async () => {
    vi.stubGlobal("window", {} as Window & typeof globalThis);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            id: "run-1000",
            workflow_id: "runtime-workflow",
            project_id: "p-1",
            status: "RUNNING",
            yolo_mode: false,
            started_by: "dev",
            started_at: "2026-05-20T00:00:00Z",
          }),
          { status: 200 },
        ),
      ),
    );
    const from = vi.fn(() => ({}));
    const auth = {
      getSession: vi.fn().mockResolvedValue({
        data: { session: { access_token: "token-123" } },
        error: null,
      }),
    };

    const gateway = new SupabaseWorkflowEngineGateway({ from, auth } as any);
    await gateway.startWorkflowRun({
      projectId: "p-1",
      startMode: "single-step",
      beginPrompt: "Build the result.",
      stepType: "business_idea",
    });

    expect(fetch).toHaveBeenCalledWith("/api/workflow-engine/start-run", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer token-123",
      },
      body: JSON.stringify({
        projectId: "p-1",
        startMode: "single-step",
        beginPrompt: "Build the result.",
        stepType: "business_idea",
      }),
    });
  });

  it("surfaces an error when the local runtime endpoint is missing", async () => {
    const edgeSpy = vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction");
    vi.stubGlobal("window", {} as Window & typeof globalThis);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(Response.json({ error: "Not Found" }, { status: 404 })),
    );
    const from = vi.fn(() => ({}));
    const auth = {
      getSession: vi.fn().mockResolvedValue({
        data: { session: { access_token: "token-123" } },
        error: null,
      }),
    };

    const gateway = new SupabaseWorkflowEngineGateway({ from, auth } as any);
    await expect(
      gateway.startWorkflowRun({
        workflowId: "w-1",
        projectId: "p-1",
        startMode: "workflow-definition",
        beginPrompt: "Build the result.",
      }),
    ).rejects.toThrow("Not Found");

    expect(fetch).toHaveBeenCalledWith("/api/workflow-engine/start-run", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer token-123",
      },
      body: JSON.stringify({
        workflowId: "w-1",
        projectId: "p-1",
        startMode: "workflow-definition",
        beginPrompt: "Build the result.",
      }),
    });
    expect(edgeSpy).not.toHaveBeenCalledWith("workflow-engine-start-run", expect.anything());
  });

  it("submits approval decisions via edge function", async () => {
    const edgeSpy = vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction").mockResolvedValueOnce(
      {
        id: "wrs-1",
        workflow_run_id: "run-1",
        step_type: "tech_spec",
        status: "DONE",
        retry_count: 3,
        rejection_note: null,
      } as any
    );
    const from = vi.fn(() => ({}));

    const gateway = new SupabaseWorkflowEngineGateway({ from } as any);
    const result = await gateway.submitStepApproval("wrs-1", true, "Looks good");

    expect(edgeSpy).toHaveBeenCalledWith(
      "workflow-engine-submit-step-approval",
      { stepId: "wrs-1", approve: true, comment: "Looks good" }
    );
    expect(result.status).toBe("DONE");
    expect(result.retryCount).toBe(3);
    expect(result.rejectionNote).toBeNull();
  });

  it("submits follow-up prompts through the local runtime API in the browser", async () => {
    vi.stubGlobal("window", {});
    const edgeSpy = vi.spyOn(edgeClient, "invokeSupabaseEdgeFunction");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          id: "wrs-1",
          workflow_run_id: "run-1",
          step_type: "tech_spec",
          status: "DONE",
          retry_count: 4,
          rejection_note: null,
        }),
      }),
    );
    const from = vi.fn(() => ({}));

    const gateway = new SupabaseWorkflowEngineGateway({
      from,
      auth: {
        getSession: vi.fn(async () => ({
          data: { session: { access_token: "token-123" } },
        })),
      },
    } as any);
    const result = await gateway.submitStepApproval("wrs-1", false, "Make it shorter");

    expect(fetch).toHaveBeenCalledWith("/api/workflow-engine/submit-step-approval", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer token-123",
      },
      body: JSON.stringify({
        stepId: "wrs-1",
        approve: false,
        comment: "Make it shorter",
      }),
    });
    expect(edgeSpy).not.toHaveBeenCalled();
    expect(result.status).toBe("DONE");
    expect(result.retryCount).toBe(4);
  });
});
