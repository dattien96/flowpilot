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
    const feature = await this.featureGateway.getFeatureById(featureId);

    if (!feature) {
      return null;
    }

    const [featureContexts, projectContexts, runs, definitions] = await Promise.all([
      this.contextGateway.listContextSourcesByFeature(featureId),
      this.contextGateway.listContextSourcesByProject(feature.projectId),
      this.workflowGateway.listWorkflowRuns(),
      this.workflowGateway.listWorkflowDefinitions(),
    ]);
    const contexts = Array.from(
      new Map(
        [...projectContexts.filter((context) => !context.featureId), ...featureContexts].map(
          (context) => [context.id, context],
        ),
      ).values(),
    );

    return {
      feature,
      contexts,
      runs: runs.filter((run) => run.featureId === featureId),
      definitions,
    };
  }
}
