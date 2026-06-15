"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const navigatorCatalog_1 = require("../../apps/desktop-flowpilot/src/app/navigatorCatalog");
(0, node_test_1.default)("mapNavigatorWorkflow keeps workflow ids and normalizes global workflows", () => {
    strict_1.default.deepEqual((0, navigatorCatalog_1.mapNavigatorWorkflow)({
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
    }), {
        id: "workflow-1",
        projectId: "",
        name: "Global Workflow",
        description: "Shared",
    });
});
(0, node_test_1.default)("mapNavigatorStep uses step type as the launch id and first required skill as hint", () => {
    strict_1.default.deepEqual((0, navigatorCatalog_1.mapNavigatorStep)({
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
    }, 3), {
        id: "tech_spec",
        name: "Tech Spec",
        order: 3,
        defaultSkill: "tech_spec_skill",
    });
});
(0, node_test_1.default)("filterNavigatorWorkflows keeps global workflows alongside project-scoped ones", () => {
    const workflows = [
        { id: "global", projectId: "", name: "Global", description: "" },
        { id: "project-a", projectId: "project-a", name: "Project A", description: "" },
        { id: "project-b", projectId: "project-b", name: "Project B", description: "" },
    ];
    strict_1.default.deepEqual((0, navigatorCatalog_1.filterNavigatorWorkflows)(workflows, "project-a").map((workflow) => workflow.id), ["global", "project-a"]);
    strict_1.default.deepEqual((0, navigatorCatalog_1.filterNavigatorWorkflows)(workflows, undefined).map((workflow) => workflow.id), ["global", "project-a", "project-b"]);
});
