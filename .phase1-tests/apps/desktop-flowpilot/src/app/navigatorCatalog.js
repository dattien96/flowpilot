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
    };
}
function mapNavigatorStep(definition, order) {
    return {
        id: definition.stepType,
        name: definition.name,
        order,
        defaultSkill: definition.requiredSkills[0],
    };
}
function filterNavigatorWorkflows(workflows, projectId) {
    if (!projectId) {
        return workflows;
    }
    return workflows.filter((workflow) => workflow.projectId === "" || workflow.projectId === projectId);
}
