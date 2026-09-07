import test from "node:test";
import assert from "node:assert/strict";
import { stepDefinitionListSubtitle, stepDefinitionRequiresModel } from "./stepModelVisibility";

test("stepDefinitionRequiresModel: shows the model field for agent.code writers", () => {
  assert.equal(stepDefinitionRequiresModel("agent.code"), true);
});

test("stepDefinitionListSubtitle: keeps the model for an agent.code node", () => {
  assert.equal(
    stepDefinitionListSubtitle({
      stepType: "task-harness__implement",
      behaviorId: "agent.code",
      model: "gpt-5.4-mini",
    }),
    "task-harness__implement / gpt-5.4-mini",
  );
});

test("stepDefinitionListSubtitle: empty agent.code model is inherit (no / null)", () => {
  assert.equal(
    stepDefinitionListSubtitle({
      stepType: "task-harness__implement",
      behaviorId: "agent.code",
      model: null,
    }),
    "task-harness__implement",
  );
});
