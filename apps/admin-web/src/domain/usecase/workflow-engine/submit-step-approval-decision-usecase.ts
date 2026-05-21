import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { WorkflowRunStep } from "@/domain/model/entity/workflow-engine";

export class SubmitStepApprovalDecisionUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(
    stepId: string,
    approve: boolean,
    comment?: string
  ): Promise<WorkflowRunStep> {
    if (!stepId) throw new Error("Step ID is required.");
    return this.gateway.submitStepApproval(stepId, approve, comment);
  }
}
