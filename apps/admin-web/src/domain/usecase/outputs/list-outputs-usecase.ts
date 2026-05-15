import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ListOutputsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  async execute() {
    const runs = await this.workflowGateway.listWorkflowRuns();
    const details = await Promise.all(
      runs.map((run) => this.workflowGateway.getWorkflowRunDetail(run.id)),
    );

    return details.flatMap((detail) => detail?.outputs ?? []);
  }
}
