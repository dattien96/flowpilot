import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { IntegrationGateway } from "@/domain/gateway/integration-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { TeamGateway } from "@/domain/gateway/team-gateway";
import type { WorkflowExecutorGateway, WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Integration } from "@/domain/model/entity/integration";
import type { Project } from "@/domain/model/entity/project";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";
import type { Team, TeamMember } from "@/domain/model/entity/team";
import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type {
  CreateContextSourcePayload,
  UpdateContextSourcePayload,
} from "@/domain/model/payload/context-source-payload";
import type {
  CreateIntegrationPayload,
  UpdateIntegrationPayload,
} from "@/domain/model/payload/integration-payload";
import type { CreateProjectPayload, UpdateProjectPayload } from "@/domain/model/payload/project-payload";
import type {
  CreateProjectWorkspaceBindingPayload,
  UpdateProjectWorkspaceBindingPayload,
} from "@/domain/model/payload/project-workspace-binding-payload";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";
import {
  buildWorkflowRun,
  buildWorkflowSteps,
  demoApprovalDecisions,
  demoApprovals,
  demoContextSources,
  demoIntegrations,
  demoLogs,
  demoOutputs,
  demoProjectMcpLinks,
  demoProjects,
  demoProjectWorkspaceBindings,
  demoProjectTeamLinks,
  demoTeamMembers,
  demoTeams,
  demoWorkflowDefinitions,
  demoWorkflowRuns,
  demoWorkflowSteps,
} from "@/data/repository/demo/demo-store";
import { MockWorkflowExecutor } from "@/data/workflow/mock-workflow-executor";

function createId(prefix: string) {
  return `${prefix}_${Math.random().toString(36).slice(2, 10)}`;
}

function activeContextSources() {
  return demoContextSources.filter((context) => !context.archivedAt);
}

class DemoGatewayBundle
  implements
    ProjectGateway,
    ContextSourceGateway,
    WorkflowGateway,
    TeamGateway,
    IntegrationGateway
{
  listProjects() {
    return Promise.resolve(demoProjects);
  }

  getProjectById(projectId: string) {
    return Promise.resolve(
      demoProjects.find((project) => project.id === projectId) ?? null,
    );
  }

  async createProject(payload: CreateProjectPayload) {
    const primaryPath = payload.directoryPath.trim();
    const project: Project = {
      id: createId("project"),
      name: payload.name,
      description: payload.description,
      platform: payload.platform,
      repositoryUrl: payload.repositoryUrl,
      directoryPath: primaryPath || null,
      ownerId: payload.ownerId ?? null,
      status: payload.status ?? "active",
      artifactStoragePreference: payload.artifactStoragePreference ?? "supabase",
      defaultProvider: payload.defaultProvider ?? null,
      defaultModel: payload.defaultModel ?? null,
      defaultReasoningEffort: payload.defaultReasoningEffort ?? null,
      createdBy: "demo-user",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    demoProjects.unshift(project);
    if (primaryPath) {
      demoProjectWorkspaceBindings.unshift({
        id: createId("binding"),
        projectId: project.id,
        localPath: primaryPath,
        label: "Primary",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      });
    }
    return project;
  }

  async updateProject(projectId: string, patch: Partial<UpdateProjectPayload>) {
    const project = demoProjects.find((item) => item.id === projectId);
    if (!project) throw new Error("Project not found.");
    Object.assign(
      project,
      Object.fromEntries(
        Object.entries({
          ...patch,
          defaultProvider: patch.defaultProvider,
          defaultModel: patch.defaultModel,
          defaultReasoningEffort: patch.defaultReasoningEffort,
        }).filter(([, value]) => value !== undefined),
      ),
    );
    return project;
  }

  async deleteProject(projectId: string) {
    const index = demoProjects.findIndex((item) => item.id === projectId);
    if (index === -1) throw new Error("Project not found.");
    demoProjects.splice(index, 1);
  }

  async listTeamsByProject(projectId: string) {
    const teamIds = new Set(
      demoProjectTeamLinks.filter((link) => link.projectId === projectId).map((link) => link.teamId),
    );
    return demoTeams.filter((team) => teamIds.has(team.id));
  }

  async listProjectWorkspaceBindings(projectId: string) {
    return demoProjectWorkspaceBindings
      .filter((binding) => binding.projectId === projectId)
      .sort((left, right) => left.createdAt.localeCompare(right.createdAt));
  }

  async createProjectWorkspaceBinding(
    projectId: string,
    payload: CreateProjectWorkspaceBindingPayload,
  ) {
    const localPath = payload.localPath.trim();
    const label = payload.label?.trim() || null;
    const binding: ProjectWorkspaceBinding = {
      id: createId("binding"),
      projectId,
      localPath,
      label,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    demoProjectWorkspaceBindings.unshift(binding);
    return binding;
  }

  async updateProjectWorkspaceBinding(
    bindingId: string,
    patch: Partial<UpdateProjectWorkspaceBindingPayload>,
  ) {
    const binding = demoProjectWorkspaceBindings.find((item) => item.id === bindingId);
    if (!binding) throw new Error("Binding not found.");
    if (patch.localPath !== undefined) binding.localPath = patch.localPath.trim();
    if (patch.label !== undefined) binding.label = patch.label?.trim() || null;
    binding.updatedAt = new Date().toISOString();
    return binding;
  }

  async deleteProjectWorkspaceBinding(bindingId: string) {
    const index = demoProjectWorkspaceBindings.findIndex((item) => item.id === bindingId);
    if (index === -1) throw new Error("Binding not found.");
    demoProjectWorkspaceBindings.splice(index, 1);
  }

  async listTeams() {
    return demoTeams;
  }
  async getTeamById(teamId: string) {
    return Promise.resolve(demoTeams.find((team) => team.id === teamId) ?? null);
  }
  async createTeam(name: string) {
    const team = { id: createId("team"), name, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
    demoTeams.unshift(team);
    return team;
  }
  async updateTeam(teamId: string, name: string) {
    const team = demoTeams.find((item) => item.id === teamId);
    if (!team) throw new Error("Team not found.");
    team.name = name;
    team.updatedAt = new Date().toISOString();
    return team;
  }
  async deleteTeam(teamId: string) {
    const index = demoTeams.findIndex((item) => item.id === teamId);
    if (index === -1) throw new Error("Team not found.");
    demoTeams.splice(index, 1);
    for (let i = demoProjectTeamLinks.length - 1; i >= 0; i -= 1) {
      if (demoProjectTeamLinks[i].teamId === teamId) demoProjectTeamLinks.splice(i, 1);
    }
    for (let i = demoTeamMembers.length - 1; i >= 0; i -= 1) {
      if (demoTeamMembers[i].teamId === teamId) demoTeamMembers.splice(i, 1);
    }
  }
  async listMembersByTeam(teamId: string) {
    return demoTeamMembers.filter((member) => member.teamId === teamId);
  }
  async addMember(member: Omit<TeamMember, "id" | "createdAt" | "updatedAt">) {
    const created = { ...member, id: createId("member"), createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
    demoTeamMembers.unshift(created);
    return created;
  }
  async updateMember(memberId: string, patch: Partial<TeamMember>) {
    const member = demoTeamMembers.find((item) => item.id === memberId);
    if (!member) throw new Error("Member not found.");
    Object.assign(member, {
      ...patch,
      teamId: patch.teamId ?? member.teamId,
      updatedAt: new Date().toISOString(),
    });
    return member;
  }
  async removeMember(memberId: string) {
    const index = demoTeamMembers.findIndex((item) => item.id === memberId);
    if (index === -1) throw new Error("Member not found.");
    demoTeamMembers.splice(index, 1);
  }
  async linkTeamToProject(projectId: string, teamId: string) {
    if (!demoProjectTeamLinks.some((link) => link.projectId === projectId && link.teamId === teamId)) {
      demoProjectTeamLinks.push({ projectId, teamId });
    }
  }
  async unlinkTeamFromProject(projectId: string, teamId: string) {
    const index = demoProjectTeamLinks.findIndex((link) => link.projectId === projectId && link.teamId === teamId);
    if (index !== -1) demoProjectTeamLinks.splice(index, 1);
  }

  async listIntegrationsByProject(projectId: string) {
    return demoIntegrations
      .filter((integration) => integration.projectId === projectId)
      .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
  }

  async listAllIntegrations() {
    return [...demoIntegrations].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
  }

  async listLinkedIntegrationsByProject(projectId: string) {
    const linkedIds = new Set(
      demoProjectMcpLinks
        .filter((link) => link.projectId === projectId)
        .map((link) => link.integrationId),
    );
    return demoIntegrations
      .filter((integration) => linkedIds.has(integration.id))
      .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
  }

  async createIntegration(payload: CreateIntegrationPayload) {
    const now = new Date().toISOString();
    const integration: Integration = {
      id: createId("integration"),
      projectId: payload.projectId,
      type: payload.type,
      label: payload.label,
      configEncrypted: payload.configEncrypted,
      status: payload.status ?? "pending",
      mcpTypeEnabled: payload.mcpTypeEnabled ?? false,
      lastSyncedAt: null,
      lastError: null,
      createdAt: now,
      updatedAt: now,
    };
    demoIntegrations.unshift(integration);
    demoProjectMcpLinks.push({
      projectId: payload.projectId,
      integrationId: integration.id,
      type: integration.type,
    });
    return integration;
  }

  async updateIntegration(
    integrationId: string,
    patch: Partial<UpdateIntegrationPayload>,
  ) {
    const integration = demoIntegrations.find((item) => item.id === integrationId);
    if (!integration) throw new Error("Integration not found.");
    const nextLastError =
      patch.status === "pending" && patch.lastError === undefined
        ? null
        : patch.lastError;
    Object.assign(integration, {
      ...patch,
      lastError:
        nextLastError !== undefined ? nextLastError : integration.lastError,
      updatedAt: new Date().toISOString(),
    });
    return integration;
  }

  async deleteIntegration(integrationId: string) {
    const index = demoIntegrations.findIndex((item) => item.id === integrationId);
    if (index === -1) throw new Error("Integration not found.");
    demoIntegrations.splice(index, 1);
    for (let i = demoProjectMcpLinks.length - 1; i >= 0; i -= 1) {
      if (demoProjectMcpLinks[i].integrationId === integrationId) {
        demoProjectMcpLinks.splice(i, 1);
      }
    }
  }

  async linkIntegrationToProject(projectId: string, integrationId: string) {
    const integration = demoIntegrations.find((item) => item.id === integrationId);
    if (!integration) throw new Error("Integration not found.");
    const existingIndex = demoProjectMcpLinks.findIndex(
      (link) => link.projectId === projectId && link.type === integration.type,
    );
    const nextLink = { projectId, integrationId, type: integration.type };
    if (existingIndex === -1) {
      demoProjectMcpLinks.push(nextLink);
      return;
    }
    demoProjectMcpLinks[existingIndex] = nextLink;
  }

  async unlinkIntegrationFromProject(projectId: string, integrationId: string) {
    const index = demoProjectMcpLinks.findIndex(
      (link) => link.projectId === projectId && link.integrationId === integrationId,
    );
    if (index !== -1) {
      demoProjectMcpLinks.splice(index, 1);
    }
  }

  listContextSources() {
    return Promise.resolve(activeContextSources());
  }

  listContextSourcesByProject(projectId: string) {
    return Promise.resolve(
      activeContextSources().filter((context) => context.projectId === projectId),
    );
  }

  getContextSourceById(contextSourceId: string) {
    return Promise.resolve(
      activeContextSources().find((context) => context.id === contextSourceId) ?? null,
    );
  }

  async createContextSource(payload: CreateContextSourcePayload) {
    const contextSource: ContextSource = {
      id: createId("context"),
      projectId: payload.projectId,
      type: payload.type,
      title: payload.title,
      rawContent: payload.rawContent,
      summarizedContent: null,
      createdBy: "demo-user",
      createdAt: new Date().toISOString(),
    };
    demoContextSources.unshift(contextSource);
    return contextSource;
  }

  async updateContextSource(payload: UpdateContextSourcePayload) {
    const contextSource = demoContextSources.find(
      (context) => context.id === payload.contextSourceId && !context.archivedAt,
    );

    if (!contextSource) {
      throw new Error("Context source not found.");
    }

    Object.assign(contextSource, {
      title: payload.title,
      type: payload.type,
      rawContent: payload.rawContent,
      summarizedContent: payload.summarizedContent ?? null,
    });

    return contextSource;
  }

  async deleteContextSource(contextSourceId: string) {
    const contextSource = demoContextSources.find(
      (context) => context.id === contextSourceId,
    );

    if (!contextSource) {
      throw new Error("Context source not found.");
    }

    contextSource.archivedAt = new Date().toISOString();
  }

  listWorkflowDefinitions() {
    return Promise.resolve(demoWorkflowDefinitions);
  }

  getWorkflowDefinitionById(workflowDefinitionId: string) {
    return Promise.resolve(
      demoWorkflowDefinitions.find((item) => item.id === workflowDefinitionId) ?? null,
    );
  }

  listWorkflowRuns() {
    return Promise.resolve(demoWorkflowRuns);
  }

  getWorkflowRunById(runId: string) {
    return Promise.resolve(demoWorkflowRuns.find((run) => run.id === runId) ?? null);
  }

  async deleteWorkflowRuns(runIds: string[]) {
    const ids = new Set(runIds);
    if (ids.size === 0) {
      return;
    }

    for (let index = demoWorkflowRuns.length - 1; index >= 0; index -= 1) {
      if (ids.has(demoWorkflowRuns[index].id)) {
        demoWorkflowRuns.splice(index, 1);
      }
    }

    for (let index = demoWorkflowSteps.length - 1; index >= 0; index -= 1) {
      if (ids.has(demoWorkflowSteps[index].workflowRunId)) {
        demoWorkflowSteps.splice(index, 1);
      }
    }

    for (let index = demoOutputs.length - 1; index >= 0; index -= 1) {
      if (ids.has(demoOutputs[index].workflowRunId)) {
        demoOutputs.splice(index, 1);
      }
    }

    for (let index = demoApprovals.length - 1; index >= 0; index -= 1) {
      if (ids.has(demoApprovals[index].workflowRunId)) {
        demoApprovals.splice(index, 1);
      }
    }

    for (let index = demoApprovalDecisions.length - 1; index >= 0; index -= 1) {
      if (ids.has(demoApprovalDecisions[index].workflowRunId)) {
        demoApprovalDecisions.splice(index, 1);
      }
    }

    for (let index = demoLogs.length - 1; index >= 0; index -= 1) {
      if (ids.has(demoLogs[index].workflowRunId)) {
        demoLogs.splice(index, 1);
      }
    }
  }

  async getWorkflowRunDetail(runId: string) {
    const run = demoWorkflowRuns.find((item) => item.id === runId);

    if (!run) {
      return null;
    }

    return {
      run,
      steps: demoWorkflowSteps
        .filter((step) => step.workflowRunId === runId)
        .sort((left, right) => left.sequenceIndex - right.sequenceIndex),
      outputs: demoOutputs.filter((output) => output.workflowRunId === runId),
      approvals: demoApprovals.filter((approval) => approval.workflowRunId === runId),
      logs: demoLogs.filter((log) => log.workflowRunId === runId),
      approvalDecisions: demoApprovalDecisions.filter(
        (decision) => decision.workflowRunId === runId,
      ),
      selectedContextSources: activeContextSources().filter((context) =>
        run.selectedContextSourceIds.includes(context.id),
      ),
      project: demoProjects.find((project) => project.id === run.projectId) ?? null,
      definition:
        demoWorkflowDefinitions.find(
          (definition) => definition.id === run.workflowDefinitionId,
        ) ?? null,
    };
  }

  async createWorkflowRun(payload: StartWorkflowRunPayload, definition: WorkflowDefinition) {
    const project = demoProjects.find((item) => item.id === payload.projectId);

    if (!project) {
      throw new Error("Project not found.");
    }

    const run = buildWorkflowRun(
      project.id,
      definition.id,
      payload.contextSourceIds,
    );

    demoWorkflowRuns.unshift(run);
    demoWorkflowSteps.push(...buildWorkflowSteps(run.id, definition));

    return run;
  }

  async updateWorkflowRun(runId: string, patch: Partial<WorkflowRun>) {
    const run = demoWorkflowRuns.find((item) => item.id === runId);

    if (!run) {
      throw new Error("Workflow run not found.");
    }

    Object.assign(run, patch);
    return run;
  }

  async updateWorkflowStep(stepId: string, patch: Partial<WorkflowStep>) {
    const step = demoWorkflowSteps.find((item) => item.id === stepId);

    if (!step) {
      throw new Error("Workflow step not found.");
    }

    Object.assign(step, patch);
    return step;
  }

  async createOutput(output: AiOutput) {
    demoOutputs.push(output);
    return output;
  }

  async createApproval(approval: Approval) {
    demoApprovals.push(approval);
    return approval;
  }

  getApprovalById(approvalId: string) {
    return Promise.resolve(
      demoApprovals.find((approval) => approval.id === approvalId) ?? null,
    );
  }

  async listPendingApprovalDetails() {
    const pendingApprovals = demoApprovals.filter(
      (approval) => approval.status === "pending",
    );

    return pendingApprovals
      .map((approval) => {
        const run =
          demoWorkflowRuns.find((item) => item.id === approval.workflowRunId) ?? null;

        if (!run) {
          return null;
        }

        return {
          approval,
          run,
          step:
            demoWorkflowSteps.find((step) => step.id === approval.workflowStepId) ??
            null,
          output:
            demoOutputs.find((output) => output.id === approval.aiOutputId) ?? null,
          project: demoProjects.find((project) => project.id === run.projectId) ?? null,
        };
      })
      .filter((item) => item !== null)
      .sort((left, right) =>
        right.approval.createdAt.localeCompare(left.approval.createdAt),
      );
  }

  async updateApproval(approvalId: string, patch: Partial<Approval>) {
    const approval = demoApprovals.find((item) => item.id === approvalId);

    if (!approval) {
      throw new Error("Approval not found.");
    }

    Object.assign(approval, patch);

    if (patch.status === "approved" && approval.aiOutputId) {
      const output = demoOutputs.find((item) => item.id === approval.aiOutputId);
      if (output) {
        output.isApproved = true;
      }
    }

    return approval;
  }

  async createApprovalDecision(decision: ApprovalDecision) {
    demoApprovalDecisions.push(decision);
    return decision;
  }

  listApprovalDecisionsByRun(runId: string) {
    return Promise.resolve(
      demoApprovalDecisions.filter((decision) => decision.workflowRunId === runId),
    );
  }

  async createLog(log: AiCallLog) {
    demoLogs.push(log);
    return log;
  }

  listOutputs(filters?: ListOutputsFilters) {
    return Promise.resolve(
      [...demoOutputs]
        .filter((output) => !filters?.projectId || output.projectId === filters.projectId)
        .filter(
          (output) =>
            !filters?.workflowRunId || output.workflowRunId === filters.workflowRunId,
        )
        .filter((output) => !filters?.outputType || output.outputType === filters.outputType)
        .filter((output) =>
          filters?.approvalState === "approved"
            ? output.isApproved
            : filters?.approvalState === "pending"
              ? !output.isApproved
              : true,
        )
        .sort((left, right) => right.createdAt.localeCompare(left.createdAt)),
    );
  }

  async getOutputDetail(outputId: string) {
    const output = demoOutputs.find((item) => item.id === outputId);

    if (!output) {
      return null;
    }

    const run = demoWorkflowRuns.find((item) => item.id === output.workflowRunId) ?? null;

    return {
      output,
      run,
      step: demoWorkflowSteps.find((step) => step.id === output.workflowStepId) ?? null,
      project: demoProjects.find((project) => project.id === output.projectId) ?? null,
      approvals: demoApprovals.filter((approval) => approval.aiOutputId === output.id),
      approvalDecisions: demoApprovalDecisions.filter(
        (decision) => decision.aiOutputId === output.id,
      ),
      versions: demoOutputs
        .filter(
          (item) =>
            item.workflowRunId === output.workflowRunId &&
            item.outputType === output.outputType,
        )
        .sort((left, right) => right.version - left.version),
    };
  }

  async listLogs(filters?: { status?: AiCallLog["status"]; provider?: string }) {
    return [...demoLogs]
      .filter((log) => !filters?.status || log.status === filters.status)
      .filter((log) => !filters?.provider || log.provider === filters.provider)
      .sort((left, right) => right.createdAt.localeCompare(left.createdAt));
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
}

const demoGatewayBundle = new DemoGatewayBundle();

export function createDemoGatewayBundle() {
  const workflowExecutor: WorkflowExecutorGateway = new MockWorkflowExecutor(
    demoGatewayBundle,
    (projectId) => demoGatewayBundle.getProjectById(projectId),
  );

  return {
    projectGateway: demoGatewayBundle,
    integrationGateway: demoGatewayBundle,
    contextSourceGateway: demoGatewayBundle,
    workflowGateway: demoGatewayBundle,
    teamGateway: demoGatewayBundle,
    workflowExecutor,
  };
}
