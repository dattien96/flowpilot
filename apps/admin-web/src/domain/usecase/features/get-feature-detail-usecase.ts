import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class GetFeatureDetailUseCase {
  constructor(
    private readonly featureGateway: FeatureGateway,
    private readonly contextGateway: ContextSourceGateway,
    private readonly workflowGateway: WorkflowGateway,
  ) {}

  async execute(featureId: string) {
    const [feature, contexts, runs, definitions] = await Promise.all([
      this.featureGateway.getFeatureById(featureId),
      this.contextGateway.listContextSourcesByFeature(featureId),
      this.workflowGateway.listWorkflowRuns(),
      this.workflowGateway.listWorkflowDefinitions(),
    ]);

    if (!feature) {
      return null;
    }

    return {
      feature,
      contexts,
      runs: runs.filter((run) => run.featureId === featureId),
      definitions,
    };
  }
}
