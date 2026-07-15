export type ProjectPlatform =
  | "none"
  | "android"
  | "ios"
  | "kmm"
  | "react-native"
  | "flutter"
  | "reactjs"
  | "vuejs"
  | "angularjs"
  | "golang"
  | "java"
  | "python"
  | "nodejs";
export type ReasoningEffort = "low" | "medium" | "high" | "xhigh";
export type ArtifactStorageProvider = "supabase" | "google_drive";

export interface Project {
  id: string;
  name: string;
  description: string;
  platform: ProjectPlatform;
  repositoryUrl: string;
  directoryPath: string | null;
  status: string;
  artifactStoragePreference: ArtifactStorageProvider;
  defaultProvider: string | null;
  defaultModel: string | null;
  defaultReasoningEffort: ReasoningEffort | null;
  sessionIdleTtlMinutes: number | null;
  xcodeScheme: string | null;
  xcodeDestination: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectWorkspaceBinding {
  id: string;
  projectId: string;
  localPath: string;
  label: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Team {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
}

export type MemberRole = "android" | "ios" | "backend" | "frontend" | "qa" | "devops" | "ai_workflow";
export type LevelLabel = "L1_intern" | "L2_junior" | "L3_middle" | "L4_senior" | "L5_lead";

export interface TeamMember {
  id: string;
  teamId: string;
  name: string;
  email: string | null;
  jiraAccountId: string | null;
  role: MemberRole;
  levelLabel: LevelLabel;
  skillTags: string[];
  weeklyCapacityHours: number;
  createdAt: string;
  updatedAt: string;
}

export type IntegrationType = "jira" | "figma" | "google_drive" | "firebase" | "telegram";
export type IntegrationStatus = "pending" | "awaiting_oauth" | "connected" | "failed";

export interface Integration {
  id: string;
  projectId: string | null;
  type: IntegrationType;
  label: string;
  mcpTypeEnabled: boolean;
  configEncrypted: Record<string, unknown>;
  status: IntegrationStatus;
  lastSyncedAt: string | null;
  lastError: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SupportedModel {
  id: string;
  providerKey: "codex" | "claude" | "gemini" | "grok";
  modelId: string;
  displayName: string;
  isEnabled: boolean;
  sortOrder: number;
  source: string;
  detectionMethod: string | null;
  detectedCliVersion: string | null;
  lastDetectedAt: string | null;
  // Task-215: per-model reasoning-effort support and context-window size,
  // detected from the same provider CLI calls Task-213 already makes
  // (codex debug models / ~/.grok/models_cache.json). Null when the
  // provider has no per-model catalog for this (Claude) or the row was
  // never detected (manually added).
  supportedReasoningEfforts: string[] | null;
  defaultReasoningEffort: string | null;
  contextWindowTokens: number | null;
  maxContextWindowTokens: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface Workflow {
  id: string;
  projectId: string | null;
  name: string;
  description: string;
  isTemplate: boolean;
  providerOverride: string | null;
  modelOverride: string | null;
  reasoningEffortOverride: string | null;
  yoloMode: boolean;
  createdAt: string;
  updatedAt: string;
  /**
   * Flow-engine attributes (CP-42/Task-175/179). A built-in workflow is
   * mirrored from the embedded agentpack (e.g. Review Loop) and is
   * read-only in the Settings UI (isBuiltin=true, editable=false); users
   * clone it into an editable copy via cloneWorkflow, which sets
   * isBuiltin=false, editable=true, and clonedFrom to the source workflow id.
   */
  isBuiltin: boolean;
  editable: boolean;
  cloneable: boolean;
  clonedFrom: string | null;
  /** Pack identity, present only on isBuiltin=true rows, used by mirror-sync
   * staleness checks (packHash changes when the source pack YAML changes). */
  packId: string | null;
  packVersion: string | null;
  packFlowId: string | null;
  packHash: string | null;
  /** Where this flow may be selected from: "chat" and/or "flow". */
  selectableIn: string[];
  /** True for the always-on Chat Mode context baseline (e.g. RAG Harness);
   * never offered as a Chat Mode orchestration picker option. */
  chatBaseline: boolean;
  /** Chat sub-modes (e.g. "bug") this flow's picker option is offered under. */
  chatSubModes: string[];
  /** Bounded-loop cap policy for this flow's cohort/hub reinvocation. */
  policyCap: number | null;
  policyOnCap: string | null;
  policyExtendBy: number | null;
  policyExtendMax: number | null;
  /** The flow graph's edges. Not yet read by the runner's live execution
   * path (which only starts the entry node today) — preserved for the
   * planned FlowEdge-driven generic executor. */
  edges: WorkflowFlowEdge[];
}

/** One edge in a Workflow's flow graph (CP-42/Task-175). */
export interface WorkflowFlowEdge {
  from: string;
  to: string;
  when: string;
  kind: string;
}

/** A selectable node behavior for the flow-authoring UI (Task-189, Q-3).
 * Static shared list mirroring the runner's DefaultBehaviorRegistry
 * (behavior_registry.go). Kept in sync manually; revisit as a runtime
 * endpoint only if the behavior set becomes dynamic. `requiresAgent` drives
 * the "a delegate node must have an agent ref" validation. */
export interface FlowBehaviorOption {
  id: string;
  label: string;
  requiresAgent: boolean;
}

export const FLOW_BEHAVIOR_OPTIONS: FlowBehaviorOption[] = [
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
export const FLOW_EDGE_TERMINALS: string[] = ["done", "ask_user"];

export interface WorkflowStep {
  id: string;
  workflowId: string;
  stepType: string;
  orderIndex: number;
  isEnabled: boolean;
  requiresApproval: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface StepDefinition {
  stepType: string;
  name: string;
  description: string;
  promptBase: string | null;
  requiredMcps: string[];
  mcpAccessMode: "read_only" | "read_write";
  requiredSkills: string[];
  teamRole: string | null;
  subagent: string | null;
  model: string;
  reasoningEffort: string | null;
  yoloMode: boolean;
  agentType: "standard" | "autonomous";
  nodeId?: string | null;
  behaviorId?: string | null;
  agentRef?: string | null;
  nodeLifecycle?: string | null;
  joinMode?: string | null;
  cohort?: string | null;
  promptTemplateRef?: string | null;
  contextRef?: string | null;
  /**
   * Enabled context-source ids for this step's Plan-time context harness
   * (CP-44 / Task-196), mirroring requiredMcps. Empty means "fall back to
   * the flow-level `contexts.sources` binding, then the runner's default
   * built-in set" (Task-194 precedence) — a step that never sets this keeps
   * pre-Task-196 behavior unchanged.
   */
  contextSources: string[];
  /**
   * CP-45/SD-23: typed artifact instances bound to this step's input/output
   * slots — see `ArtifactType`/`ArtifactInstance`.
   */
  artifactBindings: StepArtifactBinding[];
  createdAt: string;
  updatedAt: string;
}

/**
 * CP-45/SD-23 D-1/D-2: the system-owned artifact type contract. Users never
 * create rows here in v1 — only `ArtifactInstance` is user-authored. Code
 * hardcodes this layer; the catalog is seeded via migration
 * (20260709090000_add_artifact_types_catalog.sql) and is read-only to the
 * app (RLS grants `authenticated` select only).
 */
export interface ArtifactType {
  id: string;
  version: number;
  category: string;
  producerBehavior: string;
  consumerHints: Record<string, unknown>;
  configSchema: Record<string, unknown>;
  renderTemplate: string;
  systemOwned: boolean;
  status: string;
  createdAt: string;
  updatedAt: string;
}

/**
 * CP-45/SD-23 D-1/D-3: a configured instance of a built-in `ArtifactType`.
 * `isBuiltin=true` rows are seeded (service role) and read-only to users,
 * shown with a "built-in" tag, mirroring how a built-in `Workflow` is
 * read-only (`editable=false`). `isBuiltin=false` rows are user-created,
 * project-scoped (`projectId` set). `projectId` is null on built-in rows so
 * they are visible from every project, mirroring `Workflow.projectId`.
 */
export interface ArtifactInstance {
  id: string;
  projectId: string | null;
  artifactTypeId: string;
  name: string;
  description: string;
  configJson: Record<string, unknown>;
  isBuiltin: boolean;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export type ArtifactBindingDirection = "input" | "output";

/**
 * CP-45/SD-23 D-1/D-4/D-7: binds one `ArtifactInstance` to a step's input or
 * output slot. A step may carry multiple bindings per direction (SD-23:
 * "a step attaches a list of artifacts for both input and output"). Binding
 * the same instance to step A's output and step B's input models cross-step
 * artifact I/O (SD-23 D-1). Stored in `step_artifact_bindings`, never on
 * `workflow_steps` (BUG-236).
 */
export interface StepArtifactBinding {
  id: string;
  direction: ArtifactBindingDirection;
  slotName: string;
  artifactInstanceId: string;
  required: boolean;
  position: number;
  createdAt: string;
}

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
export function validateFlowGraph(
  steps: WorkflowStep[],
  stepDefinitions: StepDefinition[],
  edges: WorkflowFlowEdge[],
): string[] {
  const issues: string[] = [];
  const defsByType = new Map(stepDefinitions.map((definition) => [definition.stepType, definition]));
  const enabledSteps = steps.filter((step) => step.isEnabled);

  if (enabledSteps.length === 0) {
    issues.push("The flow has no enabled steps.");
    return issues;
  }

  const nodesMissingId = enabledSteps.filter((step) => !defsByType.get(step.stepType)?.nodeId);
  if (nodesMissingId.length > 0) {
    issues.push(
      `${nodesMissingId.length} step(s) have no Node ID set (${nodesMissingId
        .map((step) => step.stepType)
        .join(", ")}) — every step needs a Node ID before it can be part of a flow graph.`,
    );
  }

  const knownBehaviorIds = new Set(FLOW_BEHAVIOR_OPTIONS.map((option) => option.id));
  const behaviorsRequiringAgent = new Set(
    FLOW_BEHAVIOR_OPTIONS.filter((option) => option.requiresAgent).map((option) => option.id),
  );
  const nodeIds = new Set<string>();

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
        issues.push(
          `Step "${step.stepType}" has an unrecognized Behavior ID "${definition.behaviorId}" — the runner has no dispatch for it, so this node cannot execute.`,
        );
      } else if (behaviorsRequiringAgent.has(definition.behaviorId) && !definition.agentRef) {
        issues.push(
          `Step "${step.stepType}" uses behavior "${definition.behaviorId}" but has no Agent ref set.`,
        );
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
  const nodesWithIncomingForwardEdge = new Set(
    edges.filter((edge) => edge.kind === "forward").map((edge) => edge.to),
  );
  const hasEntryNode = enabledSteps.some((step) => {
    const definition = defsByType.get(step.stepType);
    return Boolean(definition?.nodeId) && !nodesWithIncomingForwardEdge.has(definition!.nodeId!);
  });

  if (!hasEntryNode) {
    issues.push(
      "No entry node found — at least one enabled step's node must have no incoming forward edge, so the flow has somewhere to start.",
    );
  }

  const validTargets = new Set([...nodeIds, ...FLOW_EDGE_TERMINALS]);
  edges.forEach((edge, index) => {
    const label = `Edge #${index + 1}`;
    if (!edge.from) {
      issues.push(`${label}: "From" is required.`);
    } else if (!nodeIds.has(edge.from)) {
      issues.push(`${label}: "From" node "${edge.from}" is not one of this flow's node ids.`);
    }
    if (!edge.to) {
      issues.push(`${label}: "To" is required.`);
    } else if (!validTargets.has(edge.to)) {
      issues.push(
        `${label}: "To" node "${edge.to}" is neither one of this flow's node ids nor a terminal (${FLOW_EDGE_TERMINALS.join(", ")}).`,
      );
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

export interface ArtifactRun {
  id: string;
  artifactDefinitionKey: string | null;
  workflowId: string;
  workflowRunId: string;
  workflowRunStepId: string | null;
  projectId: string | null;
  title: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  storageProvider: ArtifactStorageProvider | null;
  remoteObjectId: string | null;
  syncStatus: string;
  createdAt: string;
  updatedAt: string;
}

export interface WorkflowRun {
  id: string;
  workflowId: string;
  projectId: string;
  status: string;
  provider: string | null;
  model: string | null;
  reasoningEffort: string | null;
  yoloMode: boolean;
  startedAt: string;
  finishedAt: string | null;
  errorMessage: string | null;
}

// LocalRunnerProviderModel is one entry in the runner's live-detected model
// list for a provider (Task-213) — mirrors the Go ProviderModel DTO.
export interface LocalRunnerProviderModel {
  id: string;
  displayName: string;
  available: boolean;
  source: string;
  // Task-215: mirrors the Go ProviderModel's new reasoning/context-window
  // fields (see SupportedModel above) — carried on the live-detected entry
  // before it is stamped onto a persisted ai_supported_models row.
  supportedReasoningEfforts?: string[];
  defaultReasoningEffort?: string;
  contextWindowTokens?: number;
  maxContextWindowTokens?: number;
}

export interface LocalRunnerProvider {
  key: string;
  label: string;
  installed: boolean;
  version: string | null;
  authStatus?: string;
  installHint: string | null;
  // Appended last (Task-213): the runner's own detected version/model list for
  // this provider (e.g. `codex debug models`, `agy models`, or Grok's
  // ~/.grok/models_cache.json) — the source Task-213's "Detect models" sync
  // reads from.
  detectedVersion?: string | null;
  detectedBinary?: string | null;
  models?: LocalRunnerProviderModel[];
}

export interface LocalRunnerMcpBackend {
  key: string;
  providerType: string;
  label: string;
  state: string;
  action: "install" | "verify";
  actionLabel: string;
  lastCheckedAt: string | null;
  lastError: string | null;
}

export interface LocalRunnerArtifact {
  artifactId: string;
  title: string;
  sourceKind: string;
  projectId: string;
  workflowRunId: string;
  workflowStepKey: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  syncStatus: string;
  createdAt: string;
  updatedAt: string;
}

export interface LocalRunnerStorageDriver {
  driverKey: string;
  enabled: boolean;
  remoteRootPath: string;
  remoteFolderName: string;
  lastValidatedAt: string | null;
  lastSyncedAt: string | null;
  lastError: string | null;
  updatedAt: string | null;
}
