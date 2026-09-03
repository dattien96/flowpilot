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
      isBuiltin: false,
      editable: true,
      cloneable: false,
      clonedFrom: null,
      packId: null,
      packVersion: null,
      packFlowId: null,
      packHash: null,
      selectableIn: ["chat"],
      chatBaseline: false,
      chatSubModes: [],
      policyCap: null,
      policyOnCap: null,
      policyExtendBy: null,
      policyExtendMax: null,
      edges: [],
    }),
    {
      id: "workflow-1",
      projectId: "",
      name: "Global Workflow",
      description: "Shared",
      model: undefined,
      yoloMode: false,
    },
  );
});

// BUG-230: modelOverride/yoloMode were previously dropped, so the desktop's
// pre-run preview always fell through to the project's default model no
// matter what a workflow's own Settings > Workflows override said.
test("mapNavigatorWorkflow carries modelOverride and yoloMode through to the navigator Workflow", () => {
  assert.deepEqual(
    mapNavigatorWorkflow({
      id: "review-loop",
      projectId: null,
      name: "Review Loop",
      description: "Built-in review loop",
      isTemplate: false,
      providerOverride: null,
      modelOverride: "claude-haiku",
      reasoningEffortOverride: "medium",
      yoloMode: true,
      createdAt: "",
      updatedAt: "",
      isBuiltin: false,
      editable: true,
      cloneable: false,
      clonedFrom: null,
      packId: null,
      packVersion: null,
      packFlowId: null,
      packHash: null,
      selectableIn: ["chat"],
      chatBaseline: false,
      chatSubModes: [],
      policyCap: null,
      policyOnCap: null,
      policyExtendBy: null,
      policyExtendMax: null,
      edges: [],
    }),
    {
      id: "review-loop",
      projectId: "",
      name: "Review Loop",
      description: "Built-in review loop",
      model: "claude-haiku",
      yoloMode: true,
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
        contextSources: [],
        artifactBindings: [],
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
      model: "gpt-5.4",
      yoloMode: false,
    },
  );
});

// BUG-230: the step tier's own model/yoloMode were previously dropped here too.
test("mapNavigatorStep carries model and yoloMode through to the navigator Step", () => {
  assert.deepEqual(
    mapNavigatorStep(
      {
        stepType: "reviewer",
        name: "Reviewer",
        description: "",
        promptBase: null,
        requiredMcps: [],
        mcpAccessMode: "read_only",
        requiredSkills: [],
        teamRole: null,
        subagent: null,
        model: "claude-sonnet",
        reasoningEffort: "high",
        yoloMode: true,
        agentType: "standard",
        contextSources: [],
        artifactBindings: [],
        createdAt: "",
        updatedAt: "",
      },
      1,
    ),
    {
      id: "reviewer",
      name: "Reviewer",
      order: 1,
      defaultSkill: undefined,
      model: "claude-sonnet",
      yoloMode: true,
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
