import test from "node:test";
import assert from "node:assert/strict";
import { flowStepShowsModel } from "./FlowStepTimeline";

test("flowStepShowsModel: shows the model for an agent.code writer", () => {
  assert.equal(flowStepShowsModel("agent.code"), true);
});

test("flowStepShowsModel: still hides hub.inline", () => {
  assert.equal(flowStepShowsModel("hub.inline"), false);
});
