import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { DashboardSummary } from "@/domain/model/response/dashboard-response";

export class GetDashboardSummaryUseCase {
  constructor(
    private readonly projectGateway: ProjectGateway,
    private readonly featureGateway: FeatureGateway,
    private readonly contextGateway: ContextSourceGateway,
    private readonly workflowGateway: WorkflowGateway,
  ) {}

  async execute(): Promise<DashboardSummary> {
    const [projects, features, runs] = await Promise.all([
      this.projectGateway.listProjects(),
      this.featureGateway.listFeatures(),
      this.workflowGateway.listWorkflowRuns(),
    ]);

    await this.contextGateway.listContextSourcesByProject(projects[0]?.id ?? "");

    const pendingApprovalCount = runs.filter(
      (run) => run.status === "waiting_approval",
    ).length;

    const completedOutputCount = runs.filter(
      (run) => run.status === "completed",
    ).length * 3;

    return {
      activeWorkflowCount: runs.filter((run) => run.status === "running").length,
      pendingApprovalCount,
      completedOutputCount,
      recentRuns: [...runs]
        .sort((left, right) => right.startedAt.localeCompare(left.startedAt))
        .slice(0, Math.max(features.length > 0 ? 4 : 0, 4)),
    };
  }
}
