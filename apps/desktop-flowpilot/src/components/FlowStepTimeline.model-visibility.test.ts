import test from "node:test";
import assert from "node:assert/strict";
import { flowStepShowsModel } from "./FlowStepTimeline";

// BUG-290: a Context Produce step (behaviorId "context.produce") is a non-agent
// inline behavior (BehaviorScopeInline, flow_executor.go) — it never spawns a
// provider turn, so the flow step timeline must not render the run's inherited
// model on it as if it were the step's own configuration.
test("flowStepShowsModel: hides the model for context.produce (a non-agent inline behavior)", () => {
  assert.equal(flowStepShowsModel("context.produce"), false);
});

test("flowStepShowsModel: hides the model for other non-agent inline behaviors", () => {
  assert.equal(flowStepShowsModel("context.render"), false);
  assert.equal(flowStepShowsModel("command.validate"), false);
  assert.equal(flowStepShowsModel("hub.notify"), false);
});

test("flowStepShowsModel: keeps showing the model for an agent.delegate node", () => {
  assert.equal(flowStepShowsModel("agent.delegate"), true);
});

test("flowStepShowsModel: keeps showing the model for a legacy step with no behaviorId (BUG-155)", () => {
  assert.equal(flowStepShowsModel(undefined), true);
});
