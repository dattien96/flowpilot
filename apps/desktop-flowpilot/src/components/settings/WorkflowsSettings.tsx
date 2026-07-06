import { useEffect, useMemo, useState } from "react";
import type {
  ArtifactDefinition,
  Project,
  StepDefinition,
  SupportedModel,
  Workflow,
  WorkflowFlowEdge,
  WorkflowStep,
} from "@flowpilot/client-core";
import { FLOW_BEHAVIOR_OPTIONS, FLOW_EDGE_TERMINALS, validateFlowGraph } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, integrationTypes, toErrorMessage } from "@/components/settings/settingsHelpers";

type Tab = "workflows" | "steps";
type ViewMode = "list" | "create";
type DeleteTarget =
  | { kind: "workflow"; id: string; label: string }
  | { kind: "step"; id: string; label: string };

type PickerModal =
  | { kind: "artifact-input"; mode: "detail" | "create" }
  | { kind: "artifact-output"; mode: "detail" | "create" }
  | { kind: "mcp"; mode: "detail" | "create" };

type WorkflowDraft = {
  id?: string;
  projectId: string | null;
  name: string;
  description: string;
  isTemplate: boolean;
  modelOverride: string | null;
  reasoningEffortOverride: string | null;
  yoloMode: boolean;
  // Pass-through only — no UI control edits these (BUG-NOTE-CP42 #14).
  // saveWorkflow's repository upserts every field it's given, defaulting an
  // omitted key to null/[] rather than leaving the existing DB value
  // untouched, so a plain rename used to silently wipe an existing
  // workflow's policy caps and edge graph. Populated from the loaded
  // Workflow in mapWorkflowToDraft and sent back unchanged on save.
  policyCap: number | null;
  policyOnCap: string | null;
  policyExtendBy: number | null;
  policyExtendMax: number | null;
  edges: Workflow["edges"];
};

const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING = "medium";
const REASONING_OPTIONS = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "xhigh", label: "XHigh" },
] as const;

function deriveStepPromptBase(input: { stepType: string; name: string; description: string }) {
  const title = input.name.trim() || input.stepType.trim() || "workflow step";
  const summary = input.description.trim() || `Execute the ${title} step.`;
  return `Execute the ${title} step.\n\n${summary}`;
}

function createEmptyStepDraft(modelId: string): StepDefinition {
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
    model: "",
    reasoningEffort: DEFAULT_REASONING,
    yoloMode: false,
    agentType: "standard",
    nodeId: null,
    behaviorId: null,
    agentRef: null,
    nodeLifecycle: null,
    dependsOn: [],
    joinMode: null,
    cohort: null,
    promptTemplateRef: null,
    contextRef: null,
    inputs: {},
    outputs: {},
    inputArtifactDefinitions: [],
    outputArtifactDefinitions: [],
    createdAt: "",
    updatedAt: "",
  };
}

function mapWorkflowToDraft(workflow: Workflow | null): WorkflowDraft | null {
  if (!workflow) return null;
  return {
    id: workflow.id,
    projectId: workflow.projectId,
    name: workflow.name,
    description: workflow.description,
    isTemplate: workflow.isTemplate,
    modelOverride: workflow.modelOverride ?? "",
    reasoningEffortOverride: workflow.reasoningEffortOverride ?? DEFAULT_REASONING,
    yoloMode: workflow.yoloMode,
    policyCap: workflow.policyCap,
    policyOnCap: workflow.policyOnCap,
    policyExtendBy: workflow.policyExtendBy,
    policyExtendMax: workflow.policyExtendMax,
    edges: workflow.edges,
  };
}

function createEmptyWorkflowDraft(projects: Project[], modelId: string): WorkflowDraft {
  return {
    projectId: projects[0]?.id ?? null,
    name: "New Workflow",
    description: "",
    isTemplate: false,
    modelOverride: "",
    reasoningEffortOverride: DEFAULT_REASONING,
    yoloMode: false,
    policyCap: null,
    policyOnCap: null,
    policyExtendBy: null,
    policyExtendMax: null,
    edges: [],
  };
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
    dependsOn: [...(step.dependsOn ?? [])].sort(),
    joinMode: step.joinMode ?? "",
    cohort: step.cohort ?? "",
    promptTemplateRef: step.promptTemplateRef ?? "",
    contextRef: step.contextRef ?? "",
    inputs: step.inputs ?? {},
    outputs: step.outputs ?? {},
    inputArtifactDefinitions: step.inputArtifactDefinitions,
    outputArtifactDefinitions: step.outputArtifactDefinitions,
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

function stringMapToText(values: Record<string, string> | undefined) {
  return Object.entries(values ?? {})
    .map(([key, value]) => `${key}=${value}`)
    .join(", ");
}

function textToStringMap(value: string) {
  return Object.fromEntries(
    value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean)
      .map((item) => {
        const [key, ...rest] = item.split("=");
        return [key.trim(), rest.join("=").trim()];
      })
      .filter(([key]) => Boolean(key)),
  );
}

function toTitleCase(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
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
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
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
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");
  const [pickerModal, setPickerModal] = useState<PickerModal | null>(null);
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

  const refresh = async (
    workflowId?: string,
    stepType?: string,
    options?: { preserveCreateDrafts?: boolean },
  ) => {
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextWorkflows, nextStepDefinitions, nextModels, nextArtifactDefinitions] =
        await Promise.all([
          admin.projects.listProjects(),
          admin.workflows.listWorkflows(),
          admin.workflows.listStepDefinitions(),
          admin.providers.listSupportedModels(),
          admin.artifacts.listDefinitions(),
        ]);

      const enabledModels = nextModels.filter((model) => model.isEnabled);
      const defaultModel = enabledModels[0]?.modelId ?? DEFAULT_MODEL;
      const resolvedWorkflowId =
        workflowId && nextWorkflows.some((item) => item.id === workflowId)
          ? workflowId
          : nextWorkflows[0]?.id ?? "";
      const resolvedStepType =
        stepType && nextStepDefinitions.some((item) => item.stepType === stepType)
          ? stepType
          : nextStepDefinitions[0]?.stepType ?? "";

      setProjects(nextProjects);
      setWorkflows(nextWorkflows);
      setStepDefinitions(nextStepDefinitions);
      setModels(enabledModels);
      setArtifactDefinitions(nextArtifactDefinitions);
      setSelectedWorkflowId(resolvedWorkflowId);
      setSelectedStepType(resolvedStepType);

      if (!options?.preserveCreateDrafts) {
        setCreateWorkflowDraft(createEmptyWorkflowDraft(nextProjects, defaultModel));
        setCreateWorkflowSteps([]);
        setCreateStepDraft(createEmptyStepDraft(defaultModel));
      }

      const nextSelectedWorkflow =
        nextWorkflows.find((item) => item.id === resolvedWorkflowId) ?? null;
      const nextWorkflowSteps = resolvedWorkflowId
        ? await admin.workflows.listWorkflowSteps(resolvedWorkflowId)
        : [];
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

      const nextSelectedStep =
        nextStepDefinitions.find((item) => item.stepType === resolvedStepType) ?? null;
      const nextStepDraft = nextSelectedStep ? { ...nextSelectedStep } : null;
      setStepDraft(nextStepDraft);
      setStepInitialSnapshot(normalizeStepSnapshot(nextStepDraft));
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load workflows and step definitions."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

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
        steps: workflowSteps.map((step, orderIndex) => ({ ...step, orderIndex })),
      });
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
        steps: createWorkflowSteps.map((step, orderIndex) => ({ ...step, orderIndex })),
      });
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
    if (!stepDraft.model) {
      setMessage("Model is required.");
      return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.workflows.saveStepDefinition({
        ...stepDraft,
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
    if (!createStepDraft.model) {
      setMessage("Model is required.");
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

  const deleteSelected = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      if (deleteTarget.kind === "workflow") {
        await admin.workflows.deleteWorkflow(deleteTarget.id);
        setDeleteTarget(null);
        setDeleteConfirmationText("");
        await refresh(undefined, selectedStepType);
        setMessage("Workflow deleted.");
      } else {
        await admin.workflows.deleteStepDefinition(deleteTarget.id);
        setDeleteTarget(null);
        setDeleteConfirmationText("");
        await refresh(selectedWorkflowId, undefined);
        setMessage("Step definition deleted.");
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
                  <input
                    disabled={readOnly}
                    list="workflow-edge-when-suggestions"
                    onChange={(event) => updateWorkflowEdge(index, { when: event.target.value }, source)}
                    placeholder="done | continue | escalate"
                    value={edge.when}
                  />
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
        <datalist id="workflow-edge-when-suggestions">
          <option value="done" />
          <option value="continue" />
          <option value="escalate" />
        </datalist>
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
                  onPointerDown={isTerminal ? undefined : handleCanvasNodePointerDown(source, key)}
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
    const dependsOnText = (draft.dependsOn ?? []).join(", ");
    const inputsText = stringMapToText(draft.inputs);
    const outputsText = stringMapToText(draft.outputs);
    return (
      <div className="settings-grid">
        <label className="settings-field">
          <span>Step key</span>
          <input
            disabled={mode === "detail"}
            onChange={(event) => onChange({ ...draft, stepType: event.target.value })}
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
        <label className="settings-field">
          <span>Subagent</span>
          <input
            onChange={(event) =>
              onChange({ ...draft, subagent: event.target.value || null })
            }
            value={draft.subagent ?? ""}
          />
        </label>
        <label className="settings-field">
          <span>Model</span>
          <select
            onChange={(event) => onChange({ ...draft, model: event.target.value })}
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
        <label className="settings-field">
          <span>Agent type</span>
          <select
            onChange={(event) =>
              onChange({
                ...draft,
                agentType: event.target.value === "autonomous" ? "autonomous" : "standard",
              })
            }
            value={draft.agentType}
          >
            <option value="standard">standard</option>
            <option value="autonomous">autonomous</option>
          </select>
        </label>
        <label className="settings-field">
          <span>Node ID</span>
          <input
            onChange={(event) => onChange({ ...draft, nodeId: event.target.value || null })}
            placeholder="e.g. coder"
            value={draft.nodeId ?? ""}
          />
        </label>
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
          <span>Agent ref</span>
          <input
            onChange={(event) => onChange({ ...draft, agentRef: event.target.value || null })}
            placeholder="e.g. agents/coder.md"
            value={draft.agentRef ?? ""}
          />
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
          <span>Depends on</span>
          <input
            onChange={(event) =>
              onChange({
                ...draft,
                dependsOn: event.target.value
                  .split(",")
                  .map((item) => item.trim())
                  .filter(Boolean),
              })
            }
            placeholder="node ids, comma-separated"
            value={dependsOnText}
          />
        </label>
        <label className="settings-field">
          <span>Join mode</span>
          <input
            onChange={(event) => onChange({ ...draft, joinMode: event.target.value || null })}
            placeholder="all | any | quorum(n)"
            value={draft.joinMode ?? ""}
          />
        </label>
        <label className="settings-field">
          <span>Cohort</span>
          <input
            onChange={(event) => onChange({ ...draft, cohort: event.target.value || null })}
            placeholder="optional join-group key"
            value={draft.cohort ?? ""}
          />
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
        <label className="settings-field">
          <span>Inputs</span>
          <input
            onChange={(event) => onChange({ ...draft, inputs: textToStringMap(event.target.value) })}
            placeholder="main_context=flow_context_package.v1"
            value={inputsText}
          />
        </label>
        <label className="settings-field">
          <span>Outputs</span>
          <input
            onChange={(event) => onChange({ ...draft, outputs: textToStringMap(event.target.value) })}
            placeholder="main_context=flow_context_package.v1"
            value={outputsText}
          />
        </label>
        <label className="settings-checkbox settings-field-full">
          <input
            checked={draft.yoloMode}
            onChange={(event) => onChange({ ...draft, yoloMode: event.target.checked })}
            type="checkbox"
          />
          <span>YOLO for single-step runs</span>
        </label>
        <div className="settings-field-full">
          <div className="workflow-artifact-selector">
            <div className="workflow-chip-row">
              <strong>Input artifact definitions</strong>
              <button
                className="secondary-btn workflow-chip-add-btn"
                onClick={() => setPickerModal({ kind: "artifact-input", mode })}
                type="button"
              >
                + Add
              </button>
            </div>
            {draft.inputArtifactDefinitions.length === 0 ? (
              <div className="workflow-chip-empty">None selected</div>
            ) : (
              <div className="workflow-chips">
                {draft.inputArtifactDefinitions.map((key) => {
                  const def = artifactDefinitions.find((d) => d.key === key);
                  return (
                    <span className="workflow-chip" key={key}>
                      <span>{def?.name ?? key}</span>
                      <button
                        aria-label={`Remove ${def?.name ?? key}`}
                        className="workflow-chip-remove"
                        onClick={() =>
                          onChange({
                            ...draft,
                            inputArtifactDefinitions: draft.inputArtifactDefinitions.filter((k) => k !== key),
                          })
                        }
                        type="button"
                      >
                        ×
                      </button>
                    </span>
                  );
                })}
              </div>
            )}
          </div>
        </div>
        <div className="settings-field-full">
          <div className="workflow-artifact-selector">
            <div className="workflow-chip-row">
              <strong>Output artifact definitions</strong>
              <button
                className="secondary-btn workflow-chip-add-btn"
                onClick={() => setPickerModal({ kind: "artifact-output", mode })}
                type="button"
              >
                + Add
              </button>
            </div>
            {draft.outputArtifactDefinitions.length === 0 ? (
              <div className="workflow-chip-empty">None selected</div>
            ) : (
              <div className="workflow-chips">
                {draft.outputArtifactDefinitions.map((key) => {
                  const def = artifactDefinitions.find((d) => d.key === key);
                  return (
                    <span className="workflow-chip" key={key}>
                      <span>{def?.name ?? key}</span>
                      <button
                        aria-label={`Remove ${def?.name ?? key}`}
                        className="workflow-chip-remove"
                        onClick={() =>
                          onChange({
                            ...draft,
                            outputArtifactDefinitions: draft.outputArtifactDefinitions.filter((k) => k !== key),
                          })
                        }
                        type="button"
                      >
                        ×
                      </button>
                    </span>
                  );
                })}
              </div>
            )}
          </div>
        </div>
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
      pickerModal.kind === "artifact-input"
        ? "Input Artifact Definitions"
        : pickerModal.kind === "artifact-output"
          ? "Output Artifact Definitions"
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
          ) : artifactDefinitions.length === 0 ? (
            <div className="settings-empty">No artifact definitions available yet.</div>
          ) : (
            <div className="workflow-choice-grid workflow-picker-grid">
              {artifactDefinitions.map((definition) => {
                const selectedList =
                  pickerModal.kind === "artifact-input"
                    ? (draftSource?.inputArtifactDefinitions ?? [])
                    : (draftSource?.outputArtifactDefinitions ?? []);
                return (
                  <label className="workflow-choice-card" key={definition.key}>
                    <input
                      checked={selectedList.includes(definition.key)}
                      onChange={() => {
                        const next = toggleString(selectedList, definition.key);
                        updateDraft(
                          pickerModal.kind === "artifact-input"
                            ? { inputArtifactDefinitions: next }
                            : { outputArtifactDefinitions: next },
                        );
                      }}
                      type="checkbox"
                    />
                    <span>
                      <strong>{definition.name}</strong>
                      <small>{definition.key}</small>
                    </span>
                  </label>
                );
              })}
            </div>
          )}
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
            onClick={() => setTab("workflows")}
            type="button"
          >
            Workflows
          </button>
          <button
            className={`header-tab ${tab === "steps" ? "active" : ""}`}
            onClick={() => setTab("steps")}
            type="button"
          >
            Steps
          </button>
        </div>
      </div>

      {message ? <div className={`settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`}>{message}</div> : null}

      {tab === "workflows" ? (
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
                <label className="settings-checkbox settings-field-full">
                  <input
                    checked={createWorkflowDraft.yoloMode}
                    onChange={(event) =>
                      setCreateWorkflowDraft((current) => ({
                        ...current,
                        yoloMode: event.target.checked,
                      }))
                    }
                    type="checkbox"
                  />
                  <span>YOLO mode</span>
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
              <div className="settings-list">
                {workflows.length === 0 ? (
                  <div className="settings-empty">No workflows found. Use the + button to create one.</div>
                ) : (
                  workflows.map((workflow) => (
                    <button
                      className={`settings-list-item ${workflow.id === selectedWorkflowId ? "active" : ""}`}
                      key={workflow.id}
                      onClick={() => {
                        void refresh(workflow.id, selectedStepType, { preserveCreateDrafts: true });
                        setMessage(null);
                      }}
                      type="button"
                    >
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
                      </div>
                    </button>
                  ))
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
                        <p className="project-muted-copy">
                          {selectedWorkflow?.editable === false
                            ? "This is a built-in template and cannot be edited directly. Clone it to make changes."
                            : "Edit definition fields here. Save only becomes active after a change."}
                        </p>
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
                        {selectedWorkflow?.editable !== false ? (
                          <button
                            className="secondary-btn"
                            onClick={() =>
                              setDeleteTarget({
                                kind: "workflow",
                                id: selectedWorkflowId,
                                label: workflowDraft.name || "this workflow",
                              })
                            }
                            type="button"
                          >
                            Delete
                          </button>
                        ) : null}
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
                      <label className="settings-checkbox settings-field-full">
                        <input
                          checked={workflowDraft.yoloMode}
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current ? { ...current, yoloMode: event.target.checked } : current,
                            )
                          }
                          type="checkbox"
                        />
                        <span>YOLO mode</span>
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
            <div className="settings-list">
              {stepDefinitions.length === 0 ? (
                <div className="settings-empty">No step definitions found. Use the + button to create one.</div>
              ) : (
                stepDefinitions.map((step) => (
                  <button
                    className={`settings-list-item ${step.stepType === selectedStepType ? "active" : ""}`}
                    key={step.stepType}
                    onClick={() => {
                      void refresh(selectedWorkflowId, step.stepType, { preserveCreateDrafts: true });
                      setMessage(null);
                    }}
                    type="button"
                  >
                    <div>
                      <strong>{step.name}</strong>
                      <span>
                        {step.stepType} / {step.model}
                      </span>
                    </div>
                  </button>
                ))
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
                      onClick={() =>
                        setDeleteTarget({
                          kind: "step",
                          id: stepDraft.stepType,
                          label: stepDraft.name || stepDraft.stepType,
                        })
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
                <h3>{deleteTarget.kind === "workflow" ? "Delete Workflow" : "Delete Step Definition"}</h3>
                <p className="project-muted-copy">
                  Type <code>delete</code> to confirm deleting <strong>{deleteTarget.label}</strong>.
                </p>
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
                {deleteTarget.kind === "workflow" ? "Delete Workflow" : "Delete Step"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}
