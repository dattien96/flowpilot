import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { WorkflowRun, WorkflowRunLog, WorkflowRunStep } from "@/domain/model/entity/workflow-engine";

export class GetWorkflowRunDetailUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(
    runId: string
  ): Promise<{
    run: WorkflowRun;
    steps: WorkflowRunStep[];
    logs: WorkflowRunLog[];
  } | null> {
    if (!runId) throw new Error("Run ID is required.");
    return this.gateway.getWorkflowRunDetail(runId);
  }
}
