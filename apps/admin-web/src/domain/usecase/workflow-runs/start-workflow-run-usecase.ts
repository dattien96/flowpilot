import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type {
  WorkflowExecutorGateway,
  WorkflowGateway,
} from "@/domain/gateway/workflow-gateway";
import type { StartWorkflowRunPayload } from "@/domain/model/payload/workflow-payload";

export class StartWorkflowRunUseCase {
  constructor(
    private readonly featureGateway: FeatureGateway,
    private readonly workflowGateway: WorkflowGateway,
    private readonly workflowExecutor: WorkflowExecutorGateway,
  ) {}

  async execute(payload: StartWorkflowRunPayload) {
    const [feature, definition] = await Promise.all([
      this.featureGateway.getFeatureById(payload.featureId),
      this.workflowGateway.getWorkflowDefinitionById(payload.workflowDefinitionId),
    ]);

    if (!feature || !definition) {
      throw new Error("Feature or workflow definition not found.");
    }

    const run = await this.workflowGateway.createWorkflowRun(payload, definition);

    await this.workflowExecutor.executeUntilPause(run.id);

    return this.workflowGateway.getWorkflowRunDetail(run.id);
  }
}
