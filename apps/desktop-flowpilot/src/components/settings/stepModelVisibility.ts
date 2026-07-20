// BUG-290: extracted dependency-light so it can be unit tested without
// pulling in @flowpilot/client-core (WorkflowsSettings.tsx's other imports
// require a workspace alias a standalone tsx test run can't resolve).
//
// A step's Model/Reasoning effort only matter for a real provider turn: an
// agent.delegate node (spawns a child) or a step with NO behaviorId at all
// (a plain, pre-flow catalog step — still a normal agent turn). Every other
// (inline/control) behavior is Go-deterministic and never consumes them —
// resolveFlowNodeModel (flow_executor.go) returns "" for any non-delegate
// node already — so the authoring UI must not force a choice here, and the
// fields (and the list subtitle below) hide it entirely for those behaviors.
//
// Mirrors FLOW_BEHAVIOR_OPTIONS (packages/flowpilot-client-core/src/domain/adminModels.ts)
// — only the ids that require an agent matter here; kept in sync manually.
const BEHAVIOR_IDS_REQUIRING_AGENT = new Set<string>(["agent.delegate"]);

export function stepDefinitionRequiresModel(behaviorId: string | null | undefined): boolean {
  if (!behaviorId) return true;
  return BEHAVIOR_IDS_REQUIRING_AGENT.has(behaviorId);
}

export function stepDefinitionListSubtitle(step: { stepType: string; model: string | null; behaviorId?: string | null }): string {
  return stepDefinitionRequiresModel(step.behaviorId) ? `${step.stepType} / ${step.model}` : step.stepType;
}
