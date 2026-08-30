"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const stepModelVisibility_1 = require("./stepModelVisibility");
// BUG-290: the Settings > Step Definitions list hard-coded `{step.stepType} / {step.model}`,
// so a context.produce step (a non-agent inline behavior — BehaviorScopeInline,
// flow_executor.go — whose Model field is hidden in the edit form and never consumed
// at runtime) still showed its stale/inherited model in the sidebar list. The list must
// use the same requires-model semantics as the edit form (stepDefinitionRequiresModel).
(0, node_test_1.default)("stepDefinitionListSubtitle: hides a stale model for context.produce", () => {
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionRequiresModel)("context.produce"), false);
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionListSubtitle)({ stepType: "grok-context", behaviorId: "context.produce", model: "grok-context / grok-4.5" }), "grok-context");
});
(0, node_test_1.default)("stepDefinitionRequiresModel: hides the model for other non-agent inline behaviors", () => {
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionRequiresModel)("context.render"), false);
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionRequiresModel)("command.validate"), false);
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionRequiresModel)("hub.notify"), false);
});
(0, node_test_1.default)("stepDefinitionListSubtitle: keeps the model for an agent.delegate node", () => {
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionRequiresModel)("agent.delegate"), true);
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionListSubtitle)({ stepType: "delegate-coder", behaviorId: "agent.delegate", model: "claude-sonnet-4" }), "delegate-coder / claude-sonnet-4");
});
(0, node_test_1.default)("stepDefinitionListSubtitle: keeps the model for a legacy step with no behaviorId", () => {
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionRequiresModel)(undefined), true);
    strict_1.default.equal((0, stepModelVisibility_1.stepDefinitionListSubtitle)({ stepType: "legacy-coder", model: "gpt-5" }), "legacy-coder / gpt-5");
});
