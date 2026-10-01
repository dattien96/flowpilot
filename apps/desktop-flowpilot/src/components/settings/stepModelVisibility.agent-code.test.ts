import test from "node:test";
import assert from "node:assert/strict";
import { stepDefinitionListSubtitle, stepDefinitionRequiresModel } from "./stepModelVisibility";

test("stepDefinitionRequiresModel: shows the model field for agent.code writers", () => {
  assert.equal(stepDefinitionRequiresModel("agent.code"), true);
});

// CA-1077: agent.scaffold steps (vibe-sprint tdd) spawn delegate children and
// resolveFlowNodeModel now honors their step row — the Model field must show.
test("stepDefinitionRequiresModel: shows the model field for agent.scaffold steps", () => {
  assert.equal(stepDefinitionRequiresModel("agent.scaffold"), true);
});

test("stepDefinitionListSubtitle: keeps the model for an agent.scaffold node", () => {
  assert.equal(
    stepDefinitionListSubtitle({
      stepType: "flowpilot_core_flow_pack_vibe_sprint_tdd",
      behaviorId: "agent.scaffold",
      model: "grok-4.6",
    }),
    "flowpilot_core_flow_pack_vibe_sprint_tdd / grok-4.6",
  );
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
