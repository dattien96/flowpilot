import test from "node:test";
import assert from "node:assert/strict";

import {
  filterNavigatorWorkflows,
  mapNavigatorStep,
  mapNavigatorWorkflow,
} from "../../apps/desktop-flowpilot/src/app/navigatorCatalog";

test("mapNavigatorWorkflow keeps workflow ids and normalizes global workflows", () => {
  assert.deepEqual(
    mapNavigatorWorkflow({
      id: "workflow-1",
      projectId: null,
      name: "Global Workflow",
      description: "Shared",
      isTemplate: false,
      providerOverride: null,
      modelOverride: null,
      reasoningEffortOverride: null,
      yoloMode: false,
      createdAt: "",
      updatedAt: "",
    }),
    {
      id: "workflow-1",
      projectId: "",
      name: "Global Workflow",
      description: "Shared",
    },
  );
});

test("mapNavigatorStep uses step type as the launch id and first required skill as hint", () => {
  assert.deepEqual(
    mapNavigatorStep(
      {
        stepType: "tech_spec",
        name: "Tech Spec",
        description: "",
        promptBase: null,
        requiredMcps: [],
        mcpAccessMode: "read_only",
        requiredSkills: ["tech_spec_skill", "extra_skill"],
        teamRole: null,
        subagent: null,
        model: "gpt-5.4",
        reasoningEffort: "medium",
        yoloMode: false,
        agentType: "standard",
        inputArtifactDefinitions: [],
        outputArtifactDefinitions: [],
        createdAt: "",
        updatedAt: "",
      },
      3,
    ),
    {
      id: "tech_spec",
      name: "Tech Spec",
      order: 3,
      defaultSkill: "tech_spec_skill",
    },
  );
});

test("filterNavigatorWorkflows keeps global workflows alongside project-scoped ones", () => {
  const workflows = [
    { id: "global", projectId: "", name: "Global", description: "" },
    { id: "project-a", projectId: "project-a", name: "Project A", description: "" },
    { id: "project-b", projectId: "project-b", name: "Project B", description: "" },
  ];

  assert.deepEqual(
    filterNavigatorWorkflows(workflows, "project-a").map((workflow) => workflow.id),
    ["global", "project-a"],
  );
  assert.deepEqual(
    filterNavigatorWorkflows(workflows, undefined).map((workflow) => workflow.id),
    ["global", "project-a", "project-b"],
  );
});
