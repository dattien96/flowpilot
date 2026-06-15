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
