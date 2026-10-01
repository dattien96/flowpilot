// CA-1081: mirrored builtin step_definitions carry behavior ids the runner
// dispatches but the authoring picker never offers (contract.freeze,
// agent.scaffold on vibe-sprint; tournament.*; agent.reproduce). The
// validation previously checked only FLOW_BEHAVIOR_OPTIONS (the SELECTABLE
// subset) and falsely flagged builtin-mirrored steps as undispatchable.
import test from "node:test";
import assert from "node:assert/strict";
import { validateFlowGraph } from "@flowpilot/client-core";
import type { StepDefinition, WorkflowStep } from "@flowpilot/client-core";

function step(stepType: string): WorkflowStep {
  return {
    id: stepType,
    workflowId: "wf",
    stepType,
    orderIndex: 0,
    isEnabled: true,
    requiresApproval: false,
    createdAt: "",
    updatedAt: "",
  } as WorkflowStep;
}

function def(stepType: string, behaviorId: string, agentRef: string | null): StepDefinition {
  return {
    stepType,
    name: stepType,
    description: "",
    promptBase: null,
    requiredMcps: [],
    mcpAccessMode: "read_only",
    requiredSkills: [],
    teamRole: null,
    subagent: null,
    model: null,
    reasoningEffort: null,
    yoloMode: false,
    agentType: "standard",
    nodeId: stepType,
    behaviorId,
    agentRef,
    contextSources: [],
    artifactBindings: [],
    createdAt: "",
    updatedAt: "",
  } as StepDefinition;
}

test("builtin-only behavior ids (contract.freeze, agent.scaffold) are not flagged unrecognized", () => {
  const issues = validateFlowGraph(
    [step("a"), step("b")],
    [
      def("a", "contract.freeze", null),
      def("b", "agent.scaffold", "agents/scaffold-architect.md"),
    ],
    [],
  );
  const unrecognized = issues.filter((issue) => issue.includes("unrecognized Behavior ID"));
  assert.deepEqual(unrecognized, []);
});

test("builtin provider behaviors still require an agent ref", () => {
  const issues = validateFlowGraph(
    [step("a")],
    [def("a", "agent.scaffold", null)],
    [],
  );
  assert.ok(
    issues.some((issue) => issue.includes('behavior "agent.scaffold" but has no Agent ref')),
    `expected missing-agent issue, got: ${issues.join(" | ")}`,
  );
});

test("genuinely unknown behavior ids still fail", () => {
  const issues = validateFlowGraph(
    [step("a")],
    [def("a", "foo.bar", null)],
    [],
  );
  assert.ok(
    issues.some((issue) => issue.includes('unrecognized Behavior ID "foo.bar"')),
    `expected unrecognized-behavior issue, got: ${issues.join(" | ")}`,
  );
});
