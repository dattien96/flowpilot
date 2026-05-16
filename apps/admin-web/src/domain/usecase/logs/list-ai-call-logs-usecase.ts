import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ListAiCallLogsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute(filters?: Parameters<WorkflowGateway["listLogs"]>[0]) {
    return this.workflowGateway.listLogs(filters);
  }

  summarize(filters?: Parameters<WorkflowGateway["getLogSummary"]>[0]) {
    return this.workflowGateway.getLogSummary(filters);
  }
}
