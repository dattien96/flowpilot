export type AiPromptTemplateStatus = "active" | "archived";

export interface AiPromptTemplate {
  id: string;
  projectId: string | null;
  stepType: string;
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  outputSchema: Record<string, unknown>;
  templateContent: string;
  providerPreference: string | null;
  modelPreference: string | null;
  version: number;
  status: AiPromptTemplateStatus;
  createdBy: string | null;
  createdAt: string;
  updatedAt: string;
}

export type AiRunStatus = "running" | "success" | "failed";

export interface AiRun {
  id: string;
  projectId: string;
  runType: string;
  inputPayload: Record<string, unknown>;
  outputPayload: Record<string, unknown> | null;
  provider?: string | null;
  modelName: string;
  reasoningEffort?: string | null;
  triggeredBy: string | null;
  status: AiRunStatus;
  errorMessage: string | null;
  tokensInput: number | null;
  tokensOutput: number | null;
  costUsd: number | null;
  promptTemplateId: string | null;
  workflowRunId: string | null;
  workflowRunStepId: string | null;
  artifactRunId: string | null;
  createdAt: string;
  completedAt: string | null;
}

export interface AiRunSummary {
  totalRuns: number;
  runningRuns: number;
  successfulRuns: number;
  failedRuns: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
}
