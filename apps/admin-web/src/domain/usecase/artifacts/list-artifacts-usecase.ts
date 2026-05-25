import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";

function preview(contents: string) {
  const trimmed = contents.trim();
  if (trimmed.length <= 240) {
    return trimmed;
  }

  return `${trimmed.slice(0, 240)}\n...[truncated]`;
}

export class ListArtifactsUseCase {
  constructor(
    private readonly workflowGateway: WorkflowGateway,
    private readonly localRunnerGateway: LocalRunnerGateway,
  ) {}

  async execute() {
    const [workflowRuns, localArtifacts] = await Promise.all([
      this.workflowGateway.listWorkflowRuns(),
      this.localRunnerGateway.listArtifacts(),
    ]);

    const workflowDetails = await Promise.all(
      workflowRuns.map((run) => this.workflowGateway.getWorkflowRunDetail(run.id)),
    );

    const workflowArtifacts: LocalRunnerArtifact[] = workflowDetails.flatMap((detail) => {
      if (!detail) {
        return [];
      }

      const stepById = new Map(detail.steps.map((step) => [step.id, step]));
      return detail.outputs.map((output) => {
        const step = stepById.get(output.workflowStepId);
        return {
          artifactId: output.id,
          title: output.title,
          sourceKind: "workflow_output",
          projectId: output.projectId,
          workflowRunId: output.workflowRunId,
          workflowStepKey: step?.stepKey ?? output.workflowStepId,
          providerKey: "mock",
          localPath: "",
          remotePath: "",
          remoteUrl: "",
          syncStatus: "local_only",
          createdAt: output.createdAt,
          updatedAt: output.createdAt,
          contentMarkdown: output.contentMarkdown,
          previewMarkdown: preview(output.contentMarkdown),
        };
      });
    });

    const merged = new Map<string, LocalRunnerArtifact>();

    for (const artifact of [...workflowArtifacts, ...localArtifacts]) {
      const current = merged.get(artifact.artifactId);
      if (!current) {
        merged.set(artifact.artifactId, artifact);
        continue;
      }

      if (!current.localPath && artifact.localPath) {
        merged.set(artifact.artifactId, artifact);
      }
    }

    return Array.from(merged.values()).sort((left, right) =>
      right.updatedAt.localeCompare(left.updatedAt),
    );
  }
}
