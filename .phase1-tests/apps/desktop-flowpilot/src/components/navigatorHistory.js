"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.isProjectSyncing = isProjectSyncing;
exports.isAgentHistoryItem = isAgentHistoryItem;
exports.filterVisibleHistory = filterVisibleHistory;
function isProjectSyncing(history, projectId) {
    return history.some((item) => item.projectId === projectId && item.syncStatus === "syncing");
}
function isAgentHistoryItem(item) {
    return Boolean(item.parentRunId || hasBuiltInAgentPromptPrefix(item.lastPrompt));
}
function filterVisibleHistory(history) {
    return history.filter((item) => !isAgentHistoryItem(item));
}
function hasBuiltInAgentPromptPrefix(prompt) {
    const normalized = prompt?.trim().toLowerCase() ?? "";
    return (normalized.startsWith("you are the coder sub-agent.") ||
        normalized.startsWith("you are the reviewer sub-agent.") ||
        normalized.startsWith("you are the tester sub-agent."));
}
