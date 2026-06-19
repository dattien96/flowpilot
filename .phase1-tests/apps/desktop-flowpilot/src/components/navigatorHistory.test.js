"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const navigatorHistory_1 = require("./navigatorHistory");
function makeItem(overrides = {}) {
    return {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-19T10:00:00Z",
        updatedAt: "2026-06-19T10:01:00Z",
        ...overrides,
    };
}
(0, node_test_1.default)("isProjectSyncing returns true when any row in the project is syncing", () => {
    const history = [
        makeItem({ runId: "run-a", syncStatus: "syncing" }),
        makeItem({ runId: "run-b", projectId: "project-2" }),
    ];
    strict_1.default.equal((0, navigatorHistory_1.isProjectSyncing)(history, "project-1"), true);
    strict_1.default.equal((0, navigatorHistory_1.isProjectSyncing)(history, "project-2"), false);
});
(0, node_test_1.default)("isProjectSyncing ignores rows from other projects and non-syncing states", () => {
    const history = [
        makeItem({ runId: "run-a", syncStatus: "failed" }),
        makeItem({ runId: "run-b", projectId: "project-2", syncStatus: "syncing" }),
    ];
    strict_1.default.equal((0, navigatorHistory_1.isProjectSyncing)(history, "project-1"), false);
});
