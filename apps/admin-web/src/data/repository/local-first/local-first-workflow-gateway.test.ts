import { describe, expect, it, vi } from "vitest";

import { LocalFirstWorkflowGateway } from "./local-first-workflow-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";

function createBaseGatewayMock(): WorkflowGateway {
  return {
    listWorkflowDefinitions: vi.fn(async () => []),
    getWorkflowDefinitionById: vi.fn(async () => null),
    listWorkflowRuns: vi.fn(async () => []),
    getWorkflowRunById: vi.fn(async () => null),
    getWorkflowRunDetail: vi.fn(async () => null),
    deleteWorkflowRuns: vi.fn(async () => {}),
    createWorkflowRun: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    updateWorkflowRun: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    updateWorkflowStep: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    createOutput: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    createApproval: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    getApprovalById: vi.fn(async () => null),
    listPendingApprovalDetails: vi.fn(async () => []),
    updateApproval: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    createApprovalDecision: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    listApprovalDecisionsByRun: vi.fn(async () => []),
    listOutputs: vi.fn(async () => []),
    getOutputDetail: vi.fn(async () => null),
    createLog: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    listLogs: vi.fn(async () => []),
    getLogSummary: vi.fn(async () => ({
      totalCalls: 0,
      totalInputTokens: 0,
      totalOutputTokens: 0,
      totalCostEstimate: 0,
      averageLatencyMs: 0,
      failedCalls: 0,
    })),
  };
}

function createLocalRunnerMock(): LocalRunnerGateway {
  return {
    getHealth: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    pickDirectory: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    listProviders: vi.fn(async () => []),
    listSkills: vi.fn(async () => []),
    listFlows: vi.fn(async () => []),
    listArtifacts: vi.fn(async () => []),
    getArtifactById: vi.fn(async () => null),
    getStorageDriver: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    saveStorageDriver: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    validateStorageDriver: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    listMcpBackends: vi.fn(async () => []),
    installMcpBackend: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    triggerMcpBackendAction: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    listMcpTestRuns: vi.fn(async () => []),
    runMcpTest: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    deleteIntegrationConnection: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    exportArtifactSyncBundle: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    saveArtifactCloudSyncResult: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    hydrateArtifactFromRemote: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    syncArtifact: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    deleteArtifactsByWorkflowRunIds: vi.fn(async () => {}),
    createBackup: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    triggerIntegrationConnection: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    executePrompt: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    startSession: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    sendMessage: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    closeSession: vi.fn(async () => {}),
    authenticateProvider: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    readFile: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    shutdownStack: vi.fn(async () => {}),
    restartStack: vi.fn(async () => {}),
  };
}


describe("LocalFirstWorkflowGateway", () => {
  it("hydrates workflow run detail outputs from local artifacts before DB placeholders", async () => {
    const base = createBaseGatewayMock();
    const localRunner = createLocalRunnerMock();
    const gateway = new LocalFirstWorkflowGateway(base, localRunner);

    vi.mocked(base.getWorkflowRunDetail).mockResolvedValue({
      run: {
        id: "run-1",
        workflowDefinitionId: "workflow-1",
        projectId: "project-1",
        status: "completed",
        currentStepKey: null,
        selectedContextSourceIds: [],
        startedBy: "admin",
        startedAt: "2026-05-23T00:00:00Z",
        completedAt: "2026-05-23T00:01:00Z",
        errorSummary: null,
      },
      steps: [
        {
          id: "step-run-1",
          workflowRunId: "run-1",
          stepKey: "business_idea",
          stepName: "Business Idea",
          stepType: "ai_mock",
          status: "completed",
          sequenceIndex: 0,
          outputId: null,
          startedAt: null,
          completedAt: null,
          errorMessage: null,
        },
      ],
      approvals: [],
      outputs: [
        {
          id: "artifact-run-row",
          workflowRunId: "run-1",
          workflowStepId: "step-run-1",
          projectId: "project-1",
          outputType: "document",
          version: 1,
          title: "BusinessIdea.md",
          contentMarkdown: "",
          isApproved: false,
          createdAt: "2026-05-23T00:01:00Z",
        },
      ],
      logs: [],
      approvalDecisions: [],
      selectedContextSources: [],
      project: null,
      definition: null,
    });

    vi.mocked(localRunner.listArtifacts).mockResolvedValue([
      {
        artifactId: "artifact-local-1",
        title: "BusinessIdea.md",
        sourceKind: "workflow_output",
        projectId: "project-1",
        workflowRunId: "run-1",
        workflowStepKey: "business_idea",
        providerKey: "codex",
        localPath: ".flowpilot/artifacts/project-1/run-1/business_idea",
        remotePath: "",
        remoteUrl: "",
        syncStatus: "local_only",
        createdAt: "2026-05-23T00:01:00Z",
        updatedAt: "2026-05-23T00:01:00Z",
        contentMarkdown: "# Business Idea\n\nReal local content",
        previewMarkdown: "Real local content",
      },
    ]);

    const detail = await gateway.getWorkflowRunDetail("run-1");

    expect(detail?.outputs).toHaveLength(1);
    expect(detail?.outputs[0]?.id).toBe("artifact-local-1");
    expect(detail?.outputs[0]?.contentMarkdown).toContain("Real local content");
  });

  it("prefers local artifacts when listing outputs", async () => {
    const base = createBaseGatewayMock();
    const localRunner = createLocalRunnerMock();
    const gateway = new LocalFirstWorkflowGateway(base, localRunner);

    vi.mocked(base.listOutputs).mockResolvedValue([
      {
        id: "db-output-1",
        workflowRunId: "run-1",
        workflowStepId: "step-run-1",
        projectId: "project-1",
        outputType: "document",
        version: 1,
        title: "BusinessIdea.md",
        contentMarkdown: "",
        isApproved: false,
        createdAt: "2026-05-23T00:01:00Z",
      },
    ]);

    vi.mocked(localRunner.listArtifacts).mockResolvedValue([
      {
        artifactId: "artifact-local-1",
        title: "BusinessIdea.md",
        sourceKind: "workflow_output",
        projectId: "project-1",
        workflowRunId: "run-1",
        workflowStepKey: "business_idea",
        providerKey: "codex",
        localPath: ".flowpilot/artifacts/project-1/run-1/business_idea",
        remotePath: "",
        remoteUrl: "",
        syncStatus: "local_only",
        createdAt: "2026-05-23T00:01:00Z",
        updatedAt: "2026-05-23T00:01:00Z",
        contentMarkdown: "# Business Idea\n\nReal local content",
        previewMarkdown: "Real local content",
      },
    ]);

    const outputs = await gateway.listOutputs({ workflowRunId: "run-1" });

    expect(outputs).toHaveLength(1);
    expect(outputs[0]?.id).toBe("artifact-local-1");
    expect(outputs[0]?.contentMarkdown).toContain("Real local content");
  });

  it("closes active sessions before cleaning artifacts and deleting workflow runs", async () => {
    const base = createBaseGatewayMock();
    const localRunner = createLocalRunnerMock();
    const gateway = new LocalFirstWorkflowGateway(base, localRunner);

    vi.mocked(base.getWorkflowRunDetail).mockResolvedValue({
      run: {
        id: "run-1",
        workflowDefinitionId: "workflow-1",
        projectId: "project-1",
        status: "running",
        currentStepKey: "business_idea",
        selectedContextSourceIds: [],
        startedBy: "admin",
        startedAt: "2026-05-23T00:00:00Z",
        completedAt: null,
        errorSummary: null,
      },
      steps: [],
      approvals: [],
      outputs: [],
      logs: [],
      sessions: [
        {
          id: "session-1",
          workflowRunId: "run-1",
          provider: "codex",
          model: "gpt-5.4",
          transportType: "codex_mcp",
          providerSessionId: "thread-1",
          processKey: "proc-1",
          processPid: 1234,
          status: "active",
          startedAt: "2026-05-23T00:00:00Z",
          completedAt: null,
        },
        {
          id: "session-2",
          workflowRunId: "run-1",
          provider: "codex",
          model: "gpt-5.4",
          transportType: "codex_mcp",
          providerSessionId: "thread-2",
          processKey: "proc-2",
          processPid: 5678,
          status: "completed",
          startedAt: "2026-05-23T00:00:00Z",
          completedAt: "2026-05-23T00:01:00Z",
        },
      ],
      approvalDecisions: [],
      selectedContextSources: [],
      project: null,
      definition: null,
    });

    await gateway.deleteWorkflowRuns(["run-1", "run-2"]);

    expect(base.getWorkflowRunDetail).toHaveBeenCalledWith("run-1");
    expect(base.getWorkflowRunDetail).toHaveBeenCalledWith("run-2");
    expect(localRunner.closeSession).toHaveBeenCalledTimes(1);
    expect(localRunner.closeSession).toHaveBeenCalledWith({
      transportType: "codex_mcp",
      providerSessionId: "thread-1",
      processKey: "proc-1",
      processPid: 1234,
    });
    expect(localRunner.deleteArtifactsByWorkflowRunIds).toHaveBeenCalledWith(["run-1", "run-2"]);
    expect(base.deleteWorkflowRuns).toHaveBeenCalledWith(["run-1", "run-2"]);
  });

  it("preserves earlier step outputs when a follow-up creates a new artifact version", async () => {
    const base = createBaseGatewayMock();
    const localRunner = createLocalRunnerMock();
    const gateway = new LocalFirstWorkflowGateway(base, localRunner);

    vi.mocked(base.getWorkflowRunDetail).mockResolvedValue({
      run: {
        id: "run-1",
        workflowDefinitionId: "workflow-1",
        projectId: "project-1",
        status: "completed",
        currentStepKey: null,
        selectedContextSourceIds: [],
        startedBy: "admin",
        startedAt: "2026-05-23T00:00:00Z",
        completedAt: "2026-05-23T00:02:00Z",
        errorSummary: null,
      },
      steps: [
        {
          id: "step-run-1",
          workflowRunId: "run-1",
          stepKey: "business_idea",
          stepName: "Business Idea",
          stepType: "ai_mock",
          status: "completed",
          sequenceIndex: 0,
          outputId: null,
          startedAt: null,
          completedAt: null,
          errorMessage: null,
        },
      ],
      approvals: [],
      outputs: [
        {
          id: "artifact-db-1",
          workflowRunId: "run-1",
          workflowStepId: "step-run-1",
          projectId: "project-1",
          outputType: "document",
          version: 1,
          title: "BusinessIdea.md",
          contentMarkdown: "# Business Idea\n\nInitial output",
          isApproved: false,
          createdAt: "2026-05-23T00:01:00Z",
        },
        {
          id: "artifact-db-2",
          workflowRunId: "run-1",
          workflowStepId: "step-run-1",
          projectId: "project-1",
          outputType: "document",
          version: 2,
          title: "BusinessIdea.md",
          contentMarkdown: "",
          isApproved: false,
          createdAt: "2026-05-23T00:02:00Z",
        },
      ],
      logs: [],
      approvalDecisions: [],
      selectedContextSources: [],
      project: null,
      definition: null,
    });

    vi.mocked(localRunner.listArtifacts).mockResolvedValue([
      {
        artifactId: "artifact-db-2",
        title: "BusinessIdea.md",
        sourceKind: "workflow_output",
        projectId: "project-1",
        workflowRunId: "run-1",
        workflowStepKey: "business_idea",
        providerKey: "codex",
        localPath: ".flowpilot/artifacts/project-1/run-1/business_idea",
        remotePath: "",
        remoteUrl: "",
        syncStatus: "local_only",
        createdAt: "2026-05-23T00:02:00Z",
        updatedAt: "2026-05-23T00:02:00Z",
        contentMarkdown: "# Business Idea\n\nFollow-up output",
        previewMarkdown: "Follow-up output",
      },
    ]);

    const detail = await gateway.getWorkflowRunDetail("run-1");

    expect(detail?.outputs).toHaveLength(2);
    expect(detail?.outputs.map((output) => output.id)).toEqual([
      "artifact-db-1",
      "artifact-db-2",
    ]);
    expect(detail?.outputs[0]?.contentMarkdown).toContain("Initial output");
    expect(detail?.outputs[1]?.contentMarkdown).toContain("Follow-up output");
  });
});
