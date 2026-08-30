"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const FlowStepTimeline_1 = require("./FlowStepTimeline");
// BUG-290: a Context Produce step (behaviorId "context.produce") is a non-agent
// inline behavior (BehaviorScopeInline, flow_executor.go) — it never spawns a
// provider turn, so the flow step timeline must not render the run's inherited
// model on it as if it were the step's own configuration.
(0, node_test_1.default)("flowStepShowsModel: hides the model for context.produce (a non-agent inline behavior)", () => {
    strict_1.default.equal((0, FlowStepTimeline_1.flowStepShowsModel)("context.produce"), false);
});
(0, node_test_1.default)("flowStepShowsModel: hides the model for other non-agent inline behaviors", () => {
    strict_1.default.equal((0, FlowStepTimeline_1.flowStepShowsModel)("context.render"), false);
    strict_1.default.equal((0, FlowStepTimeline_1.flowStepShowsModel)("command.validate"), false);
    strict_1.default.equal((0, FlowStepTimeline_1.flowStepShowsModel)("hub.notify"), false);
});
(0, node_test_1.default)("flowStepShowsModel: keeps showing the model for an agent.delegate node", () => {
    strict_1.default.equal((0, FlowStepTimeline_1.flowStepShowsModel)("agent.delegate"), true);
});
(0, node_test_1.default)("flowStepShowsModel: keeps showing the model for a legacy step with no behaviorId (BUG-155)", () => {
    strict_1.default.equal((0, FlowStepTimeline_1.flowStepShowsModel)(undefined), true);
});
