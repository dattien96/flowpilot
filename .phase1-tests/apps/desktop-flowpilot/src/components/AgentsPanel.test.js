"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const agentDependencies_1 = require("./agentDependencies");
const AgentsPanel_1 = require("./AgentsPanel");
const runs = [
    {
        runId: "run-reviewer",
        agentName: "reviewer",
        role: "reviewer",
        status: "completed",
        createdAt: "2026-06-20T00:00:00Z",
    },
    {
        runId: "run-tester",
        agentName: "tester",
        role: "tester",
        status: "starting",
        createdAt: "2026-06-20T00:00:01Z",
    },
];
(0, node_test_1.default)("formatDependencyLabels resolves dependency run ids to agent names", () => {
    strict_1.default.deepEqual((0, agentDependencies_1.formatDependencyLabels)(["run-reviewer", "missing-run"], runs), ["reviewer", "missing-run"]);
});
(0, node_test_1.default)("agentRunDisplayName prefers flow node label over generic agent name", () => {
    strict_1.default.equal((0, AgentsPanel_1.agentRunDisplayName)({ agentName: "reviewer-agent", label: "review-security-gpt" }), "review-security-gpt");
    strict_1.default.equal((0, AgentsPanel_1.agentRunDisplayName)({ agentName: "reviewer-agent" }), "reviewer-agent");
});
// BUG-227: a started run's actual resolved posture (workflowStepRuntimeMeta,
// e.g. Claude Haiku from the workflow's model_override) must win on the main
// card even when the pre-run catalog preview (selectedWorkflow?.model ||
// project?.model) resolves to a different provider/model, such as a
// project-level Codex default masking the run's real Claude posture.
(0, node_test_1.default)("resolveMainAgentDisplay: runtime meta wins over the pre-run catalog preview", () => {
    const { mainProvider, mainModel } = (0, AgentsPanel_1.resolveMainAgentDisplay)({
        resolvedProvider: "codex",
        resolvedModel: "gpt-5.4-mini",
        runtimeMetaProvider: "claude",
        runtimeMetaModel: "claude-haiku",
        selectedProvider: "codex",
        selectedModel: "gpt-5.4-mini",
    });
    strict_1.default.equal(mainProvider, "claude");
    strict_1.default.equal(mainModel, "claude-haiku");
});
(0, node_test_1.default)("resolveMainAgentDisplay: falls back to the pre-run catalog preview before any run has started", () => {
    const { mainProvider, mainModel } = (0, AgentsPanel_1.resolveMainAgentDisplay)({
        resolvedProvider: "claude",
        resolvedModel: "claude-haiku",
        runtimeMetaProvider: undefined,
        runtimeMetaModel: undefined,
        selectedProvider: "codex",
        selectedModel: "gpt-5.4-mini",
    });
    strict_1.default.equal(mainProvider, "claude");
    strict_1.default.equal(mainModel, "claude-haiku");
});
(0, node_test_1.default)("resolveMainAgentDisplay: falls back to the last chat selection, then codex, when nothing else resolves", () => {
    const withSelection = (0, AgentsPanel_1.resolveMainAgentDisplay)({
        resolvedProvider: "",
        resolvedModel: "",
        runtimeMetaProvider: undefined,
        runtimeMetaModel: undefined,
        selectedProvider: "codex",
        selectedModel: "gpt-5.4-mini",
    });
    strict_1.default.equal(withSelection.mainProvider, "codex");
    strict_1.default.equal(withSelection.mainModel, "gpt-5.4-mini");
    const withNothing = (0, AgentsPanel_1.resolveMainAgentDisplay)({
        resolvedProvider: "",
        resolvedModel: "",
        runtimeMetaProvider: undefined,
        runtimeMetaModel: undefined,
        selectedProvider: "",
        selectedModel: "",
    });
    strict_1.default.equal(withNothing.mainProvider, "codex");
    strict_1.default.equal(withNothing.mainModel, "");
});
