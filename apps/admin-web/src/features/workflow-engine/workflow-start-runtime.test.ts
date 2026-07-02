import { mkdtemp, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  buildWorkflowStepFollowUpPrompt,
  buildWorkflowStepPrompt,
  createArtifactOutputs,
  createGoogleDriveWriteAuditArtifacts,
  createSingleStepWorkflow,
  createLocalWorkflowOutputArtifactSnapshot,
  deactivateWorkflowRunSession,
  finalizeWorkflowRunSessions,
  resolveArtifactWorkspaceRoot,
  resolveProviderKeyFromModel,
  submitGoogleDriveWriteApprovalRuntime,
  submitWorkflowStepFollowUpRuntime,
  syncWorkflowRunSessionProviderSessionId,
  sendMessageWithRetry,
} from "./workflow-start-runtime";
import { INTERRUPTED_RUN_ERROR } from "./workflow-run-interruption";

describe("workflow-start-runtime", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("routes supported models to the correct local provider", () => {
    expect(resolveProviderKeyFromModel("gpt-5.5")).toBe("codex");
    expect(resolveProviderKeyFromModel("gemini-pro")).toBe("gemini");
    expect(resolveProviderKeyFromModel("auto-gemini-3")).toBe("gemini");
    expect(resolveProviderKeyFromModel("gemini-3.5-flash-medium")).toBe("gemini");
    expect(resolveProviderKeyFromModel("claude-sonnet")).toBe("claude");
  });

  it("builds prompts with runtime context and artifact paths", () => {
    const prompt = buildWorkflowStepPrompt({
      beginPrompt: "Create the implementation plan.",
      inputArtifactPaths: ["C:/repo/.flowpilot/artifacts/input.md"],
      model: "gpt-5.5",
      outputArtifactPaths: ["C:/repo/.flowpilot/artifacts/output.md"],
      promptBase: "Turn the requirement into a concrete implementation plan.",
      stepType: "make_plan_coding",
      subagent: "planner-agent",
      teamRole: "tech_lead",
      workingDirectory: "C:/repo",
      requiredSkills: ["planner-skill"],
    });

    expect(prompt).toContain("Step type: make_plan_coding");
    expect(prompt).toContain("Model: gpt-5.5");
    expect(prompt).toContain("Working directory: C:/repo");
    expect(prompt).toContain("tech_lead");
    expect(prompt).toContain("planner-agent");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/input.md");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/output.md");
  });

  it("builds follow-up prompts without replaying the initial begin prompt", () => {
    const prompt = buildWorkflowStepFollowUpPrompt({
      followUpPrompt: "Make the artifact shorter.",
      inputArtifactPaths: ["C:/repo/.flowpilot/artifacts/input.md"],
      outputArtifactPaths: ["C:/repo/.flowpilot/artifacts/output.md"],
      workingDirectory: "C:/repo",
    });

    expect(prompt).toContain("Make the artifact shorter.");
    expect(prompt).toContain("Artifacts To Review");
    expect(prompt).toContain("C:/repo/.flowpilot/artifacts/output.md");
    expect(prompt).not.toContain("## Begin Prompt");
    expect(prompt).not.toContain("## Prompt Base");
  });

  it("omits empty follow-up prompt sections instead of rendering none markers", () => {
    const prompt = buildWorkflowStepFollowUpPrompt({
      followUpPrompt: "Make the artifact shorter.",
      inputArtifactPaths: [],
      outputArtifactPaths: [],
      workingDirectory: "C:/repo",
    });

    expect(prompt).not.toContain("## Artifacts To Review");
    expect(prompt).not.toContain("## Related Input Artifacts");
    expect(prompt).not.toContain("- None");
    expect(prompt).toContain("## Execution Context");
  });

  it("uses the local runner cwd as the workflow artifact workspace root", async () => {
    await expect(
      resolveArtifactWorkspaceRoot({
        getHealth: vi.fn(async () => ({
          status: "online" as const,
          runnerVersion: "test",
          cwd: "/Users/tiendat/Desktop/flowpilot/flowpilot",
          os: "darwin",
          startedAt: "2026-06-05T00:00:00.000Z",
          baseUrl: "http://127.0.0.1:3900",
          errorMessage: null,
        })),
      }),
    ).resolves.toBe("/Users/tiendat/Desktop/flowpilot/flowpilot");
  });

  it("allows replay gating for interrupted failed steps", async () => {
    const stepRow = {
      id: "step-run-1",
      workflow_run_id: "run-1",
      workflow_step_id: null,
      execution_order_index: 0,
      step_type: "test_codex_step",
      status: "FAILED",
      retry_count: 0,
      error_message: INTERRUPTED_RUN_ERROR,
    };
    const runSteps = [stepRow];
    const adminClient = {
      from: vi.fn((table: string) => {
        if (table !== "workflow_run_steps") {
          throw new Error(`Unexpected table ${table}`);
        }

        return {
          select: vi.fn().mockReturnThis(),
          eq(column: string, value: string) {
            if (column === "id") {
              return {
                maybeSingle: vi.fn().mockResolvedValue({
                  data: stepRow,
                  error: null,
                }),
              };
            }

            if (column === "workflow_run_id") {
              return {
                order: vi.fn().mockResolvedValue({
                  data: runSteps,
                  error: null,
                }),
              };
            }

            throw new Error(`Unexpected eq filter ${column}=${value}`);
          },
        };
      }),
    } as any;

    await expect(
      submitWorkflowStepFollowUpRuntime({
        adminClient,
        localRunnerGateway: {} as any,
        stepId: "step-run-1",
        comment: "Replay the prompt.",
      }),
    ).rejects.toThrow("Workflow step definition is missing for this run step.");
  });

  it("captures a local artifact snapshot even when a step has no output binding", async () => {
    const workingDirectory = await mkdtemp(path.join(os.tmpdir(), "flowpilot-runtime-"));
    const artifactWorkspaceRoot = await mkdtemp(
      path.join(os.tmpdir(), "flowpilot-artifact-root-"),
    );
    const snapshot = await createLocalWorkflowOutputArtifactSnapshot({
      outputMarkdown: "# Result\n\nHello from the step.",
      promptText: "## Begin Prompt\nsay hello",
      actualPromptText: "# Previous Conversation Context\n\n## User Prompt 1\nhello",
      projectId: "project-1",
      stepType: "test_codex_step",
      stderrText: "",
      stdoutText: "Hello from the step.",
      workflowRunId: "run-123",
      commandText: "codex exec < /tmp/prompt.txt",
      providerKey: "codex",
      artifactWorkspaceRoot,
    });

    const manifest = JSON.parse(await readFile(snapshot.manifestPath, "utf8"));
    const content = await readFile(snapshot.contentPath, "utf8");
    const actualPrompt = await readFile(manifest.actualPromptPath, "utf8");

    expect(snapshot.snapshotDirectory).toContain(
      path.join(".flowpilot", "artifacts", "project-1", "run-123", "test_codex_step"),
    );
    expect(snapshot.snapshotDirectory.startsWith(artifactWorkspaceRoot)).toBe(true);
    expect(snapshot.snapshotDirectory.startsWith(workingDirectory)).toBe(false);
    expect(manifest.workflowRunId).toBe("run-123");
    expect(manifest.sourceKind).toBe("workflow_output");
    expect(manifest.title).toBe("Response.md");
    expect(content).toContain("Hello from the step.");
    expect(actualPrompt).toContain("# Previous Conversation Context");
  });

  it("uses step-definition YOLO as the single-step launch policy", async () => {
    const workflowInsertRows: Array<Record<string, unknown>> = [];
    const workflowStepInsertRows: Array<Record<string, unknown>> = [];
    let selectedStepDefinitionColumns = "";
    const workflowsBuilder = {
      insert(row: Record<string, unknown>) {
        workflowInsertRows.push(row);
        return {
          select() {
            return {
              single: vi.fn().mockResolvedValue({
                data: {
                  id: "wf-single",
                  name: row.name,
                  project_id: row.project_id,
                  provider_override: row.provider_override,
                  model_override: row.model_override,
                  reasoning_effort_override: row.reasoning_effort_override,
                  yolo_mode: row.yolo_mode,
                },
                error: null,
              }),
            };
          },
        };
      },
    };
    const workflowStepsBuilder = {
      insert(row: Record<string, unknown>) {
        workflowStepInsertRows.push(row);
        return {
          select() {
            return {
              single: vi.fn().mockResolvedValue({
                data: {
                  id: "wf-step-single",
                  step_type: row.step_type,
                  order_index: row.order_index,
                  is_enabled: row.is_enabled,
                  provider_override: row.provider_override,
                  model_override: row.model_override,
                  reasoning_effort_override: row.reasoning_effort_override,
                  yolo_mode: row.yolo_mode,
                  requires_approval: row.requires_approval,
                },
                error: null,
              }),
            };
          },
        };
      },
    };
    const stepDefinitionsBuilder = {
      select: vi.fn((columns: string) => {
        selectedStepDefinitionColumns = columns;
        return stepDefinitionsBuilder;
      }),
      in: vi.fn().mockResolvedValue({
        data: [
          {
            step_type: "test_codex_step",
            name: "Test - Codex Single Step",
            description: "Single-step test definition.",
            prompt_base: null,
            required_mcps: ["google_drive"],
            mcp_access_mode: "read_only",
            required_skills: [],
            team_role: null,
            subagent: null,
            model: "gpt-5.4",
            reasoning_effort: "medium",
            yolo_mode: true,
          },
        ],
        error: null,
      }),
    };
    const aiSupportedModelsBuilder = {
      select: vi.fn().mockReturnThis(),
      eq: vi.fn().mockReturnThis(),
      maybeSingle: vi.fn().mockResolvedValue({
        data: { model_id: "gpt-5.4", is_enabled: true },
        error: null,
      }),
    };
    const adminClient = {
      from: vi.fn((table: string) => {
        if (table === "step_definitions") return stepDefinitionsBuilder;
        if (table === "workflows") return workflowsBuilder;
        if (table === "workflow_steps") return workflowStepsBuilder;
        if (table === "ai_supported_models") return aiSupportedModelsBuilder;
        throw new Error(`Unexpected table ${table}`);
      }),
    } as any;

    const result = await createSingleStepWorkflow(adminClient, {
      projectId: "proj-1",
      stepType: "test_codex_step",
    });

    expect(workflowInsertRows[0]).toMatchObject({
      yolo_mode: true,
    });
    expect(selectedStepDefinitionColumns).toContain("yolo_mode");
    expect(workflowStepInsertRows[0]).not.toHaveProperty("yolo_mode");
    expect(result.workflow).toMatchObject({
      yolo_mode: true,
    });
    expect(result.workflowSteps[0].yolo_mode).toBeUndefined();
  });

  it("creates a fallback artifact run when a step has no output binding", async () => {
    const workingDirectory = await mkdtemp(path.join(os.tmpdir(), "flowpilot-runtime-fallback-"));
    const artifactWorkspaceRoot = await mkdtemp(
      path.join(os.tmpdir(), "flowpilot-artifact-fallback-root-"),
    );
    const insertedRows: Array<Record<string, unknown>> = [];
    const adminClient = {
      from: vi.fn(() => ({
        insert(rows: Array<Record<string, unknown>>) {
          insertedRows.push(...rows);
          return {
            select() {
              return Promise.resolve({
                data: rows.map((row) => ({ id: row.id })),
                error: null,
              });
            },
          };
        },
      })),
    } as any;

    const result = await createArtifactOutputs({
      adminClient,
      artifactDefinitions: new Map(),
      outputArtifactKeys: [],
      outputMarkdown: "# Result\n\nHello from the fallback artifact.",
      promptText: "Prompt",
      actualPromptText: "Actual Prompt",
      projectId: "project-1",
      stepRunId: "step-run-1",
      stepType: "test_codex_step",
      stderrText: "",
      stdoutText: "ok",
      workflowId: "workflow-1",
      workflowRunId: "run-1",
      workingDirectory,
      commandText: "codex exec",
      providerKey: "codex",
      artifactWorkspaceRoot,
    });

    expect(insertedRows).toHaveLength(1);
    expect(insertedRows[0]).toMatchObject({
      id: result.artifactRunId,
      artifact_definition_key: null,
      project_id: "project-1",
      workflow_id: "workflow-1",
      workflow_run_id: "run-1",
      workflow_run_step_id: "step-run-1",
      title: "Response.md",
      remote_path: "",
      remote_url: "",
      sync_status: "local_only",
    });
    expect(String(insertedRows[0]?.local_path)).toMatch(
      /\.flowpilot[\\/]artifacts[\\/]project-1[\\/]run-1[\\/]test_codex_step[\\/]\.snapshots[\\/][^\\/]+[\\/]Response\.md$/,
    );
    expect(result.artifactRunIds).toEqual([result.artifactRunId]);
    await expect(readFile(result.checkpoint.outputContentPath, "utf8")).resolves.toContain(
      "Hello from the fallback artifact.",
    );
    await expect(
      readFile(path.join(artifactWorkspaceRoot, String(insertedRows[0]?.local_path)), "utf8"),
    ).resolves.toContain("Hello from the fallback artifact.");
    await expect(
      readFile(path.join(workingDirectory, String(insertedRows[0]?.local_path)), "utf8"),
    ).rejects.toThrow();
  });

  it("stores workflow step artifact local_path inside the snapshot folder", async () => {
    const workingDirectory = await mkdtemp(path.join(os.tmpdir(), "flowpilot-artifact-output-"));
    const artifactWorkspaceRoot = await mkdtemp(
      path.join(os.tmpdir(), "flowpilot-artifact-output-root-"),
    );
    const insertedRows: Array<Record<string, unknown>> = [];
    const adminClient = {
      from: vi.fn(() => ({
        insert(rows: Array<Record<string, unknown>>) {
          insertedRows.push(...rows);
          return {
            select() {
              return Promise.resolve({ data: [{ id: "artifact-1" }], error: null });
            },
          };
        },
      })),
    } as any;

    await createArtifactOutputs({
      adminClient,
      artifactDefinitions: new Map([
        [
          "business_idea_artifact",
          {
            key: "business_idea_artifact",
            name: "Business Idea",
            local_path_template:
              ".flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/{defaultFileName}",
            remote_path_template: "",
            default_file_name: "BusinessIdea.md",
          } as any,
        ],
      ]),
      outputArtifactKeys: ["business_idea_artifact"],
      outputMarkdown: "# Result",
      promptText: "Prompt",
      actualPromptText: "Bootstrap Prompt",
      projectId: "project-1",
      stepRunId: "step-run-1",
      stepType: "business_idea",
      stderrText: "",
      stdoutText: "ok",
      workflowId: "workflow-1",
      workflowRunId: "run-1",
      workingDirectory,
      commandText: "codex exec",
      providerKey: "codex",
      artifactWorkspaceRoot,
    });

    expect(insertedRows).toHaveLength(1);
    expect(String(insertedRows[0]?.local_path)).toMatch(
      /\.flowpilot[\\/]artifacts[\\/]project-1[\\/]run-1[\\/]business_idea[\\/]\.snapshots[\\/][^\\/]+[\\/]BusinessIdea\.md$/,
    );
    const snapshotFilePath = path.join(
      artifactWorkspaceRoot,
      String(insertedRows[0]?.local_path),
    );
    const actualPromptPath = path.join(path.dirname(snapshotFilePath), "actual-prompt.md");
    await expect(readFile(actualPromptPath, "utf8")).resolves.toBe("Bootstrap Prompt");
    await expect(
      readFile(
        path.join(
          artifactWorkspaceRoot,
          ".flowpilot",
          "artifacts",
          "project-1",
          "run-1",
          "business_idea",
          "BusinessIdea.md",
        ),
        "utf8",
      ),
    ).resolves.toContain("# Result");
    await expect(
      readFile(
        path.join(
          workingDirectory,
          ".flowpilot",
          "artifacts",
          "project-1",
          "run-1",
          "business_idea",
          "BusinessIdea.md",
        ),
        "utf8",
      ),
    ).rejects.toThrow();
  });

  it("reroutes dot-slash flowpilot output targets to the FlowPilot workspace root", async () => {
    const workingDirectory = await mkdtemp(path.join(os.tmpdir(), "flowpilot-dot-output-"));
    const artifactWorkspaceRoot = await mkdtemp(
      path.join(os.tmpdir(), "flowpilot-dot-output-root-"),
    );
    const insertedRows: Array<Record<string, unknown>> = [];
    const adminClient = {
      from: vi.fn(() => ({
        insert(rows: Array<Record<string, unknown>>) {
          insertedRows.push(...rows);
          return {
            select() {
              return Promise.resolve({ data: [{ id: "artifact-1" }], error: null });
            },
          };
        },
      })),
    } as any;

    await createArtifactOutputs({
      adminClient,
      artifactDefinitions: new Map([
        [
          "business_idea_artifact",
          {
            key: "business_idea_artifact",
            name: "Business Idea",
            local_path_template:
              "./.flowpilot/artifacts/{projectId}/{workflowRunId}/{stepType}/{defaultFileName}",
            remote_path_template: "",
            default_file_name: "BusinessIdea.md",
          } as any,
        ],
      ]),
      outputArtifactKeys: ["business_idea_artifact"],
      outputMarkdown: "# Result",
      promptText: "Prompt",
      actualPromptText: "Bootstrap Prompt",
      projectId: "project-1",
      stepRunId: "step-run-1",
      stepType: "business_idea",
      stderrText: "",
      stdoutText: "ok",
      workflowId: "workflow-1",
      workflowRunId: "run-1",
      workingDirectory,
      commandText: "codex exec",
      providerKey: "codex",
      artifactWorkspaceRoot,
    });

    expect(insertedRows).toHaveLength(1);
    await expect(
      readFile(
        path.join(
          artifactWorkspaceRoot,
          ".flowpilot",
          "artifacts",
          "project-1",
          "run-1",
          "business_idea",
          "BusinessIdea.md",
        ),
        "utf8",
      ),
    ).resolves.toContain("# Result");
    await expect(
      readFile(
        path.join(
          workingDirectory,
          ".flowpilot",
          "artifacts",
          "project-1",
          "run-1",
          "business_idea",
          "BusinessIdea.md",
        ),
        "utf8",
      ),
    ).rejects.toThrow();
  });

  it("keeps non-flowpilot output targets inside the project working directory", async () => {
    const workingDirectory = await mkdtemp(path.join(os.tmpdir(), "flowpilot-project-output-"));
    const artifactWorkspaceRoot = await mkdtemp(
      path.join(os.tmpdir(), "flowpilot-project-output-root-"),
    );
    const insertedRows: Array<Record<string, unknown>> = [];
    const adminClient = {
      from: vi.fn(() => ({
        insert(rows: Array<Record<string, unknown>>) {
          insertedRows.push(...rows);
          return {
            select() {
              return Promise.resolve({ data: [{ id: "artifact-1" }], error: null });
            },
          };
        },
      })),
    } as any;

    await createArtifactOutputs({
      adminClient,
      artifactDefinitions: new Map([
        [
          "project_note",
          {
            key: "project_note",
            name: "Project Note",
            local_path_template: "docs/{defaultFileName}",
            remote_path_template: "",
            default_file_name: "ProjectNote.md",
          } as any,
        ],
      ]),
      outputArtifactKeys: ["project_note"],
      outputMarkdown: "# Project Note",
      promptText: "Prompt",
      actualPromptText: "Bootstrap Prompt",
      projectId: "project-1",
      stepRunId: "step-run-1",
      stepType: "business_idea",
      stderrText: "",
      stdoutText: "ok",
      workflowId: "workflow-1",
      workflowRunId: "run-1",
      workingDirectory,
      commandText: "codex exec",
      providerKey: "codex",
      artifactWorkspaceRoot,
    });

    expect(insertedRows).toHaveLength(1);
    await expect(
      readFile(path.join(workingDirectory, "docs", "ProjectNote.md"), "utf8"),
    ).resolves.toContain("# Project Note");
    await expect(
      readFile(path.join(artifactWorkspaceRoot, "docs", "ProjectNote.md"), "utf8"),
    ).rejects.toThrow();
  });

  it("persists the real provider session id back to the workflow session row", async () => {
    const calls: {
      updatePayload?: Record<string, unknown>;
      eqArgs: Array<[string, string]>;
      filterArgs: Array<[string, string, string]>;
    } = {
      eqArgs: [],
      filterArgs: [],
    };
    const query = {
      insert() {
        return query;
      },
      update(payload: Record<string, unknown>) {
        calls.updatePayload = payload;
        return query;
      },
      eq(column: string, value: string) {
        calls.eqArgs.push([column, value]);
        return query;
      },
      filter(column: string, operator: string, value: string) {
        calls.filterArgs.push([column, operator, value]);
        return query;
      },
      then(resolve: (value: { error: null }) => void) {
        resolve({ error: null });
      },
    };
    const adminClient = {
      from: vi.fn(() => query),
    } as any;

    await syncWorkflowRunSessionProviderSessionId({
      adminClient,
      workflowRunId: "run-123",
      stepRunId: "step-456",
      subagent: null,
      providerKey: "claude",
      modelName: "claude-sonnet",
      handle: {
        transportType: "claude_stream_json",
        providerSessionId: "sess-initial",
        processKey: "proc-1",
      },
      providerSessionId: "sess-real",
    });

    expect(calls.updatePayload).toMatchObject({
      provider: "claude",
      model: "claude-sonnet",
      transport_type: "claude_stream_json",
      provider_session_id: "sess-real",
      process_key: "proc-1",
      status: "active",
    });
    expect(calls.eqArgs).toEqual([["workflow_run_id", "run-123"]]);
    expect(calls.filterArgs).toEqual([["metadata_json->>is_main", "eq", "true"]]);
  });

  it("deactivates a workflow session before falling back to one-shot execution", async () => {
    const calls: {
      updatePayload?: Record<string, unknown>;
      eqArgs: Array<[string, string]>;
      filterArgs: Array<[string, string, string]>;
    } = {
      eqArgs: [],
      filterArgs: [],
    };
    const query = {
      insert() {
        return query;
      },
      update(payload: Record<string, unknown>) {
        calls.updatePayload = payload;
        return query;
      },
      eq(column: string, value: string) {
        calls.eqArgs.push([column, value]);
        return query;
      },
      filter(column: string, operator: string, value: string) {
        calls.filterArgs.push([column, operator, value]);
        return query;
      },
      then(resolve: (value: { error: null }) => void) {
        resolve({ error: null });
      },
    };
    const localRunnerGateway = {
      closeSession: vi.fn().mockResolvedValue(undefined),
    } as any;
    const adminClient = {
      from: vi.fn(() => query),
    } as any;

    await deactivateWorkflowRunSession({
      adminClient,
      workflowRunId: "run-123",
      stepRunId: "step-456",
      subagent: "review-agent",
      localRunnerGateway,
      handle: {
        transportType: "gemini_acp",
        providerSessionId: "sess-789",
        processKey: "proc-9",
      },
    });

    expect(localRunnerGateway.closeSession).toHaveBeenCalledWith({
      transportType: "gemini_acp",
      providerSessionId: "sess-789",
      processKey: "proc-9",
    });
    expect(calls.updatePayload).toMatchObject({
      status: "completed",
      process_key: null,
    });
    expect(typeof calls.updatePayload?.completed_at).toBe("string");
    expect(calls.eqArgs).toEqual([["workflow_run_id", "run-123"]]);
    expect(calls.filterArgs).toEqual([["metadata_json->>step_run_id", "eq", "step-456"]]);
  });

  it("preserves the main session after a successful run while closing subagent sessions", async () => {
    const closedSessions: Array<Record<string, string | null>> = [];
    const updatedIds: string[] = [];
    const sessions = [
      {
        id: "session-main",
        status: "active",
        transport_type: "codex_mcp",
        provider_session_id: "thread-main",
        process_key: "proc-main",
        metadata_json: { is_main: true },
      },
      {
        id: "session-subagent",
        status: "active",
        transport_type: "gemini_acp",
        provider_session_id: "thread-sub",
        process_key: "proc-sub",
        metadata_json: { step_run_id: "step-456" },
      },
    ];

    const updateQuery = {
      eq(column: string, value: string) {
        if (column === "id") {
          updatedIds.push(value);
        }
        return Promise.resolve({ error: null });
      },
    };

    const selectQuery = {
      eq(column: string, value: string) {
        if (column === "status") {
          return Promise.resolve({ data: sessions, error: null });
        }
        return selectQuery;
      },
    };

    const adminClient = {
      from: vi.fn((table: string) => {
        if (table !== "workflow_run_sessions") {
          throw new Error(`Unexpected table ${table}`);
        }
        return {
          select() {
            return selectQuery;
          },
          update() {
            return updateQuery;
          },
        };
      }),
    } as any;

    const localRunnerGateway = {
      closeSession: vi.fn(async (payload: Record<string, string | null>) => {
        closedSessions.push(payload);
      }),
    } as any;

    await finalizeWorkflowRunSessions(
      adminClient,
      localRunnerGateway,
      "run-123",
      { preserveMainSession: true },
    );

    expect(localRunnerGateway.closeSession).toHaveBeenCalledTimes(1);
    expect(closedSessions).toEqual([
      {
        transportType: "gemini_acp",
        providerSessionId: "thread-sub",
        processKey: "proc-sub",
      },
    ]);
    expect(updatedIds).toEqual(["session-subagent"]);
  });

  it("creates sanitized Google Drive write audit artifacts", async () => {
    const artifactWorkspaceRoot = await mkdtemp(
      path.join(os.tmpdir(), "flowpilot-gdrive-audit-"),
    );
    const insertedRows: Array<Record<string, unknown>> = [];
    const queryBuilder = {
      select: vi.fn().mockReturnThis(),
      eq: vi.fn().mockReturnThis(),
      maybeSingle: vi.fn().mockResolvedValue({ data: null, error: null }),
      insert: vi.fn((rows: Array<Record<string, unknown>>) => {
        insertedRows.push(...rows);
        return { error: null };
      }),
    };
    const adminClient = {
      from: vi.fn(() => queryBuilder),
    } as any;
    const localRunnerGateway = {
      listGoogleDriveProxyApprovals: vi.fn().mockResolvedValue([
        {
          id: "approval-1",
          workflowRunId: "run-1",
          workflowStepRunId: "step-1",
          processKey: "proc-1",
          toolName: "createGoogleDoc",
          operation: "write",
          canonicalArgsJson: "{\"content\":\"secret body\"}",
          argumentsHash: "hash-1",
          targetSummary: "Create Google Doc \"Plan\"",
          status: "executed",
          decisionMode: "yolo",
          requestedAt: "2026-06-09T00:00:00Z",
          decidedAt: "2026-06-09T00:00:01Z",
          expiresAt: "2026-06-09T01:00:00Z",
          resultDriveId: "drive-1",
          resultDriveUrl: "https://docs.google.com/document/d/drive-1",
        },
        {
          id: "approval-2",
          workflowRunId: "run-1",
          workflowStepRunId: "step-1",
          processKey: "proc-1",
          toolName: "createFolder",
          operation: "write",
          canonicalArgsJson: "{\"name\":\"blocked\"}",
          argumentsHash: "hash-2",
          targetSummary: "Create Drive folder",
          status: "rejected",
          decisionMode: "manual",
          requestedAt: "2026-06-09T00:10:00Z",
          decidedAt: "2026-06-09T00:10:01Z",
          expiresAt: "2026-06-09T01:00:00Z",
          errorMessage: "not allowed",
        },
        {
          id: "approval-3",
          workflowRunId: "run-1",
          workflowStepRunId: "step-1",
          processKey: "proc-1",
          toolName: "search",
          operation: "read",
          canonicalArgsJson: "{\"query\":\"plan\"}",
          argumentsHash: "hash-3",
          targetSummary: "Search Google Drive for \"plan\"",
          status: "executed",
          decisionMode: "manual",
          requestedAt: "2026-06-09T00:20:00Z",
          decidedAt: "2026-06-09T00:20:01Z",
          expiresAt: "2026-06-09T01:00:00Z",
        },
      ]),
    } as any;

    const artifactIds = await createGoogleDriveWriteAuditArtifacts({
      adminClient,
      localRunnerGateway,
      projectId: "project-1",
      workflowId: "workflow-1",
      workflowRunId: "run-1",
      stepRunId: "step-1",
      stepType: "write_doc",
      mcpAccessMode: "read_write",
      yoloMode: true,
      artifactWorkspaceRoot,
    });

    expect(artifactIds).toHaveLength(2);
    expect(insertedRows).toHaveLength(2);
    expect(insertedRows[0]).toEqual(
      expect.objectContaining({
        artifact_definition_key: null,
        project_id: "project-1",
        workflow_id: "workflow-1",
        workflow_run_id: "run-1",
        workflow_run_step_id: "step-1",
        title: "Google Drive Write Audit - approval-1.json",
        sync_status: "local_only",
      }),
    );
    expect(insertedRows[1]).toEqual(
      expect.objectContaining({
        title: "Google Drive Write Audit - approval-2.json",
      }),
    );
    const executedAuditContent = await readFile(
      path.join(artifactWorkspaceRoot, String(insertedRows[0].local_path)),
      "utf8",
    );
    const rejectedAuditContent = await readFile(
      path.join(artifactWorkspaceRoot, String(insertedRows[1].local_path)),
      "utf8",
    );
    expect(executedAuditContent).toContain("\"mcpAccessMode\": \"read_write\"");
    expect(executedAuditContent).toContain("\"decisionMode\": \"yolo\"");
    expect(executedAuditContent).toContain("\"resultDriveId\": \"drive-1\"");
    expect(rejectedAuditContent).toContain("\"decisionMode\": \"manual\"");
    expect(rejectedAuditContent).toContain("\"errorMessage\": \"not allowed\"");
    expect(executedAuditContent).not.toContain("secret body");
    expect(rejectedAuditContent).not.toContain("secret body");
    expect(localRunnerGateway.listGoogleDriveProxyApprovals).toHaveBeenCalledWith(
      "run-1",
      "step-1",
    );
  });

  describe("sendMessageWithRetry", () => {
    beforeEach(() => {
      vi.spyOn(globalThis, "fetch").mockResolvedValue({
        ok: true,
        json: async () => ({
          account: {
            id: "account-b",
            provider_key: "codex",
            home_path: "/accounts/b",
            extra_env: {},
          },
        }),
      } as Response);
    });

    it("passes required MCPs, read-only mode, and accountHomePath through the session message request", async () => {
      const mockSendMessage = vi.fn().mockResolvedValue({
        outputMarkdown: "success",
        actualPromptText: "Injected MCP prompt",
      });
      const mockEnsureGoogleDriveMcpProviderConfig = vi.fn().mockResolvedValue({
        configChanged: false,
      });
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: mockEnsureGoogleDriveMcpProviderConfig,
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({ data: null, error: null }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "hello",
        skillIds: [],
        requiredMcps: ["google_drive"],
        allowWrite: false,
        yoloMode: false,
        idleTTLSeconds: 60,
      });

      expect(mockEnsureGoogleDriveMcpProviderConfig).toHaveBeenCalledWith({
        providerKey: "codex",
        accountHomePath: "/accounts/b",
        scope: "account",
        mode: "read_only",
        yoloMode: false,
        workflowRunId: "run-123",
        workflowStepRunId: "step-456",
        processKey: "workflow-run-123-step-step-456",
      });
      expect(mockStartSession).toHaveBeenCalledWith(
        expect.objectContaining({
          accountHomePath: "/accounts/b",
          providerAccountHomePath: "/accounts/b",
          customEnv: expect.objectContaining({
            FLOWPILOT_WORKFLOW_RUN_ID: "run-123",
            FLOWPILOT_WORKFLOW_STEP_RUN_ID: "step-456",
            FLOWPILOT_PROCESS_KEY: "workflow-run-123-step-step-456",
          }),
        }),
      );
      expect(mockSendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          prompt: "hello",
          requiredMcps: ["google_drive"],
          allowWrite: false,
          yoloMode: false,
          accountHomePath: "/accounts/b",
        }),
        expect.any(Object),
      );
      expect(result.actualPromptText).toBe("Injected MCP prompt");
    });

    it("asks Codex for a final answer when a required MCP response is empty", async () => {
      const mockSendMessage = vi.fn()
        .mockResolvedValueOnce({
          outputMarkdown: "",
          actualPromptText: "Injected MCP prompt",
        })
        .mockResolvedValueOnce({
          outputMarkdown: "Recent files:\n- File A\n- File B",
          actualPromptText: "Final answer prompt",
        });
      const mockEnsureGoogleDriveMcpProviderConfig = vi.fn().mockResolvedValue({
        configChanged: false,
      });
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: mockEnsureGoogleDriveMcpProviderConfig,
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({ data: null, error: null }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "list 2 recent files",
        skillIds: [],
        requiredMcps: ["google_drive"],
        allowWrite: false,
        yoloMode: false,
        idleTTLSeconds: 60,
      });

      expect(mockSendMessage).toHaveBeenCalledTimes(2);
      expect(mockSendMessage).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({
          prompt: expect.stringContaining("previous response was empty"),
          requiredMcps: ["google_drive"],
        }),
        expect.any(Object),
      );
      expect(result.outputMarkdown).toContain("File A");
    });

    it("fails immediately for deterministic MCP setup errors even when tagged as session_dead", async () => {
      const err = new Error(
        "accountHomePath is required when requiredMcps includes google_drive",
      );
      (err as any).code = "session_dead";

      const mockEnsureGoogleDriveMcpProviderConfig = vi.fn().mockResolvedValue({
        configChanged: false,
      });
      const mockSendMessage = vi.fn().mockRejectedValue(err);
      const mockCloseSession = vi.fn().mockResolvedValue(undefined);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: mockEnsureGoogleDriveMcpProviderConfig,
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "session-old",
            process_key: "proc-old",
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "active",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-old" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      await expect(
        sendMessageWithRetry({
          adminClient,
          localRunnerGateway,
          workflowRunId: "run-123",
          stepRunId: "step-456",
          providerKey: "codex",
          modelName: "codex-mcp",
          reasoningEffort: null,
          workingDirectory: "/repo",
          subagent: null,
          prompt: "hello",
          skillIds: [],
          requiredMcps: ["google_drive"],
          yoloMode: false,
          idleTTLSeconds: 60,
        }),
      ).rejects.toThrow("accountHomePath is required when requiredMcps includes google_drive");

      expect(mockSendMessage).toHaveBeenCalledTimes(1);
      expect(mockCloseSession).toHaveBeenCalledTimes(1);
      expect(mockStartSession).toHaveBeenCalledTimes(1);
      expect(mockQueryBuilder.single).toHaveBeenCalled();
    });

    it("fails without reconnecting when a Google Drive write approval is required", async () => {
      const err = new Error(
        "manual approval is required before retrying this Google Drive write",
      );
      (err as any).code = "mcp_write_approval_required";
      (err as any).details =
        "mcp_write_approval_required: MCP_WRITE_APPROVAL_REQUIRED: FlowPilot created approval request appr-123";

      const mockEnsureGoogleDriveMcpProviderConfig = vi.fn().mockResolvedValue({
        configChanged: false,
      });
      const mockSendMessage = vi.fn().mockRejectedValue(err);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: mockEnsureGoogleDriveMcpProviderConfig,
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({ data: null, error: null }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      await expect(
        sendMessageWithRetry({
          adminClient,
          localRunnerGateway,
          workflowRunId: "run-123",
          stepRunId: "step-456",
          providerKey: "codex",
          modelName: "codex-mcp",
          reasoningEffort: null,
          workingDirectory: "/repo",
          subagent: null,
          prompt: "hello",
          skillIds: [],
          requiredMcps: ["google_drive"],
          yoloMode: false,
          idleTTLSeconds: 60,
        }),
      ).rejects.toThrow("manual approval is required before retrying this Google Drive write");

      expect(mockSendMessage).toHaveBeenCalledTimes(1);
      expect(mockStartSession).toHaveBeenCalledTimes(1);
    });

    it("routes a rejected Google Drive approval into the follow-up path for the waiting step", async () => {
      const approvals = [
        { id: "approval-old", status: "pending", toolName: "search", operation: "read" },
        { id: "approval-live", status: "pending", toolName: "search", operation: "read" },
      ];
      const workflowRunId = "run-1";
      const workflowId = "wf-1";
      const projectId = "proj-1";
      const workflowStepId = "workflow-step-1";
      const decideGoogleDriveProxyApproval = vi.fn().mockResolvedValue(undefined);
      const listMcpBackends = vi.fn().mockResolvedValue([]);
      const workflowRunStep = {
        id: "step-1",
        workflow_run_id: workflowRunId,
        workflow_step_id: workflowStepId,
        execution_order_index: 1,
        step_type: "create_doc",
        retry_count: 0,
        status: "WAITING_USER_APPROVAL",
        error_message: "Google Drive MCP approval required: approval-live",
      };
      const projectRow = {
        id: projectId,
        default_provider: null,
        default_model: null,
        default_reasoning_effort: null,
        session_idle_ttl_minutes: null,
      };
      const workflowRow = {
        id: workflowId,
        name: "Test Workflow",
        project_id: projectId,
        provider_override: null,
        model_override: null,
        reasoning_effort_override: null,
        yolo_mode: true,
      };
      const workflowStepRow = {
        id: workflowStepId,
        step_type: "create_doc",
        order_index: 1,
        is_enabled: true,
        provider_override: null,
        model_override: null,
        reasoning_effort_override: null,
        yolo_mode: false,
        provider_account_override_id: null,
        requires_approval: true,
      };
      const stepDefinitionRow = {
        step_type: "create_doc",
        name: "Create doc",
        description: "Create a document.",
        prompt_base: null,
        required_mcps: ["google_drive"],
        mcp_access_mode: "read_only",
        required_skills: [],
        team_role: null,
        subagent: null,
        model: "gpt-5.4",
        reasoning_effort: "medium",
      };

      const workflowRunStepsBuilder = {
        select: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: workflowRunStep,
          error: null,
        }),
        order: vi.fn().mockResolvedValue({
          data: [workflowRunStep],
          error: null,
        }),
      };
      const workflowRunsBuilder = {
        select: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: workflowRunId,
            workflow_id: workflowId,
            project_id: projectId,
            provider_account_id: null,
            yolo_mode: false,
          },
          error: null,
        }),
      };
      const projectsBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: projectRow,
          error: null,
        }),
      };
      const workflowsBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        or: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: workflowRow,
          error: null,
        }),
      };
      const workflowStepsBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: workflowStepRow,
          error: null,
        }),
      };
      const stepDefinitionsBuilder = {
        select: vi.fn().mockReturnThis(),
        in: vi.fn().mockResolvedValue({
          data: [stepDefinitionRow],
          error: null,
        }),
      };
      const emptyArtifactBindingsBuilder = {
        select: vi.fn().mockReturnThis(),
        in: vi.fn().mockReturnThis(),
        order: vi.fn().mockResolvedValue({
          data: [],
          error: null,
        }),
      };
      const aiSupportedModelsBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: { model_id: "gpt-5.4", is_enabled: true },
          error: null,
        }),
      };
      const projectWorkspaceBindingsBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        order: vi.fn().mockResolvedValue({
          data: [{ local_path: "/repo" }],
          error: null,
        }),
      };
      const workflowRunLogsBuilder = {
        insert: vi.fn().mockResolvedValue({ data: null, error: null }),
      };
      const listGoogleDriveProxyApprovals = vi.fn().mockResolvedValue(approvals);
      const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue({
        ok: true,
        json: async () => ({ usable: true, path: "/repo" }),
      } as any);
      const adminClient = {
        from: vi.fn((table: string) => {
          if (table === "workflow_run_steps") return workflowRunStepsBuilder;
          if (table === "workflow_runs") return workflowRunsBuilder;
          if (table === "projects") return projectsBuilder;
          if (table === "workflows") return workflowsBuilder;
          if (table === "workflow_steps") return workflowStepsBuilder;
          if (table === "step_definitions") return stepDefinitionsBuilder;
          if (table === "step_input_artifact_definitions") return emptyArtifactBindingsBuilder;
          if (table === "step_output_artifact_definitions") return emptyArtifactBindingsBuilder;
          if (table === "ai_supported_models") return aiSupportedModelsBuilder;
          if (table === "project_workspace_bindings") return projectWorkspaceBindingsBuilder;
          if (table === "workflow_run_logs") return workflowRunLogsBuilder;
          throw new Error(`Unexpected table ${table}`);
        }),
      } as any;

      await expect(
        submitGoogleDriveWriteApprovalRuntime({
          adminClient,
          localRunnerGateway: {
            listGoogleDriveProxyApprovals,
            decideGoogleDriveProxyApproval,
            listMcpBackends,
          } as any,
          stepId: "step-1",
          decision: "rejected",
          comment: "Do not create this folder.",
        }),
      ).rejects.toThrow("Missing required MCPs for create_doc: google_drive");

      expect(decideGoogleDriveProxyApproval).toHaveBeenCalledWith("approval-live", {
        decision: "rejected",
        comment: "Do not create this folder.",
      });
      expect(listGoogleDriveProxyApprovals).toHaveBeenCalledWith(
        workflowRunId,
        "step-1",
        "pending",
      );
      expect(workflowStepsBuilder.eq).toHaveBeenCalledWith("id", workflowStepId);
      expect(listMcpBackends).toHaveBeenCalledTimes(1);
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });

    it("fails when a waiting step does not carry the exact Google Drive approval id", async () => {
      const workflowRunStepsBuilder = {
        select: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "step-1",
            workflow_run_id: "run-1",
            status: "WAITING_USER_APPROVAL",
            error_message: "Google Drive MCP approval required.",
          },
          error: null,
        }),
        eq: vi.fn().mockReturnThis(),
      };
      const workflowRunsBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: { id: "run-1", workflow_id: "wf-1", project_id: "proj-1", provider_account_id: null, yolo_mode: false },
          error: null,
        }),
      };
      const adminClient = {
        from: vi.fn((table: string) => {
          if (table === "workflow_run_steps") return workflowRunStepsBuilder;
          if (table === "workflow_runs") return workflowRunsBuilder;
          throw new Error(`Unexpected table ${table}`);
        }),
      } as any;

      await expect(
        submitGoogleDriveWriteApprovalRuntime({
          adminClient,
          localRunnerGateway: {
            listGoogleDriveProxyApprovals: vi.fn().mockResolvedValue([{ id: "approval-other" }]),
            decideGoogleDriveProxyApproval: vi.fn(),
          } as any,
          stepId: "step-1",
          decision: "rejected",
        }),
      ).rejects.toThrow("Google Drive MCP approval id is missing for this waiting step.");
    });

    it("fails immediately for Google Drive auth/config bootstrap errors instead of replaying", async () => {
      const mockEnsureGoogleDriveMcpProviderConfig = vi.fn().mockResolvedValue({
        configChanged: false,
      });
      const mockSendMessage = vi.fn().mockRejectedValue(
        new Error("provider error: Google Drive auth token not found for /accounts/b"),
      );
      const mockCloseSession = vi.fn().mockResolvedValue(undefined);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: mockEnsureGoogleDriveMcpProviderConfig,
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "session-old",
            process_key: "proc-old",
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "active",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-old" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      await expect(
        sendMessageWithRetry({
          adminClient,
          localRunnerGateway,
          workflowRunId: "run-123",
          stepRunId: "step-456",
          providerKey: "codex",
          modelName: "codex-mcp",
          reasoningEffort: null,
          workingDirectory: "/repo",
          subagent: null,
          prompt: "hello",
          skillIds: [],
          requiredMcps: ["google_drive"],
          yoloMode: false,
          idleTTLSeconds: 60,
        }),
      ).rejects.toThrow("provider error: Google Drive auth token not found for /accounts/b");

      expect(mockSendMessage).toHaveBeenCalledTimes(1);
      expect(mockCloseSession).toHaveBeenCalledTimes(1);
      expect(mockStartSession).toHaveBeenCalledTimes(1);
      expect(mockQueryBuilder.single).toHaveBeenCalled();
    });

    it("reconnects with old thread on session_dead", async () => {
      let callCount = 0;
      const mockSendMessage = vi.fn().mockImplementation(() => {
        callCount++;
        if (callCount === 1) {
          const err = new Error("Local runner session message failed: session process exited or is no longer registered");
          (err as any).code = "session_dead";
          throw err;
        }
        return Promise.resolve({ outputMarkdown: "success" });
      });
      const mockCloseSession = vi.fn().mockResolvedValue(undefined);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-old",
        transportType: "codex_mcp"
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({ data: { id: "session-old", process_key: "proc-old", provider_session_id: "thread-old", transport_type: "codex_mcp", status: "completed", provider: "codex", model: "codex-mcp", metadata_json: { providerAccountId: "account-b", providerAccountHomePath: "/accounts/b" } }, error: null }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null })
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder)
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "hello",
        skillIds: [],
        idleTTLSeconds: 60,
      });

      expect(callCount).toBe(2);
      expect(localRunnerGateway.ensureGoogleDriveMcpProviderConfig).toHaveBeenCalledWith({
        providerKey: "codex",
        accountHomePath: "/accounts/b",
        scope: "account",
        mode: "read_only",
        yoloMode: false,
        workflowRunId: "run-123",
        workflowStepRunId: "step-456",
        processKey: undefined,
      });
      expect(mockStartSession).toHaveBeenCalledWith(expect.objectContaining({
        resumeProviderSessionId: "thread-old",
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(1, expect.objectContaining({
        idleTTLSeconds: 60,
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        idleTTLSeconds: 60,
      }), expect.any(Object));
      expect(result.outputMarkdown).toBe("success");
      expect(result.actualPromptText).toBe("hello");
    });

    it("keeps the step-scoped process key when recovering a Google Drive session after session_dead", async () => {
      let callCount = 0;
      const sessionDeadError = new Error(
        "Local runner session message failed: session process exited or is no longer registered",
      );
      (sessionDeadError as any).code = "session_dead";

      const mockSendMessage = vi.fn().mockImplementation(() => {
        callCount++;
        if (callCount === 1) {
          throw sessionDeadError;
        }
        return Promise.resolve({
          status: "success",
          outputMarkdown: "approved replay completed",
          providerSessionId: "thread-old",
          actualPromptText: "hello",
        });
      });
      const mockCloseSession = vi.fn().mockResolvedValue(undefined);
      const mockStartSession = vi
        .fn()
        .mockResolvedValueOnce({
          processKey: "workflow-run-123-step-step-456",
          providerSessionId: "thread-old",
          transportType: "codex_mcp",
        })
        .mockResolvedValueOnce({
          processKey: "workflow-run-123-step-step-456",
          providerSessionId: "thread-old",
          transportType: "codex_mcp",
        });
      const mockEnsureGoogleDriveMcpProviderConfig = vi.fn().mockResolvedValue({
        configChanged: false,
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: mockEnsureGoogleDriveMcpProviderConfig,
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi
          .fn()
          .mockResolvedValueOnce({ data: null, error: null })
          .mockResolvedValueOnce({
            data: {
              id: "session-old",
              process_key: "workflow-run-123-step-step-456",
              provider_session_id: "thread-old",
              transport_type: "codex_mcp",
              status: "completed",
              provider: "codex",
              model: "codex-mcp",
              metadata_json: {
                step_run_id: "step-456",
                sessionScopeKey: "step-456",
                providerAccountId: "account-b",
                providerAccountHomePath: "/accounts/b",
              },
            },
            error: null,
          }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi
          .fn()
          .mockResolvedValueOnce({ data: { id: "session-new" }, error: null })
          .mockResolvedValueOnce({ data: { id: "session-old" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: "worker",
        prompt: "hello",
        skillIds: [],
        requiredMcps: ["google_drive"],
        yoloMode: false,
        idleTTLSeconds: 60,
      });

      expect(callCount).toBe(2);
      expect(mockEnsureGoogleDriveMcpProviderConfig).toHaveBeenCalledTimes(1);
      expect(mockStartSession).toHaveBeenNthCalledWith(
        1,
        expect.objectContaining({
          customEnv: expect.objectContaining({
            FLOWPILOT_PROCESS_KEY: "workflow-run-123-step-step-456",
          }),
        }),
      );
      expect(mockStartSession).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({
          customEnv: expect.objectContaining({
            FLOWPILOT_PROCESS_KEY: "workflow-run-123-step-step-456",
          }),
          resumeProviderSessionId: "thread-old",
        }),
      );
      expect(result.outputMarkdown).toBe("approved replay completed");
    });

    it("does not resume an old provider thread after the active account changes", async () => {
      const mockSendMessage = vi.fn().mockResolvedValue({ outputMarkdown: "success" });
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "session-old",
            process_key: null,
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "completed",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-a",
              providerAccountHomePath: "/accounts/a",
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "hello",
        skillIds: [],
        idleTTLSeconds: 60,
      });

      expect(mockStartSession).toHaveBeenCalledWith(
        expect.not.objectContaining({
          resumeProviderSessionId: expect.any(String),
        }),
      );
      expect(mockStartSession).toHaveBeenCalledWith(
        expect.objectContaining({
          providerAccountId: "account-b",
          providerAccountHomePath: "/accounts/b",
        }),
      );
      expect(result.outputMarkdown).toBe("success");
    });

    it("keeps the workflow running when writing a workflow log fails", async () => {
      const mockSendMessage = vi.fn().mockResolvedValue({
        outputMarkdown: "success",
        actualPromptText: "hello",
      });
      const mockInsert = vi.fn().mockRejectedValue(new TypeError("fetch failed"));
      const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => undefined);

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "session-old",
            process_key: null,
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "completed",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null }),
      };
      const adminClient = {
        from: vi.fn((table: string) => {
          if (table === "workflow_run_logs") {
            return { insert: mockInsert };
          }

          return mockQueryBuilder;
        }),
      } as any;
      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: vi.fn().mockResolvedValue({
          processKey: "proc-new",
          providerSessionId: "thread-new",
          transportType: "codex_mcp",
        }),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "hello",
        skillIds: [],
        idleTTLSeconds: 60,
      });

      expect(result.outputMarkdown).toBe("success");
      expect(mockInsert).toHaveBeenCalled();
      expect(warnSpy).toHaveBeenCalledWith(
        expect.stringContaining("Skipping workflow run log write after request failure:"),
        expect.objectContaining({
          workflowRunStepId: "step-456",
          errorMessage: "fetch failed",
        }),
      );
      warnSpy.mockRestore();
    });

    it("does not reconnect when the session was intentionally terminated", async () => {
      const err = new Error("session was intentionally terminated");
      (err as any).code = "session_terminated";

      const mockSendMessage = vi.fn().mockRejectedValue(err);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-old",
        providerSessionId: "thread-old",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "session-old",
            process_key: "proc-old",
            provider_session_id: "codex_mcp_session_prompt_20260529_072201_3000",
            transport_type: "codex_mcp",
            status: "active",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-old" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      await expect(sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "hello",
        skillIds: [],
        idleTTLSeconds: 60,
      })).rejects.toThrow("session was intentionally terminated");

      expect(mockSendMessage).toHaveBeenCalledTimes(1);
      expect(mockStartSession).not.toHaveBeenCalled();
    });

    it("starts a fresh provider session when forced instead of resuming the old thread", async () => {
      const mockSendMessage = vi.fn().mockResolvedValue({ outputMarkdown: "success" });
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-new",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "session-old",
            process_key: null,
            provider_session_id: "codex_mcp_session_prompt_20260528_072228_2900",
            transport_type: "codex_mcp",
            status: "completed",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {},
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null }),
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "try again",
        skillIds: [],
        idleTTLSeconds: 60,
        forceNewProviderSession: true,
      });

      expect(mockStartSession).toHaveBeenCalledWith(
        expect.not.objectContaining({
          resumeProviderSessionId: expect.any(String),
        }),
      );
      expect(result.outputMarkdown).toBe("success");
      expect(result.actualPromptText).toBe("try again");
    });

    it("throws directly without reconnect on non-session_dead error", async () => {
      const mockSendMessage = vi.fn().mockRejectedValue(new Error("Some other error"));
      const mockCloseSession = vi.fn().mockResolvedValue(undefined);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-old",
        transportType: "codex_mcp"
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession
      } as any;

      const mockQueryBuilder2 = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({ data: { id: "sess-id", process_key: "proc-old", provider_session_id: "thread-old", transport_type: "codex_mcp", status: "active", provider: "codex", model: "codex-mcp", metadata_json: { providerAccountId: "account-b", providerAccountHomePath: "/accounts/b" } }, error: null }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({ data: { id: "session-new" }, error: null })
      };
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder2)
      } as any;

      await expect(sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "hello",
        skillIds: [],
        idleTTLSeconds: 60,
      })).rejects.toThrow("Some other error");

      expect(mockStartSession).not.toHaveBeenCalled();
    });

    it("bootstraps context cross-machine when provider thread is missing", async () => {
      const mockSendMessage = vi.fn()
        .mockRejectedValueOnce(new Error("provider error: thread not found"))
        .mockResolvedValueOnce({
          providerSessionId: "thread-bootstrap",
          outputMarkdown: "ok"
        });

      let sessionCloseCount = 0;
      const mockCloseSession = vi.fn().mockImplementation(() => {
        sessionCloseCount++;
        return Promise.resolve();
      });

      let startSessionCount = 0;
      const mockStartSession = vi.fn().mockImplementation(() => {
        startSessionCount++;
        return Promise.resolve({
          processKey: "proc-new",
          providerSessionId: "thread-bootstrap",
          processPid: 4321,
          transportType: "codex_mcp"
        });
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "sess-id",
            process_key: "proc-old",
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "active",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
              checkpoints: [
                {
                  promptPath: "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md",
                  outputContentPath:
                    "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/BusinessIdea.md",
                  artifactOutputPaths: ["/repo/.flowpilot/artifacts/run-1/step-1/BusinessIdea.md"],
                },
              ]
            }
          },
          error: null
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockImplementation(() => {
          return Promise.resolve({
            data: {
              id: "sess-new",
              metadata_json: {
                checkpoints: [
                  {
                    promptPath: "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md",
                    outputContentPath:
                      "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/BusinessIdea.md",
                    artifactOutputPaths: ["/repo/.flowpilot/artifacts/run-1/step-1/BusinessIdea.md"],
                  },
                ]
              }
            }, error: null
          });
        })
      };
      
      const adminClient = {
        from: vi.fn(() => mockQueryBuilder)
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "new-prompt",
        skillIds: [],
        idleTTLSeconds: 60,
      });

      expect(result.outputMarkdown).toBe("ok");
      expect(mockStartSession).toHaveBeenCalledTimes(1);
      expect(mockStartSession).toHaveBeenCalledWith(expect.not.objectContaining({
        resumeProviderSessionId: expect.any(String)
      }));
      expect(mockQueryBuilder.update).toHaveBeenCalledWith(expect.objectContaining({
        process_pid: 4321,
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("# Previous Conversation Context")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("## Prompt Path 1")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("## Output Content Path 1")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/BusinessIdea.md")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("## Artifact Output Path 1")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/BusinessIdea.md")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("new-prompt")
      }), expect.any(Object));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.not.stringContaining("## Assistant Reply")
      }), expect.any(Object));
      expect(result.actualPromptText).toContain("# Previous Conversation Context");
    });

    it("treats 'session not found for thread_id' output as a bootstrap retry signal", async () => {
      const mockSendMessage = vi.fn()
        .mockResolvedValueOnce({
          outputMarkdown: "Session not found for thread_id: 019e6879-60d2-77c0-a264-2c1bd689550e",
        })
        .mockResolvedValueOnce({
          providerSessionId: "thread-bootstrap",
          outputMarkdown: "ok",
        });

      const mockCloseSession = vi.fn().mockResolvedValue(undefined);
      const mockStartSession = vi.fn().mockResolvedValue({
        processKey: "proc-new",
        providerSessionId: "thread-bootstrap",
        transportType: "codex_mcp",
      });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: mockCloseSession,
        startSession: mockStartSession,
      } as any;

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "sess-id",
            process_key: "proc-old",
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "active",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
              checkpoints: [
                {
                  promptPath: "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md",
                  outputContentPath:
                    "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/BusinessIdea.md",
                  artifactOutputPaths: ["/repo/.flowpilot/artifacts/run-1/step-1/BusinessIdea.md"],
                },
              ],
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({
          data: {
            id: "sess-new",
            metadata_json: {
              checkpoints: [
                {
                  promptPath: "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md",
                  outputContentPath:
                    "/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/BusinessIdea.md",
                  artifactOutputPaths: ["/repo/.flowpilot/artifacts/run-1/step-1/BusinessIdea.md"],
                },
              ],
            },
          },
          error: null,
        }),
      };

      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "new-prompt",
        skillIds: [],
        idleTTLSeconds: 60,
      });

      expect(result.outputMarkdown).toBe("ok");
      expect(mockCloseSession).toHaveBeenCalled();
      expect(mockStartSession).toHaveBeenCalledWith(
        expect.not.objectContaining({
          resumeProviderSessionId: expect.any(String),
        }),
      );
      expect(mockSendMessage).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({
          prompt: expect.stringContaining("# Previous Conversation Context"),
        }),
        expect.any(Object),
      );
      expect(mockSendMessage).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({
          prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md"),
        }),
        expect.any(Object),
      );
      expect(result.actualPromptText).toContain("# Previous Conversation Context");
    });

    it("limits bootstrap replay context to the latest five checkpoints", async () => {
      const mockSendMessage = vi.fn()
        .mockResolvedValueOnce({
          outputMarkdown: "Session not found for thread_id: missing",
        })
        .mockResolvedValueOnce({
          providerSessionId: "thread-bootstrap",
          outputMarkdown: "ok",
        });

      const localRunnerGateway = {
        ensureGoogleDriveMcpProviderConfig: vi.fn().mockResolvedValue({
          configChanged: false,
        }),
        sendMessage: mockSendMessage,
        closeSession: vi.fn().mockResolvedValue(undefined),
        startSession: vi.fn().mockResolvedValue({
          processKey: "proc-new",
          providerSessionId: "thread-bootstrap",
          transportType: "codex_mcp",
        }),
      } as any;

      const checkpoints = Array.from({ length: 6 }, (_, index) => ({
        promptPath: `/repo/snap-${index + 1}/prompt.md`,
        outputContentPath: `/repo/snap-${index + 1}/output.md`,
        artifactOutputPaths: [`/repo/artifact-${index + 1}.md`],
      }));

      const mockQueryBuilder = {
        select: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        filter: vi.fn().mockReturnThis(),
        order: vi.fn().mockReturnThis(),
        limit: vi.fn().mockReturnThis(),
        maybeSingle: vi.fn().mockResolvedValue({
          data: {
            id: "sess-id",
            process_key: "proc-old",
            provider_session_id: "thread-old",
            transport_type: "codex_mcp",
            status: "active",
            provider: "codex",
            model: "codex-mcp",
            metadata_json: {
              providerAccountId: "account-b",
              providerAccountHomePath: "/accounts/b",
              checkpoints,
            },
          },
          error: null,
        }),
        insert: vi.fn().mockReturnThis(),
        update: vi.fn().mockReturnThis(),
        single: vi.fn().mockResolvedValue({
          data: {
            id: "sess-new",
            metadata_json: { checkpoints },
          },
          error: null,
        }),
      };

      const adminClient = {
        from: vi.fn(() => mockQueryBuilder),
      } as any;

      await sendMessageWithRetry({
        adminClient,
        localRunnerGateway,
        workflowRunId: "run-123",
        stepRunId: "step-456",
        providerKey: "codex",
        modelName: "codex-mcp",
        reasoningEffort: null,
        workingDirectory: "/repo",
        subagent: null,
        prompt: "new-prompt",
        skillIds: [],
        idleTTLSeconds: 60,
      });

      const replayPrompt = mockSendMessage.mock.calls[1]?.[0]?.prompt as string;
      expect(replayPrompt).toContain("/repo/snap-2/prompt.md");
      expect(replayPrompt).toContain("/repo/snap-6/prompt.md");
      expect(replayPrompt).not.toContain("/repo/snap-1/prompt.md");
      expect(replayPrompt).toContain("/repo/snap-6/output.md");
      expect(replayPrompt).not.toContain("/repo/snap-1/output.md");
      expect(replayPrompt).toContain("/repo/artifact-6.md");
      expect(replayPrompt).not.toContain("/repo/artifact-1.md");
    });
  });
});
