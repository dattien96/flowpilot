import { useEffect, useMemo, useRef, useState } from "react";
import type {
  ArtifactInstance,
  ArtifactType,
  Project,
  StepArtifactBinding,
  StepDefinition,
  SupportedModel,
  Workflow,
  WorkflowFlowEdge,
  WorkflowStep,
} from "@flowpilot/client-core";
import { FLOW_BEHAVIOR_OPTIONS, FLOW_EDGE_TERMINALS, validateFlowGraph } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, integrationTypes, toErrorMessage } from "@/components/settings/settingsHelpers";
import {
  DEFAULT_CODING_MEMO_SECTIONS,
  emptyFileArtifactConfig,
  isFileArtifactStructureEnabled,
  normalizeFileArtifactConfigForSave,
  parseFileArtifactStructure,
  sectionsToTextarea,
  withFileArtifactStructure,
} from "@/components/settings/fileArtifactConfig";
import { createRunnerClient } from "@/client/createRunnerClient";
import type { AgentDefinition } from "@/types/contract";
import { stepDefinitionListSubtitle, stepDefinitionRequiresModel } from "@/components/settings/stepModelVisibility";

export { stepDefinitionRequiresModel };

type Tab = "workflows" | "steps" | "artifacts";

// CP-58 Task-307 T-7: display labels for the Bug/Task/CP harness tier family.
// Purely cosmetic — the /flow picker itself is data-driven via selectableIn.
// rag-harness carries the Bug tier (no byte-identical bug-harness clone was
// added; see CA-712), cp-harness-smoke is the opt-in variant.
const HARNESS_LABELS: Record<string, { label: string; description: string }> = {
  "rag-harness": {
    label: "Bug / Hotfix",
    description: "9-step: TDD + Code Review (fast, no plan overhead)",
  },
  "task-harness": {
    label: "Task / Feature",
    description: "12-node: Plan Writer + Plan Review + freeze + TDD + Code Review",
  },
  "cp-harness": {
    label: "Coding Plan",
    description: "7-node slice-only: CP Plan + Review + Task Splitter",
  },
  "cp-harness-smoke": {
    label: "Coding Plan (smoke)",
    description: "13-node opt-in: CP Plan + Review + Split + first-Task coding",
  },
};

function harnessLabelFor(packFlowId: string | null | undefined): string | null {
  if (!packFlowId) return null;
  return HARNESS_LABELS[packFlowId]?.label ?? null;
}
type ViewMode = "list" | "create";
type DeleteTarget =
  | { kind: "workflow"; ids: string[]; labels: string[] }
  | {
      kind: "step";
      ids: string[];
      labels: string[];
      // BUG-247-follow-up: a workflow's step list is a pure relation to
      // step_definitions (BUG-236) with no FK-cascade backing it, so
      // deleting a step definition out from under a workflow that still
      // lists it leaves that workflow with a dangling stepType it can never
      // resolve at runtime. These are the workflows deleteSelected removes
      // alongside the step(s) themselves.
      cascadeWorkflows: { id: string; name: string }[];
    };
type BulkSelect = { kind: "workflow" | "step"; ids: Set<string> };

type PickerModal =
  | { kind: "mcp"; mode: "detail" | "create" }
  | { kind: "context-source"; mode: "detail" | "create" }
  // CP-45/SD-23 Task-200: pick an ArtifactInstance to bind to this step's
  // input or output slot.
  | { kind: "artifact-binding-input"; mode: "detail" | "create" }
  | { kind: "artifact-binding-output"; mode: "detail" | "create" };

// Task-196 (CP-44 P-7): the selectable context-source ids, limited to what
// the runner's ContextSourceRegistry actually supports (Task-191/192/195) —
// never free text. This list is a manually-synced descriptor (CP-44 Q-1
// option a); the Go registry stays the validation authority: an id here that
// drifts out of sync with the registry fails flow load fast (Task-194 T-2),
// it does not silently run without it. mcp.driver is backed by a production
// Google Drive adapter (Task-204) — it reads from the project-level Google
// Drive account connected in Google Drive setup. This editor only enables the
// source; when no legacy default file id is stored, the run asks for the file
// URL/id at runtime instead of forcing a settings edit for every run.
// Task-229 (CP-05-06 Q-3, resolved 2026-07-13): jira.issue and jira.sprint
// are two independent sources, not one source with a "target mode" — mirrors
// mcp.driver: the run asks for the issue key / sprint at runtime when
// enabled, instead of forcing a settings edit for every run.
const contextSourceOptions: { id: string; label: string }[] = [
  { id: "canonical.head", label: "Canonical Head" },
  { id: "feature.history", label: "Feature History" },
  { id: "change.contract", label: "Change Contract" },
  { id: "source.dependence", label: "Source Dependence (impact)" },
  { id: "chat.summary", label: "Chat Summary" },
  { id: "source.excerpt", label: "Source Excerpt" },
  { id: "mcp.driver", label: "MCP Driver (Google Drive)" },
  { id: "jira.issue", label: "Jira Issue" },
  { id: "jira.sprint", label: "Jira Sprint" },
  { id: "firebase.crashlytics", label: "Firebase Crashlytics" },
];

type WorkflowDraft = {
  id?: string;
  projectId: string | null;
  name: string;
  description: string;
  isTemplate: boolean;
  modelOverride: string | null;
  reasoningEffortOverride: string | null;
  yoloMode: boolean;
  // saveWorkflow's repository upserts every field it's given, defaulting an
  // omitted key to null/[] rather than leaving the existing DB value
  // untouched, so a plain rename used to silently wipe an existing
  // workflow's policy caps and edge graph if these were ever left out of the
  // save payload — populated from the loaded Workflow in mapWorkflowToDraft
  // and always sent back on save (BUG-NOTE-CP42 #14).
  // policyCap/policyExtendBy are now editable (the "Cap" and "Extend by"
  // fields below) — startResolvedFlow applies them to the run's actual loop
  // state, replacing a prior hardcoded cap=3/extendBy=2 that ignored these
  // columns entirely. policyOnCap/policyExtendMax remain pass-through only:
  // no runtime code reads either today (the cap-hit path always escalates
  // regardless of policyOnCap, and BUG-231 retired the policyExtendMax
  // ceiling check), so exposing an input for them would imply a behavior
  // that doesn't exist.
  policyCap: number | null;
  policyOnCap: string | null;
  policyExtendBy: number | null;
  policyExtendMax: number | null;
  edges: Workflow["edges"];
  // CP-55 P-1 (Task-263/CA-424 pass 3): read-only/display-only in this
  // Settings UI (see the Workflow.acceptanceNodes doc comment in
  // adminModels.ts for why authoring is deferred) — carried through the
  // same "always sent back on save" contract as edges/policy* above
  // (BUG-NOTE-CP42 #14) so a save can never silently erase a declared
  // acceptance boundary just because there is no editor for it yet.
  acceptanceNodes: string[];
};

const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING = "medium";
const REASONING_OPTIONS = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "xhigh", label: "XHigh" },
] as const;
// The 3 outcome statuses a hub.inline node's flow_control call can actually
// produce (mapped via reviewOutcomeFace() in the runner). A plain
// agent.delegate node only ever completes as "done" - it has no way to
// produce "continue"/"escalate" itself. A native <select> here (not a
// datalist "suggestion" input) avoids a real browser quirk: an <input
// list=...> only filters the datalist to entries matching what's ALREADY
// typed, so once a field held "done", the other two options never showed.
const WORKFLOW_EDGE_WHEN_OPTIONS = ["done", "continue", "escalate"] as const;

function deriveStepPromptBase(input: { stepType: string; name: string; description: string }) {
  const title = input.name.trim() || input.stepType.trim() || "workflow step";
  const summary = input.description.trim() || `Execute the ${title} step.`;
  return `Execute the ${title} step.\n\n${summary}`;
}

// Flow/Workflow launches (single-step and multi-step alike) always run YOLO=true — the
// gate/approval/reinvoke machinery a workflow's own hub/cohort orchestration depends on is
// not robust against a paused approval card mid-flow (a stalled/failed child, a missed
// approval render while another child is focused, the hub's own stall-timeout firing while
// a sibling is still legitimately working). YOLO=false is only supported in Normal Chat,
// which has no hub/cohort/gate layer to race against. Every yoloMode default and mapper
// below is pinned to true; the corresponding settings checkboxes are shown checked+disabled
// (see renderStepDefinitionForm / the workflow create+edit forms) rather than removed, so the
// UI still explains why the option is unavailable here.
export function createEmptyStepDraft(modelId: string): StepDefinition {
  return {
    stepType: "",
    name: "",
    description: "",
    promptBase: "",
    requiredMcps: [],
    mcpAccessMode: "read_only",
    requiredSkills: [],
    teamRole: null,
    subagent: null,
    model: null,
    reasoningEffort: DEFAULT_REASONING,
    yoloMode: true,
    agentType: "standard",
    nodeId: null,
    behaviorId: null,
    agentRef: null,
    nodeLifecycle: null,
    joinMode: null,
    cohort: null,
    promptTemplateRef: null,
    contextRef: null,
    contextSources: [],
    artifactBindings: [],
    createdAt: "",
    updatedAt: "",
  };
}

export function mapWorkflowToDraft(workflow: Workflow | null): WorkflowDraft | null {
  if (!workflow) return null;
  return {
    id: workflow.id,
    projectId: workflow.projectId,
    name: workflow.name,
    description: workflow.description,
    isTemplate: workflow.isTemplate,
    modelOverride: workflow.modelOverride ?? "",
    reasoningEffortOverride: workflow.reasoningEffortOverride ?? DEFAULT_REASONING,
    // Force true regardless of what is persisted — see the comment above createEmptyStepDraft.
    // A legacy workflow saved with yoloMode=false is normalized the moment it is loaded into
    // the edit draft, so simply opening it (even without touching the checkbox) corrects it
    // on next save.
    yoloMode: true,
    policyCap: workflow.policyCap,
    policyOnCap: workflow.policyOnCap,
    policyExtendBy: workflow.policyExtendBy,
    policyExtendMax: workflow.policyExtendMax,
    edges: workflow.edges,
    acceptanceNodes: workflow.acceptanceNodes ?? [],
  };
}

export function createEmptyWorkflowDraft(projects: Project[], modelId: string): WorkflowDraft {
  return {
    projectId: projects[0]?.id ?? null,
    name: "New Workflow",
    description: "",
    isTemplate: false,
    modelOverride: "",
    reasoningEffortOverride: DEFAULT_REASONING,
    yoloMode: true,
    policyCap: null,
    policyOnCap: null,
    policyExtendBy: null,
    policyExtendMax: null,
    edges: [],
    acceptanceNodes: [],
  };
}

// normalizeWorkflowEdgesSnapshot stabilizes edges for dirty comparison
// (BUG-274: order is non-semantic for save enablement).
function normalizeWorkflowEdgesSnapshot(edges: Workflow["edges"] | undefined) {
  return [...(edges ?? [])]
    .map((edge) => ({
      from: edge.from ?? "",
      to: edge.to ?? "",
      kind: edge.kind ?? "",
      when: edge.when ?? "",
    }))
    .sort((a, b) => {
      const ak = `${a.from}\0${a.to}\0${a.kind}\0${a.when}`;
      const bk = `${b.from}\0${b.to}\0${b.kind}\0${b.when}`;
      return ak.localeCompare(bk);
    });
}

function normalizeWorkflowSnapshot(draft: WorkflowDraft | null, steps: WorkflowStep[]) {
  if (!draft) return "";
  return JSON.stringify({
    projectId: draft.projectId ?? null,
    name: draft.name,
    description: draft.description,
    isTemplate: draft.isTemplate,
    modelOverride: draft.modelOverride ?? "",
    reasoningEffortOverride: draft.reasoningEffortOverride ?? "",
    yoloMode: draft.yoloMode,
    // BUG-274: edges + policy must participate in dirty fingerprint or
    // edges-only edits leave Save Workflow disabled.
    policyCap: draft.policyCap ?? null,
    policyOnCap: draft.policyOnCap ?? null,
    policyExtendBy: draft.policyExtendBy ?? null,
    policyExtendMax: draft.policyExtendMax ?? null,
    edges: normalizeWorkflowEdgesSnapshot(draft.edges),
    // Read-only in this UI (no editor mutates it), included for the same
    // reason edges/policy* are: a dirty-fingerprint that silently ignored a
    // real field of the draft would be quietly wrong if that ever changed.
    acceptanceNodes: [...draft.acceptanceNodes].sort(),
    steps: steps.map((step, index) => ({
      stepType: step.stepType,
      orderIndex: index,
      isEnabled: step.isEnabled,
      requiresApproval: step.requiresApproval,
    })),
  });
}

function normalizeStepSnapshot(step: StepDefinition | null) {
  if (!step) return "";
  return JSON.stringify({
    stepType: step.stepType,
    name: step.name,
    description: step.description,
    promptBase: step.promptBase ?? "",
    requiredMcps: [...step.requiredMcps].sort(),
    mcpAccessMode: step.mcpAccessMode,
    requiredSkills: [...step.requiredSkills].sort(),
    teamRole: step.teamRole ?? "",
    subagent: step.subagent ?? "",
    model: step.model,
    reasoningEffort: step.reasoningEffort ?? "",
    yoloMode: step.yoloMode,
    agentType: step.agentType,
    nodeId: step.nodeId ?? "",
    behaviorId: step.behaviorId ?? "",
    agentRef: step.agentRef ?? "",
    nodeLifecycle: step.nodeLifecycle ?? "",
    joinMode: step.joinMode ?? "",
    cohort: step.cohort ?? "",
    promptTemplateRef: step.promptTemplateRef ?? "",
    contextRef: step.contextRef ?? "",
    artifactBindings: [...step.artifactBindings]
      .map((binding) => `${binding.direction}:${binding.artifactInstanceId}:${binding.required}`)
      .sort(),
  });
}

function buildModelOptions(models: SupportedModel[]) {
  const enabledModels = models.filter((model) => model.isEnabled);
  return enabledModels.length > 0
    ? enabledModels.map((model) => ({ value: model.modelId, label: model.displayName }))
    : [{ value: DEFAULT_MODEL, label: DEFAULT_MODEL }];
}

function toggleString(list: string[], value: string) {
  return list.includes(value) ? list.filter((item) => item !== value) : [...list, value];
}

// A step whose Behavior ID requires an agent (agent.delegate) but has no
// Agent ref set resolves at runtime to flowNodeAgentName(node) === "" — the
// executor logs "entry node has no resolvable agent; skipped" and the node
// never spawns, with no error surfaced to the user anywhere in the UI. This
// mirrors validateFlowGraph's same check (which only ever runs at Save
// Workflow time, and only once a flow has edges) at the point the step
// itself is actually authored, so the gap can't be saved in the first place.
function stepDefinitionAgentRefIssue(
  behaviorId: string | null | undefined,
  agentRef: string | null | undefined,
): string | null {
  const requiresAgent = FLOW_BEHAVIOR_OPTIONS.find((option) => option.id === behaviorId)?.requiresAgent ?? false;
  if (requiresAgent && !agentRef?.trim()) {
    return `Behavior "${behaviorId}" requires an Agent ref — set one before saving this step.`;
  }
  return null;
}

// CP-45/SD-23 Task-200 D-7 (simplified v1 type-compat rule): a
// `context.produce` step's only meaningful artifact slot is the
// `context_artifact.v1` instance it outputs (resolveArtifactBoundContextSources
// reads OUTPUT bindings of that type — artifact_type_registry.go); any other
// step consumes/produces non-context artifact types only (e.g.
// `file_artifact.v1` — resolveInputArtifactPrompt explicitly skips
// context_artifact bindings). This keeps the picker from ever offering a
// type-incompatible instance, without needing a fuller per-slot type-contract
// system.
// direction (Task-233): telegram.v1 is an OUTPUT-only "action" artifact — it
// has no meaningful INPUT semantics (there is nothing to read back from a
// sent Telegram message), so it must never appear in an INPUT picker.
function compatibleArtifactInstancesFor(
  behaviorId: string | null | undefined,
  instances: ArtifactInstance[],
  direction: "input" | "output" = "output",
): ArtifactInstance[] {
  const isContextStep = behaviorId === "context.produce";
  return instances.filter((instance) => {
    if (instance.artifactTypeId === "telegram.v1") return direction === "output";
    return isContextStep ? instance.artifactTypeId === "context_artifact.v1" : instance.artifactTypeId !== "context_artifact.v1";
  });
}

function artifactInstanceLabel(instance: ArtifactInstance | undefined, id: string): string {
  if (!instance) return id;
  return instance.isBuiltin ? `${instance.name} (built-in)` : instance.name;
}

function toTitleCase(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

// CP-45/SD-23 Task-199: the Flow settings "Artifacts" tab. Types are a
// read-only catalog (system-owned, Task-197); instances split into built-in
// (isBuiltin=true, seeded, read-only — shown with a "built-in" tag,
// mirroring how a built-in Workflow is read-only) and user-authored
// (isBuiltin=false, full CRUD here). Step binding happens in the Steps tab
// (Task-200), not here — this page only authors reusable instances.
function ArtifactsTabContent(props: {
  artifactTypes: ArtifactType[];
  artifactInstances: ArtifactInstance[];
  artifactInstanceView: ViewMode;
  artifactInstanceDraft: ArtifactInstance | null;
  busy: boolean;
  onCreateNew: (artifactTypeId: string) => void;
  onSelect: (instance: ArtifactInstance) => void;
  onSave: () => void;
  onDelete: (instanceId: string) => void;
  setArtifactInstanceView: (view: ViewMode) => void;
  setArtifactInstanceDraft: (draft: ArtifactInstance | null) => void;
}): React.ReactElement {
  const {
    artifactTypes,
    artifactInstances,
    artifactInstanceView,
    artifactInstanceDraft,
    busy,
    onCreateNew,
    onSelect,
    onSave,
    onDelete,
    setArtifactInstanceView,
    setArtifactInstanceDraft,
  } = props;

  const draftType = artifactTypes.find((type) => type.id === artifactInstanceDraft?.artifactTypeId);
  const isFileType = artifactInstanceDraft?.artifactTypeId === "file_artifact.v1";
  const isContextType = artifactInstanceDraft?.artifactTypeId === "context_artifact.v1";
  const isTelegramType = artifactInstanceDraft?.artifactTypeId === "telegram.v1";
  const draftTelegramChatId = isTelegramType ? ((artifactInstanceDraft?.configJson.chatId as string | undefined) ?? "") : "";
  const draftTelegramTemplate = isTelegramType
    ? ((artifactInstanceDraft?.configJson.messageTemplate as string | undefined) ?? "")
    : "";
  const draftPaths = isFileType
    ? ((artifactInstanceDraft?.configJson.paths as string[] | undefined) ?? []).join("\n")
    : "";
  const draftConfig = artifactInstanceDraft?.configJson as Record<string, unknown> | undefined;
  const structureEnabled = isFileType && isFileArtifactStructureEnabled(draftConfig);
  const draftFileStructure = isFileType ? parseFileArtifactStructure(draftConfig) : null;
  // When structure is off, keep sections text empty so enabling does not
  // silently inject coding-memo titles (defaults apply on Save or preset button).
  const draftStructureSections = structureEnabled
    ? sectionsToTextarea(draftFileStructure?.sections ?? [])
    : "";
  const draftSources = isContextType
    ? ((artifactInstanceDraft?.configJson.sources as string[] | undefined) ?? [])
    : [];

  if (artifactInstanceView === "create" && artifactInstanceDraft) {
    return (
      <div className="settings-subpanel">
        <div className="settings-actions">
          <button
            className="secondary-btn"
            onClick={() => {
              setArtifactInstanceView("list");
              setArtifactInstanceDraft(null);
            }}
            type="button"
          >
            ← Back
          </button>
        </div>
        <h3>{artifactInstanceDraft.id ? "Edit Artifact Instance" : "New Artifact Instance"}</h3>
        {artifactInstanceDraft.isBuiltin ? (
          <div className="artifact-info-panel">
            <strong>Built-in artifact instance</strong>
            <p>Built-in instances are seeded by the system and cannot be edited here.</p>
          </div>
        ) : null}
        <div className="settings-grid">
          <label className="settings-field">
            <span>Artifact Type</span>
            <input disabled value={draftType?.id ?? artifactInstanceDraft.artifactTypeId} />
          </label>
          <label className="settings-field">
            <span>Name</span>
            <input
              disabled={artifactInstanceDraft.isBuiltin}
              onChange={(event) =>
                setArtifactInstanceDraft({ ...artifactInstanceDraft, name: event.target.value })
              }
              value={artifactInstanceDraft.name}
            />
          </label>
          <label className="settings-field settings-field-full">
            <span>Description</span>
            <textarea
              disabled={artifactInstanceDraft.isBuiltin}
              onChange={(event) =>
                setArtifactInstanceDraft({ ...artifactInstanceDraft, description: event.target.value })
              }
              value={artifactInstanceDraft.description}
            />
          </label>
          {isContextType ? (
            <div className="settings-field settings-field-full">
              <span>Context Sources</span>
              <small className="settings-field-help">
                Primary authoring path (CP-45): sources live on this <code>context_artifact.v1</code> instance.
                Bind the instance on a <code>context.produce</code> step as Artifact Output — do not use raw
                Step-level Context Sources chips.
              </small>
              <div className="workflow-chips">
                {contextSourceOptions.map((option) => {
                  const checked = draftSources.includes(option.id);
                  return (
                    <label className="settings-checkbox" key={option.id}>
                      <input
                        checked={checked}
                        disabled={artifactInstanceDraft.isBuiltin}
                        onChange={() =>
                          setArtifactInstanceDraft({
                            ...artifactInstanceDraft,
                            configJson: { ...artifactInstanceDraft.configJson, sources: toggleString(draftSources, option.id) },
                          })
                        }
                        type="checkbox"
                      />
                      <span>{option.label}</span>
                    </label>
                  );
                })}
              </div>
              {draftSources.includes("mcp.driver") ? (
                <small>
                  Uses the Google Drive account selected in Google Drive setup. The file URL or ID is chosen at runtime.
                </small>
              ) : null}
              {draftSources.includes("jira.issue") || draftSources.includes("jira.sprint") ? (
                <small>
                  Uses the Jira MCP connected in MCP Servers settings. The issue key or sprint is chosen at runtime — the
                  run asks before it starts, and the AI reads only the selected issue/sprint via the Jira MCP tools.
                </small>
              ) : null}
              {draftSources.includes("firebase.crashlytics") ? (
                <small>
                  Uses the Firebase MCP connected in MCP Servers settings. The crash issue id is chosen at runtime — the
                  run asks before it starts, and the AI reads only the selected crash via the Firebase Crashlytics MCP
                  tools.
                </small>
              ) : null}
            </div>
          ) : null}
          {isFileType ? (
            <>
              <label className="settings-field settings-field-full">
                <span>File Paths (one per line, workspace-relative)</span>
                <small className="settings-field-help">
                  Workspace-relative path(s) this instance designates (Task-223 / BUG-276).{" "}
                  <strong>Output</strong> = write contract (agent must create/update these files; gate reprompts if
                  missing). <strong>Input</strong> = path mention only — agent is told to open/read those paths with
                  tools; file body is not pasted into the prompt (unlike context packages).
                </small>
                <textarea
                  disabled={artifactInstanceDraft.isBuiltin}
                  onChange={(event) =>
                    setArtifactInstanceDraft({
                      ...artifactInstanceDraft,
                      configJson: {
                        ...artifactInstanceDraft.configJson,
                        paths: event.target.value.split("\n").map((line) => line.trim()).filter(Boolean),
                      },
                    })
                  }
                  placeholder={"docs/coder-summary.md"}
                  value={draftPaths}
                />
              </label>
              {/* Checkbox is NOT nested as a .settings-field > input (text-field CSS
                  stretched checkboxes into full-width bars). Use the same pattern as YOLO. */}
              <div className="settings-field-full" style={{ display: "grid", gap: "8px" }}>
                <label className="settings-checkbox">
                  <input
                    checked={structureEnabled}
                    disabled={artifactInstanceDraft.isBuiltin}
                    onChange={(event) => {
                      const enabled = event.target.checked;
                      setArtifactInstanceDraft({
                        ...artifactInstanceDraft,
                        configJson: withFileArtifactStructure(
                          artifactInstanceDraft.configJson as Record<string, unknown>,
                          enabled,
                          // Enable with empty sections; Save or "Use coding memo" fills defaults.
                          "",
                          { fillDefaultWhenEmpty: false },
                        ),
                      });
                    }}
                    type="checkbox"
                  />
                  <span>Require markdown sections (OUTPUT template + gate)</span>
                </label>
                <small className="settings-field-help">
                  Task-225: optional per-instance structure. When enabled and this instance is bound as a{" "}
                  <strong>required Output</strong>, the agent prompt lists these section titles and the flow gate
                  checks they exist as ATX headings after the file is written (flexible level/casing). Leave off for
                  testing/planning/raw notes (existence-only). Does not affect Input path-only behavior.
                </small>
                {structureEnabled ? (
                  <>
                    <label className="settings-field settings-field-full">
                      <span>Section titles (one per line)</span>
                      <textarea
                        disabled={artifactInstanceDraft.isBuiltin}
                        onChange={(event) =>
                          setArtifactInstanceDraft({
                            ...artifactInstanceDraft,
                            configJson: withFileArtifactStructure(
                              artifactInstanceDraft.configJson as Record<string, unknown>,
                              true,
                              event.target.value,
                            ),
                          })
                        }
                        placeholder={"What\nWhy\nBaseline"}
                        rows={4}
                        value={draftStructureSections}
                      />
                    </label>
                    {!artifactInstanceDraft.isBuiltin ? (
                      <div className="settings-actions">
                        <button
                          className="secondary-btn"
                          onClick={() =>
                            setArtifactInstanceDraft({
                              ...artifactInstanceDraft,
                              configJson: withFileArtifactStructure(
                                artifactInstanceDraft.configJson as Record<string, unknown>,
                                true,
                                sectionsToTextarea([...DEFAULT_CODING_MEMO_SECTIONS]),
                              ),
                            })
                          }
                          type="button"
                        >
                          Use coding memo (What / Why / Baseline)
                        </button>
                      </div>
                    ) : null}
                  </>
                ) : null}
              </div>
            </>
          ) : null}
          {isTelegramType ? (
            <>
              <label className="settings-field settings-field-full">
                <span>Telegram Chat/Channel ID</span>
                <small className="settings-field-help">
                  The Telegram chat or channel id this instance sends to. Bind this instance as a step's{" "}
                  <strong>Output only</strong> (Task-233) — the bound step must send a notification here via the
                  Telegram MCP tool before finishing; the flow gate verifies a real message was sent.
                </small>
                <input
                  disabled={artifactInstanceDraft.isBuiltin}
                  onChange={(event) =>
                    setArtifactInstanceDraft({
                      ...artifactInstanceDraft,
                      configJson: { ...artifactInstanceDraft.configJson, chatId: event.target.value.trim() },
                    })
                  }
                  placeholder="-100123456789"
                  value={draftTelegramChatId}
                />
              </label>
              <label className="settings-field settings-field-full">
                <span>Message Template (optional)</span>
                <small className="settings-field-help">
                  Optional template for the notification text (CP-05-05 Q-5). Leave blank to let the agent summarize
                  the run's outcome freely.
                </small>
                <textarea
                  disabled={artifactInstanceDraft.isBuiltin}
                  onChange={(event) =>
                    setArtifactInstanceDraft({
                      ...artifactInstanceDraft,
                      configJson: { ...artifactInstanceDraft.configJson, messageTemplate: event.target.value },
                    })
                  }
                  placeholder={"Run finished: {{status}}\nSummary: ..."}
                  rows={3}
                  value={draftTelegramTemplate}
                />
              </label>
            </>
          ) : null}
        </div>
        {!artifactInstanceDraft.isBuiltin ? (
          <div className="settings-actions">
            <button
              className="primary-btn"
              disabled={busy || !artifactInstanceDraft.name.trim()}
              onClick={onSave}
              type="button"
            >
              Save Artifact Instance
            </button>
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div className="settings-two-column">
      <div className="settings-subpanel">
        <h3>Artifact Types (built-in catalog)</h3>
        <div className="settings-list">
          {artifactTypes.map((type) => (
            <div className="settings-list-item static" key={type.id}>
              <strong>{type.id}</strong>
              <span>{type.category} · {type.status}</span>
              {type.id === "file_artifact.v1" ? (
                <small>
                  Path-designated file artifact: bind as <strong>Output</strong> so the step must write those
                  path(s); bind as <strong>Input</strong> so the step reads them (e.g. coder → review). Optional{" "}
                  <strong>markdown sections</strong> on the instance configure OUTPUT template + structure gate
                  (Task-225).
                </small>
              ) : null}
              {type.id === "context_artifact.v1" ? (
                <small>Context package producer: configure sources on the instance, then bind as step Artifact I/O.</small>
              ) : null}
              {type.id === "telegram.v1" ? (
                <small>
                  Send-only notification action (Task-233): bind as a step's <strong>Output only</strong> — the step
                  must send a Telegram message via the MCP tool before finishing; the flow gate verifies a real
                  message_id, not a file.
                </small>
              ) : null}
            </div>
          ))}
          {artifactTypes.length === 0 ? <div className="settings-list-empty">No artifact types.</div> : null}
        </div>
        <div className="settings-actions" style={{ marginTop: "12px" }}>
          {artifactTypes.map((type) => (
            <button
              className="secondary-btn"
              key={type.id}
              onClick={() => onCreateNew(type.id)}
              type="button"
            >
              + New {type.id} instance
            </button>
          ))}
        </div>
      </div>
      <div className="settings-subpanel">
        <h3>Artifact Instances</h3>
        <div className="settings-list">
          {artifactInstances.map((instance) => (
            <button
              className="settings-list-item"
              key={instance.id}
              onClick={() => onSelect(instance)}
              type="button"
            >
              <strong>
                {instance.name}
                {instance.isBuiltin ? " (built-in)" : ""}
              </strong>
              <span>{instance.artifactTypeId}</span>
              {!instance.isBuiltin ? (
                <button
                  aria-label={`Delete ${instance.name}`}
                  className="workflow-chip-remove"
                  onClick={(event) => {
                    event.stopPropagation();
                    void onDelete(instance.id);
                  }}
                  type="button"
                >
                  ×
                </button>
              ) : null}
            </button>
          ))}
          {artifactInstances.length === 0 ? <div className="settings-list-empty">No artifact instances yet.</div> : null}
        </div>
      </div>
    </div>
  );
}

export function WorkflowsSettings(): React.ReactElement {
  const [tab, setTab] = useState<Tab>("workflows");
  const [workflowView, setWorkflowView] = useState<ViewMode>("list");
  const [stepView, setStepView] = useState<ViewMode>("list");
  const [projects, setProjects] = useState<Project[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [stepDefinitions, setStepDefinitions] = useState<StepDefinition[]>([]);
  // CP-45/SD-23: the generic typed-artifact framework's catalog + instances.
  // artifactTypes is read-only (system-owned, Task-197);
  // artifactInstances mixes built-in (isBuiltin=true, seeded, read-only) and
  // user-authored rows, managed from the new "Artifacts" tab (Task-199).
  const [artifactTypes, setArtifactTypes] = useState<ArtifactType[]>([]);
  const [artifactInstances, setArtifactInstances] = useState<ArtifactInstance[]>([]);
  const [artifactInstanceDraft, setArtifactInstanceDraft] = useState<ArtifactInstance | null>(null);
  const [artifactInstanceView, setArtifactInstanceView] = useState<ViewMode>("list");
  // Built-in workflows are read-only (see workflowDetailReadOnly below) and
  // can never be edited to drop a step, so a step definition still listed by
  // any built-in workflow can't be deleted either — doing so would leave
  // that (undeletable, uneditable) workflow with a dangling stepType it can
  // never resolve at runtime. Recomputed on every refresh() from the small
  // set of built-in workflows' own step lists.
  const [stepTypesUsedByBuiltin, setStepTypesUsedByBuiltin] = useState<Set<string>>(new Set());
  const [selectedWorkflowId, setSelectedWorkflowId] = useState("");
  const [selectedStepType, setSelectedStepType] = useState("");
  const [detailWorkflowStepType, setDetailWorkflowStepType] = useState("");
  const [workflowDraft, setWorkflowDraft] = useState<WorkflowDraft | null>(null);
  const [workflowInitialSnapshot, setWorkflowInitialSnapshot] = useState("");
  const [stepDraft, setStepDraft] = useState<StepDefinition | null>(null);
  const [stepInitialSnapshot, setStepInitialSnapshot] = useState("");
  const [createWorkflowDraft, setCreateWorkflowDraft] = useState<WorkflowDraft>(() =>
    createEmptyWorkflowDraft([], DEFAULT_MODEL),
  );
  const [createWorkflowSteps, setCreateWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [createWorkflowStepType, setCreateWorkflowStepType] = useState("");
  const [createStepDraft, setCreateStepDraft] = useState<StepDefinition>(() =>
    createEmptyStepDraft(DEFAULT_MODEL),
  );
  // Owner finding (2026-07-06): step-definitions are edited in the Steps tab,
  // independent of any one workflow, so there's no workflow.projectId to
  // resolve a real target-project cwd from automatically. This lets the user
  // explicitly pick which project's agents to preview/select for Agent ref —
  // "" (Workspace global) loads only built-in + provider-home agents (no
  // project-local .claude/agents, since there's no fixed cwd for those).
  // Whatever gets picked here is a real, working agent at runtime: Go's
  // spawnChildRun falls back to agentCatalog.listAgents(cwd) — cwd = the
  // RUN's own workspaceCwd at that time — whenever resolvePackAgentDefinition
  // (built-ins only) doesn't recognize the name, so a project-local agent
  // works as long as the flow actually runs against that same project later.
  const [agentPreviewProjectId, setAgentPreviewProjectId] = useState("");
  const [agentOptions, setAgentOptions] = useState<AgentDefinition[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");
  const [pickerModal, setPickerModal] = useState<PickerModal | null>(null);
  // Multi-select delete: entered via long-press on a Workflow/Step Registry
  // row instead of a mode toggle. `bulkSelect` is null outside select mode;
  // once set, every row in that same list renders a checkbox and a tap
  // toggles selection instead of opening the row for edit.
  const [bulkSelect, setBulkSelect] = useState<BulkSelect | null>(null);
  const longPressTimerRef = useRef<number | null>(null);
  const longPressFiredRef = useRef(false);
  // Built-in flow cloning (CP-42/Task-179): a built-in (isBuiltin=true)
  // workflow is read-only in this screen — cloneTarget drives the "name your
  // copy" modal that creates an editable, non-builtin copy.
  const [cloneTarget, setCloneTarget] = useState<{ workflowId: string; sourceName: string } | null>(null);
  const [cloneName, setCloneName] = useState("");
  // Workflow step cards default to collapsed; expansion is tracked per
  // source+step id so detail and create editors don't share state.
  const [expandedStepKeys, setExpandedStepKeys] = useState<Set<string>>(new Set());
  // Task-189 slice 4: the visual canvas is a second view over the SAME
  // `edges` array as the form-list editor (no separate edge state — that's
  // what keeps the two in sync). Node x/y is purely a view-layout concern:
  // it is never sent to saveWorkflow/saveNewWorkflow and has no backing
  // column, so it can't violate the "node data lives only in
  // step_definitions" contract — it just remembers where a node box was
  // last dragged to, keyed by `${source}:${nodeKey}`.
  const [canvasPositions, setCanvasPositions] = useState<Record<string, { x: number; y: number }>>({});
  const [draggingCanvasNode, setDraggingCanvasNode] = useState<{ source: "detail" | "create"; key: string } | null>(
    null,
  );
  const [pendingCanvasEdgeFrom, setPendingCanvasEdgeFrom] = useState<
    { source: "detail" | "create"; nodeId: string } | null
  >(null);

  const toggleStepExpanded = (key: string) => {
    setExpandedStepKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const clearLongPressTimer = () => {
    if (longPressTimerRef.current !== null) {
      window.clearTimeout(longPressTimerRef.current);
      longPressTimerRef.current = null;
    }
  };

  // Long-press (works for touch and mouse via Pointer Events) enters select
  // mode and selects the pressed row; a short tap afterwards falls through
  // to the row's normal click handler. `longPressFiredRef` lets that click
  // handler tell the two apart, since a pointerup after a fired long-press
  // still dispatches a click. If select mode is already active, long-pressing
  // another row adds it to the existing selection instead of resetting it —
  // once inside select mode a plain tap already toggles rows one at a time,
  // but a user may still long-press out of habit and shouldn't lose progress.
  const startLongPress = (kind: "workflow" | "step", id: string) => {
    clearLongPressTimer();
    longPressFiredRef.current = false;
    longPressTimerRef.current = window.setTimeout(() => {
      longPressFiredRef.current = true;
      setBulkSelect((current) => {
        if (current && current.kind === kind) {
          const next = new Set(current.ids);
          next.add(id);
          return { ...current, ids: next };
        }
        return { kind, ids: new Set([id]) };
      });
    }, 500);
  };

  const consumeLongPressClick = () => {
    if (!longPressFiredRef.current) return false;
    longPressFiredRef.current = false;
    return true;
  };

  const toggleBulkSelected = (id: string) => {
    setBulkSelect((current) => {
      if (!current) return current;
      const next = new Set(current.ids);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return { ...current, ids: next };
    });
  };

  const exitBulkSelect = () => setBulkSelect(null);

  const selectedWorkflow = useMemo(
    () => workflows.find((item) => item.id === selectedWorkflowId) ?? null,
    [workflows, selectedWorkflowId],
  );
  // BUG-NOTE-CP42 #8: the detail copy already says a built-in can't be
  // edited and Save/Delete are hidden for it, but the actual form fields
  // below had no disabled binding at all — a user could type changes into a
  // built-in workflow's Name/Description/Model/etc. that could never be
  // saved, undercutting the read-only UX the copy promises.
  const workflowDetailReadOnly = selectedWorkflow?.editable === false;
  const selectedStep = useMemo(
    () => stepDefinitions.find((item) => item.stepType === selectedStepType) ?? null,
    [stepDefinitions, selectedStepType],
  );
  const modelOptions = useMemo(() => buildModelOptions(models), [models]);
  const availableCreateWorkflowSteps = useMemo(
    () =>
      stepDefinitions.filter(
        (definition) =>
          !createWorkflowSteps.some((step) => step.stepType === definition.stepType),
      ),
    [createWorkflowSteps, stepDefinitions],
  );
  const workflowDirty =
    normalizeWorkflowSnapshot(workflowDraft, workflowSteps) !== workflowInitialSnapshot;
  const stepDirty = normalizeStepSnapshot(stepDraft) !== stepInitialSnapshot;

  const syncWorkflowSelection = (
    workflowId: string,
    nextWorkflows: Workflow[] = workflows,
    nextWorkflowSteps: WorkflowStep[],
    nextStepDefinitions: StepDefinition[] = stepDefinitions,
  ) => {
    setSelectedWorkflowId(workflowId);
    const nextSelectedWorkflow =
      nextWorkflows.find((item) => item.id === workflowId) ?? null;
    const nextDetailWorkflowStepType =
      nextStepDefinitions.find(
        (definition) =>
          !nextWorkflowSteps.some((step) => step.stepType === definition.stepType),
      )?.stepType ?? "";
    setDetailWorkflowStepType(nextDetailWorkflowStepType);
    setWorkflowSteps(nextWorkflowSteps);
    const nextWorkflowDraft = mapWorkflowToDraft(nextSelectedWorkflow);
    setWorkflowDraft(nextWorkflowDraft);
    setWorkflowInitialSnapshot(
      normalizeWorkflowSnapshot(nextWorkflowDraft, nextWorkflowSteps),
    );
  };

  const selectStepDefinition = (stepType: string, definitions: StepDefinition[] = stepDefinitions) => {
    setSelectedStepType(stepType);
    const nextSelectedStep = definitions.find((item) => item.stepType === stepType) ?? null;
    // Force true regardless of what is persisted — see the comment above createEmptyStepDraft.
    const nextStepDraft = nextSelectedStep ? { ...nextSelectedStep, yoloMode: true } : null;
    setStepDraft(nextStepDraft);
    setStepInitialSnapshot(normalizeStepSnapshot(nextStepDraft));
  };

  const selectWorkflowDefinition = async (workflowId: string) => {
    const admin = await getAdminUseCases();
    const nextWorkflowSteps = workflowId
      ? await admin.workflows.listWorkflowSteps(workflowId)
      : [];
    syncWorkflowSelection(workflowId, workflows, nextWorkflowSteps, stepDefinitions);
  };

  const refresh = async (
    workflowId?: string,
    stepType?: string,
    options?: { preserveCreateDrafts?: boolean },
  ) => {
    try {
      const admin = await getAdminUseCases();
      const [
        nextProjects,
        nextWorkflows,
        nextStepDefinitions,
        nextModels,
        nextArtifactTypes,
        nextArtifactInstances,
      ] = await Promise.all([
        admin.projects.listProjects(),
        admin.workflows.listWorkflows(),
        admin.workflows.listStepDefinitions(),
        admin.providers.listSupportedModels(),
        admin.workflows.listArtifactTypes(),
        admin.workflows.listArtifactInstances(),
      ]);

      const enabledModels = nextModels.filter((model) => model.isEnabled);
      const defaultModel = enabledModels[0]?.modelId ?? DEFAULT_MODEL;
      const resolvedWorkflowId =
        workflowId && nextWorkflows.some((item) => item.id === workflowId)
          ? workflowId
          : nextWorkflows[0]?.id ?? "";
      // A cloned workflow's own steps carry a freshly namespaced step_type
      // (BUG-262), distinct from the source's. A caller (e.g. cloneSelected)
      // may still pass the PREVIOUSLY selected step's type, left over from
      // before the switch — validating that against the full cross-workflow
      // catalog (nextStepDefinitions) let a stale, unrelated-workflow
      // step_type pass through as "valid" whenever it happened to also exist
      // elsewhere in the catalog, silently loading (and, on Save, mutating)
      // that OTHER workflow's step instead of one actually belonging to
      // resolvedWorkflowId. Must validate against this workflow's own steps.
      const nextWorkflowSteps = resolvedWorkflowId
        ? await admin.workflows.listWorkflowSteps(resolvedWorkflowId)
        : [];
      const resolvedStepType =
        stepType && nextWorkflowSteps.some((item) => item.stepType === stepType)
          ? stepType
          : nextWorkflowSteps[0]?.stepType ?? "";

      const builtinWorkflows = nextWorkflows.filter((workflow) => workflow.isBuiltin);
      const builtinStepLists = await Promise.all(
        builtinWorkflows.map((workflow) => admin.workflows.listWorkflowSteps(workflow.id)),
      );
      const nextStepTypesUsedByBuiltin = new Set(builtinStepLists.flat().map((step) => step.stepType));

      setProjects(nextProjects);
      setWorkflows(nextWorkflows);
      setStepDefinitions(nextStepDefinitions);
      setModels(enabledModels);
      setArtifactTypes(nextArtifactTypes);
      setArtifactInstances(nextArtifactInstances);
      setStepTypesUsedByBuiltin(nextStepTypesUsedByBuiltin);

      if (!options?.preserveCreateDrafts) {
        setCreateWorkflowDraft(createEmptyWorkflowDraft(nextProjects, defaultModel));
        setCreateWorkflowSteps([]);
        setCreateStepDraft(createEmptyStepDraft(defaultModel));
      }

      syncWorkflowSelection(
        resolvedWorkflowId,
        nextWorkflows,
        nextWorkflowSteps,
        nextStepDefinitions,
      );
      selectStepDefinition(resolvedStepType, nextStepDefinitions);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load workflows and step definitions."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    const cwd = projects.find((project) => project.id === agentPreviewProjectId)?.directoryPath ?? undefined;
    let cancelled = false;
    const client = createRunnerClient();
    if (!client.listAgents) {
      setAgentOptions([]);
      return;
    }
    client
      .listAgents(cwd)
      .then((agents) => {
        if (!cancelled) setAgentOptions(agents);
      })
      .catch(() => {
        if (!cancelled) setAgentOptions([]);
      });
    return () => {
      cancelled = true;
    };
  }, [agentPreviewProjectId, projects]);

  useEffect(() => {
    if (
      workflowView === "list" &&
      tab === "workflows"
    ) {
      const availableDetailStepTypes = stepDefinitions.filter(
        (definition) => !workflowSteps.some((step) => step.stepType === definition.stepType),
      );
      if (
        availableDetailStepTypes.length > 0 &&
        !availableDetailStepTypes.some((item) => item.stepType === detailWorkflowStepType)
      ) {
        setDetailWorkflowStepType(availableDetailStepTypes[0].stepType);
      }
      if (availableDetailStepTypes.length === 0 && detailWorkflowStepType !== "") {
        setDetailWorkflowStepType("");
      }
    }
  }, [detailWorkflowStepType, stepDefinitions, tab, workflowSteps, workflowView]);

  useEffect(() => {
    if (
      workflowView === "create" &&
      availableCreateWorkflowSteps.length > 0 &&
      !availableCreateWorkflowSteps.some((item) => item.stepType === createWorkflowStepType)
    ) {
      setCreateWorkflowStepType(availableCreateWorkflowSteps[0].stepType);
    }
    if (workflowView === "create" && availableCreateWorkflowSteps.length === 0) {
      setCreateWorkflowStepType("");
    }
  }, [availableCreateWorkflowSteps, createWorkflowStepType, workflowView]);

  const startCreateWorkflow = () => {
    setCreateWorkflowDraft(createEmptyWorkflowDraft(projects, ""));
    setCreateWorkflowSteps([]);
    setCreateWorkflowStepType(stepDefinitions[0]?.stepType ?? "");
    setWorkflowView("create");
    setMessage(null);
  };

  const startCreateStep = () => {
    setCreateStepDraft(createEmptyStepDraft(""));
    setStepView("create");
    setMessage(null);
  };

  const updateWorkflowStep = (index: number, patch: Partial<WorkflowStep>) => {
    setWorkflowSteps((current) =>
      current.map((item, currentIndex) =>
        currentIndex === index ? { ...item, ...patch } : item,
      ),
    );
  };

  const updateCreateWorkflowStep = (index: number, patch: Partial<WorkflowStep>) => {
    setCreateWorkflowSteps((current) =>
      current.map((item, currentIndex) =>
        currentIndex === index ? { ...item, ...patch } : item,
      ),
    );
  };

  const moveWorkflowStep = (
    index: number,
    direction: "up" | "down",
    source: "detail" | "create",
  ) => {
    const current = source === "detail" ? workflowSteps : createWorkflowSteps;
    const nextIndex = direction === "up" ? index - 1 : index + 1;
    if (nextIndex < 0 || nextIndex >= current.length) return;
    const updated = [...current];
    const temp = updated[index];
    updated[index] = updated[nextIndex];
    updated[nextIndex] = temp;
    const normalized = updated.map((step, orderIndex) => ({ ...step, orderIndex }));
    if (source === "detail") {
      setWorkflowSteps(normalized);
    } else {
      setCreateWorkflowSteps(normalized);
    }
  };

  const removeWorkflowStep = (index: number, source: "detail" | "create") => {
    const next = (source === "detail" ? workflowSteps : createWorkflowSteps)
      .filter((_, currentIndex) => currentIndex !== index)
      .map((step, orderIndex) => ({ ...step, orderIndex }));
    if (source === "detail") {
      setWorkflowSteps(next);
    } else {
      setCreateWorkflowSteps(next);
    }
  };

  const addWorkflowStep = (source: "detail" | "create") => {
    const chosenStepType =
      source === "detail"
        ? detailWorkflowStepType
        : createWorkflowStepType;
    if (!chosenStepType) return;
    const target = source === "detail" ? workflowSteps : createWorkflowSteps;
    const nextStep: WorkflowStep = {
      id: `${chosenStepType}-${Date.now()}`,
      workflowId: source === "detail" ? selectedWorkflowId : "__new__",
      stepType: chosenStepType,
      orderIndex: target.length,
      isEnabled: true,
      requiresApproval: true,
      createdAt: "",
      updatedAt: "",
    };
    if (source === "detail") {
      setWorkflowSteps((current) => [...current, nextStep]);
    } else {
      setCreateWorkflowSteps((current) => [...current, nextStep]);
    }
  };

  // Task-189: an edge's from/to references a node's `nodeId` (the flow-graph
  // identity resolved onto FlowNode.ID at runtime — recordFromWorkflowRow in
  // supabase_workflow_flow_store.go only sets it when the step_definition's
  // node_id is non-empty, with NO fallback to step_type), not the step's
  // `stepType`. A step whose step_definition has no Node ID set can't be
  // referenced by an edge yet; it's filtered out here and flagged by the
  // edges-editor validation (Task-189 slice 3) instead of silently producing
  // an edge that can never resolve to a real node.
  const workflowEdgeNodeOptions = (steps: WorkflowStep[]): string[] => {
    const ids = steps
      .map((step) => stepDefinitions.find((definition) => definition.stepType === step.stepType)?.nodeId)
      .filter((id): id is string => Boolean(id));
    return Array.from(new Set(ids));
  };

  // BUG-282: a step's flow dependency is no longer persisted anywhere on the
  // step definition — topology lives only on the workflow's own edges
  // (workflows.edges_json), and the Go runner derives each node's dependency
  // from those edges at flow-load time (forwardEdgeSources, flow_executor.go).
  // There is therefore nothing edge-derived to write back onto step_definitions
  // when a workflow saves, so the former persistEdgeDerivedDependsOn /
  // computeDependsOnByStepType helpers (and their BUG-262 shared-step guard)
  // are gone; a step reused across flows can no longer carry another flow's
  // node ids because it carries no topology at all.

  // Task-189: the flow graph's edges (Workflow.edges/edges_json) were already
  // round-tripped by saveWorkflow/saveNewWorkflow (BUG-NOTE-CP42 #14 passes
  // them through unchanged), but no UI control ever wrote to them — a new
  // workflow always saved with edges: [] and could never advance past its
  // entry node. These three helpers are the edges-editor equivalent of
  // addWorkflowStep/updateWorkflowStep/removeWorkflowStep above.
  const addWorkflowEdge = (source: "detail" | "create") => {
    const nodeOptions = workflowEdgeNodeOptions(source === "detail" ? workflowSteps : createWorkflowSteps);
    const nextEdge: WorkflowFlowEdge = {
      from: nodeOptions[0] ?? "",
      to: nodeOptions[1] ?? FLOW_EDGE_TERMINALS[0],
      when: "done",
      kind: "forward",
    };
    if (source === "detail") {
      setWorkflowDraft((current) => (current ? { ...current, edges: [...current.edges, nextEdge] } : current));
    } else {
      setCreateWorkflowDraft((current) => ({ ...current, edges: [...current.edges, nextEdge] }));
    }
  };

  const updateWorkflowEdge = (
    index: number,
    patch: Partial<WorkflowFlowEdge>,
    source: "detail" | "create",
  ) => {
    const apply = (edges: WorkflowFlowEdge[]) =>
      edges.map((edge, currentIndex) => (currentIndex === index ? { ...edge, ...patch } : edge));
    if (source === "detail") {
      setWorkflowDraft((current) => (current ? { ...current, edges: apply(current.edges) } : current));
    } else {
      setCreateWorkflowDraft((current) => ({ ...current, edges: apply(current.edges) }));
    }
  };

  const removeWorkflowEdge = (index: number, source: "detail" | "create") => {
    const apply = (edges: WorkflowFlowEdge[]) => edges.filter((_, currentIndex) => currentIndex !== index);
    if (source === "detail") {
      setWorkflowDraft((current) => (current ? { ...current, edges: apply(current.edges) } : current));
    } else {
      setCreateWorkflowDraft((current) => ({ ...current, edges: apply(current.edges) }));
    }
  };

  // Task-189 slice 4: canvas equivalent of addWorkflowEdge, but with an
  // explicit from/to (the two nodes the user connected by clicking their
  // connector dots in sequence) instead of defaulting to the first two node
  // options.
  const addWorkflowEdgeBetween = (from: string, to: string, source: "detail" | "create") => {
    const nextEdge: WorkflowFlowEdge = { from, to, when: "done", kind: "forward" };
    if (source === "detail") {
      setWorkflowDraft((current) => (current ? { ...current, edges: [...current.edges, nextEdge] } : current));
    } else {
      setCreateWorkflowDraft((current) => ({ ...current, edges: [...current.edges, nextEdge] }));
    }
  };

  // Default grid layout for any canvas node that hasn't been manually
  // dragged yet, keyed by `${source}:${nodeKey}` so detail/create canvases
  // (and re-renders after adding/removing nodes) never collide.
  const canvasLayoutFor = (
    source: "detail" | "create",
    keys: string[],
  ): Record<string, { x: number; y: number }> => {
    const layout: Record<string, { x: number; y: number }> = {};
    keys.forEach((key, index) => {
      const stored = canvasPositions[`${source}:${key}`];
      if (stored) {
        layout[key] = stored;
        return;
      }
      const column = index % 3;
      const row = Math.floor(index / 3);
      layout[key] = { x: 70 + column * 190, y: 50 + row * 110 };
    });
    return layout;
  };

  const handleCanvasNodePointerDown = (source: "detail" | "create", key: string) => (
    event: React.PointerEvent<HTMLDivElement>,
  ) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    setDraggingCanvasNode({ source, key });
  };

  const handleCanvasSurfacePointerMove = (source: "detail" | "create") => (
    event: React.PointerEvent<HTMLDivElement>,
  ) => {
    if (!draggingCanvasNode || draggingCanvasNode.source !== source) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const x = Math.max(30, event.clientX - rect.left);
    const y = Math.max(24, event.clientY - rect.top);
    setCanvasPositions((current) => ({ ...current, [`${source}:${draggingCanvasNode.key}`]: { x, y } }));
  };

  const handleCanvasSurfacePointerUp = () => setDraggingCanvasNode(null);

  // Clicking a node's connector dot arms a pending "from" node; clicking a
  // second (different) node's dot completes the edge. Clicking the same
  // node again, or starting a fresh click while a different source's canvas
  // is armed, resets the pending state instead of connecting.
  const handleCanvasConnectorClick = (source: "detail" | "create", key: string) => {
    if (!pendingCanvasEdgeFrom || pendingCanvasEdgeFrom.source !== source) {
      setPendingCanvasEdgeFrom({ source, nodeId: key });
      return;
    }
    if (pendingCanvasEdgeFrom.nodeId === key) {
      setPendingCanvasEdgeFrom(null);
      return;
    }
    addWorkflowEdgeBetween(pendingCanvasEdgeFrom.nodeId, key, source);
    setPendingCanvasEdgeFrom(null);
  };

  const saveWorkflow = async () => {
    if (!workflowDraft) return;
    if (!workflowDraft.modelOverride) {
      setMessage("Model is required.");
      return;
    }
    // Task-189 slice 3: only enforce flow-graph correctness once the user has
    // actually started wiring edges — a plain linear/legacy workflow with no
    // edges never intended to run as a flow-engine graph at all (it relies on
    // order_index sequencing), so it must not be newly blocked by these checks.
    if (workflowDraft.edges.length > 0) {
      const issues = validateFlowGraph(workflowSteps, stepDefinitions, workflowDraft.edges);
      if (issues.length > 0) {
        setMessage(issues.join(" "));
        return;
      }
    }
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.workflows.saveWorkflow({
        id: workflowDraft.id,
        projectId: workflowDraft.projectId,
        name: workflowDraft.name,
        description: workflowDraft.description,
        isTemplate: workflowDraft.isTemplate,
        modelOverride: workflowDraft.modelOverride,
        reasoningEffortOverride: workflowDraft.reasoningEffortOverride,
        yoloMode: workflowDraft.yoloMode,
        // BUG-NOTE-CP42 #14: pass these through unchanged so a plain edit
        // (e.g. renaming) doesn't null out the workflow's existing policy
        // caps and edge graph — saveWorkflow's repository has no notion of
        // "field omitted, leave unchanged."
        policyCap: workflowDraft.policyCap,
        policyOnCap: workflowDraft.policyOnCap,
        policyExtendBy: workflowDraft.policyExtendBy,
        policyExtendMax: workflowDraft.policyExtendMax,
        edges: workflowDraft.edges,
        // CP-55 P-1 (Task-263/CA-424 pass 3): no editor writes this today —
        // sent back unchanged, same as edges/policy* above, so a plain save
        // (e.g. a rename) can't silently erase a declared acceptance
        // boundary the way an omitted field would (saveWorkflow's repository
        // always overwrites every column it's given).
        acceptanceNodes: workflowDraft.acceptanceNodes,
        steps: workflowSteps.map((step, orderIndex) => ({ ...step, orderIndex })),
      });
      // BUG-282: the workflow's edges are the only home for flow topology, and
      // they were just persisted above. Nothing further to sync onto the step
      // definitions.
      await refresh(saved.id, selectedStepType, { preserveCreateDrafts: true });
      setMessage("Workflow saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save workflow."));
    } finally {
      setBusy(false);
    }
  };

  const saveNewWorkflow = async () => {
    if (!createWorkflowDraft.modelOverride) {
      setMessage("Model is required.");
      return;
    }
    if (createWorkflowDraft.edges.length > 0) {
      const issues = validateFlowGraph(createWorkflowSteps, stepDefinitions, createWorkflowDraft.edges);
      if (issues.length > 0) {
        setMessage(issues.join(" "));
        return;
      }
    }
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.workflows.saveWorkflow({
        projectId: createWorkflowDraft.projectId,
        name: createWorkflowDraft.name.trim() || "New Workflow",
        description: createWorkflowDraft.description,
        isTemplate: createWorkflowDraft.isTemplate,
        modelOverride: createWorkflowDraft.modelOverride,
        reasoningEffortOverride:
          createWorkflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING,
        yoloMode: createWorkflowDraft.yoloMode,
        policyCap: createWorkflowDraft.policyCap,
        policyOnCap: createWorkflowDraft.policyOnCap,
        policyExtendBy: createWorkflowDraft.policyExtendBy,
        policyExtendMax: createWorkflowDraft.policyExtendMax,
        edges: createWorkflowDraft.edges,
        // See the equivalent comment in saveWorkflow above — a brand-new
        // workflow simply has none declared yet (createEmptyWorkflowDraft
        // defaults to []), so this sends the same empty set the DB default
        // already produces; it exists for symmetry with saveWorkflow, not
        // because create needs different behavior.
        acceptanceNodes: createWorkflowDraft.acceptanceNodes,
        steps: createWorkflowSteps.map((step, orderIndex) => ({ ...step, orderIndex })),
      });
      // BUG-282: flow topology lives only on the workflow's edges (persisted
      // above); nothing further to sync onto the step definitions.
      setWorkflowView("list");
      await refresh(saved.id, selectedStepType);
      setMessage("Workflow created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create workflow."));
    } finally {
      setBusy(false);
    }
  };

  const saveStepDefinition = async () => {
    if (!stepDraft) return;
    const requiresModel = stepDefinitionRequiresModel(stepDraft.behaviorId);
    if (requiresModel && !stepDraft.model) {
      setMessage("Model is required.");
      return;
    }
    const agentRefIssue = stepDefinitionAgentRefIssue(stepDraft.behaviorId, stepDraft.agentRef);
    if (agentRefIssue) {
      setMessage(agentRefIssue);
      return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.workflows.saveStepDefinition({
        ...stepDraft,
        // A behavior that doesn't need a model (inline/control) must not
        // silently persist a stale value left over from before the user
        // switched Behavior ID — the fields are hidden in the form, so there
        // is no UI left to clear them manually.
        model: requiresModel ? stepDraft.model : null,
        reasoningEffort: requiresModel ? stepDraft.reasoningEffort : null,
        promptBase:
          stepDraft.promptBase?.trim() ||
          deriveStepPromptBase({
            stepType: stepDraft.stepType,
            name: stepDraft.name,
            description: stepDraft.description,
          }),
      });
      await refresh(selectedWorkflowId, saved.stepType, { preserveCreateDrafts: true });
      setMessage("Step definition saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save step definition."));
    } finally {
      setBusy(false);
    }
  };

  const saveNewStepDefinition = async () => {
    if (!createStepDraft.stepType.trim()) {
      setMessage("Step key is required.");
      return;
    }
    const requiresModel = stepDefinitionRequiresModel(createStepDraft.behaviorId);
    if (requiresModel && !createStepDraft.model) {
      setMessage("Model is required.");
      return;
    }
    const agentRefIssue = stepDefinitionAgentRefIssue(createStepDraft.behaviorId, createStepDraft.agentRef);
    if (agentRefIssue) {
      setMessage(agentRefIssue);
      return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.workflows.saveStepDefinition({
        ...createStepDraft,
        stepType: createStepDraft.stepType.trim(),
        name: createStepDraft.name.trim() || createStepDraft.stepType.trim(),
        description: createStepDraft.description.trim(),
        model: requiresModel ? createStepDraft.model : null,
        reasoningEffort: requiresModel ? createStepDraft.reasoningEffort : null,
        promptBase:
          createStepDraft.promptBase?.trim() ||
          deriveStepPromptBase({
            stepType: createStepDraft.stepType.trim(),
            name: createStepDraft.name,
            description: createStepDraft.description,
          }),
      });
      setStepView("list");
      await refresh(selectedWorkflowId, saved.stepType);
      setMessage("Step definition created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create step definition."));
    } finally {
      setBusy(false);
    }
  };

  const openDeleteWorkflowConfirm = (ids: string[], labels: string[]) => {
    if (ids.length === 0) return;
    setDeleteTarget({ kind: "workflow", ids, labels });
    setDeleteConfirmationText("");
  };

  // Steps have no FK-cascade from workflow_steps back to step_definitions
  // (BUG-236: a workflow's step list is a pure relation), so deleting a step
  // definition that's still referenced by a workflow would leave that
  // workflow with a dangling stepType it can never resolve at runtime. Look
  // up every workflow currently using any of the given step types and fold
  // them into the same confirmation so they're deleted together.
  const openDeleteStepConfirm = async (ids: string[], labels: string[]) => {
    if (ids.length === 0) return;
    // A step still listed by a built-in workflow can't be deleted at all —
    // that workflow is read-only, so there's no cascade that could clear it
    // out first. Block before the usage lookup instead of only relying on
    // the disabled row/button, since bulk selections can mix deletable and
    // blocked steps.
    const blockedIds = ids.filter((id) => stepTypesUsedByBuiltin.has(id));
    if (blockedIds.length > 0) {
      const blockedLabels = blockedIds.map(
        (id) => stepDefinitions.find((step) => step.stepType === id)?.name ?? id,
      );
      setMessage(
        `Can't delete ${blockedLabels.join(", ")}: still used by a built-in workflow, which can't be edited.`,
      );
      return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const usages = await admin.workflows.listWorkflowsUsingSteps(ids);
      const cascadeWorkflowIds = Array.from(new Set(usages.map((usage) => usage.workflowId)));
      const cascadeWorkflows = cascadeWorkflowIds.map((workflowId) => ({
        id: workflowId,
        name: workflows.find((workflow) => workflow.id === workflowId)?.name ?? workflowId,
      }));
      setDeleteTarget({ kind: "step", ids, labels, cascadeWorkflows });
      setDeleteConfirmationText("");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to check step usage before delete."));
    } finally {
      setBusy(false);
    }
  };

  const deleteSelected = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      if (deleteTarget.kind === "workflow") {
        // Independent rows — run the deletes concurrently instead of one at
        // a time so a bulk delete of N workflows takes roughly as long as
        // the slowest single delete, not N times that.
        await Promise.all(deleteTarget.ids.map((id) => admin.workflows.deleteWorkflow(id)));
        setDeleteTarget(null);
        setDeleteConfirmationText("");
        exitBulkSelect();
        await refresh(undefined, selectedStepType);
        setMessage(
          deleteTarget.ids.length > 1 ? `${deleteTarget.ids.length} workflows deleted.` : "Workflow deleted.",
        );
      } else {
        // Cascade workflows must be gone before the step definitions they
        // reference are deleted, but within each phase the rows are
        // independent, so fan each phase out concurrently rather than
        // serializing every single delete.
        await Promise.all(
          deleteTarget.cascadeWorkflows.map((cascadeWorkflow) => admin.workflows.deleteWorkflow(cascadeWorkflow.id)),
        );
        await Promise.all(deleteTarget.ids.map((id) => admin.workflows.deleteStepDefinition(id)));
        setDeleteTarget(null);
        setDeleteConfirmationText("");
        exitBulkSelect();
        await refresh(undefined, undefined);
        const cascadeNote =
          deleteTarget.cascadeWorkflows.length > 0
            ? ` (also removed ${deleteTarget.cascadeWorkflows.length} workflow(s) that used it)`
            : "";
        setMessage(
          (deleteTarget.ids.length > 1
            ? `${deleteTarget.ids.length} step definitions deleted.`
            : "Step definition deleted.") + cascadeNote,
        );
      }
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to delete item."));
    } finally {
      setBusy(false);
    }
  };

  const cloneSelected = async () => {
    if (!cloneTarget) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const cloned = await admin.workflows.cloneWorkflow(cloneTarget.workflowId, cloneName.trim() || `${cloneTarget.sourceName} (copy)`);
      setCloneTarget(null);
      setCloneName("");
      await refresh(cloned.id, selectedStepType, { preserveCreateDrafts: true });
      setMessage("Workflow cloned. You can now edit the copy.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to clone workflow."));
    } finally {
      setBusy(false);
    }
  };

  const renderWorkflowStepsEditor = (
    steps: WorkflowStep[],
    source: "detail" | "create",
    selectedStepTypeValue: string,
    onSelectedStepTypeChange: (value: string) => void,
  ) => {
    const availableSteps =
      source === "detail"
        ? stepDefinitions.filter(
            (definition) => !steps.some((step) => step.stepType === definition.stepType),
          )
        : availableCreateWorkflowSteps;
    // A built-in workflow's steps can't be persisted (Save is hidden for it),
    // so block the mutating actions here too rather than let the user add/
    // remove steps that silently go nowhere.
    const readOnly = source === "detail" && selectedWorkflow?.editable === false;

    return (
      <div className="settings-subpanel workflow-steps-panel">
        <div className="project-panel-head">
          <div>
            <strong>Workflow Steps</strong>
            <p className="project-muted-copy">
              {readOnly
                ? "Read-only: clone this workflow to edit its steps."
                : "Add, remove, and reorder the reusable steps that make up this workflow."}
            </p>
          </div>
        </div>
        <div className="workflow-step-add-row">
          <label className="settings-field workflow-step-add-field">
            <span>Add step</span>
            <select
              disabled={readOnly}
              onChange={(event) => onSelectedStepTypeChange(event.target.value)}
              value={selectedStepTypeValue}
            >
              {availableSteps.length === 0 ? <option value="">No more steps available</option> : null}
              {availableSteps.map((definition) => (
                <option key={definition.stepType} value={definition.stepType}>
                  {definition.name}
                </option>
              ))}
            </select>
          </label>
          <button
            className="secondary-btn"
            disabled={readOnly || !selectedStepTypeValue}
            onClick={() => addWorkflowStep(source)}
            type="button"
          >
            Add Step
          </button>
        </div>
        <div className="settings-list">
          {steps.length === 0 ? (
            <div className="settings-empty">No steps added yet.</div>
          ) : (
            steps.map((step, index) => {
              const stepKey = `${source}:${step.id}`;
              const isExpanded = expandedStepKeys.has(stepKey);
              const definition = stepDefinitions.find((item) => item.stepType === step.stepType);
              return (
              <div className="settings-list-item static workflow-step-card" key={`${step.stepType}-${index}`}>
                <div className="workflow-step-card-head">
                  <button
                    aria-expanded={isExpanded}
                    className="workflow-step-card-toggle"
                    onClick={() => toggleStepExpanded(stepKey)}
                    type="button"
                  >
                    <span className={`workflow-step-card-chevron ${isExpanded ? "expanded" : ""}`}>&#9656;</span>
                    <div>
                      <strong>
                        {index + 1}.{" "}
                        {definition?.nodeId || definition?.agentRef || definition?.name || step.stepType}
                      </strong>
                      <span>{step.stepType}</span>
                    </div>
                  </button>
                  <div className="settings-inline-actions">
                    <button
                      className="secondary-btn project-icon-btn"
                      disabled={readOnly || index === 0}
                      onClick={() => moveWorkflowStep(index, "up", source)}
                      title="Move up"
                      type="button"
                    >
                      ^
                    </button>
                    <button
                      className="secondary-btn project-icon-btn"
                      disabled={readOnly || index === steps.length - 1}
                      onClick={() => moveWorkflowStep(index, "down", source)}
                      title="Move down"
                      type="button"
                    >
                      v
                    </button>
                    <button
                      className="secondary-btn"
                      disabled={readOnly}
                      onClick={() => removeWorkflowStep(index, source)}
                      type="button"
                    >
                      Remove
                    </button>
                  </div>
                </div>
                {isExpanded ? (
                <>
                <div className="settings-grid workflow-step-grid">
                  {/* BUG-164: model/reasoning are no longer per-step overrides — a step's
                      model is always its step type's own step_definitions.model. There is
                      nothing to configure here anymore; edit the step type's catalog entry
                      (below) to change what model it runs on. */}
                  <label className="settings-checkbox">
                    <input
                      checked={step.isEnabled}
                      disabled={readOnly}
                      onChange={(event) =>
                        source === "detail"
                          ? updateWorkflowStep(index, { isEnabled: event.target.checked })
                          : updateCreateWorkflowStep(index, { isEnabled: event.target.checked })
                      }
                      type="checkbox"
                    />
                    <span>Enabled</span>
                  </label>
                  <label className="settings-checkbox">
                    <input
                      checked={step.requiresApproval}
                      disabled={readOnly}
                      onChange={(event) =>
                        source === "detail"
                          ? updateWorkflowStep(index, { requiresApproval: event.target.checked })
                          : updateCreateWorkflowStep(index, {
                              requiresApproval: event.target.checked,
                            })
                      }
                      type="checkbox"
                    />
                    <span>Requires approval</span>
                  </label>
                </div>
                </>
                ) : null}
              </div>
              );
            })
          )}
        </div>
      </div>
    );
  };

  // Task-189 slice 2: the form-list edge editor. `steps` is this workflow's
  // OWN step list (workflowSteps for "detail", createWorkflowSteps for
  // "create") so the from/to pickers only ever offer nodes that are actually
  // part of this flow. A canvas-based graph editor (Task-189 slice 4) will be
  // added as a second, synchronized view over the same `edges` array — not a
  // replacement for this one.
  const renderWorkflowEdgesEditor = (
    edges: WorkflowFlowEdge[],
    steps: WorkflowStep[],
    source: "detail" | "create",
  ) => {
    const readOnly = source === "detail" && selectedWorkflow?.editable === false;
    const nodeOptions = workflowEdgeNodeOptions(steps);
    const toOptions = [...nodeOptions, ...FLOW_EDGE_TERMINALS];
    const nodesMissingId = steps.filter(
      (step) => !stepDefinitions.find((definition) => definition.stepType === step.stepType)?.nodeId,
    );

    return (
      <div className="settings-subpanel workflow-edges-panel">
        <div className="project-panel-head">
          <div>
            <strong>Flow Edges</strong>
            <p className="project-muted-copy">
              {readOnly
                ? "Read-only: clone this workflow to edit its edges."
                : "Wire the flow graph: which node follows which, and under what outcome. " +
                  "Without at least one edge, this flow's steps run in isolation and the " +
                  "engine cannot advance past the entry node."}
            </p>
          </div>
        </div>
        {nodesMissingId.length > 0 ? (
          <div className="settings-warning">
            {nodesMissingId.length} step(s) have no Node ID set on their step type — set a Node
            ID (in the step type's catalog entry) before they can be used as an edge endpoint:{" "}
            {nodesMissingId.map((step) => step.stepType).join(", ")}
          </div>
        ) : null}
        <div className="settings-list">
          {edges.length === 0 ? (
            <div className="settings-empty">No edges yet — add one below.</div>
          ) : (
            edges.map((edge, index) => (
              <div className="settings-list-item static workflow-edge-row" key={index}>
                <label className="settings-field">
                  <span>From</span>
                  <select
                    disabled={readOnly}
                    onChange={(event) => updateWorkflowEdge(index, { from: event.target.value }, source)}
                    value={edge.from}
                  >
                    <option value="">(select a node)</option>
                    {nodeOptions.map((id) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="settings-field">
                  <span>To</span>
                  <select
                    disabled={readOnly}
                    onChange={(event) => updateWorkflowEdge(index, { to: event.target.value }, source)}
                    value={edge.to}
                  >
                    <option value="">(select a node or terminal)</option>
                    {toOptions.map((id) => (
                      <option key={id} value={id}>
                        {FLOW_EDGE_TERMINALS.includes(id) ? `(terminal) ${id}` : id}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="settings-field">
                  <span>When (outcome status)</span>
                  <select
                    disabled={readOnly}
                    onChange={(event) => updateWorkflowEdge(index, { when: event.target.value }, source)}
                    value={edge.when}
                  >
                    <option value="">(select an outcome)</option>
                    {WORKFLOW_EDGE_WHEN_OPTIONS.map((option) => (
                      <option key={option} value={option}>
                        {option}
                      </option>
                    ))}
                    {edge.when && !(WORKFLOW_EDGE_WHEN_OPTIONS as readonly string[]).includes(edge.when) ? (
                      <option value={edge.when}>{edge.when} (current value)</option>
                    ) : null}
                  </select>
                </label>
                <label className="settings-field">
                  <span>Kind</span>
                  <select
                    disabled={readOnly}
                    onChange={(event) => updateWorkflowEdge(index, { kind: event.target.value }, source)}
                    value={edge.kind}
                  >
                    <option value="forward">Forward</option>
                    <option value="back">Back (loop)</option>
                  </select>
                </label>
                <div className="settings-inline-actions">
                  <button
                    className="secondary-btn"
                    disabled={readOnly}
                    onClick={() => removeWorkflowEdge(index, source)}
                    type="button"
                  >
                    Remove
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
        <button
          className="secondary-btn"
          disabled={readOnly || nodeOptions.length === 0}
          onClick={() => addWorkflowEdge(source)}
          type="button"
        >
          + Add Edge
        </button>
      </div>
    );
  };

  // Task-189 slice 4: the visual canvas. It reads/writes the SAME
  // `edges` array as renderWorkflowEdgesEditor above (passed in verbatim) —
  // there is no separate canvas-edge state, so editing an edge's "when" in
  // the form-list is reflected here immediately and vice versa. Only node
  // *position* is canvas-local view state (canvasPositions), never
  // persisted.
  const renderWorkflowFlowCanvas = (
    edges: WorkflowFlowEdge[],
    steps: WorkflowStep[],
    source: "detail" | "create",
  ) => {
    const readOnly = source === "detail" && selectedWorkflow?.editable === false;
    const nodeOptions = workflowEdgeNodeOptions(steps);
    const nodeKeys = [...nodeOptions, ...FLOW_EDGE_TERMINALS];
    const layout = canvasLayoutFor(source, nodeKeys);
    const pending = pendingCanvasEdgeFrom?.source === source ? pendingCanvasEdgeFrom : null;

    return (
      <div className="settings-subpanel workflow-canvas-panel">
        <div className="project-panel-head">
          <div>
            <strong>Flow Canvas</strong>
            <p className="project-muted-copy">
              Drag a node to arrange it. Click a node's dot, then another node's dot, to draw an
              edge between them — it's added to the Flow Edges list above.
            </p>
          </div>
        </div>
        {pending ? (
          <div className="settings-warning">
            Drawing an edge from &quot;{pending.nodeId}&quot; — click another node&apos;s dot to
            connect it, or{" "}
            <button
              className="link-btn"
              onClick={() => setPendingCanvasEdgeFrom(null)}
              type="button"
            >
              cancel
            </button>
            .
          </div>
        ) : null}
        {nodeOptions.length === 0 ? (
          <div className="settings-empty">
            No nodes with a Node ID yet — set a Node ID on a step type before it can appear here.
          </div>
        ) : (
          <div
            className="workflow-canvas-surface"
            onPointerLeave={handleCanvasSurfacePointerUp}
            onPointerMove={handleCanvasSurfacePointerMove(source)}
            onPointerUp={handleCanvasSurfacePointerUp}
          >
            <svg className="workflow-canvas-edges">
              <defs>
                <marker
                  id={`workflow-canvas-arrow-${source}`}
                  markerHeight="8"
                  markerWidth="8"
                  orient="auto"
                  refX="7"
                  refY="4"
                >
                  <path className="workflow-canvas-arrowhead" d="M0,0 L8,4 L0,8 z" />
                </marker>
              </defs>
              {edges.map((edge, index) => {
                const from = layout[edge.from];
                const to = layout[edge.to];
                if (!from || !to) return null;
                return (
                  <g key={index}>
                    <line
                      className={edge.kind === "back" ? "workflow-canvas-edge back" : "workflow-canvas-edge"}
                      markerEnd={`url(#workflow-canvas-arrow-${source})`}
                      x1={from.x}
                      y1={from.y}
                      x2={to.x}
                      y2={to.y}
                    />
                    <text className="workflow-canvas-edge-label" x={(from.x + to.x) / 2} y={(from.y + to.y) / 2 - 6}>
                      {edge.when}
                    </text>
                  </g>
                );
              })}
            </svg>
            {nodeKeys.map((key) => {
              const isTerminal = FLOW_EDGE_TERMINALS.includes(key);
              const position = layout[key];
              return (
                <div
                  className={[
                    "workflow-canvas-node",
                    isTerminal ? "terminal" : "",
                    pending?.nodeId === key ? "connecting" : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                  key={key}
                  onPointerDown={handleCanvasNodePointerDown(source, key)}
                  style={{ left: position.x, top: position.y }}
                >
                  <span>{key}</span>
                  <button
                    className="workflow-canvas-connector"
                    disabled={readOnly}
                    onClick={() => handleCanvasConnectorClick(source, key)}
                    title="Click, then click another node, to draw an edge"
                    type="button"
                  >
                    ●
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </div>
    );
  };

  const renderStepDefinitionForm = (
    draft: StepDefinition,
    onChange: (next: StepDefinition) => void,
    mode: "detail" | "create",
  ) => {
    const requiredSkillsText = draft.requiredSkills.join(", ");
    return (
      <div className="settings-grid">
        <label className="settings-field">
          <span>Step key</span>
          <input
            disabled={mode === "detail"}
            onChange={(event) => {
              const nextStepType = event.target.value;
              // Node ID auto-follows the Step key while it's untouched (still
              // empty, or still equal to the previous Step key value) — Step
              // key is only ever editable at creation time (disabled above in
              // "detail" mode), so this only ever runs while a new
              // step-definition is being drafted. Once the user types their
              // own Node ID, it stops matching the old Step key and this
              // no-ops, leaving their choice alone.
              const nodeIdFollowsStepType = !draft.nodeId || draft.nodeId === draft.stepType;
              onChange({
                ...draft,
                stepType: nextStepType,
                nodeId: nodeIdFollowsStepType ? nextStepType : draft.nodeId,
              });
            }}
            value={draft.stepType}
          />
        </label>
        <label className="settings-field">
          <span>Display name</span>
          <input
            onChange={(event) => onChange({ ...draft, name: event.target.value })}
            value={draft.name}
          />
        </label>
        <label className="settings-field settings-field-full">
          <span>Description</span>
          <textarea
            onChange={(event) => onChange({ ...draft, description: event.target.value })}
            value={draft.description}
          />
        </label>
        <div className="settings-field settings-field-full">
          <div className="workflow-chip-row">
            <span>Required MCPs</span>
            <button
              className="secondary-btn workflow-chip-add-btn"
              onClick={() => setPickerModal({ kind: "mcp", mode })}
              type="button"
            >
              + Add
            </button>
          </div>
          {draft.requiredMcps.length === 0 ? (
            <div className="workflow-chip-empty">None selected</div>
          ) : (
            <div className="workflow-chips">
              {draft.requiredMcps.map((mcp) => (
                <span className="workflow-chip" key={mcp}>
                  <span>{toTitleCase(mcp)}</span>
                  <button
                    aria-label={`Remove ${mcp}`}
                    className="workflow-chip-remove"
                    onClick={() =>
                      onChange({ ...draft, requiredMcps: draft.requiredMcps.filter((m) => m !== mcp) })
                    }
                    type="button"
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          )}
        </div>
        {draft.requiredMcps.includes("google_drive") ? (
          <label className="settings-field">
            <span>Google Drive access</span>
            <select
              onChange={(event) =>
                onChange({
                  ...draft,
                  mcpAccessMode:
                    event.target.value === "read_write" ? "read_write" : "read_only",
                })
              }
              value={draft.mcpAccessMode}
            >
              <option value="read_only">Read only</option>
              <option value="read_write">Read + write</option>
            </select>
          </label>
        ) : null}
        <label className="settings-field">
          <span>Required skills</span>
          <input
            onChange={(event) =>
              onChange({
                ...draft,
                requiredSkills: event.target.value
                  .split(",")
                  .map((item) => item.trim())
                  .filter(Boolean),
              })
            }
            placeholder="planner-skill, coding-skill"
            value={requiredSkillsText}
          />
        </label>
        <label className="settings-field">
          <span>Team role</span>
          <input
            onChange={(event) =>
              onChange({ ...draft, teamRole: event.target.value || null })
            }
            value={draft.teamRole ?? ""}
          />
        </label>
        <label className="settings-field settings-field-full">
          <span>Prompt base</span>
          <textarea
            onChange={(event) =>
              onChange({ ...draft, promptBase: event.target.value })
            }
            placeholder="Describe the execution intent for this step."
            value={draft.promptBase ?? ""}
          />
        </label>
        {stepDefinitionRequiresModel(draft.behaviorId) ? (
          <>
            <label className="settings-field">
              <span>Model</span>
              <select
                onChange={(event) => onChange({ ...draft, model: event.target.value || null })}
                value={draft.model ?? ""}
              >
                <option value="">Select a model...</option>
                {modelOptions.map((model) => (
                  <option key={model.value} value={model.value}>
                    {model.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="settings-field">
              <span>Reasoning effort</span>
              <select
                onChange={(event) =>
                  onChange({ ...draft, reasoningEffort: event.target.value })
                }
                value={draft.reasoningEffort ?? DEFAULT_REASONING}
              >
                {REASONING_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
          </>
        ) : (
          // Inline/control behaviors (context.produce, command.validate,
          // artifact.audit_draft, telegram.notify, ...) are Go-deterministic
          // and never spawn a provider turn, so Model/Reasoning effort do not
          // apply — hidden rather than forcing a meaningless choice.
          <div className="settings-field settings-field-full project-muted-copy">
            Model / Reasoning effort not applicable — Behavior "{draft.behaviorId}" runs inline,
            with no provider turn.
          </div>
        )}
        <label className="settings-field">
          <span>Behavior ID</span>
          <select
            onChange={(event) => onChange({ ...draft, behaviorId: event.target.value || null })}
            value={draft.behaviorId ?? ""}
          >
            <option value="">(none / inherit from step type)</option>
            {FLOW_BEHAVIOR_OPTIONS.map((b) => (
              <option key={b.id} value={b.id}>
                {b.label}
              </option>
            ))}
          </select>
        </label>
        <label className="settings-field">
          <span>Preview agents for project</span>
          <select
            onChange={(event) => setAgentPreviewProjectId(event.target.value)}
            value={agentPreviewProjectId}
          >
            <option value="">Workspace global (built-in + provider-home only)</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </label>
        <label className="settings-field">
          <span>Agent ref</span>
          <select
            onChange={(event) => onChange({ ...draft, agentRef: event.target.value || null })}
            value={draft.agentRef ?? ""}
          >
            <option value="">(none)</option>
            {agentOptions.map((agent) => {
              // Prefer the full path so same-named agents across sources
              // (project .claude/agents vs provider-home .codex/agents) stay
              // distinct. Built-in flow-pack agents have no path, so they fall
              // back to their bare name. Runtime resolution (spawnChildRun)
              // matches either form, so both work.
              const value = agent.path || agent.name;
              return (
                <option key={value} value={value}>
                  {agent.name} ({agent.source})
                </option>
              );
            })}
            {draft.agentRef && !agentOptions.some((agent) => (agent.path || agent.name) === draft.agentRef) ? (
              <option value={draft.agentRef}>{draft.agentRef} (current value, not in this project's list)</option>
            ) : null}
          </select>
        </label>
        <label className="settings-field">
          <span>Lifecycle</span>
          <select
            onChange={(event) => onChange({ ...draft, nodeLifecycle: event.target.value || null })}
            value={draft.nodeLifecycle ?? ""}
          >
            <option value="">Default (reinvoke)</option>
            <option value="reinvoke">Reinvoke</option>
            <option value="spawn">Spawn new</option>
            <option value="once">Once</option>
          </select>
        </label>
        <label className="settings-field">
          <span>Prompt template ref</span>
          <input
            onChange={(event) => onChange({ ...draft, promptTemplateRef: event.target.value || null })}
            placeholder="prompts/example.md"
            value={draft.promptTemplateRef ?? ""}
          />
        </label>
        <label className="settings-field">
          <span>Context ref</span>
          <input
            onChange={(event) => onChange({ ...draft, contextRef: event.target.value || null })}
            placeholder="contexts/example.yaml"
            value={draft.contextRef ?? ""}
          />
        </label>
        {/* Task-222 / CP-45 residual: default Step UX is artifact-only.
            Raw step contextSources remains a D-6 transition/fallback data field
            (runner still honors it when no context_artifact binding wins). Hide
            the authoring chips by default; only surface a legacy readout when
            saved data still has raw ids so old flows are not silent. */}
        {draft.contextSources.length > 0 ? (
          <div className="settings-field settings-field-full">
            <div className="workflow-chip-row">
              <span>Legacy context sources (clear only)</span>
            </div>
            <small className="settings-field-help">
              Transition data from CP-44/Task-196. Prefer Artifacts tab →{" "}
              <code>context_artifact.v1</code> instance sources, then bind under Artifact
              Inputs/Outputs. Runner still applies these raw ids only when no artifact
              binding supplies sources (SD-23 D-6). Clear by removing chips if you no longer need them.
            </small>
            <div className="workflow-chips">
              {draft.contextSources.map((sourceId) => (
                <span className="workflow-chip" key={sourceId}>
                  <span>
                    {contextSourceOptions.find((option) => option.id === sourceId)?.label ?? sourceId}
                  </span>
                  <button
                    aria-label={`Remove legacy ${sourceId}`}
                    className="workflow-chip-remove"
                    onClick={() =>
                      onChange({
                        ...draft,
                        contextSources: draft.contextSources.filter((id) => id !== sourceId),
                      })
                    }
                    type="button"
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          </div>
        ) : null}
        {/* CP-45/SD-23 Task-200 + Task-222: primary Step authoring is typed
            artifact instance bindings. Raw step contextSources is fallback-only
            data (D-6), not the default UX. A step attaches a LIST of instances
            per direction (D-1); picker options are filtered by behavior (D-7 —
            context.produce → context_artifact; other behaviors → non-context,
            e.g. file_artifact). */}
        {(["output", "input"] as const).map((direction) => {
          const bindings = draft.artifactBindings.filter((b) => b.direction === direction);
          return (
            <div className="settings-field settings-field-full" key={direction}>
              <div className="workflow-chip-row">
                <span>Artifact {direction === "input" ? "Inputs" : "Outputs"}</span>
                <button
                  className="secondary-btn workflow-chip-add-btn"
                  onClick={() =>
                    setPickerModal({
                      kind: direction === "input" ? "artifact-binding-input" : "artifact-binding-output",
                      mode,
                    })
                  }
                  type="button"
                >
                  + Add
                </button>
              </div>
              <small className="settings-field-help">
                {direction === "output"
                  ? "Output: this step must produce the bound file_artifact path(s) (write contract)."
                  : "Input: prompt mentions bound file path(s); agent reads via tools (no full-file paste)."}
              </small>
              {bindings.length === 0 ? (
                <div className="workflow-chip-empty">None bound</div>
              ) : (
                <div className="workflow-chips">
                  {bindings.map((binding) => (
                    <span className="workflow-chip" key={binding.artifactInstanceId}>
                      <span>
                        {artifactInstanceLabel(
                          artifactInstances.find((i) => i.id === binding.artifactInstanceId),
                          binding.artifactInstanceId,
                        )}
                      </span>
                      <button
                        aria-label={`Remove ${binding.artifactInstanceId}`}
                        className="workflow-chip-remove"
                        onClick={() =>
                          onChange({
                            ...draft,
                            artifactBindings: draft.artifactBindings.filter(
                              (b) => !(b.direction === direction && b.artifactInstanceId === binding.artifactInstanceId),
                            ),
                          })
                        }
                        type="button"
                      >
                        ×
                      </button>
                    </span>
                  ))}
                </div>
              )}
            </div>
          );
        })}
        <label className="settings-checkbox settings-field-full" title="Flow/Workflow launches always run YOLO — the gate/approval machinery here is not robust against a paused approval mid-flow. Use Normal Chat for gated (YOLO=off) runs.">
          <input checked disabled type="checkbox" />
          <span>YOLO for single-step runs (always on for Flow mode)</span>
        </label>
      </div>
    );
  };

  const renderPickerModal = () => {
    if (!pickerModal) return null;
    const isDetail = pickerModal.mode === "detail";
    const draftSource = isDetail ? stepDraft : createStepDraft;
    const updateDraft = (patch: Partial<StepDefinition>) => {
      if (isDetail) {
        setStepDraft((d) => (d ? { ...d, ...patch } : d));
      } else {
        setCreateStepDraft((d) => ({ ...d, ...patch }));
      }
    };

    const title =
      pickerModal.kind === "context-source"
        ? "Context Sources"
        : pickerModal.kind === "artifact-binding-input"
          ? "Artifact Inputs"
          : pickerModal.kind === "artifact-binding-output"
            ? "Artifact Outputs"
            : "Required MCPs";

    return (
      <div className="settings-modal-backdrop" role="presentation">
        <div className="settings-modal">
          <div className="project-create-head">
            <div>
              <div className="settings-eyebrow">Select</div>
              <h3>{title}</h3>
            </div>
            <button
              className="secondary-btn"
              onClick={() => setPickerModal(null)}
              type="button"
            >
              Done
            </button>
          </div>
          {pickerModal.kind === "mcp" ? (
            <>
              <div className="workflow-choice-grid workflow-picker-grid">
                {integrationTypes.map((mcpType) => (
                  <label className="workflow-choice-card" key={mcpType}>
                    <input
                      checked={draftSource?.requiredMcps.includes(mcpType) ?? false}
                      onChange={() => {
                        if (!draftSource) return;
                        const nextMcps = toggleString(draftSource.requiredMcps, mcpType);
                        updateDraft({
                          requiredMcps: nextMcps,
                          mcpAccessMode:
                            mcpType === "google_drive" ||
                            draftSource.requiredMcps.includes("google_drive")
                              ? draftSource.mcpAccessMode
                              : "read_only",
                        });
                      }}
                      type="checkbox"
                    />
                    <span>
                      <strong>{toTitleCase(mcpType)}</strong>
                      <small>{mcpType}</small>
                    </span>
                  </label>
                ))}
              </div>
              {draftSource?.requiredMcps.includes("google_drive") ? (
                <label className="settings-field workflow-picker-sub">
                  <span>Google Drive access</span>
                  <select
                    onChange={(event) =>
                      updateDraft({
                        mcpAccessMode:
                          event.target.value === "read_write" ? "read_write" : "read_only",
                      })
                    }
                    value={draftSource.mcpAccessMode}
                  >
                    <option value="read_only">Read only</option>
                    <option value="read_write">Read + write</option>
                  </select>
                </label>
              ) : null}
            </>
          ) : pickerModal.kind === "context-source" ? (
            <div className="workflow-choice-grid workflow-picker-grid">
              {contextSourceOptions.map((option) => (
                <label className="workflow-choice-card" key={option.id}>
                  <input
                    checked={draftSource?.contextSources.includes(option.id) ?? false}
                    onChange={() => {
                      if (!draftSource) return;
                      updateDraft({ contextSources: toggleString(draftSource.contextSources, option.id) });
                    }}
                    type="checkbox"
                  />
                  <span>
                    <strong>{option.label}</strong>
                    <small>{option.id}</small>
                  </span>
                </label>
              ))}
            </div>
          ) : pickerModal.kind === "artifact-binding-input" || pickerModal.kind === "artifact-binding-output" ? (
            // CP-45/SD-23 Task-200: options are filtered to instances whose
            // type is compatible with this step's behavior (D-7), never the
            // full unfiltered instance list.
            (() => {
              const direction = pickerModal.kind === "artifact-binding-input" ? "input" : "output";
              const options = compatibleArtifactInstancesFor(draftSource?.behaviorId, artifactInstances, direction);
              if (options.length === 0) {
                return (
                  <div className="settings-empty">
                    No compatible artifact instances yet — create one in the Artifacts tab.
                  </div>
                );
              }
              return (
                <div className="workflow-choice-grid workflow-picker-grid">
                  {options.map((instance) => {
                    const bound = draftSource?.artifactBindings.some(
                      (b) => b.direction === direction && b.artifactInstanceId === instance.id,
                    );
                    return (
                      <label className="workflow-choice-card" key={instance.id}>
                        <input
                          checked={bound ?? false}
                          onChange={() => {
                            if (!draftSource) return;
                            const existing = draftSource.artifactBindings.filter(
                              (b) => !(b.direction === direction && b.artifactInstanceId === instance.id),
                            );
                            const next: StepArtifactBinding[] = bound
                              ? existing
                              : [
                                  ...existing,
                                  {
                                    id: "",
                                    direction,
                                    slotName: "",
                                    artifactInstanceId: instance.id,
                                    required: true,
                                    position: draftSource.artifactBindings.filter((b) => b.direction === direction).length,
                                    createdAt: "",
                                  },
                                ];
                            updateDraft({ artifactBindings: next });
                          }}
                          type="checkbox"
                        />
                        <span>
                          <strong>{artifactInstanceLabel(instance, instance.id)}</strong>
                          <small>{instance.artifactTypeId}</small>
                        </span>
                      </label>
                    );
                  })}
                </div>
              );
            })()
          ) : null}
        </div>
      </div>
    );
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Workflows/Steps</div>
          <h2>Workflows/Steps</h2>
          <p>Manage workflow definitions and reusable step definitions in separate create and detail views.</p>
        </div>
        <div className="header-tabs">
          <button
            className={`header-tab ${tab === "workflows" ? "active" : ""}`}
            onClick={() => {
              setTab("workflows");
              exitBulkSelect();
            }}
            type="button"
          >
            Workflows
          </button>
          <button
            className={`header-tab ${tab === "steps" ? "active" : ""}`}
            onClick={() => {
              setTab("steps");
              exitBulkSelect();
            }}
            type="button"
          >
            Steps
          </button>
          <button
            className={`header-tab ${tab === "artifacts" ? "active" : ""}`}
            onClick={() => {
              setTab("artifacts");
              exitBulkSelect();
            }}
            type="button"
          >
            Artifacts
          </button>
        </div>
      </div>

      {message ? <div className={`settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`}>{message}</div> : null}

      {tab === "artifacts" ? (
        <ArtifactsTabContent
          artifactInstanceDraft={artifactInstanceDraft}
          artifactInstances={artifactInstances}
          artifactInstanceView={artifactInstanceView}
          artifactTypes={artifactTypes}
          busy={busy}
          onCreateNew={(artifactTypeId) => {
            setArtifactInstanceDraft({
              id: "",
              projectId: null,
              artifactTypeId,
              name: "",
              description: "",
              configJson:
                artifactTypeId === "file_artifact.v1"
                  ? emptyFileArtifactConfig()
                  : artifactTypeId === "telegram.v1"
                    ? { chatId: "", messageTemplate: "" }
                    : { sources: [] },
              isBuiltin: false,
              status: "active",
              createdAt: "",
              updatedAt: "",
            });
            setArtifactInstanceView("create");
          }}
          onDelete={async (instanceId) => {
            setBusy(true);
            try {
              const admin = await getAdminUseCases();
              await admin.workflows.deleteArtifactInstance(instanceId);
              await refresh(selectedWorkflowId, selectedStepType, { preserveCreateDrafts: true });
              setMessage("Artifact instance deleted.");
            } catch (error) {
              setMessage(toErrorMessage(error, "Unable to delete artifact instance."));
            } finally {
              setBusy(false);
            }
          }}
          onSave={async () => {
            if (!artifactInstanceDraft) return;
            setBusy(true);
            try {
              const admin = await getAdminUseCases();
              const draftToSave =
                artifactInstanceDraft.artifactTypeId === "file_artifact.v1"
                  ? {
                      ...artifactInstanceDraft,
                      configJson: normalizeFileArtifactConfigForSave(
                        artifactInstanceDraft.configJson as Record<string, unknown>,
                      ),
                    }
                  : artifactInstanceDraft;
              await admin.workflows.saveArtifactInstance(draftToSave);
              await refresh(selectedWorkflowId, selectedStepType, { preserveCreateDrafts: true });
              setArtifactInstanceView("list");
              setArtifactInstanceDraft(null);
              setMessage("Artifact instance saved.");
            } catch (error) {
              setMessage(toErrorMessage(error, "Unable to save artifact instance."));
            } finally {
              setBusy(false);
            }
          }}
          onSelect={(instance) => {
            setArtifactInstanceDraft(instance);
            setArtifactInstanceView("create");
          }}
          setArtifactInstanceDraft={setArtifactInstanceDraft}
          setArtifactInstanceView={setArtifactInstanceView}
        />
      ) : tab === "workflows" ? (
        workflowView === "create" ? (
          <div className="workflow-editor-column">
            <div className="settings-subpanel">
              <div className="project-create-back-row">
                <button
                  aria-label="Back to workflows"
                  className="secondary-btn project-icon-btn"
                  onClick={() => setWorkflowView("list")}
                  title="Back to workflows"
                  type="button"
                >
                  &lt;
                </button>
              </div>
              <div className="project-create-head">
                <div>
                  <div className="settings-eyebrow">Create Workflow</div>
                  <h3>New Workflow</h3>
                  <p className="project-muted-copy">
                    Create first, then return to the list/detail layout for future edits.
                  </p>
                </div>
                <button
                  className="primary-btn"
                  disabled={busy || !createWorkflowDraft.name.trim()}
                  onClick={() => void saveNewWorkflow()}
                  type="button"
                >
                  Save Workflow
                </button>
              </div>
              <div className="settings-grid">
                <label className="settings-field">
                  <span>Owner project</span>
                  <select
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        projectId: event.target.value || null,
                      }))
                    }
                    value={createWorkflowDraft.projectId ?? ""}
                  >
                    <option value="">Workspace global</option>
                    {projects.map((project) => (
                      <option key={project.id} value={project.id}>
                        {project.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="settings-field">
                  <span>Name</span>
                  <input
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        name: event.target.value,
                      }))
                    }
                    value={createWorkflowDraft.name}
                  />
                </label>
                <label className="settings-field settings-field-full">
                  <span>Description</span>
                  <textarea
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        description: event.target.value,
                      }))
                    }
                    value={createWorkflowDraft.description}
                  />
                </label>
                <label className="settings-field">
                  <span>Model override</span>
                  <select
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        modelOverride: event.target.value,
                      }))
                    }
                    value={createWorkflowDraft.modelOverride ?? ""}
                  >
                    <option value="">Select a model...</option>
                    {modelOptions.map((model) => (
                      <option key={model.value} value={model.value}>
                        {model.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="settings-field">
                  <span>Reasoning effort</span>
                  <select
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        reasoningEffortOverride: event.target.value,
                      }))
                    }
                    value={createWorkflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING}
                  >
                    {REASONING_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="settings-checkbox settings-field-full" title="Flow/Workflow launches always run YOLO — the gate/approval machinery here is not robust against a paused approval mid-flow. Use Normal Chat for gated (YOLO=off) runs.">
                  <input checked disabled type="checkbox" />
                  <span>YOLO mode (always on for Flow mode)</span>
                </label>
                <label className="settings-field">
                  <span>Cap (max rounds before blocking)</span>
                  <input
                    min={1}
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        policyCap: event.target.value ? Number(event.target.value) : null,
                      }))
                    }
                    placeholder="3 (default)"
                    type="number"
                    value={createWorkflowDraft.policyCap ?? ""}
                  />
                </label>
                <label className="settings-field">
                  <span>Extend by (rounds added when a cap is extended)</span>
                  <input
                    min={1}
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        policyExtendBy: event.target.value ? Number(event.target.value) : null,
                      }))
                    }
                    placeholder="2 (default)"
                    type="number"
                    value={createWorkflowDraft.policyExtendBy ?? ""}
                  />
                </label>
              </div>
            </div>
            {renderWorkflowStepsEditor(
              createWorkflowSteps,
              "create",
              createWorkflowStepType,
              setCreateWorkflowStepType,
            )}
            {renderWorkflowEdgesEditor(createWorkflowDraft.edges, createWorkflowSteps, "create")}
            {renderWorkflowFlowCanvas(createWorkflowDraft.edges, createWorkflowSteps, "create")}
          </div>
        ) : (
          <div className="project-layout">
            <aside className="settings-subpanel project-registry-panel">
              <div className="project-registry-head">
                <div>
                  <div className="settings-eyebrow">Workflow Registry</div>
                  <h3>Definitions</h3>
                  <p className="project-muted-copy">
                    Select a workflow on the left to edit it on the right.
                  </p>
                </div>
                <button
                  aria-label="Create workflow"
                  className="primary-btn project-create-fab"
                  onClick={startCreateWorkflow}
                  type="button"
                >
                  +
                </button>
              </div>
              {bulkSelect?.kind === "workflow" ? (
                <div className="project-bulk-bar">
                  <span>{bulkSelect.ids.size} selected</span>
                  <div className="settings-inline-actions">
                    <button className="secondary-btn" onClick={exitBulkSelect} type="button">
                      Cancel
                    </button>
                    <button
                      className="secondary-btn danger-btn"
                      disabled={bulkSelect.ids.size === 0}
                      onClick={() => {
                        const ids = Array.from(bulkSelect.ids);
                        openDeleteWorkflowConfirm(
                          ids,
                          ids.map((id) => workflows.find((workflow) => workflow.id === id)?.name ?? id),
                        );
                      }}
                      type="button"
                    >
                      Delete {bulkSelect.ids.size}
                    </button>
                  </div>
                </div>
              ) : null}
              <div className="settings-list">
                {workflows.length === 0 ? (
                  <div className="settings-empty">No workflows found. Use the + button to create one.</div>
                ) : (
                  workflows.map((workflow) => {
                    const bulkModeActive = bulkSelect?.kind === "workflow";
                    const isBulkSelected = bulkModeActive && bulkSelect.ids.has(workflow.id);
                    // Built-ins can't be deleted (Delete is disabled for them
                    // below too), so long-press on one is a no-op instead of
                    // starting a select mode that can only ever fail to delete.
                    const longPressSelectable = workflow.editable !== false;
                    return (
                      <button
                        className={`settings-list-item ${
                          workflow.id === selectedWorkflowId && !bulkModeActive ? "active" : ""
                        } ${isBulkSelected ? "bulk-selected" : ""} ${bulkModeActive ? "checkable" : ""} ${
                          !longPressSelectable ? "not-deletable" : ""
                        }`}
                        key={workflow.id}
                        onClick={() => {
                          if (consumeLongPressClick()) return;
                        if (bulkModeActive) {
                          if (longPressSelectable) toggleBulkSelected(workflow.id);
                          return;
                        }
                          void selectWorkflowDefinition(workflow.id);
                          setMessage(null);
                        }}
                        onPointerCancel={clearLongPressTimer}
                        onPointerDown={() => {
                          if (longPressSelectable) startLongPress("workflow", workflow.id);
                        }}
                        onPointerLeave={clearLongPressTimer}
                        onPointerUp={clearLongPressTimer}
                        title={longPressSelectable ? undefined : "Built-in workflows can't be deleted."}
                        type="button"
                      >
                        {bulkModeActive ? (
                          <input
                            aria-hidden="true"
                            checked={isBulkSelected}
                            className="settings-bulk-checkbox"
                            disabled={!longPressSelectable}
                            readOnly
                            tabIndex={-1}
                            type="checkbox"
                          />
                        ) : null}
                        <div>
                          <strong>
                            {workflow.name}
                            {workflow.isBuiltin ? <span className="settings-badge">Built-in</span> : null}
                          </strong>
                          <span>
                            {workflow.projectId
                              ? projects.find((project) => project.id === workflow.projectId)?.name ??
                                workflow.projectId
                              : "Workspace global"}{" "}
                            / {formatTimestamp(workflow.updatedAt)}
                          </span>
                          {workflow.isBuiltin ? (
                            <span className="settings-list-item-meta">
                              {workflow.packId ?? "unknown pack"}
                              {workflow.packVersion ? ` v${workflow.packVersion}` : ""}
                              {harnessLabelFor(workflow.packFlowId)
                                ? ` · ${harnessLabelFor(workflow.packFlowId)}`
                                : ""}
                              {workflow.selectableIn.length > 0
                                ? ` · selectable in: ${workflow.selectableIn.join(", ")}`
                                : ""}
                            </span>
                          ) : null}
                        </div>
                      </button>
                    );
                  })
                )}
              </div>
            </aside>

            <div className="project-detail-column">
              {workflowDraft ? (
                <>
                  <div className="settings-subpanel project-detail-panel">
                    <div className="project-create-head">
                      <div>
                        <div className="settings-eyebrow">
                          Workflow Detail
                          {selectedWorkflow?.isBuiltin ? <span className="settings-badge">Built-in</span> : null}
                        </div>
                        <h3>{workflowDraft.name || "Untitled Workflow"}</h3>
                        {(() => {
                          const harnessLabel = harnessLabelFor(selectedWorkflow?.packFlowId);
                          if (!harnessLabel) return null;
                          return (
                            <p className="project-muted-copy">
                              {harnessLabel} — {HARNESS_LABELS[selectedWorkflow?.packFlowId ?? ""]?.description}
                            </p>
                          );
                        })()}
                        <p className="project-muted-copy">
                          {selectedWorkflow?.editable === false
                            ? "This is a built-in template and cannot be edited directly. Clone it to make changes."
                            : "Edit definition fields here. Save only becomes active after a change."}
                        </p>
                        {selectedWorkflow?.isBuiltin ? (
                          <p className="project-muted-copy settings-list-item-meta">
                            Pack: {selectedWorkflow.packId ?? "unknown"}
                            {selectedWorkflow.packVersion ? ` v${selectedWorkflow.packVersion}` : ""} · Selectable
                            in: {selectedWorkflow.selectableIn.length > 0 ? selectedWorkflow.selectableIn.join(", ") : "none"}
                            {selectedWorkflow.chatBaseline ? " (chat baseline, always on)" : ""}
                          </p>
                        ) : null}
                        {/* CP-55 P-1 (Task-263/CA-424): display-only — no flow declares an
                            agent.code writer yet (P-1 migrates none), so there is nothing to
                            author here today; an editor is deferred to whichever slice first
                            lets a user author their own agent.code node. Shown for both
                            built-in and cloned/user workflows so preservation through
                            save/clone is visible, not just internally correct. */}
                        {workflowDraft.acceptanceNodes.length > 0 ? (
                          <p className="project-muted-copy settings-list-item-meta">
                            Acceptance nodes (read-only in this UI): {workflowDraft.acceptanceNodes.join(", ")}
                          </p>
                        ) : null}
                      </div>
                      <div className="settings-inline-actions">
                        {selectedWorkflow?.cloneable !== false ? (
                          <button
                            className="secondary-btn"
                            onClick={() => {
                              setCloneTarget({ workflowId: selectedWorkflowId, sourceName: workflowDraft.name || "Untitled Workflow" });
                              setCloneName(`${workflowDraft.name || "Untitled Workflow"} (copy)`);
                            }}
                            type="button"
                          >
                            Clone
                          </button>
                        ) : null}
                        <button
                          className="secondary-btn"
                          disabled={selectedWorkflow?.editable === false}
                          onClick={() =>
                            openDeleteWorkflowConfirm(
                              [selectedWorkflowId],
                              [workflowDraft.name || "this workflow"],
                            )
                          }
                          title={
                            selectedWorkflow?.editable === false
                              ? "Built-in workflows can't be deleted."
                              : undefined
                          }
                          type="button"
                        >
                          Delete
                        </button>
                        <button
                          className="primary-btn"
                          disabled={busy || !workflowDirty}
                          onClick={() => void saveWorkflow()}
                          type="button"
                        >
                          Save Workflow
                        </button>
                      </div>
                    </div>
                    <div className="settings-grid">
                      <label className="settings-field">
                        <span>Owner project</span>
                        <select
                          disabled={workflowDetailReadOnly}
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current
                                ? { ...current, projectId: event.target.value || null }
                                : current,
                            )
                          }
                          value={workflowDraft.projectId ?? ""}
                        >
                          <option value="">Workspace global</option>
                          {projects.map((project) => (
                            <option key={project.id} value={project.id}>
                              {project.name}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="settings-field">
                        <span>Name</span>
                        <input
                          disabled={workflowDetailReadOnly}
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current ? { ...current, name: event.target.value } : current,
                            )
                          }
                          value={workflowDraft.name}
                        />
                      </label>
                      <label className="settings-field settings-field-full">
                        <span>Description</span>
                        <textarea
                          disabled={workflowDetailReadOnly}
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current ? { ...current, description: event.target.value } : current,
                            )
                          }
                        />
                      </label>
                      <label className="settings-field">
                        <span>Model override</span>
                        <select
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current
                                ? { ...current, modelOverride: event.target.value }
                                : current,
                            )
                          }
                          value={workflowDraft.modelOverride ?? ""}
                        >
                          <option value="">Select a model...</option>
                          {modelOptions.map((model) => (
                            <option key={model.value} value={model.value}>
                              {model.label}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="settings-field">
                        <span>Reasoning effort</span>
                        <select
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current
                                ? {
                                    ...current,
                                    reasoningEffortOverride: event.target.value,
                                  }
                                : current,
                            )
                          }
                          value={workflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING}
                        >
                          {REASONING_OPTIONS.map((option) => (
                            <option key={option.value} value={option.value}>
                              {option.label}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="settings-checkbox settings-field-full" title="Flow/Workflow launches always run YOLO — the gate/approval machinery here is not robust against a paused approval mid-flow. Use Normal Chat for gated (YOLO=off) runs.">
                        <input checked disabled type="checkbox" />
                        <span>YOLO mode (always on for Flow mode)</span>
                      </label>
                      <label className="settings-field">
                        <span>Cap (max rounds before blocking)</span>
                        <input
                          disabled={workflowDetailReadOnly}
                          min={1}
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current
                                ? { ...current, policyCap: event.target.value ? Number(event.target.value) : null }
                                : current,
                            )
                          }
                          placeholder="3 (default)"
                          type="number"
                          value={workflowDraft.policyCap ?? ""}
                        />
                      </label>
                      <label className="settings-field">
                        <span>Extend by (rounds added when a cap is extended)</span>
                        <input
                          disabled={workflowDetailReadOnly}
                          min={1}
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current
                                ? {
                                    ...current,
                                    policyExtendBy: event.target.value ? Number(event.target.value) : null,
                                  }
                                : current,
                            )
                          }
                          placeholder="2 (default)"
                          type="number"
                          value={workflowDraft.policyExtendBy ?? ""}
                        />
                      </label>
                    </div>
                  </div>
                    {renderWorkflowStepsEditor(
                    workflowSteps,
                    "detail",
                    detailWorkflowStepType,
                    setDetailWorkflowStepType,
                  )}
                  {renderWorkflowEdgesEditor(workflowDraft.edges, workflowSteps, "detail")}
                  {renderWorkflowFlowCanvas(workflowDraft.edges, workflowSteps, "detail")}
                </>
              ) : (
                <div className="settings-subpanel">
                  <div className="settings-empty">Select a workflow to inspect its detail page.</div>
                </div>
              )}
            </div>
          </div>
        )
      ) : stepView === "create" ? (
        <div className="workflow-editor-column">
          <div className="settings-subpanel">
            <div className="project-create-back-row">
              <button
                aria-label="Back to steps"
                className="secondary-btn project-icon-btn"
                onClick={() => setStepView("list")}
                title="Back to steps"
                type="button"
              >
                &lt;
              </button>
            </div>
            <div className="project-create-head">
              <div>
                <div className="settings-eyebrow">Create Step</div>
                <h3>New Step Definition</h3>
                <p className="project-muted-copy">
                  Create a reusable step first, then attach it from the workflow editor.
                </p>
              </div>
              <button
                className="primary-btn"
                disabled={busy || !createStepDraft.stepType.trim() || !createStepDraft.name.trim()}
                onClick={() => void saveNewStepDefinition()}
                type="button"
              >
                Save Step
              </button>
            </div>
            {renderStepDefinitionForm(createStepDraft, setCreateStepDraft, "create")}
          </div>
        </div>
      ) : (
        <div className="project-layout">
          <aside className="settings-subpanel project-registry-panel">
            <div className="project-registry-head">
              <div>
                <div className="settings-eyebrow">Step Registry</div>
                <h3>Definitions</h3>
                <p className="project-muted-copy">
                  Reusable execution units for workflows and direct single-step runs.
                </p>
              </div>
              <button
                aria-label="Create step definition"
                className="primary-btn project-create-fab"
                onClick={startCreateStep}
                type="button"
              >
                +
              </button>
            </div>
            {bulkSelect?.kind === "step" ? (
              <div className="project-bulk-bar">
                <span>{bulkSelect.ids.size} selected</span>
                <div className="settings-inline-actions">
                  <button className="secondary-btn" onClick={exitBulkSelect} type="button">
                    Cancel
                  </button>
                  <button
                    className="secondary-btn danger-btn"
                    disabled={bulkSelect.ids.size === 0}
                    onClick={() => {
                      const ids = Array.from(bulkSelect.ids);
                      void openDeleteStepConfirm(
                        ids,
                        ids.map((id) => stepDefinitions.find((step) => step.stepType === id)?.name ?? id),
                      );
                    }}
                    type="button"
                  >
                    Delete {bulkSelect.ids.size}
                  </button>
                </div>
              </div>
            ) : null}
            <div className="settings-list">
              {stepDefinitions.length === 0 ? (
                <div className="settings-empty">No step definitions found. Use the + button to create one.</div>
              ) : (
                stepDefinitions.map((step) => {
                  const bulkModeActive = bulkSelect?.kind === "step";
                  const isBulkSelected = bulkModeActive && bulkSelect.ids.has(step.stepType);
                  // A step still listed by a built-in workflow can't be
                  // deleted (that workflow is read-only), so it can't be
                  // long-pressed into a select mode that can only fail.
                  const longPressSelectable = !stepTypesUsedByBuiltin.has(step.stepType);
                  return (
                    <button
                      className={`settings-list-item ${
                        step.stepType === selectedStepType && !bulkModeActive ? "active" : ""
                      } ${isBulkSelected ? "bulk-selected" : ""} ${bulkModeActive ? "checkable" : ""} ${
                        !longPressSelectable ? "not-deletable" : ""
                      }`}
                      key={step.stepType}
                      onClick={() => {
                        if (consumeLongPressClick()) return;
                        if (bulkModeActive) {
                          if (longPressSelectable) toggleBulkSelected(step.stepType);
                          return;
                        }
                        selectStepDefinition(step.stepType);
                        setMessage(null);
                      }}
                      onPointerCancel={clearLongPressTimer}
                      onPointerDown={() => {
                        if (longPressSelectable) startLongPress("step", step.stepType);
                      }}
                      onPointerLeave={clearLongPressTimer}
                      onPointerUp={clearLongPressTimer}
                      title={longPressSelectable ? undefined : "Used by a built-in workflow — can't be deleted."}
                      type="button"
                    >
                      {bulkModeActive ? (
                        <input
                          aria-hidden="true"
                          checked={isBulkSelected}
                          className="settings-bulk-checkbox"
                          disabled={!longPressSelectable}
                          readOnly
                          tabIndex={-1}
                          type="checkbox"
                        />
                      ) : null}
                      <div>
                        <strong>{step.name}</strong>
                        <span>{stepDefinitionListSubtitle(step)}</span>
                      </div>
                    </button>
                  );
                })
              )}
            </div>
          </aside>

          <div className="project-detail-column">
            {stepDraft ? (
              <div className="settings-subpanel project-detail-panel">
                <div className="project-create-head">
                  <div>
                    <div className="settings-eyebrow">Step Detail</div>
                    <h3>{stepDraft.name || stepDraft.stepType || "Untitled Step"}</h3>
                    <p className="project-muted-copy">
                      Edit runtime contract fields here. Save only becomes active after a change.
                    </p>
                  </div>
                  <div className="settings-inline-actions">
                    <button
                      className="secondary-btn"
                      disabled={stepTypesUsedByBuiltin.has(stepDraft.stepType)}
                      onClick={() =>
                        void openDeleteStepConfirm([stepDraft.stepType], [stepDraft.name || stepDraft.stepType])
                      }
                      title={
                        stepTypesUsedByBuiltin.has(stepDraft.stepType)
                          ? "Used by a built-in workflow — can't be deleted."
                          : undefined
                      }
                      type="button"
                    >
                      Delete
                    </button>
                    <button
                      className="primary-btn"
                      disabled={busy || !stepDirty}
                      onClick={() => void saveStepDefinition()}
                      type="button"
                    >
                      Save Step
                    </button>
                  </div>
                </div>
                {renderStepDefinitionForm(stepDraft, setStepDraft, "detail")}
              </div>
            ) : (
              <div className="settings-subpanel">
                <div className="settings-empty">Select a step definition to inspect its detail page.</div>
              </div>
            )}
          </div>
        </div>
      )}

      {renderPickerModal()}

      {cloneTarget ? (
        <div className="settings-modal-backdrop" role="presentation">
          <div className="settings-modal">
            <div className="project-create-head">
              <div>
                <div className="settings-eyebrow">Clone</div>
                <h3>Clone Workflow</h3>
                <p className="project-muted-copy">
                  Creates an editable copy of <strong>{cloneTarget.sourceName}</strong>. The original stays unchanged.
                </p>
              </div>
            </div>
            <div className="settings-grid">
              <label className="settings-field settings-field-full">
                <span>New workflow name</span>
                <input onChange={(event) => setCloneName(event.target.value)} value={cloneName} />
              </label>
            </div>
            <div className="settings-actions">
              <button
                className="secondary-btn"
                onClick={() => {
                  setCloneTarget(null);
                  setCloneName("");
                }}
                type="button"
              >
                Cancel
              </button>
              <button
                className="primary-btn"
                disabled={busy || !cloneName.trim()}
                onClick={() => void cloneSelected()}
                type="button"
              >
                Clone Workflow
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deleteTarget ? (
        <div className="settings-modal-backdrop" role="presentation">
          <div className="settings-modal project-delete-modal">
            <div className="project-create-head">
              <div>
                <div className="settings-eyebrow">Delete</div>
                <h3>
                  {deleteTarget.kind === "workflow"
                    ? deleteTarget.ids.length > 1
                      ? `Delete ${deleteTarget.ids.length} Workflows`
                      : "Delete Workflow"
                    : deleteTarget.ids.length > 1
                      ? `Delete ${deleteTarget.ids.length} Step Definitions`
                      : "Delete Step Definition"}
                </h3>
                <p className="project-muted-copy">
                  Type <code>delete</code> to confirm deleting{" "}
                  {deleteTarget.labels.length > 1 ? (
                    <>{deleteTarget.labels.length} items</>
                  ) : (
                    <strong>{deleteTarget.labels[0]}</strong>
                  )}
                  .
                </p>
                {deleteTarget.labels.length > 1 ? (
                  <p className="project-muted-copy settings-list-item-meta">{deleteTarget.labels.join(", ")}</p>
                ) : null}
                {deleteTarget.kind === "step" && deleteTarget.cascadeWorkflows.length > 0 ? (
                  <div className="settings-warning">
                    Still used by {deleteTarget.cascadeWorkflows.length} workflow
                    {deleteTarget.cascadeWorkflows.length > 1 ? "s" : ""} — deleting{" "}
                    {deleteTarget.ids.length > 1 ? "these steps" : "this step"} will also delete{" "}
                    {deleteTarget.cascadeWorkflows.map((workflow) => workflow.name).join(", ")}.
                  </div>
                ) : null}
              </div>
            </div>
            <div className="settings-grid">
              <label className="settings-field">
                <span>Confirmation</span>
                <input
                  onChange={(event) => setDeleteConfirmationText(event.target.value)}
                  value={deleteConfirmationText}
                />
              </label>
            </div>
            <div className="settings-actions">
              <button
                className="secondary-btn"
                onClick={() => {
                  setDeleteTarget(null);
                  setDeleteConfirmationText("");
                }}
                type="button"
              >
                Cancel
              </button>
              <button
                className="primary-btn"
                disabled={busy || deleteConfirmationText !== "delete"}
                onClick={() => void deleteSelected()}
                type="button"
              >
                {deleteTarget.kind === "workflow"
                  ? deleteTarget.ids.length > 1
                    ? `Delete ${deleteTarget.ids.length} Workflows`
                    : "Delete Workflow"
                  : deleteTarget.ids.length > 1
                    ? `Delete ${deleteTarget.ids.length} Steps`
                    : "Delete Step"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {busy ? (
        <div className="settings-modal-backdrop workflow-settings-loading-modal-backdrop" role="presentation">
          <div
            aria-live="polite"
            aria-modal="true"
            className="settings-modal workflow-settings-loading-modal"
            role="status"
          >
            <span className="workflow-settings-loading-dot" />
            <strong>Syncing changes…</strong>
            <p>Saving updates and refreshing the latest list.</p>
          </div>
        </div>
      ) : null}
    </section>
  );
}
