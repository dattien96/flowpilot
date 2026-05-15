import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { WorkflowExecutorGateway, WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Feature } from "@/domain/model/entity/feature";
import type { Project } from "@/domain/model/entity/project";
import type {
  AiCallLog,
  AiOutput,
  Approval,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type { CreateContextSourcePayload } from "@/domain/model/payload/context-source-payload";
import type { CreateFeaturePayload } from "@/domain/model/payload/feature-payload";
import type { CreateProjectPayload } from "@/domain/model/payload/project-payload";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";
import {
  buildWorkflowRun,
  buildWorkflowSteps,
  demoApprovals,
  demoContextSources,
  demoFeatures,
  demoLogs,
  demoOutputs,
  demoProjects,
  demoWorkflowDefinitions,
  demoWorkflowRuns,
  demoWorkflowSteps,
} from "@/data/repository/demo/demo-store";
import { MockWorkflowExecutor } from "@/data/workflow/mock-workflow-executor";

function createId(prefix: string) {
  return `${prefix}_${Math.random().toString(36).slice(2, 10)}`;
}

class DemoGatewayBundle
  implements
    ProjectGateway,
    FeatureGateway,
    ContextSourceGateway,
    WorkflowGateway
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
    const project: Project = {
      id: createId("project"),
      name: payload.name,
      description: payload.description,
      platform: payload.platform,
      repositoryUrl: payload.repositoryUrl,
      createdBy: "demo-user",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    demoProjects.unshift(project);
    return project;
  }

  listFeatures() {
    return Promise.resolve(demoFeatures);
  }

  listFeaturesByProject(projectId: string) {
    return Promise.resolve(
      demoFeatures.filter((feature) => feature.projectId === projectId),
    );
  }

  getFeatureById(featureId: string) {
    return Promise.resolve(
      demoFeatures.find((feature) => feature.id === featureId) ?? null,
    );
  }

  async createFeature(payload: CreateFeaturePayload) {
    const feature: Feature = {
      id: createId("feature"),
      projectId: payload.projectId,
      title: payload.title,
      businessGoal: payload.businessGoal,
      userProblem: payload.userProblem,
      expectedFlow: payload.expectedFlow,
      acceptanceCriteria: payload.acceptanceCriteria,
      priority: payload.priority,
      status: "draft",
      ownerId: "demo-user",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    demoFeatures.unshift(feature);
    return feature;
  }

  listContextSourcesByFeature(featureId: string) {
    return Promise.resolve(
      demoContextSources.filter((context) => context.featureId === featureId),
    );
  }

  listContextSourcesByProject(projectId: string) {
    return Promise.resolve(
      demoContextSources.filter((context) => context.projectId === projectId),
    );
  }

  async createContextSource(payload: CreateContextSourcePayload) {
    const contextSource: ContextSource = {
      id: createId("context"),
      projectId: payload.projectId,
      featureId: payload.featureId,
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
    };
  }

  async createWorkflowRun(payload: StartWorkflowRunPayload, definition: WorkflowDefinition) {
    const feature = demoFeatures.find((item) => item.id === payload.featureId);

    if (!feature) {
      throw new Error("Feature not found.");
    }

    const run = buildWorkflowRun(
      feature.id,
      feature.projectId,
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

  async createLog(log: AiCallLog) {
    demoLogs.push(log);
    return log;
  }
}

const demoGatewayBundle = new DemoGatewayBundle();

export function createDemoGatewayBundle() {
  const workflowExecutor: WorkflowExecutorGateway = new MockWorkflowExecutor(
    demoGatewayBundle,
    (projectId) => demoGatewayBundle.getProjectById(projectId),
    (featureId) => demoGatewayBundle.getFeatureById(featureId),
  );

  return {
    projectGateway: demoGatewayBundle,
    featureGateway: demoGatewayBundle,
    contextSourceGateway: demoGatewayBundle,
    workflowGateway: demoGatewayBundle,
    workflowExecutor,
  };
}
