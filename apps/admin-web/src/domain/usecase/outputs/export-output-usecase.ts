import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ExportOutputUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  async execute(outputId: string) {
    const detail = await this.workflowGateway.getOutputDetail(outputId);

    if (!detail) {
      throw new Error("Output not found.");
    }

    return {
      filename: `${detail.output.outputType}-${detail.output.version}.md`,
      contentMarkdown: detail.output.contentMarkdown,
    };
  }
}
