import test from "node:test";
import assert from "node:assert/strict";
import { stepDefinitionListSubtitle, stepDefinitionRequiresModel } from "./stepModelVisibility";

// BUG-290: the Settings > Step Definitions list hard-coded `{step.stepType} / {step.model}`,
// so a context.produce step (a non-agent inline behavior — BehaviorScopeInline,
// flow_executor.go — whose Model field is hidden in the edit form and never consumed
// at runtime) still showed its stale/inherited model in the sidebar list. The list must
// use the same requires-model semantics as the edit form (stepDefinitionRequiresModel).
test("stepDefinitionListSubtitle: hides a stale model for context.produce", () => {
  assert.equal(stepDefinitionRequiresModel("context.produce"), false);
  assert.equal(
    stepDefinitionListSubtitle({ stepType: "grok-context", behaviorId: "context.produce", model: "grok-context / grok-4.5" }),
    "grok-context",
  );
});

test("stepDefinitionRequiresModel: hides the model for other non-agent inline behaviors", () => {
  assert.equal(stepDefinitionRequiresModel("context.render"), false);
  assert.equal(stepDefinitionRequiresModel("command.validate"), false);
  assert.equal(stepDefinitionRequiresModel("hub.notify"), false);
});

test("stepDefinitionListSubtitle: keeps the model for an agent.delegate node", () => {
  assert.equal(stepDefinitionRequiresModel("agent.delegate"), true);
  assert.equal(
    stepDefinitionListSubtitle({ stepType: "delegate-coder", behaviorId: "agent.delegate", model: "claude-sonnet-4" }),
    "delegate-coder / claude-sonnet-4",
  );
});

test("stepDefinitionListSubtitle: keeps the model for a legacy step with no behaviorId", () => {
  assert.equal(stepDefinitionRequiresModel(undefined), true);
  assert.equal(
    stepDefinitionListSubtitle({ stepType: "legacy-coder", model: "gpt-5" }),
    "legacy-coder / gpt-5",
  );
});
