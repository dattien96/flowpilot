import type { SupabaseClient } from "@supabase/supabase-js";

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

type SupabaseRow = Record<string, unknown>;

function assertNoError(error: { message: string } | null, fallback: string) {
  if (error) {
    throw new Error(error.message || fallback);
  }
}

function mapPromptTemplate(row: SupabaseRow): AiPromptTemplate {
  return {
    id: String(row.id),
    projectId: row.project_id ? String(row.project_id) : null,
    stepType: String(row.step_type),
    name: String(row.name),
    description: String(row.description ?? ""),
    inputSchema:
      row.input_schema && typeof row.input_schema === "object"
        ? (row.input_schema as Record<string, unknown>)
        : {},
    outputSchema:
      row.output_schema && typeof row.output_schema === "object"
        ? (row.output_schema as Record<string, unknown>)
        : {},
    templateContent: String(row.template_content ?? ""),
    providerPreference: row.provider_preference ? String(row.provider_preference) : null,
    modelPreference: row.model_preference ? String(row.model_preference) : null,
    version: Number(row.version ?? 1),
    status: (row.status as AiPromptTemplate["status"]) ?? "active",
    createdBy: row.created_by ? String(row.created_by) : null,
    createdAt: String(row.created_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

function deriveArtifactRunId(payload: Record<string, unknown> | null) {
  if (!payload) return null;
  const direct =
    payload.artifactRunId ??
    payload.artifact_run_id ??
    payload.outputArtifactRunId ??
    payload.output_artifact_run_id;
  return direct ? String(direct) : null;
}

function deriveProviderKeyFromModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (model.startsWith("gemini-")) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  return null;
}

function mapAiRun(row: SupabaseRow): AiRun {
  const inputPayload =
    row.input_payload && typeof row.input_payload === "object"
      ? (row.input_payload as Record<string, unknown>)
      : {};
  const outputPayload =
    row.output_payload && typeof row.output_payload === "object"
      ? (row.output_payload as Record<string, unknown>)
      : null;

  return {
    id: String(row.id),
    projectId: String(row.project_id),
    runType: String(row.run_type),
    inputPayload,
    outputPayload,
    provider: row.provider ? String(row.provider) : deriveProviderKeyFromModel(String(row.model_name)),
    modelName: String(row.model_name),
    reasoningEffort: row.reasoning_effort ? String(row.reasoning_effort) : null,
    triggeredBy: row.triggered_by ? String(row.triggered_by) : null,
    status: (row.status as AiRun["status"]) ?? "running",
    errorMessage: row.error_message ? String(row.error_message) : null,
    tokensInput: row.tokens_input == null ? null : Number(row.tokens_input),
    tokensOutput: row.tokens_output == null ? null : Number(row.tokens_output),
    costUsd: row.cost_usd == null ? null : Number(row.cost_usd),
    promptTemplateId: row.prompt_template_id ? String(row.prompt_template_id) : null,
    workflowRunId: row.workflow_run_id ? String(row.workflow_run_id) : null,
    workflowRunStepId: row.workflow_run_step_id ? String(row.workflow_run_step_id) : null,
    artifactRunId: deriveArtifactRunId(outputPayload),
    createdAt: String(row.created_at ?? ""),
    completedAt: row.completed_at ? String(row.completed_at) : null,
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

export class SupabaseAiOrchestrationGateway implements AiOrchestrationGateway {
  constructor(private readonly supabase: SupabaseClient) {}

  async listPromptTemplates(projectId?: string): Promise<AiPromptTemplate[]> {
    let query = this.supabase
      .from("ai_prompt_templates")
      .select("*")
      .order("updated_at", { ascending: false });

    if (projectId) {
      query = query.or(`project_id.is.null,project_id.eq.${projectId}`);
    }

    const { data, error } = await query;
    assertNoError(error, "Unable to list prompt templates.");
    return (data ?? []).map(mapPromptTemplate);
  }

  async savePromptTemplate(input: SaveAiPromptTemplateInput): Promise<AiPromptTemplate> {
    const payload = {
      id: input.id,
      project_id: input.projectId ?? null,
      step_type: input.stepType,
      name: input.name,
      description: input.description,
      input_schema: input.inputSchema ?? {},
      output_schema: input.outputSchema ?? {},
      template_content: input.templateContent,
      provider_preference: input.providerPreference ?? null,
      model_preference: input.modelPreference ?? null,
      version: input.version ?? 1,
      status: input.status ?? "active",
      updated_at: new Date().toISOString(),
    };

    const { data, error } = await this.supabase
      .from("ai_prompt_templates")
      .upsert(payload)
      .select("*")
      .maybeSingle();

    assertNoError(error, "Unable to save prompt template.");
    if (!data) {
      throw new Error("Unable to save prompt template: no row returned.");
    }

    return mapPromptTemplate(data);
  }

  async listAiRuns(filters?: ListAiRunsFilters): Promise<AiRun[]> {
    let query = this.supabase.from("ai_runs").select("*").order("created_at", { ascending: false });

    if (filters?.projectId) {
      query = query.eq("project_id", filters.projectId);
    }

    if (filters?.provider) {
      query = query.eq("provider", filters.provider);
    }

    if (filters?.status) {
      query = query.eq("status", filters.status);
    }

    if (filters?.modelName) {
      query = query.eq("model_name", filters.modelName);
    }

    const { data, error } = await query;
    assertNoError(error, "Unable to list AI runs.");
    return (data ?? []).map(mapAiRun);
  }

  async getAiRunSummary(filters?: ListAiRunsFilters): Promise<AiRunSummary> {
    return computeSummary(await this.listAiRuns(filters));
  }
}
