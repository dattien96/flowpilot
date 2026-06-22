"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.formatDependencyLabels = formatDependencyLabels;
function formatDependencyLabels(dependsOn, agentRuns) {
    if (!dependsOn || dependsOn.length === 0)
        return [];
    const namesByRunId = new Map(agentRuns.map((run) => [run.runId, run.agentName]));
    return dependsOn.map((runId) => namesByRunId.get(runId) ?? runId);
}
