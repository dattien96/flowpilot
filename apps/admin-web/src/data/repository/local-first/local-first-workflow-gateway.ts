import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type {
  AiCallLog,
  AiOutput,
  Approval,
  ApprovalDecision,
  WorkflowDefinition,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";
import type {
  AiLogSummary,
  AiOutputDetail,
  PendingApprovalDetail,
  WorkflowRunDetail,
} from "@/domain/model/response/workflow-response";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

function hasReadableContent(contentMarkdown: string | null | undefined) {
  return (contentMarkdown ?? "").trim().length > 0;
}

function isWorkflowOutputArtifact(artifact: LocalRunnerArtifact) {
  return (
    normalize(artifact.sourceKind) === "workflow_output" &&
    normalize(artifact.workflowRunId) !== ""
  );
}

function mapLocalArtifactToOutput(
  artifact: LocalRunnerArtifact,
  workflowStepId?: string | null,
): AiOutput {
  return {
    id: artifact.artifactId,
    workflowRunId: artifact.workflowRunId,
    workflowStepId: workflowStepId ?? artifact.workflowStepKey,
    projectId: artifact.projectId,
    outputType: "document",
    version: 1,
    title: artifact.title,
    contentMarkdown: artifact.contentMarkdown,
    isApproved: false,
    createdAt: artifact.createdAt,
  };
}

function findStepForArtifact(
  steps: WorkflowStep[],
  artifact: LocalRunnerArtifact,
) {
  const artifactStepKey = normalize(artifact.workflowStepKey);
  return (
    steps.find((step) => normalize(step.stepKey) === artifactStepKey) ??
    steps.find((step) => normalize(step.stepName) === artifactStepKey) ??
    null
  );
}

export class LocalFirstWorkflowGateway implements WorkflowGateway {
  constructor(
    private readonly base: WorkflowGateway,
    private readonly localRunnerGateway: LocalRunnerGateway,
  ) {}

  listWorkflowDefinitions(): Promise<WorkflowDefinition[]> {
    return this.base.listWorkflowDefinitions();
  }

  getWorkflowDefinitionById(
    workflowDefinitionId: string,
  ): Promise<WorkflowDefinition | null> {
    return this.base.getWorkflowDefinitionById(workflowDefinitionId);
  }

  listWorkflowRuns(): Promise<WorkflowRun[]> {
    return this.base.listWorkflowRuns();
  }

  getWorkflowRunById(runId: string): Promise<WorkflowRun | null> {
    return this.base.getWorkflowRunById(runId);
  }

  async getWorkflowRunDetail(runId: string): Promise<WorkflowRunDetail | null> {
    const detail = await this.base.getWorkflowRunDetail(runId);
    if (!detail) {
      return null;
    }

    const localArtifacts = await this.listWorkflowArtifacts({
      projectId: detail.run.projectId,
      workflowRunId: runId,
    });

    return {
      ...detail,
      outputs: this.mergeOutputs(detail.outputs, localArtifacts, detail.steps),
    };
  }

  createWorkflowRun(
    payload: StartWorkflowRunPayload,
    definition: WorkflowDefinition,
  ): Promise<WorkflowRun> {
    return this.base.createWorkflowRun(payload, definition);
  }

  updateWorkflowRun(
    runId: string,
    patch: Partial<WorkflowRun>,
  ): Promise<WorkflowRun> {
    return this.base.updateWorkflowRun(runId, patch);
  }

  updateWorkflowStep(
    stepId: string,
    patch: Partial<WorkflowStep>,
  ): Promise<WorkflowStep> {
    return this.base.updateWorkflowStep(stepId, patch);
  }

  createOutput(output: AiOutput): Promise<AiOutput> {
    return this.base.createOutput(output);
  }

  createApproval(approval: Approval): Promise<Approval> {
    return this.base.createApproval(approval);
  }

  getApprovalById(approvalId: string): Promise<Approval | null> {
    return this.base.getApprovalById(approvalId);
  }

  async listPendingApprovalDetails(): Promise<PendingApprovalDetail[]> {
    const details = await this.base.listPendingApprovalDetails();
    if (details.length === 0) {
      return details;
    }

    const localArtifacts = await this.listWorkflowArtifacts();
    return details.map((detail) => {
      if (detail.output && hasReadableContent(detail.output.contentMarkdown)) {
        return detail;
      }

      const artifact = localArtifacts.find(
        (candidate) =>
          normalize(candidate.workflowRunId) === normalize(detail.run.id) &&
          normalize(candidate.workflowStepKey) ===
            normalize(detail.step?.stepKey),
      );

      if (!artifact) {
        return detail;
      }

      return {
        ...detail,
        output: mapLocalArtifactToOutput(artifact, detail.step?.id ?? null),
      };
    });
  }

  updateApproval(
    approvalId: string,
    patch: Partial<Approval>,
  ): Promise<Approval> {
    return this.base.updateApproval(approvalId, patch);
  }

  createApprovalDecision(
    decision: ApprovalDecision,
  ): Promise<ApprovalDecision> {
    return this.base.createApprovalDecision(decision);
  }

  listApprovalDecisionsByRun(runId: string): Promise<ApprovalDecision[]> {
    return this.base.listApprovalDecisionsByRun(runId);
  }

  async listOutputs(filters?: ListOutputsFilters): Promise<AiOutput[]> {
    const [baseOutputs, localArtifacts] = await Promise.all([
      this.base.listOutputs(filters),
      this.listWorkflowArtifacts(filters),
    ]);

    return this.mergeOutputs(baseOutputs, localArtifacts).sort((left, right) =>
      right.createdAt.localeCompare(left.createdAt),
    );
  }

  async getOutputDetail(outputId: string): Promise<AiOutputDetail | null> {
    const localArtifact = await this.getLocalWorkflowArtifactById(outputId);
    if (localArtifact) {
      return this.buildLocalOutputDetail(localArtifact);
    }

    const detail = await this.base.getOutputDetail(outputId);
    if (!detail) {
      return null;
    }

    if (hasReadableContent(detail.output.contentMarkdown)) {
      return detail;
    }

    const localArtifacts = await this.listWorkflowArtifacts({
      projectId: detail.project?.id ?? detail.output.projectId,
      workflowRunId: detail.output.workflowRunId,
    });
    const matchingArtifact = localArtifacts.find(
      (artifact) =>
        normalize(artifact.title) === normalize(detail.output.title) ||
        normalize(artifact.workflowStepKey) === normalize(detail.step?.stepKey),
    );

    if (!matchingArtifact) {
      return detail;
    }

    return {
      ...detail,
      output: mapLocalArtifactToOutput(
        matchingArtifact,
        detail.step?.id ?? detail.output.workflowStepId,
      ),
      versions: this.mergeOutputs(
        detail.versions,
        [matchingArtifact],
        detail.step ? [detail.step] : undefined,
      ),
    };
  }

  createLog(log: AiCallLog): Promise<AiCallLog> {
    return this.base.createLog(log);
  }

  listLogs(filters?: {
    status?: AiCallLog["status"];
    provider?: string;
  }): Promise<AiCallLog[]> {
    return this.base.listLogs(filters);
  }

  getLogSummary(filters?: {
    status?: AiCallLog["status"];
    provider?: string;
  }): Promise<AiLogSummary> {
    return this.base.getLogSummary(filters);
  }

  private async listWorkflowArtifacts(filters?: {
    projectId?: string;
    workflowRunId?: string;
  }): Promise<LocalRunnerArtifact[]> {
    try {
      const artifacts = await this.localRunnerGateway.listArtifacts();
      return artifacts.filter((artifact) => {
        if (!isWorkflowOutputArtifact(artifact)) {
          return false;
        }
        if (
          filters?.projectId &&
          normalize(artifact.projectId) !== normalize(filters.projectId)
        ) {
          return false;
        }
        if (
          filters?.workflowRunId &&
          normalize(artifact.workflowRunId) !== normalize(filters.workflowRunId)
        ) {
          return false;
        }
        return true;
      });
    } catch {
      return [];
    }
  }

  private async getLocalWorkflowArtifactById(
    outputId: string,
  ): Promise<LocalRunnerArtifact | null> {
    try {
      const artifact = await this.localRunnerGateway.getArtifactById(outputId);
      return artifact && isWorkflowOutputArtifact(artifact) ? artifact : null;
    } catch {
      return null;
    }
  }

  private mergeOutputs(
    baseOutputs: AiOutput[],
    localArtifacts: LocalRunnerArtifact[],
    steps?: WorkflowStep[],
  ): AiOutput[] {
    const merged = new Map<string, AiOutput>();

    for (const output of baseOutputs) {
      if (!hasReadableContent(output.contentMarkdown)) {
        continue;
      }
      merged.set(output.id, output);
    }

    for (const artifact of localArtifacts) {
      const step = steps ? findStepForArtifact(steps, artifact) : null;
      const localOutput = mapLocalArtifactToOutput(artifact, step?.id ?? null);
      const current = merged.get(localOutput.id);
      merged.set(localOutput.id, current ? { ...current, ...localOutput } : localOutput);
    }

    return Array.from(merged.values()).sort((left, right) =>
      left.createdAt.localeCompare(right.createdAt),
    );
  }

  private async buildLocalOutputDetail(
    artifact: LocalRunnerArtifact,
  ): Promise<AiOutputDetail | null> {
    const runDetail = await this.getWorkflowRunDetail(artifact.workflowRunId);
    if (!runDetail) {
      return null;
    }

    const step = findStepForArtifact(runDetail.steps, artifact);
    const output = mapLocalArtifactToOutput(artifact, step?.id ?? null);
    const approvals = step
      ? runDetail.approvals.filter(
          (approval) => approval.workflowStepId === step.id,
        )
      : [];
    const approvalDecisions = step
      ? (runDetail.approvalDecisions ?? []).filter(
          (decision) => decision.workflowStepId === step.id,
        )
      : [];
    const siblingArtifacts = runDetail.outputs.filter(
      (candidate) =>
        normalize(candidate.workflowRunId) ===
          normalize(output.workflowRunId) &&
        normalize(candidate.title) === normalize(output.title),
    );

    return {
      output,
      run: runDetail.run,
      step,
      project: runDetail.project ?? null,
      approvals,
      approvalDecisions,
      versions: siblingArtifacts.length > 0 ? siblingArtifacts : [output],
    };
  }
}
