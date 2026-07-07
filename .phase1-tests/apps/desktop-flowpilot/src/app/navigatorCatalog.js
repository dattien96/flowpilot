"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.mapNavigatorWorkflow = mapNavigatorWorkflow;
exports.mapNavigatorStep = mapNavigatorStep;
exports.filterNavigatorWorkflows = filterNavigatorWorkflows;
function mapNavigatorWorkflow(workflow) {
    return {
        id: workflow.id,
        projectId: workflow.projectId ?? "",
        name: workflow.name,
        description: workflow.description,
        // BUG-230: model/yoloMode were dropped here, so the desktop's pre-run
        // preview (AgentsPanel's resolvedModel, FlowTimelineSidebar) always fell
        // through to the project's default model — never a workflow's own
        // model_override, no matter what Settings > Workflows displayed.
        model: workflow.modelOverride ?? undefined,
        yoloMode: workflow.yoloMode,
    };
}
function mapNavigatorStep(definition, order) {
    return {
        id: definition.stepType,
        name: definition.name,
        order,
        defaultSkill: definition.requiredSkills[0],
        // BUG-230: same gap as mapNavigatorWorkflow, for the Step tier.
        model: definition.model || undefined,
        yoloMode: definition.yoloMode,
    };
}
function filterNavigatorWorkflows(workflows, projectId) {
    if (!projectId) {
        return workflows;
    }
    return workflows.filter((workflow) => workflow.projectId === "" || workflow.projectId === projectId);
}
