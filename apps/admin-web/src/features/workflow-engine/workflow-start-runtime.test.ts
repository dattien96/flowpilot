import { describe, expect, it, vi } from "vitest";

import {
  buildWorkflowStepFollowUpPrompt,
  buildWorkflowStepPrompt,
  deactivateWorkflowRunSession,
  finalizeWorkflowRunSessions,
  resolveProviderKeyFromModel,
  syncWorkflowRunSessionProviderSessionId,
} from "./workflow-start-runtime";

describe("workflow-start-runtime", () => {
  it("routes supported models to the correct local provider", () => {
    expect(resolveProviderKeyFromModel("gpt-5.5")).toBe("codex");
    expect(resolveProviderKeyFromModel("gemini-pro")).toBe("gemini");
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
});
