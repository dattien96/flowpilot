import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { WorkflowRun } from "@/domain/model/entity/workflow-engine";

export class ToggleYoloModeUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(runId: string, yoloMode: boolean): Promise<WorkflowRun> {
    if (!runId) throw new Error("Run ID is required.");
    return this.gateway.toggleYoloMode(runId, yoloMode);
  }
}
