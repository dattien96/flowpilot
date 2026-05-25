import type { SupabaseClient } from "@supabase/supabase-js";
import { invokeSupabaseEdgeFunction } from "@/data/datasource/supabase/edge-function-client";

import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type {
  ArtifactDefinition,
  ArtifactRun,
  StepDefinition,
  Workflow,
  WorkflowRunStartRequest,
  WorkflowRun,
  WorkflowRunLog,
  WorkflowRunStep,
  WorkflowStep,
} from "@/domain/model/entity/workflow-engine";
import {
  mapArtifactDefinition,
  mapArtifactRun,
  mapStepDefinition,
  mapWorkflow,
  mapWorkflowRun,
  mapWorkflowRunLog,
  mapWorkflowRunStep,
  mapWorkflowStep,
} from "./workflow-engine-mappers";

const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING_EFFORT = "medium";

function bindingOrder(row: { order_index?: unknown }, fallbackIndex: number) {
  const raw = Number(row.order_index);
  return Number.isFinite(raw) ? raw : fallbackIndex;
}

function resolveProviderKeyFromModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (model.startsWith("gemini-")) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  throw new Error(`Model "${model}" is not supported by the workflow runner.`);
}

function normalizeModel(model: string | null | undefined, fallback = DEFAULT_MODEL) {
  return model?.trim() || fallback;
}

function normalizeReasoningEffort(
  reasoningEffort: string | null | undefined,
  fallback = DEFAULT_REASONING_EFFORT,
) {
  return reasoningEffort?.trim() || fallback;
}

async function invokeWorkflowStartRuntime<TResponse>(
  supabase: SupabaseClient,
  payload: WorkflowRunStartRequest,
) {
  if (typeof window === "undefined") {
    return invokeSupabaseEdgeFunction<TResponse>("workflow-engine-start-run", payload);
  }

  const accessToken = (await supabase.auth.getSession()).data.session?.access_token ?? null;
  let response: Response;
  try {
    response = await fetch("/api/workflow-engine/start-run", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      },
      body: JSON.stringify(payload),
    });
  } catch (error) {
    throw new Error(
      error instanceof Error
        ? `Workflow start runtime is unavailable: ${error.message}`
        : "Workflow start runtime is unavailable.",
    );
  }

  if (response.ok) {
    return (await response.json()) as TResponse;
  }

  const body = await response.json().catch(() => null);
  throw new Error(
    typeof body?.error === "string"
      ? body.error
      : `Workflow start runtime failed (${response.status}).`,
  );
}

export class SupabaseWorkflowEngineGateway implements WorkflowEngineGateway {
  constructor(private readonly supabase: SupabaseClient) {}

  async listArtifactDefinitions(): Promise<ArtifactDefinition[]> {
    const { data, error } = await this.supabase
      .from("artifact_definitions")
      .select("*")
      .order("name", { ascending: true });

    if (error) throw new Error(`Unable to list artifact definitions: ${error.message}`);
    return (data ?? []).map(mapArtifactDefinition);
  }

  async saveArtifactDefinition(definition: ArtifactDefinition): Promise<ArtifactDefinition> {
    const { data, error } = await this.supabase
      .from("artifact_definitions")
      .upsert(
        {
          key: definition.key,
          name: definition.name,
          description: definition.description,
          local_path_template: definition.localPathTemplate,
          remote_path_template: definition.remotePathTemplate,
          default_file_name: definition.defaultFileName,
          updated_at: new Date().toISOString(),
        },
        { onConflict: "key" }
      )
      .select("*")
      .maybeSingle();

    if (error) throw new Error(`Unable to save artifact definition: ${error.message}`);
    if (!data) {
      throw new Error("Unable to save artifact definition: no row was returned.");
    }

    return mapArtifactDefinition(data);
  }

  async listArtifactRuns(projectId?: string): Promise<ArtifactRun[]> {
    let query = this.supabase.from("artifact_runs").select("*");

    if (projectId) {
      query = query.eq("project_id", projectId);
    }

    const { data, error } = await query.order("created_at", { ascending: false });

    if (error) throw new Error(`Unable to list artifact runs: ${error.message}`);
    return (data ?? []).map(mapArtifactRun);
  }

  async listStepDefinitions(): Promise<StepDefinition[]> {
    const [definitionsResult, inputBindingsResult, outputBindingsResult] = await Promise.all([
      this.supabase.from("step_definitions").select("*").order("name", { ascending: true }),
      this.supabase
        .from("step_input_artifact_definitions")
        .select("step_type, artifact_definition_key, order_index")
        .order("order_index", { ascending: true }),
      this.supabase
        .from("step_output_artifact_definitions")
        .select("step_type, artifact_definition_key, order_index")
        .order("order_index", { ascending: true }),
    ]);

    if (definitionsResult.error) {
      throw new Error(`Unable to list step definitions: ${definitionsResult.error.message}`);
    }
    if (inputBindingsResult.error) {
      throw new Error(
        `Unable to list step input artifact bindings: ${inputBindingsResult.error.message}`
      );
    }
    if (outputBindingsResult.error) {
      throw new Error(
        `Unable to list step output artifact bindings: ${outputBindingsResult.error.message}`
      );
    }

    const inputBindings = new Map<string, string[]>();
    for (const row of inputBindingsResult.data ?? []) {
      const stepType = String(row.step_type);
      const current = inputBindings.get(stepType) ?? [];
      current[bindingOrder(row, current.length)] = String(row.artifact_definition_key);
      inputBindings.set(stepType, current.filter(Boolean));
    }

    const outputBindings = new Map<string, string[]>();
    for (const row of outputBindingsResult.data ?? []) {
      const stepType = String(row.step_type);
      const current = outputBindings.get(stepType) ?? [];
      current[bindingOrder(row, current.length)] = String(row.artifact_definition_key);
      outputBindings.set(stepType, current.filter(Boolean));
    }

    return (definitionsResult.data ?? []).map((row) =>
      mapStepDefinition({
        ...row,
        input_artifact_definition_keys:
          inputBindings.get(String(row.step_type)) ?? row.input_artifact_definitions ?? [],
        output_artifact_definition_keys:
          outputBindings.get(String(row.step_type)) ?? row.output_artifact_definitions ?? [],
      })
    );
  }

  async saveStepDefinition(step: StepDefinition): Promise<StepDefinition> {
    const { data, error } = await this.supabase
      .from("step_definitions")
      .upsert(
        {
          step_type: step.stepType,
          name: step.name,
          description: step.description,
          prompt_base: step.promptBase,
          required_mcps: step.requiredMcps,
          required_skills: step.requiredSkills,
          team_role: step.teamRole ?? null,
          subagent: step.subagent ?? null,
          model: step.model,
          reasoning_effort: normalizeReasoningEffort(step.reasoningEffort),
          agent_type: step.agentType,
        },
        { onConflict: "step_type" }
      )
      .select("*")
      .maybeSingle();

    if (error) throw new Error(`Unable to save step definition: ${error.message}`);
    if (!data) {
      throw new Error("Unable to save step definition: no row was returned.");
    }

    const stepType = String(data.step_type);
    const inputArtifactDefinitions = step.inputArtifactDefinitions ?? [];
    const outputArtifactDefinitions = step.outputArtifactDefinitions ?? [];

    const { error: deleteInputError } = await this.supabase
      .from("step_input_artifact_definitions")
      .delete()
      .eq("step_type", stepType);
    if (deleteInputError) {
      throw new Error(`Unable to reset step input artifact bindings: ${deleteInputError.message}`);
    }

    const { error: deleteOutputError } = await this.supabase
      .from("step_output_artifact_definitions")
      .delete()
      .eq("step_type", stepType);
    if (deleteOutputError) {
      throw new Error(
        `Unable to reset step output artifact bindings: ${deleteOutputError.message}`
      );
    }

    if (inputArtifactDefinitions.length > 0) {
      const { error: inputError } = await this.supabase
        .from("step_input_artifact_definitions")
        .insert(
          inputArtifactDefinitions.map((artifactDefinitionKey, orderIndex) => ({
            step_type: stepType,
            artifact_definition_key: artifactDefinitionKey,
            order_index: orderIndex,
          }))
        );

      if (inputError) {
        throw new Error(`Unable to save step input artifact bindings: ${inputError.message}`);
      }
    }

    if (outputArtifactDefinitions.length > 0) {
      const { error: outputError } = await this.supabase
        .from("step_output_artifact_definitions")
        .insert(
          outputArtifactDefinitions.map((artifactDefinitionKey, orderIndex) => ({
            step_type: stepType,
            artifact_definition_key: artifactDefinitionKey,
            order_index: orderIndex,
          }))
        );

      if (outputError) {
        throw new Error(`Unable to save step output artifact bindings: ${outputError.message}`);
      }
    }

    const saved = (await this.listStepDefinitions()).find(
      (definition) => definition.stepType === stepType
    );
    if (!saved) {
      throw new Error("Saved step definition not found.");
    }

    return saved;
  }

  async listWorkflows(projectId?: string): Promise<Workflow[]> {
    let query = this.supabase.from("workflows").select("*").neq("created_by", "flowpilot-runtime");

    if (projectId) {
      query = query.or(`project_id.is.null,project_id.eq.${projectId}`);
    }

    const { data, error } = await query.order("created_at", { ascending: false });

    if (error) throw new Error(`Unable to list workflows: ${error.message}`);
    return (data ?? []).map(mapWorkflow);
  }

  async getWorkflowDetail(workflowId: string): Promise<Workflow | null> {
    const { data: workflowData, error: workflowError } = await this.supabase
      .from("workflows")
      .select("*")
      .eq("id", workflowId)
      .maybeSingle();

    if (workflowError) throw new Error(`Unable to load workflow: ${workflowError.message}`);
    if (!workflowData) return null;

    const workflow = mapWorkflow(workflowData);

    const { data: stepsData, error: stepsError } = await this.supabase
      .from("workflow_steps")
      .select("*")
      .eq("workflow_id", workflowId)
      .order("order_index", { ascending: true });

    if (stepsError) throw new Error(`Unable to load workflow steps: ${stepsError.message}`);

    workflow.steps = (stepsData ?? []).map(mapWorkflowStep);
    return workflow;
  }

  async saveWorkflow(
    workflow: Omit<Partial<Workflow>, "steps"> & { steps: Partial<WorkflowStep>[] }
  ): Promise<Workflow> {
    const isNew = !workflow.id;
    let savedWorkflowRow: any;
    const resolvedWorkflowModel = normalizeModel(workflow.modelOverride);
    const resolvedWorkflowReasoning = normalizeReasoningEffort(workflow.reasoningEffortOverride);
    const resolvedWorkflowProvider = resolveProviderKeyFromModel(resolvedWorkflowModel);

    if (isNew) {
      const { data, error } = await this.supabase
        .from("workflows")
        .insert({
          project_id: workflow.projectId || null,
          name: workflow.name || "Untitled Workflow",
          description: workflow.description || "",
          is_template: workflow.isTemplate ?? false,
          provider_override: resolvedWorkflowProvider,
          model_override: resolvedWorkflowModel,
          reasoning_effort_override: resolvedWorkflowReasoning,
        })
        .select("*")
        .maybeSingle();

      if (error) throw new Error(`Unable to create workflow: ${error.message}`);
      if (!data) {
        throw new Error(
          "Unable to create workflow: no row was returned. Check workflow write policies."
        );
      }
      savedWorkflowRow = data;
    } else {
      const { data, error } = await this.supabase
        .from("workflows")
        .update({
          name: workflow.name,
          description: workflow.description,
          is_template: workflow.isTemplate,
          provider_override: resolvedWorkflowProvider,
          model_override: resolvedWorkflowModel,
          reasoning_effort_override: resolvedWorkflowReasoning,
          updated_at: new Date().toISOString(),
        })
        .eq("id", workflow.id)
        .select("*")
        .maybeSingle();

      if (error) throw new Error(`Unable to update workflow: ${error.message}`);
      if (!data) {
        throw new Error(
          "Unable to update workflow: no row was returned. Check workflow write policies."
        );
      }
      savedWorkflowRow = data;
    }

    const workflowId = savedWorkflowRow.id;

    if (!isNew) {
      const { error: deleteError } = await this.supabase
        .from("workflow_steps")
        .delete()
        .eq("workflow_id", workflowId);

      if (deleteError) throw new Error(`Unable to clear old steps: ${deleteError.message}`);
    }

    if (workflow.steps && workflow.steps.length > 0) {
      const stepRows = workflow.steps.map((step, idx) => ({
        workflow_id: workflowId,
        step_type: step.stepType,
        order_index: step.orderIndex ?? idx,
        is_enabled: step.isEnabled ?? true,
        provider_override: resolveProviderKeyFromModel(
          normalizeModel(step.modelOverride, resolvedWorkflowModel),
        ),
        model_override: normalizeModel(step.modelOverride, resolvedWorkflowModel),
        reasoning_effort_override: normalizeReasoningEffort(
          step.reasoningEffortOverride,
          resolvedWorkflowReasoning,
        ),
        requires_approval: step.requiresApproval ?? true,
      }));

      const { error: stepsError } = await this.supabase.from("workflow_steps").insert(stepRows);

      if (stepsError) throw new Error(`Unable to save workflow steps: ${stepsError.message}`);
    }

    const result = await this.getWorkflowDetail(workflowId);
    if (!result) throw new Error("Saved workflow not found.");
    return result;
  }

  async listWorkflowRuns(projectId?: string): Promise<WorkflowRun[]> {
    let query = this.supabase.from("workflow_runs").select("*");

    if (projectId) {
      query = query.eq("project_id", projectId);
    }

    const { data, error } = await query.order("started_at", { ascending: false });

    if (error) throw new Error(`Unable to list runs: ${error.message}`);
    return (data ?? []).map(mapWorkflowRun);
  }

  async getWorkflowRunDetail(
    runId: string
  ): Promise<{
    run: WorkflowRun;
    steps: WorkflowRunStep[];
    logs: WorkflowRunLog[];
  } | null> {
    const { data: runData, error: runError } = await this.supabase
      .from("workflow_runs")
      .select("*")
      .eq("id", runId)
      .maybeSingle();

    if (runError) throw new Error(`Unable to load run: ${runError.message}`);
    if (!runData) return null;

    const run = mapWorkflowRun(runData);

    const { data: stepsData, error: stepsError } = await this.supabase
      .from("workflow_run_steps")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("execution_order_index", { ascending: true });

    if (stepsError) throw new Error(`Unable to load run steps: ${stepsError.message}`);
    const steps = (stepsData ?? []).map(mapWorkflowRunStep);

    let logs: WorkflowRunLog[] = [];
    if (steps.length > 0) {
      const stepIds = steps.map((s) => s.id);
      const { data: logsData, error: logsError } = await this.supabase
        .from("workflow_run_logs")
        .select("*")
        .in("workflow_run_step_id", stepIds)
        .order("created_at", { ascending: true });

      if (logsError) throw new Error(`Unable to load logs: ${logsError.message}`);
      logs = (logsData ?? []).map(mapWorkflowRunLog);
    }

    return { run, steps, logs };
  }

  async startWorkflowRun(request: WorkflowRunStartRequest): Promise<WorkflowRun> {
    const data = await invokeWorkflowStartRuntime<any>(this.supabase, request);
    return mapWorkflowRun(data);
  }

  async toggleYoloMode(runId: string, yoloMode: boolean): Promise<WorkflowRun> {
    const data = await invokeSupabaseEdgeFunction<any>("workflow-engine-toggle-yolo-mode", {
      runId,
      yoloMode,
    });
    return mapWorkflowRun(data);
  }

  async submitStepApproval(
    stepId: string,
    approve: boolean,
    comment?: string
  ): Promise<WorkflowRunStep> {
    const data = await invokeSupabaseEdgeFunction<any>("workflow-engine-submit-step-approval", {
      stepId,
      approve,
      comment,
    });
    return mapWorkflowRunStep(data);
  }
}
