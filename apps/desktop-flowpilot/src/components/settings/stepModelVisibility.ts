// BUG-290: extracted dependency-light so it can be unit tested without
// pulling in @flowpilot/client-core (WorkflowsSettings.tsx's other imports
// require a workspace alias a standalone tsx test run can't resolve).
//
// A step's Model/Reasoning effort only matter for a real provider turn: an
// agent.delegate or agent.code node (both BehaviorScopeDelegate — spawn a
// child) or a step with NO behaviorId at all (a plain, pre-flow catalog
// step — still a normal agent turn). Every other (inline/control) behavior
// is Go-deterministic and never consumes them — resolveFlowNodeModel
// (flow_executor.go) returns "" for inline nodes — so the authoring UI must
// not force a choice here, and the fields (and the list subtitle below)
// hide it entirely for those behaviors.
//
// Mirrors FLOW_BEHAVIOR_OPTIONS (packages/flowpilot-client-core/src/domain/adminModels.ts)
// — only the ids that require an agent matter here; kept in sync manually.
// Empty model is inherit (run/flow baseline), not a missing required field.
const BEHAVIOR_IDS_REQUIRING_AGENT: Record<string, true> = {
  "agent.delegate": true,
  "agent.code": true,
};

export function stepDefinitionRequiresModel(behaviorId: string | null | undefined): boolean {
  if (!behaviorId) return true;
  return BEHAVIOR_IDS_REQUIRING_AGENT[behaviorId] === true;
}

export function stepDefinitionListSubtitle(step: { stepType: string; model: string | null; behaviorId?: string | null }): string {
  if (!stepDefinitionRequiresModel(step.behaviorId)) return step.stepType;
  return step.model ? `${step.stepType} / ${step.model}` : step.stepType;
}
