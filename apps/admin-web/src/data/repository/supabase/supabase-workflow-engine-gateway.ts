import type { SupabaseClient } from "@supabase/supabase-js";
import { invokeSupabaseEdgeFunction } from "@/data/datasource/supabase/edge-function-client";

import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type {
  StepDefinition,
  Workflow,
  WorkflowRun,
  WorkflowRunLog,
  WorkflowRunStep,
  WorkflowStep,
} from "@/domain/model/entity/workflow-engine";
import {
  mapStepDefinition,
  mapWorkflow,
  mapWorkflowStep,
  mapWorkflowRun,
  mapWorkflowRunStep,
  mapWorkflowRunLog,
} from "./workflow-engine-mappers";

export class SupabaseWorkflowEngineGateway implements WorkflowEngineGateway {
  constructor(private readonly supabase: SupabaseClient) {}

  async listStepDefinitions(): Promise<StepDefinition[]> {
    const { data, error } = await this.supabase
      .from("step_definitions")
      .select("*")
      .order("name", { ascending: true });

    if (error) throw new Error(`Unable to list step definitions: ${error.message}`);
    return (data ?? []).map(mapStepDefinition);
  }

  async saveStepDefinition(step: StepDefinition): Promise<StepDefinition> {
    const { data, error } = await this.supabase
      .from("step_definitions")
      .upsert(
        {
          step_type: step.stepType,
          name: step.name,
          description: step.description,
          required_mcps: step.requiredMcps,
          required_skills: step.requiredSkills,
          agent_type: step.agentType,
        },
        { onConflict: "step_type" }
      )
      .select("*")
      .single();

    if (error) throw new Error(`Unable to save step definition: ${error.message}`);
    return mapStepDefinition(data);
  }

  async listWorkflows(projectId?: string): Promise<Workflow[]> {
    let query = this.supabase.from("workflows").select("*");

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

    if (isNew) {
      const { data, error } = await this.supabase
        .from("workflows")
        .insert({
          project_id: workflow.projectId || null,
          name: workflow.name || "Untitled Workflow",
          description: workflow.description || "",
          is_template: workflow.isTemplate ?? false,
          provider_override: workflow.providerOverride || null,
          model_override: workflow.modelOverride || null,
        })
        .select("*")
        .single();

      if (error) throw new Error(`Unable to create workflow: ${error.message}`);
      savedWorkflowRow = data;
    } else {
      const { data, error } = await this.supabase
        .from("workflows")
        .update({
          name: workflow.name,
          description: workflow.description,
          is_template: workflow.isTemplate,
          provider_override: workflow.providerOverride,
          model_override: workflow.modelOverride,
          updated_at: new Date().toISOString(),
        })
        .eq("id", workflow.id)
        .select("*")
        .single();

      if (error) throw new Error(`Unable to update workflow: ${error.message}`);
      savedWorkflowRow = data;
    }

    const workflowId = savedWorkflowRow.id;

    // Delete existing steps if updating
    if (!isNew) {
      const { error: deleteError } = await this.supabase
        .from("workflow_steps")
        .delete()
        .eq("workflow_id", workflowId);

      if (deleteError) throw new Error(`Unable to clear old steps: ${deleteError.message}`);
    }

    // Insert new steps
    if (workflow.steps && workflow.steps.length > 0) {
      const stepRows = workflow.steps.map((step, idx) => ({
        workflow_id: workflowId,
        step_type: step.stepType,
        order_index: step.orderIndex ?? idx,
        is_enabled: step.isEnabled ?? true,
        provider_override: step.providerOverride || null,
        model_override: step.modelOverride || null,
        requires_approval: step.requiresApproval ?? true,
      }));

      const { error: stepsError } = await this.supabase
        .from("workflow_steps")
        .insert(stepRows);

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

  async startWorkflowRun(workflowId: string, projectId: string): Promise<WorkflowRun> {
    const data = await invokeSupabaseEdgeFunction<any>("workflow-engine-start-run", {
      workflowId,
      projectId,
    });
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
