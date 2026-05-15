import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { ProjectDetail } from "@/domain/model/response/project-response";

export class GetProjectDetailUseCase {
  constructor(
    private readonly projectGateway: ProjectGateway,
    private readonly featureGateway: FeatureGateway,
    private readonly workflowGateway: WorkflowGateway,
  ) {}

  async execute(projectId: string): Promise<ProjectDetail | null> {
    const [project, features, runs] = await Promise.all([
      this.projectGateway.getProjectById(projectId),
      this.featureGateway.listFeaturesByProject(projectId),
      this.workflowGateway.listWorkflowRuns(),
    ]);

    if (!project) {
      return null;
    }

    return {
      project,
      features,
      workflowRuns: runs.filter((run) => run.projectId === projectId),
    };
  }
}
