"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.isProjectSyncing = isProjectSyncing;
function isProjectSyncing(history, projectId) {
    return history.some((item) => item.projectId === projectId && item.syncStatus === "syncing");
}
