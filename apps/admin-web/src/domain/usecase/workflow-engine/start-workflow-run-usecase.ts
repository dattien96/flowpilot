import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type {
  WorkflowRun,
  WorkflowRunStartRequest,
} from "@/domain/model/entity/workflow-engine";

export class StartWorkflowRunUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(request: WorkflowRunStartRequest): Promise<WorkflowRun> {
    if (!request.projectId) {
      throw new Error("Project ID is required.");
    }
    if (!request.beginPrompt.trim()) {
      throw new Error("Begin prompt is required.");
    }
    if (request.startMode === "workflow-definition" && !request.workflowId) {
      throw new Error("Workflow ID is required for workflow definition launches.");
    }
    if (request.startMode === "single-step" && !request.stepType) {
      throw new Error("Step type is required for single-step launches.");
    }

    return this.gateway.startWorkflowRun({
      ...request,
      beginPrompt: request.beginPrompt.trim(),
    });
  }
}
