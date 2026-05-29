import { mkdtemp, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  buildWorkflowStepFollowUpPrompt,
  buildWorkflowStepPrompt,
  createArtifactOutputs,
  createLocalWorkflowOutputArtifactSnapshot,
  deactivateWorkflowRunSession,
  finalizeWorkflowRunSessions,
  resolveProviderKeyFromModel,
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
    expect(resolveProviderKeyFromModel("gemini-2.5-flash")).toBe("gemini");
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
    const snapshot = await createLocalWorkflowOutputArtifactSnapshot({
      outputMarkdown: "# Result\n\nHello from the step.",
      promptText: "## Begin Prompt\nsay hello",
      actualPromptText: "# Previous Conversation Context\n\n## User Prompt 1\nhello",
      projectId: "project-1",
      stepType: "test_codex_step",
      stderrText: "",
      stdoutText: "Hello from the step.",
      workflowRunId: "run-123",
      workingDirectory,
      commandText: "codex exec < /tmp/prompt.txt",
      providerKey: "codex",
    });

    const manifest = JSON.parse(await readFile(snapshot.manifestPath, "utf8"));
    const content = await readFile(snapshot.contentPath, "utf8");
    const actualPrompt = await readFile(manifest.actualPromptPath, "utf8");

    expect(snapshot.snapshotDirectory).toContain(
      path.join(".flowpilot", "artifacts", "project-1", "run-123", "test_codex_step"),
    );
    expect(manifest.workflowRunId).toBe("run-123");
    expect(manifest.sourceKind).toBe("workflow_output");
    expect(manifest.title).toBe("Response.md");
    expect(content).toContain("Hello from the step.");
    expect(actualPrompt).toContain("# Previous Conversation Context");
  });

  it("stores workflow step artifact local_path inside the snapshot folder", async () => {
    const workingDirectory = await mkdtemp(path.join(os.tmpdir(), "flowpilot-artifact-output-"));
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
    });

    expect(insertedRows).toHaveLength(1);
    expect(String(insertedRows[0]?.local_path)).toMatch(
      /\.flowpilot[\\/]artifacts[\\/]project-1[\\/]run-1[\\/]business_idea[\\/]\.snapshots[\\/][^\\/]+[\\/]BusinessIdea\.md$/,
    );
    const snapshotFilePath = path.join(workingDirectory, String(insertedRows[0]?.local_path));
    const actualPromptPath = path.join(path.dirname(snapshotFilePath), "actual-prompt.md");
    await expect(readFile(actualPromptPath, "utf8")).resolves.toBe("Bootstrap Prompt");
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

  describe("sendMessageWithRetry", () => {
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
        maybeSingle: vi.fn().mockResolvedValue({ data: { id: "session-old", process_key: "proc-old", provider_session_id: "thread-old", transport_type: "codex_mcp", status: "completed", provider: "codex", model: "codex-mcp" }, error: null }),
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
      expect(mockStartSession).toHaveBeenCalledWith(expect.objectContaining({
        resumeProviderSessionId: "thread-old",
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(1, expect.objectContaining({
        idleTTLSeconds: 60,
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        idleTTLSeconds: 60,
      }));
      expect(result.outputMarkdown).toBe("success");
      expect(result.actualPromptText).toBe("hello");
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
            metadata_json: {},
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
        maybeSingle: vi.fn().mockResolvedValue({ data: { id: "sess-id", process_key: "proc-old", provider_session_id: "thread-old", transport_type: "codex_mcp", status: "active", provider: "codex", model: "codex-mcp" }, error: null }),
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
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("## Prompt Path 1")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("## Output Content Path 1")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/BusinessIdea.md")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("## Artifact Output Path 1")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/BusinessIdea.md")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.stringContaining("new-prompt")
      }));
      expect(mockSendMessage).toHaveBeenNthCalledWith(2, expect.objectContaining({
        prompt: expect.not.stringContaining("## Assistant Reply")
      }));
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
      );
      expect(mockSendMessage).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({
          prompt: expect.stringContaining("/repo/.flowpilot/artifacts/run-1/step-1/.snapshots/snap-1/prompt.md"),
        }),
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
            metadata_json: { checkpoints },
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
