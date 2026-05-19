import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { TeamGateway } from "@/domain/gateway/team-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { ProjectDetail } from "@/domain/model/response/project-response";

export class GetProjectDetailUseCase {
  constructor(
    private readonly projectGateway: ProjectGateway,
    private readonly featureGateway: FeatureGateway,
    private readonly workflowGateway: WorkflowGateway,
    private readonly teamGateway?: TeamGateway,
  ) {}

  async execute(projectId: string): Promise<ProjectDetail | null> {
    const [project, features, runs, teams] = await Promise.all([
      this.projectGateway.getProjectById(projectId),
      this.featureGateway.listFeaturesByProject(projectId),
      this.workflowGateway.listWorkflowRuns(),
      this.teamGateway?.listTeamsByProject(projectId) ?? Promise.resolve([]),
    ]);

    if (!project) {
      return null;
    }

    return {
      project,
      features,
      workflowRuns: runs.filter((run) => run.projectId === projectId),
      teams,
      members: this.teamGateway
        ? (
            await Promise.all(
              teams.map((team) => this.teamGateway!.listMembersByTeam(team.id)),
            )
          ).flat()
        : [],
    };
  }
}
