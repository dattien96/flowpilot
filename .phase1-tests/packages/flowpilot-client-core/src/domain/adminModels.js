"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.FLOW_EDGE_TERMINALS = exports.FLOW_BEHAVIOR_OPTIONS = void 0;
exports.validateFlowGraph = validateFlowGraph;
exports.FLOW_BEHAVIOR_OPTIONS = [
    { id: "agent.delegate", label: "Agent delegate — spawn an agent", requiresAgent: true },
    { id: "hub.inline", label: "Hub inline — synthesis / orchestration turn", requiresAgent: false },
    // Task-238: renamed for clarity after a user picked telegram.notify expecting
    // an AI-composed message — the two labels must read as opposites at a glance,
    // not as near-synonyms differing only in "(no agent)" vs "(no child agent)".
    { id: "telegram.notify", label: "Telegram notify — STATIC message, no AI (fixed text / template sent verbatim)", requiresAgent: false },
    { id: "hub.notify", label: "Telegram notify — AI-COMPOSED message (main agent's own turn, no child agent)", requiresAgent: false },
    { id: "context.produce", label: "Context produce — build a context package", requiresAgent: false },
    { id: "context.render", label: "Context render — render a context package into a prompt", requiresAgent: false },
    { id: "command.validate", label: "Command validate — run a validation command", requiresAgent: false },
    { id: "validation.summarize", label: "Validation summarize — reduce validation output", requiresAgent: false },
    { id: "artifact.audit_draft", label: "Artifact audit draft — prepare an audit/commit draft", requiresAgent: false },
    { id: "flow.control", label: "Flow control — map a tool outcome to flow control", requiresAgent: false },
    { id: "user.confirm", label: "User confirm — gate on explicit user confirmation", requiresAgent: false },
];
/** Edge terminal pseudo-nodes an edge may point at besides a declared node. */
exports.FLOW_EDGE_TERMINALS = ["done", "ask_user"];
/**
 * Task-189 slice 3: validate a workflow's flow graph before save.
 *
 * Node identity/behavior/agent live ONLY on `StepDefinition` (BUG-236 — a
 * workflow's `WorkflowStep` rows are a pure relation, never node data), and
 * the runner's FlowNode.ID is set from `StepDefinition.nodeId` with NO
 * fallback when it's unset (recordFromWorkflowRow in
 * supabase_workflow_flow_store.go) — so most of these checks are really
 * "will `resolveWorkflowFlowRef` find a runnable graph here," not generic
 * form validation.
 *
 * `stepDefinitions` is the full global catalog; `steps` is this workflow's
 * own step list (each referencing a `stepType` in that catalog).
 *
 * Returns a list of human-readable issue messages; an empty array means the
 * graph is safe to save and (as far as this checks) will resolve to a
 * runnable flow. This does not — and cannot — guarantee the flow completes;
 * it only guards against structurally broken graphs that resolveWorkflowFlowRef
 * would silently bail on (falling back to the legacy non-flow path) or that
 * would leave a node permanently unreachable.
 */
function validateFlowGraph(steps, stepDefinitions, edges) {
    const issues = [];
    const defsByType = new Map(stepDefinitions.map((definition) => [definition.stepType, definition]));
    const enabledSteps = steps.filter((step) => step.isEnabled);
    if (enabledSteps.length === 0) {
        issues.push("The flow has no enabled steps.");
        return issues;
    }
    const nodesMissingId = enabledSteps.filter((step) => !defsByType.get(step.stepType)?.nodeId);
    if (nodesMissingId.length > 0) {
        issues.push(`${nodesMissingId.length} step(s) have no Node ID set (${nodesMissingId
            .map((step) => step.stepType)
            .join(", ")}) — every step needs a Node ID before it can be part of a flow graph.`);
    }
    const knownBehaviorIds = new Set(exports.FLOW_BEHAVIOR_OPTIONS.map((option) => option.id));
    const behaviorsRequiringAgent = new Set(exports.FLOW_BEHAVIOR_OPTIONS.filter((option) => option.requiresAgent).map((option) => option.id));
    const nodeIds = new Set();
    for (const step of enabledSteps) {
        const definition = defsByType.get(step.stepType);
        if (!definition) {
            issues.push(`Step "${step.stepType}" has no matching step-definition catalog entry.`);
            continue;
        }
        if (definition.nodeId) {
            nodeIds.add(definition.nodeId);
        }
        if (definition.behaviorId) {
            if (!knownBehaviorIds.has(definition.behaviorId)) {
                issues.push(`Step "${step.stepType}" has an unrecognized Behavior ID "${definition.behaviorId}" — the runner has no dispatch for it, so this node cannot execute.`);
            }
            else if (behaviorsRequiringAgent.has(definition.behaviorId) && !definition.agentRef) {
                issues.push(`Step "${step.stepType}" uses behavior "${definition.behaviorId}" but has no Agent ref set.`);
            }
        }
    }
    // BUG-282 (was a 2026-07-06 owner finding): a step's flow dependency is NOT
    // stored on the step definition — topology lives only on the flow's own
    // `edges` (workflows.edges_json), so a step reused across flows resolves
    // against each flow's edges instead of dragging in another flow's node ids.
    // Entry-node detection therefore reads the edges directly: a node is an
    // entry node iff no *forward* edge targets it (a back edge, e.g. a
    // synthesis->coder loop re-entry, must not disqualify the true entry node).
    const nodesWithIncomingForwardEdge = new Set(edges.filter((edge) => edge.kind === "forward").map((edge) => edge.to));
    const hasEntryNode = enabledSteps.some((step) => {
        const definition = defsByType.get(step.stepType);
        return Boolean(definition?.nodeId) && !nodesWithIncomingForwardEdge.has(definition.nodeId);
    });
    if (!hasEntryNode) {
        issues.push("No entry node found — at least one enabled step's node must have no incoming forward edge, so the flow has somewhere to start.");
    }
    const validTargets = new Set([...nodeIds, ...exports.FLOW_EDGE_TERMINALS]);
    edges.forEach((edge, index) => {
        const label = `Edge #${index + 1}`;
        if (!edge.from) {
            issues.push(`${label}: "From" is required.`);
        }
        else if (!nodeIds.has(edge.from)) {
            issues.push(`${label}: "From" node "${edge.from}" is not one of this flow's node ids.`);
        }
        if (!edge.to) {
            issues.push(`${label}: "To" is required.`);
        }
        else if (!validTargets.has(edge.to)) {
            issues.push(`${label}: "To" node "${edge.to}" is neither one of this flow's node ids nor a terminal (${exports.FLOW_EDGE_TERMINALS.join(", ")}).`);
        }
        if (!edge.when.trim()) {
            issues.push(`${label}: "When" (outcome status) is required.`);
        }
        if (edge.kind !== "forward" && edge.kind !== "back") {
            issues.push(`${label}: "Kind" must be "forward" or "back", got "${edge.kind}".`);
        }
    });
    return issues;
}
