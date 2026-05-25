import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { DashboardSummary } from "@/domain/model/response/dashboard-response";

export class GetDashboardSummaryUseCase {
  constructor(
    private readonly projectGateway: ProjectGateway,
    private readonly contextGateway: ContextSourceGateway,
    private readonly workflowGateway: WorkflowGateway,
  ) {}

  async execute(): Promise<DashboardSummary> {
    const [projects, runs, outputs] = await Promise.all([
      this.projectGateway.listProjects(),
      this.workflowGateway.listWorkflowRuns(),
      this.workflowGateway.listOutputs(),
    ]);

    await this.contextGateway.listContextSourcesByProject(projects[0]?.id ?? "");

    const pendingApprovalCount = runs.filter(
      (run) => run.status === "waiting_approval",
    ).length;

    return {
      activeWorkflowCount: runs.filter((run) => run.status === "running").length,
      pendingApprovalCount,
      completedOutputCount: outputs.length,
      projectCount: projects.length,
      recentRuns: [...runs]
        .sort((left, right) => right.startedAt.localeCompare(left.startedAt))
        .slice(0, 4),
    };
  }
}
