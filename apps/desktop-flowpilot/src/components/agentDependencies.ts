import type { AgentRunSummary } from "@/types/contract";

export function formatDependencyLabels(dependsOn: string[] | undefined, agentRuns: AgentRunSummary[]): string[] {
  if (!dependsOn || dependsOn.length === 0) return [];
  const namesByRunId = new Map(agentRuns.map((run) => [run.runId, run.agentName]));
  return dependsOn.map((runId) => namesByRunId.get(runId) ?? runId);
}
