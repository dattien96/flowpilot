import type {
  AiOrchestrationGateway,
  ListAiRunsFilters,
  SaveAiPromptTemplateInput,
} from "@/domain/gateway/ai-orchestration-gateway";
import type {
  AiPromptTemplate,
  AiRun,
  AiRunSummary,
} from "@/domain/model/entity/ai-orchestration";

const TEMPLATE_TIMESTAMP = "2026-05-21T10:00:00.000Z";

function makeTemplate(input: Partial<AiPromptTemplate> & Pick<AiPromptTemplate, "id" | "stepType" | "name" | "templateContent">): AiPromptTemplate {
  return {
    id: input.id,
    projectId: input.projectId ?? null,
    stepType: input.stepType,
    name: input.name,
    description: input.description ?? "",
    inputSchema: input.inputSchema ?? {},
    outputSchema: input.outputSchema ?? {},
    templateContent: input.templateContent,
    providerPreference: input.providerPreference ?? null,
    modelPreference: input.modelPreference ?? null,
    version: input.version ?? 1,
    status: input.status ?? "active",
    createdBy: input.createdBy ?? "demo-user",
    createdAt: input.createdAt ?? TEMPLATE_TIMESTAMP,
    updatedAt: input.updatedAt ?? TEMPLATE_TIMESTAMP,
  };
}

function computeSummary(runs: AiRun[]): AiRunSummary {
  return {
    totalRuns: runs.length,
    runningRuns: runs.filter((run) => run.status === "running").length,
    successfulRuns: runs.filter((run) => run.status === "success").length,
    failedRuns: runs.filter((run) => run.status === "failed").length,
    totalInputTokens: runs.reduce((sum, run) => sum + (run.tokensInput ?? 0), 0),
    totalOutputTokens: runs.reduce((sum, run) => sum + (run.tokensOutput ?? 0), 0),
    totalCostUsd: runs.reduce((sum, run) => sum + (run.costUsd ?? 0), 0),
  };
}

export class InMemoryAiOrchestrationGateway implements AiOrchestrationGateway {
  private promptTemplates: AiPromptTemplate[] = [
    makeTemplate({
      id: "tmpl-tech-spec-global-v1",
      stepType: "tech_spec",
      name: "Global Tech Spec",
      description: "Default tech spec generation prompt.",
      providerPreference: "claude",
      modelPreference: "claude-sonnet-4",
      templateContent: "# Task\nGenerate a technical specification from the approved source artifact.",
    }),
    makeTemplate({
      id: "tmpl-coding-plan-project-v2",
      projectId: "project_mobile_replatform",
      stepType: "make_plan_coding",
      name: "Mobile Coding Plan",
      description: "Project-specific coding plan tuned for Kotlin and Compose.",
      providerPreference: "codex",
      modelPreference: "codex-5.5",
      version: 2,
      templateContent: "# Task\nBreak the source artifact into a coding plan with test-first sequencing.",
    }),
  ];

  private aiRuns: AiRun[] = [
    {
      id: "ai-run-001",
      projectId: "project_mobile_replatform",
      runType: "tech_spec_generation",
      inputPayload: { projectId: "project_mobile_replatform", sourceArtifactRunId: "artifact-run-001" },
      outputPayload: { artifactRunId: "artifact-run-002", workflowRunId: "wf-run-001" },
      modelName: "claude-sonnet-4",
      reasoningEffort: "high",
      triggeredBy: "demo-user",
      status: "success",
      errorMessage: null,
      tokensInput: 1640,
      tokensOutput: 932,
      costUsd: 0.84,
      promptTemplateId: "tmpl-tech-spec-global-v1",
      workflowRunId: "wf-run-001",
      workflowRunStepId: "wf-step-001",
      artifactRunId: "artifact-run-002",
      createdAt: "2026-05-21T09:30:00.000Z",
      completedAt: "2026-05-21T09:31:30.000Z",
    },
    {
      id: "ai-run-002",
      projectId: "project_mobile_replatform",
      runType: "coding_plan_generation",
      inputPayload: { projectId: "project_mobile_replatform", sourceArtifactRunId: "artifact-run-002" },
      outputPayload: null,
      modelName: "codex-5.5",
      reasoningEffort: "medium",
      triggeredBy: "demo-user",
      status: "running",
      errorMessage: null,
      tokensInput: 2104,
      tokensOutput: null,
      costUsd: 0.61,
      promptTemplateId: "tmpl-coding-plan-project-v2",
      workflowRunId: "wf-run-002",
      workflowRunStepId: "wf-step-002",
      artifactRunId: null,
      createdAt: "2026-05-21T11:00:00.000Z",
      completedAt: null,
    },
    {
      id: "ai-run-003",
      projectId: "project_data_platform",
      runType: "business_review",
      inputPayload: { projectId: "project_data_platform", sourceArtifactRunId: "artifact-run-010" },
      outputPayload: null,
      modelName: "gemini-3.1-pro-high",
      reasoningEffort: null,
      triggeredBy: "demo-user",
      status: "failed",
      errorMessage: "Provider timeout while waiting for structured review output.",
      tokensInput: 980,
      tokensOutput: 0,
      costUsd: 0.19,
      promptTemplateId: null,
      workflowRunId: "wf-run-009",
      workflowRunStepId: "wf-step-009",
      artifactRunId: null,
      createdAt: "2026-05-20T15:15:00.000Z",
      completedAt: "2026-05-20T15:17:00.000Z",
    },
  ];

  async listPromptTemplates(projectId?: string): Promise<AiPromptTemplate[]> {
    return this.promptTemplates
      .filter(
        (template) =>
          !projectId || template.projectId === null || template.projectId === projectId
      )
      .slice()
      .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
  }

  async savePromptTemplate(input: SaveAiPromptTemplateInput): Promise<AiPromptTemplate> {
    const now = new Date().toISOString();
    const next: AiPromptTemplate = {
      id: input.id ?? `tmpl-${Math.random().toString(36).slice(2, 10)}`,
      projectId: input.projectId ?? null,
      stepType: input.stepType,
      name: input.name,
      description: input.description,
      inputSchema: input.inputSchema ?? {},
      outputSchema: input.outputSchema ?? {},
      templateContent: input.templateContent,
      providerPreference: input.providerPreference ?? null,
      modelPreference: input.modelPreference ?? null,
      version: input.version ?? 1,
      status: input.status ?? "active",
      createdBy: "demo-user",
      createdAt: now,
      updatedAt: now,
    };

    const existingIndex = this.promptTemplates.findIndex((template) => template.id === next.id);
    if (existingIndex >= 0) {
      this.promptTemplates[existingIndex] = { ...this.promptTemplates[existingIndex], ...next };
      return this.promptTemplates[existingIndex];
    }

    this.promptTemplates.unshift(next);
    return next;
  }

  async listAiRuns(filters?: ListAiRunsFilters): Promise<AiRun[]> {
    return this.aiRuns
      .filter((run) => !filters?.projectId || run.projectId === filters.projectId)
      .filter((run) => !filters?.status || run.status === filters.status)
      .filter((run) => !filters?.modelName || run.modelName === filters.modelName)
      .slice()
      .sort((left, right) => right.createdAt.localeCompare(left.createdAt));
  }

  async getAiRunSummary(filters?: ListAiRunsFilters): Promise<AiRunSummary> {
    return computeSummary(await this.listAiRuns(filters));
  }
}
