import type {
  StepDefinition as AdminStepDefinition,
  Workflow as AdminWorkflow,
} from "@flowpilot/client-core";
import type { Step, Workflow } from "../types/contract";

export function mapNavigatorWorkflow(workflow: AdminWorkflow): Workflow {
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

export function mapNavigatorStep(
  definition: AdminStepDefinition,
  order: number,
): Step {
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

export function filterNavigatorWorkflows(
  workflows: Workflow[],
  projectId?: string,
): Workflow[] {
  if (!projectId) {
    return workflows;
  }
  return workflows.filter(
    (workflow) => workflow.projectId === "" || workflow.projectId === projectId,
  );
}
