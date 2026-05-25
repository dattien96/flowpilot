import type {
  AiPromptTemplate,
  AiRun,
  AiRunSummary,
} from "@/domain/model/entity/ai-orchestration";

export interface ListAiRunsFilters {
  provider?: string;
  modelName?: string;
  projectId?: string;
  status?: AiRun["status"];
}

export interface SaveAiPromptTemplateInput {
  id?: string;
  projectId?: string | null;
  stepType: string;
  name: string;
  description: string;
  inputSchema?: Record<string, unknown>;
  outputSchema?: Record<string, unknown>;
  templateContent: string;
  providerPreference?: string | null;
  modelPreference?: string | null;
  version?: number;
  status?: AiPromptTemplate["status"];
}

export interface AiOrchestrationGateway {
  listPromptTemplates(projectId?: string): Promise<AiPromptTemplate[]>;
  savePromptTemplate(template: SaveAiPromptTemplateInput): Promise<AiPromptTemplate>;
  listAiRuns(filters?: ListAiRunsFilters): Promise<AiRun[]>;
  getAiRunSummary(filters?: ListAiRunsFilters): Promise<AiRunSummary>;
}
