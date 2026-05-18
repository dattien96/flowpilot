import type { SupabaseClient } from "@supabase/supabase-js";

import { MockWorkflowExecutor } from "@/data/workflow/mock-workflow-executor";
import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type {
  WorkflowExecutorGateway,
  WorkflowGateway,
} from "@/domain/gateway/workflow-gateway";
import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Feature } from "@/domain/model/entity/feature";
import type { Project } from "@/domain/model/entity/project";
import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowDefinitionStep,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type {
  CreateContextSourcePayload,
  UpdateContextSourcePayload,
} from "@/domain/model/payload/context-source-payload";
import type { CreateFeaturePayload } from "@/domain/model/payload/feature-payload";
import type { CreateProjectPayload } from "@/domain/model/payload/project-payload";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";

type SupabaseRow = Record<string, unknown>;

function createId(prefix: string) {
  return `${prefix}_${crypto.randomUUID().replaceAll("-", "").slice(0, 18)}`;
}

function assertRow<T>(row: T | null, message: string): T {
  if (!row) {
    throw new Error(message);
  }

  return row;
}

function assertNoError(error: { message: string } | null, fallback: string) {
  if (error) {
    throw new Error(error.message || fallback);
  }
}

function mapProject(row: SupabaseRow): Project {
  return {
    id: String(row.id),
    name: String(row.name),
    description: String(row.description),
    platform: row.platform as Project["platform"],
    repositoryUrl: String(row.repository_url),
    createdBy: String(row.created_by),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapFeature(row: SupabaseRow): Feature {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    title: String(row.title),
    businessGoal: String(row.business_goal),
    userProblem: String(row.user_problem),
    expectedFlow: String(row.expected_flow),
    acceptanceCriteria: String(row.acceptance_criteria),
    priority: row.priority as Feature["priority"],
    status: row.status as Feature["status"],
    ownerId: String(row.owner_id),
    createdAt: String(row.created_at),
    updatedAt: String(row.updated_at),
  };
}

function mapContextSource(row: SupabaseRow): ContextSource {
  return {
    id: String(row.id),
    projectId: String(row.project_id),
    featureId: row.feature_id ? String(row.feature_id) : null,
    type: row.type as ContextSource["type"],
    title: String(row.title),
    rawContent: String(row.raw_content),
    summarizedContent: row.summarized_content ? String(row.summarized_content) : null,
    archivedAt: row.archived_at ? String(row.archived_at) : null,
    createdBy: String(row.created_by),
    createdAt: String(row.created_at),
  };
}

function normalizeDefinitionSteps(definition: unknown): WorkflowDefinitionStep[] {
  if (!definition || typeof definition !== "object") {
    return [];
  }

  const steps = Array.isArray((definition as { steps?: unknown }).steps)
    ? (definition as { steps: unknown[] }).steps
    : [];

  return steps.map((step) => {
    const value = step as Record<string, unknown>;
    return {
      key: String(value.key),
      name: String(value.name),
      type: value.type as WorkflowDefinitionStep["type"],
      outputType: value.outputType as WorkflowDefinitionStep["outputType"],
    };
  });
}

function mapWorkflowDefinition(row: SupabaseRow): WorkflowDefinition {
  return {
    id: String(row.id),
    name: String(row.name),
    description: String(row.description),
    version: Number(row.version),
    status: row.status as WorkflowDefinition["status"],
    steps: normalizeDefinitionSteps(row.definition),
  };
}

function mapWorkflowRun(row: SupabaseRow): WorkflowRun {
  const selected = Array.isArray(row.selected_context_source_ids)
    ? row.selected_context_source_ids.map(String)
    : [];

  return {
    id: String(row.id),
    workflowDefinitionId: String(row.workflow_definition_id),
    projectId: String(row.project_id),
    featureId: String(row.feature_id),
    status: row.status as WorkflowRun["status"],
    currentStepKey: row.current_step_key ? String(row.current_step_key) : null,
    selectedContextSourceIds: selected,
    startedBy: String(row.started_by),
    startedAt: String(row.started_at),
    completedAt: row.completed_at ? String(row.completed_at) : null,
    errorSummary: row.error_summary ? String(row.error_summary) : null,
  };
}

function mapWorkflowStep(row: SupabaseRow): WorkflowStep {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    stepKey: String(row.step_key),
    stepName: String(row.step_name),
    stepType: row.step_type as WorkflowStep["stepType"],
    status: row.status as WorkflowStep["status"],
    sequenceIndex: Number(row.sequence_index),
    outputId: row.output_id ? String(row.output_id) : null,
    startedAt: row.started_at ? String(row.started_at) : null,
    completedAt: row.completed_at ? String(row.completed_at) : null,
    errorMessage: row.error_message ? String(row.error_message) : null,
  };
}

function mapOutput(row: SupabaseRow): AiOutput {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: String(row.workflow_step_id),
    projectId: String(row.project_id),
    featureId: String(row.feature_id),
    outputType: row.output_type as AiOutput["outputType"],
    version: Number(row.version),
    title: String(row.title),
    contentMarkdown: String(row.content_markdown),
    isApproved: Boolean(row.is_approved),
    createdAt: String(row.created_at),
  };
}

function mapApproval(row: SupabaseRow): Approval {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: String(row.workflow_step_id),
    aiOutputId: row.ai_output_id ? String(row.ai_output_id) : null,
    status: row.status as Approval["status"],
    reviewerId: row.reviewer_id ? String(row.reviewer_id) : null,
    comment: row.comment ? String(row.comment) : null,
    decidedAt: row.decided_at ? String(row.decided_at) : null,
    createdAt: String(row.created_at),
  };
}

function mapApprovalDecision(row: SupabaseRow): ApprovalDecision {
  return {
    id: String(row.id),
    approvalId: String(row.approval_id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: String(row.workflow_step_id),
    aiOutputId: row.ai_output_id ? String(row.ai_output_id) : null,
    decision: row.decision as ApprovalDecision["decision"],
    reviewerId: row.reviewer_id ? String(row.reviewer_id) : null,
    comment: row.comment ? String(row.comment) : null,
    createdAt: String(row.created_at),
  };
}

function approvalDecisionRow(decision: ApprovalDecision) {
  return {
    id: decision.id,
    approval_id: decision.approvalId,
    workflow_run_id: decision.workflowRunId,
    workflow_step_id: decision.workflowStepId,
    ai_output_id: decision.aiOutputId,
    decision: decision.decision,
    reviewer_id: decision.reviewerId,
    comment: decision.comment,
    created_at: decision.createdAt,
  };
}

function mapLog(row: SupabaseRow): AiCallLog {
  return {
    id: String(row.id),
    workflowRunId: String(row.workflow_run_id),
    workflowStepId: String(row.workflow_step_id),
    provider: String(row.provider),
    model: String(row.model),
    inputTokens: Number(row.input_tokens),
    outputTokens: Number(row.output_tokens),
    costEstimate: Number(row.cost_estimate),
    latencyMs: Number(row.latency_ms),
    status: row.status as AiCallLog["status"],
    createdAt: String(row.created_at),
  };
}

function workflowRunPatch(patch: Partial<WorkflowRun>) {
  return {
    status: patch.status,
    current_step_key: patch.currentStepKey,
    completed_at: patch.completedAt,
    error_summary: patch.errorSummary,
  };
}

function workflowStepPatch(patch: Partial<WorkflowStep>) {
  return {
    status: patch.status,
    output_id: patch.outputId,
    started_at: patch.startedAt,
    completed_at: patch.completedAt,
    error_message: patch.errorMessage,
  };
}

function approvalPatch(patch: Partial<Approval>) {
  return {
    status: patch.status,
    reviewer_id: patch.reviewerId,
    comment: patch.comment,
    decided_at: patch.decidedAt,
  };
}

class SupabaseGatewayBundle
  implements ProjectGateway, FeatureGateway, ContextSourceGateway, WorkflowGateway
{
  constructor(private readonly supabase: SupabaseClient) {}

  async listProjects() {
    const { data, error } = await this.supabase
      .from("projects")
      .select("*")
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list projects.");
    return (data ?? []).map(mapProject);
  }

  async getProjectById(projectId: string) {
    const { data, error } = await this.supabase
      .from("projects")
      .select("*")
      .eq("id", projectId)
      .maybeSingle();
    assertNoError(error, "Unable to load project.");
    return data ? mapProject(data) : null;
  }

  async createProject(payload: CreateProjectPayload) {
    const { data, error } = await this.supabase
      .from("projects")
      .insert({
        id: createId("project"),
        name: payload.name,
        description: payload.description,
        platform: payload.platform,
        repository_url: payload.repositoryUrl,
        created_by: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create project.");
    return mapProject(assertRow(data, "Project insert returned no row."));
  }

  async listFeatures() {
    const { data, error } = await this.supabase
      .from("features")
      .select("*")
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list features.");
    return (data ?? []).map(mapFeature);
  }

  async listFeaturesByProject(projectId: string) {
    const { data, error } = await this.supabase
      .from("features")
      .select("*")
      .eq("project_id", projectId)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list project features.");
    return (data ?? []).map(mapFeature);
  }

  async getFeatureById(featureId: string) {
    const { data, error } = await this.supabase
      .from("features")
      .select("*")
      .eq("id", featureId)
      .maybeSingle();
    assertNoError(error, "Unable to load feature.");
    return data ? mapFeature(data) : null;
  }

  async createFeature(payload: CreateFeaturePayload) {
    const { data, error } = await this.supabase
      .from("features")
      .insert({
        id: createId("feature"),
        project_id: payload.projectId,
        title: payload.title,
        business_goal: payload.businessGoal,
        user_problem: payload.userProblem,
        expected_flow: payload.expectedFlow,
        acceptance_criteria: payload.acceptanceCriteria,
        priority: payload.priority,
        status: "draft",
        owner_id: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create feature.");
    return mapFeature(assertRow(data, "Feature insert returned no row."));
  }

  async listContextSources() {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .is("archived_at", null)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list context sources.");
    return (data ?? []).map(mapContextSource);
  }

  async listContextSourcesByFeature(featureId: string) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .eq("feature_id", featureId)
      .is("archived_at", null)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list feature context sources.");
    return (data ?? []).map(mapContextSource);
  }

  async listContextSourcesByProject(projectId: string) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .eq("project_id", projectId)
      .is("archived_at", null)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list project context sources.");
    return (data ?? []).map(mapContextSource);
  }

  async getContextSourceById(contextSourceId: string) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .eq("id", contextSourceId)
      .is("archived_at", null)
      .maybeSingle();
    assertNoError(error, "Unable to load context source.");
    return data ? mapContextSource(data) : null;
  }

  async createContextSource(payload: CreateContextSourcePayload) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .insert({
        id: createId("context"),
        project_id: payload.projectId,
        feature_id: payload.featureId,
        type: payload.type,
        title: payload.title,
        raw_content: payload.rawContent,
        summarized_content: null,
        created_by: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create context source.");
    return mapContextSource(assertRow(data, "Context source insert returned no row."));
  }

  async updateContextSource(payload: UpdateContextSourcePayload) {
    const { data, error } = await this.supabase
      .from("context_sources")
      .update({
        title: payload.title,
        type: payload.type,
        raw_content: payload.rawContent,
        summarized_content: payload.summarizedContent ?? null,
      })
      .eq("id", payload.contextSourceId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update context source.");
    return mapContextSource(assertRow(data, "Context source update returned no row."));
  }

  async deleteContextSource(contextSourceId: string) {
    const { error } = await this.supabase
      .from("context_sources")
      .update({ archived_at: new Date().toISOString() })
      .eq("id", contextSourceId);
    assertNoError(error, "Unable to archive context source.");
  }

  async listWorkflowDefinitions() {
    const { data, error } = await this.supabase
      .from("workflow_definitions")
      .select("*")
      .order("version", { ascending: false });
    assertNoError(error, "Unable to list workflow definitions.");
    return (data ?? []).map(mapWorkflowDefinition);
  }

  async getWorkflowDefinitionById(workflowDefinitionId: string) {
    const { data, error } = await this.supabase
      .from("workflow_definitions")
      .select("*")
      .eq("id", workflowDefinitionId)
      .maybeSingle();
    assertNoError(error, "Unable to load workflow definition.");
    return data ? mapWorkflowDefinition(data) : null;
  }

  async listWorkflowRuns() {
    const { data, error } = await this.supabase
      .from("workflow_runs")
      .select("*")
      .order("started_at", { ascending: false });
    assertNoError(error, "Unable to list workflow runs.");
    return (data ?? []).map(mapWorkflowRun);
  }

  async getWorkflowRunById(runId: string) {
    const { data, error } = await this.supabase
      .from("workflow_runs")
      .select("*")
      .eq("id", runId)
      .maybeSingle();
    assertNoError(error, "Unable to load workflow run.");
    return data ? mapWorkflowRun(data) : null;
  }

  async getWorkflowRunDetail(runId: string) {
    const run = await this.getWorkflowRunById(runId);

    if (!run) {
      return null;
    }

    const [
      steps,
      outputs,
      approvals,
      logs,
      approvalDecisions,
      contexts,
      project,
      feature,
      definition,
    ] =
      await Promise.all([
        this.listStepsByRun(run.id),
        this.listOutputsByRun(run.id),
        this.listApprovalsByRun(run.id),
        this.listLogsByRun(run.id),
        this.listApprovalDecisionsByRun(run.id),
        this.listContextSourcesByIds(run.selectedContextSourceIds),
        this.getProjectById(run.projectId),
        this.getFeatureById(run.featureId),
        this.getWorkflowDefinitionById(run.workflowDefinitionId),
      ]);

    return {
      run,
      steps,
      outputs,
      approvals,
      logs,
      approvalDecisions,
      selectedContextSources: contexts,
      project,
      feature,
      definition,
    };
  }

  async createWorkflowRun(payload: StartWorkflowRunPayload, definition: WorkflowDefinition) {
    const feature = await this.getFeatureById(payload.featureId);

    if (!feature) {
      throw new Error("Feature not found.");
    }

    const runId = createId("run");
    const { data, error } = await this.supabase
      .from("workflow_runs")
      .insert({
        id: runId,
        workflow_definition_id: definition.id,
        project_id: feature.projectId,
        feature_id: feature.id,
        status: "running",
        current_step_key: null,
        selected_context_source_ids: payload.contextSourceIds,
        started_by: "supabase-admin",
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create workflow run.");

    const stepRows = definition.steps.map((step, index) => ({
      id: createId("step"),
      workflow_run_id: runId,
      step_key: step.key,
      step_name: step.name,
      step_type: step.type,
      status: "pending",
      sequence_index: index,
    }));

    if (stepRows.length > 0) {
      const { error: stepError } = await this.supabase
        .from("workflow_steps")
        .insert(stepRows);
      assertNoError(stepError, "Unable to create workflow steps.");
    }

    return mapWorkflowRun(assertRow(data, "Workflow run insert returned no row."));
  }

  async updateWorkflowRun(runId: string, patch: Partial<WorkflowRun>) {
    const { data, error } = await this.supabase
      .from("workflow_runs")
      .update(workflowRunPatch(patch))
      .eq("id", runId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update workflow run.");
    return mapWorkflowRun(assertRow(data, "Workflow run update returned no row."));
  }

  async updateWorkflowStep(stepId: string, patch: Partial<WorkflowStep>) {
    const { data, error } = await this.supabase
      .from("workflow_steps")
      .update(workflowStepPatch(patch))
      .eq("id", stepId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update workflow step.");
    return mapWorkflowStep(assertRow(data, "Workflow step update returned no row."));
  }

  async createOutput(output: AiOutput) {
    const { data, error } = await this.supabase
      .from("ai_outputs")
      .insert({
        id: output.id,
        workflow_run_id: output.workflowRunId,
        workflow_step_id: output.workflowStepId,
        project_id: output.projectId,
        feature_id: output.featureId,
        output_type: output.outputType,
        version: output.version,
        title: output.title,
        content_markdown: output.contentMarkdown,
        is_approved: output.isApproved,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create AI output.");
    return mapOutput(assertRow(data, "AI output insert returned no row."));
  }

  async createApproval(approval: Approval) {
    const { data, error } = await this.supabase
      .from("approvals")
      .insert({
        id: approval.id,
        workflow_run_id: approval.workflowRunId,
        workflow_step_id: approval.workflowStepId,
        ai_output_id: approval.aiOutputId,
        status: approval.status,
        reviewer_id: approval.reviewerId,
        comment: approval.comment,
        decided_at: approval.decidedAt,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create approval.");
    return mapApproval(assertRow(data, "Approval insert returned no row."));
  }

  async getApprovalById(approvalId: string) {
    const { data, error } = await this.supabase
      .from("approvals")
      .select("*")
      .eq("id", approvalId)
      .maybeSingle();
    assertNoError(error, "Unable to load approval.");
    return data ? mapApproval(data) : null;
  }

  async listPendingApprovalDetails() {
    const { data, error } = await this.supabase
      .from("approvals")
      .select("*")
      .eq("status", "pending")
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list pending approvals.");

    const approvals = (data ?? []).map(mapApproval);
    const details = await Promise.all(
      approvals.map(async (approval) => {
        const run = await this.getWorkflowRunById(approval.workflowRunId);

        if (!run) {
          return null;
        }

        const [steps, output, project, feature] = await Promise.all([
          this.listStepsByRun(run.id),
          approval.aiOutputId ? this.getOutputById(approval.aiOutputId) : null,
          this.getProjectById(run.projectId),
          this.getFeatureById(run.featureId),
        ]);

        return {
          approval,
          run,
          step: steps.find((step) => step.id === approval.workflowStepId) ?? null,
          output,
          project,
          feature,
        };
      }),
    );

    return details.filter((detail) => detail !== null);
  }

  async updateApproval(approvalId: string, patch: Partial<Approval>) {
    const { data, error } = await this.supabase
      .from("approvals")
      .update(approvalPatch(patch))
      .eq("id", approvalId)
      .select("*")
      .single();
    assertNoError(error, "Unable to update approval.");
    const approval = mapApproval(assertRow(data, "Approval update returned no row."));

    if (approval.status === "approved" && approval.aiOutputId) {
      const { error: outputError } = await this.supabase
        .from("ai_outputs")
        .update({ is_approved: true })
        .eq("id", approval.aiOutputId);
      assertNoError(outputError, "Unable to mark output approved.");
    }

    return approval;
  }

  async createApprovalDecision(decision: ApprovalDecision) {
    const { data, error } = await this.supabase
      .from("approval_decisions")
      .insert(approvalDecisionRow(decision))
      .select("*")
      .single();
    assertNoError(error, "Unable to create approval decision.");
    return mapApprovalDecision(assertRow(data, "Approval decision insert returned no row."));
  }

  async listApprovalDecisionsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("approval_decisions")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list approval decisions.");
    return (data ?? []).map(mapApprovalDecision);
  }

  async listOutputs(filters?: ListOutputsFilters) {
    let query = this.supabase
      .from("ai_outputs")
      .select("*")
      .order("created_at", { ascending: false });

    if (filters?.projectId) {
      query = query.eq("project_id", filters.projectId);
    }

    if (filters?.featureId) {
      query = query.eq("feature_id", filters.featureId);
    }

    if (filters?.workflowRunId) {
      query = query.eq("workflow_run_id", filters.workflowRunId);
    }

    if (filters?.outputType) {
      query = query.eq("output_type", filters.outputType);
    }

    if (filters?.approvalState) {
      query = query.eq("is_approved", filters.approvalState === "approved");
    }

    const { data, error } = await query;
    assertNoError(error, "Unable to list AI outputs.");
    return (data ?? []).map(mapOutput);
  }

  async getOutputDetail(outputId: string) {
    const output = await this.getOutputById(outputId);

    if (!output) {
      return null;
    }

    const [run, steps, feature, project, approvals, approvalDecisions, versions] =
      await Promise.all([
      this.getWorkflowRunById(output.workflowRunId),
      this.listStepsByRun(output.workflowRunId),
      this.getFeatureById(output.featureId),
      this.getProjectById(output.projectId),
      this.listApprovalsByOutput(output.id),
      this.listApprovalDecisionsByOutput(output.id),
      this.listOutputVersions(output.workflowRunId, output.outputType),
    ]);

    return {
      output,
      run,
      step: steps.find((step) => step.id === output.workflowStepId) ?? null,
      feature,
      project,
      approvals,
      approvalDecisions,
      versions,
    };
  }

  async createLog(log: AiCallLog) {
    const { data, error } = await this.supabase
      .from("ai_call_logs")
      .insert({
        id: log.id,
        workflow_run_id: log.workflowRunId,
        workflow_step_id: log.workflowStepId,
        provider: log.provider,
        model: log.model,
        input_tokens: log.inputTokens,
        output_tokens: log.outputTokens,
        cost_estimate: log.costEstimate,
        latency_ms: log.latencyMs,
        status: log.status,
      })
      .select("*")
      .single();
    assertNoError(error, "Unable to create AI call log.");
    return mapLog(assertRow(data, "AI call log insert returned no row."));
  }

  async listLogs(filters?: { status?: AiCallLog["status"]; provider?: string }) {
    let query = this.supabase
      .from("ai_call_logs")
      .select("*")
      .order("created_at", { ascending: false });

    if (filters?.status) {
      query = query.eq("status", filters.status);
    }

    if (filters?.provider) {
      query = query.eq("provider", filters.provider);
    }

    const { data, error } = await query;
    assertNoError(error, "Unable to list AI call logs.");
    return (data ?? []).map(mapLog);
  }

  async getLogSummary(filters?: { status?: AiCallLog["status"]; provider?: string }) {
    const logs = await this.listLogs(filters);
    const totalLatency = logs.reduce((sum, log) => sum + log.latencyMs, 0);

    return {
      totalCalls: logs.length,
      totalInputTokens: logs.reduce((sum, log) => sum + log.inputTokens, 0),
      totalOutputTokens: logs.reduce((sum, log) => sum + log.outputTokens, 0),
      totalCostEstimate: logs.reduce((sum, log) => sum + log.costEstimate, 0),
      averageLatencyMs: logs.length === 0 ? 0 : Math.round(totalLatency / logs.length),
      failedCalls: logs.filter((log) => log.status === "failed").length,
    };
  }

  private async listStepsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("workflow_steps")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("sequence_index", { ascending: true });
    assertNoError(error, "Unable to list workflow steps.");
    return (data ?? []).map(mapWorkflowStep);
  }

  private async listOutputsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("ai_outputs")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("created_at", { ascending: true });
    assertNoError(error, "Unable to list run outputs.");
    return (data ?? []).map(mapOutput);
  }

  private async listApprovalsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("approvals")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("created_at", { ascending: true });
    assertNoError(error, "Unable to list run approvals.");
    return (data ?? []).map(mapApproval);
  }

  private async listLogsByRun(runId: string) {
    const { data, error } = await this.supabase
      .from("ai_call_logs")
      .select("*")
      .eq("workflow_run_id", runId)
      .order("created_at", { ascending: true });
    assertNoError(error, "Unable to list run logs.");
    return (data ?? []).map(mapLog);
  }

  private async listContextSourcesByIds(contextSourceIds: string[]) {
    if (contextSourceIds.length === 0) {
      return [];
    }

    const { data, error } = await this.supabase
      .from("context_sources")
      .select("*")
      .in("id", contextSourceIds)
      .is("archived_at", null);
    assertNoError(error, "Unable to list selected context sources.");
    return (data ?? []).map(mapContextSource);
  }

  private async getOutputById(outputId: string) {
    const { data, error } = await this.supabase
      .from("ai_outputs")
      .select("*")
      .eq("id", outputId)
      .maybeSingle();
    assertNoError(error, "Unable to load AI output.");
    return data ? mapOutput(data) : null;
  }

  private async listApprovalsByOutput(outputId: string) {
    const { data, error } = await this.supabase
      .from("approvals")
      .select("*")
      .eq("ai_output_id", outputId)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list output approvals.");
    return (data ?? []).map(mapApproval);
  }

  private async listApprovalDecisionsByOutput(outputId: string) {
    const { data, error } = await this.supabase
      .from("approval_decisions")
      .select("*")
      .eq("ai_output_id", outputId)
      .order("created_at", { ascending: false });
    assertNoError(error, "Unable to list output approval decisions.");
    return (data ?? []).map(mapApprovalDecision);
  }

  private async listOutputVersions(workflowRunId: string, outputType: AiOutput["outputType"]) {
    const { data, error } = await this.supabase
      .from("ai_outputs")
      .select("*")
      .eq("workflow_run_id", workflowRunId)
      .eq("output_type", outputType)
      .order("version", { ascending: false });
    assertNoError(error, "Unable to list output versions.");
    return (data ?? []).map(mapOutput);
  }
}

export function createSupabaseGatewayBundle(supabaseClient: SupabaseClient) {
  const supabaseGatewayBundle = new SupabaseGatewayBundle(supabaseClient);
  const workflowExecutor: WorkflowExecutorGateway = new MockWorkflowExecutor(
    supabaseGatewayBundle,
    (projectId) => supabaseGatewayBundle.getProjectById(projectId),
    (featureId) => supabaseGatewayBundle.getFeatureById(featureId),
  );

  return {
    projectGateway: supabaseGatewayBundle,
    featureGateway: supabaseGatewayBundle,
    contextSourceGateway: supabaseGatewayBundle,
    workflowGateway: supabaseGatewayBundle,
    workflowExecutor,
  };
}
