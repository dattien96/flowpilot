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
(0, node_test_1.default)("filterVisibleHistory removes child agent runs from navigator history", () => {
    const history = [
        makeItem({ runId: "main-run" }),
        makeItem({ runId: "child-run", parentRunId: "main-run", agentName: "coder" }),
    ];
    strict_1.default.deepEqual((0, navigatorHistory_1.filterVisibleHistory)(history).map((item) => item.runId), ["main-run"]);
});
(0, node_test_1.default)("filterVisibleHistory removes orphan agent rows with built-in child prompts", () => {
    const history = [
        makeItem({ runId: "main-run" }),
        makeItem({
            runId: "orphan-agent",
            agentName: "reviewer",
            role: "review",
            agentStatus: "completed",
            lastPrompt: "You are the reviewer sub-agent. Review the coder's diff.",
        }),
    ];
    strict_1.default.deepEqual((0, navigatorHistory_1.filterVisibleHistory)(history).map((item) => item.runId), ["main-run"]);
});
(0, node_test_1.default)("filterVisibleHistory keeps main rows with agent-like metadata", () => {
    const history = [
        makeItem({ runId: "main-run", agentName: "main", agentStatus: "completed", lastPrompt: "Main agent prompt" }),
    ];
    strict_1.default.deepEqual((0, navigatorHistory_1.filterVisibleHistory)(history).map((item) => item.runId), ["main-run"]);
});
(0, node_test_1.default)("isSyncableRun accepts a plain chat run that is not yet synced", () => {
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: "chat" })), true);
});
(0, node_test_1.default)("isSyncableRun accepts a flow-engine (workflow) run -- Task-190 / CP-36 P-5", () => {
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: "workflow" })), true);
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: undefined })), true);
});
(0, node_test_1.default)("isSyncableRun rejects an already-synced or unavailable run", () => {
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: "chat", syncStatus: "synced" })), false);
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: "workflow", unavailableReason: "missing" })), false);
});
(0, node_test_1.default)("isSyncableRun rejects a child agent run even though children carry runKind chat", () => {
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: "chat", parentRunId: "run-hub" })), false);
});
(0, node_test_1.default)("isSyncableRun permanently excludes an unsyncable run (BUG-311)", () => {
    // "unsyncable" is a permanent backend fact (no resumable session file will
    // ever exist for this run, e.g. cancelled before the provider wrote one) --
    // unlike "failed", which is expected to be retried on the next sync attempt.
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(makeItem({ runKind: "chat", syncStatus: "unsyncable" })), false);
});
(0, node_test_1.default)("isSyncableRun reconciles a stale local syncStatus against the confirmed remote index", () => {
    // A local syncStatus flag can go stale (e.g. a background history poll wins a
    // race against a just-set "synced" flag and overwrites it with the pre-sync
    // snapshot it fetched). When the remote index already carries this exact
    // sourceMachineId/sourceRunId, treat it as synced regardless of the local flag.
    const item = makeItem({ runKind: "chat", syncStatus: "failed", sourceMachineId: "mch_abc", sourceRunId: "run-1" });
    const remoteChatSessions = [
        { runId: "run-1", projectId: "project-1", providerKey: "codex", sourceMachineId: "mch_abc", sourceRunId: "run-1" },
    ];
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(item), true, "without remote data, the stale local flag still marks it unsynced");
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(item, remoteChatSessions), false, "the confirmed remote record corrects the stale flag");
});
(0, node_test_1.default)("isSyncableRun does not reconcile a run that has never actually synced", () => {
    // Before any sync attempt, sourceMachineId/sourceRunId are unset -- there is
    // nothing to match against the remote index, so a never-synced run must stay
    // syncable even when other unrelated runs already exist remotely.
    const item = makeItem({ runKind: "chat" });
    const remoteChatSessions = [
        { runId: "run-other", projectId: "project-1", providerKey: "codex", sourceMachineId: "mch_abc", sourceRunId: "run-other" },
    ];
    strict_1.default.equal((0, navigatorHistory_1.isSyncableRun)(item, remoteChatSessions), true);
});
