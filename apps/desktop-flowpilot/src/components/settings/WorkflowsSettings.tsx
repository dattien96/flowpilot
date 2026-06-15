import { useEffect, useMemo, useState } from "react";
import type {
  ArtifactDefinition,
  Project,
  StepDefinition,
  SupportedModel,
  Workflow,
  WorkflowStep,
} from "@flowpilot/client-core";
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
    model: modelId,
    reasoningEffort: DEFAULT_REASONING,
    yoloMode: false,
    agentType: "standard",
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
    modelOverride: workflow.modelOverride ?? DEFAULT_MODEL,
    reasoningEffortOverride: workflow.reasoningEffortOverride ?? DEFAULT_REASONING,
    yoloMode: workflow.yoloMode,
  };
}

function createEmptyWorkflowDraft(projects: Project[], modelId: string): WorkflowDraft {
  return {
    projectId: projects[0]?.id ?? null,
    name: "New Workflow",
    description: "",
    isTemplate: false,
    modelOverride: modelId,
    reasoningEffortOverride: DEFAULT_REASONING,
    yoloMode: false,
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
      modelOverride: step.modelOverride ?? "",
      reasoningEffortOverride: step.reasoningEffortOverride ?? "",
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

  const selectedWorkflow = useMemo(
    () => workflows.find((item) => item.id === selectedWorkflowId) ?? null,
    [workflows, selectedWorkflowId],
  );
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

  useEffect(() => {
    if (stepView === "create" && !createStepDraft.model && modelOptions.length > 0) {
      setCreateStepDraft((current) => ({ ...current, model: modelOptions[0].value }));
    }
  }, [createStepDraft.model, modelOptions, stepView]);

  const startCreateWorkflow = () => {
    const defaultModel = modelOptions[0]?.value ?? DEFAULT_MODEL;
    setCreateWorkflowDraft(createEmptyWorkflowDraft(projects, defaultModel));
    setCreateWorkflowSteps([]);
    setCreateWorkflowStepType(stepDefinitions[0]?.stepType ?? "");
    setWorkflowView("create");
    setMessage(null);
  };

  const startCreateStep = () => {
    const defaultModel = modelOptions[0]?.value ?? DEFAULT_MODEL;
    setCreateStepDraft(createEmptyStepDraft(defaultModel));
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
    const stepDefinition = stepDefinitions.find((item) => item.stepType === chosenStepType);
    const target = source === "detail" ? workflowSteps : createWorkflowSteps;
    const nextStep: WorkflowStep = {
      id: `${chosenStepType}-${Date.now()}`,
      workflowId: source === "detail" ? selectedWorkflowId : "__new__",
      stepType: chosenStepType,
      orderIndex: target.length,
      isEnabled: true,
      providerOverride: null,
      modelOverride: stepDefinition?.model ?? (modelOptions[0]?.value ?? DEFAULT_MODEL),
      reasoningEffortOverride:
        stepDefinition?.reasoningEffort ?? DEFAULT_REASONING,
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

  const saveWorkflow = async () => {
    if (!workflowDraft) return;
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
    setBusy(true);
    setMessage(null);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.workflows.saveWorkflow({
        projectId: createWorkflowDraft.projectId,
        name: createWorkflowDraft.name.trim() || "New Workflow",
        description: createWorkflowDraft.description,
        isTemplate: createWorkflowDraft.isTemplate,
        modelOverride: createWorkflowDraft.modelOverride ?? DEFAULT_MODEL,
        reasoningEffortOverride:
          createWorkflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING,
        yoloMode: createWorkflowDraft.yoloMode,
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

    return (
      <div className="settings-subpanel workflow-steps-panel">
        <div className="project-panel-head">
          <div>
            <strong>Workflow Steps</strong>
            <p className="project-muted-copy">
              Add, remove, and reorder the reusable steps that make up this workflow.
            </p>
          </div>
        </div>
        <div className="workflow-step-add-row">
          <label className="settings-field workflow-step-add-field">
            <span>Add step</span>
            <select
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
            disabled={!selectedStepTypeValue}
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
            steps.map((step, index) => (
              <div className="settings-list-item static workflow-step-card" key={`${step.stepType}-${index}`}>
                <div className="workflow-step-card-head">
                  <div>
                    <strong>
                      {index + 1}.{" "}
                      {stepDefinitions.find((definition) => definition.stepType === step.stepType)?.name ??
                        step.stepType}
                    </strong>
                    <span>{step.stepType}</span>
                  </div>
                  <div className="settings-inline-actions">
                    <button
                      className="secondary-btn project-icon-btn"
                      disabled={index === 0}
                      onClick={() => moveWorkflowStep(index, "up", source)}
                      title="Move up"
                      type="button"
                    >
                      ^
                    </button>
                    <button
                      className="secondary-btn project-icon-btn"
                      disabled={index === steps.length - 1}
                      onClick={() => moveWorkflowStep(index, "down", source)}
                      title="Move down"
                      type="button"
                    >
                      v
                    </button>
                    <button
                      className="secondary-btn"
                      onClick={() => removeWorkflowStep(index, source)}
                      type="button"
                    >
                      Remove
                    </button>
                  </div>
                </div>
                <div className="settings-grid workflow-step-grid">
                  <label className="settings-field">
                    <span>Model override</span>
                    <select
                      onChange={(event) =>
                        source === "detail"
                          ? updateWorkflowStep(index, { modelOverride: event.target.value || null })
                          : updateCreateWorkflowStep(index, {
                              modelOverride: event.target.value || null,
                            })
                      }
                      value={step.modelOverride ?? ""}
                    >
                      {modelOptions.map((model) => (
                        <option key={model.value} value={model.value}>
                          {model.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="settings-field">
                    <span>Reasoning</span>
                    <select
                      onChange={(event) =>
                        source === "detail"
                          ? updateWorkflowStep(index, {
                              reasoningEffortOverride: event.target.value || null,
                            })
                          : updateCreateWorkflowStep(index, {
                              reasoningEffortOverride: event.target.value || null,
                            })
                      }
                      value={step.reasoningEffortOverride ?? DEFAULT_REASONING}
                    >
                      {REASONING_OPTIONS.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="settings-checkbox">
                    <input
                      checked={step.isEnabled}
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
              </div>
            ))
          )}
        </div>
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
            value={draft.model}
          >
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
                    value={createWorkflowDraft.modelOverride ?? DEFAULT_MODEL}
                  >
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
                        <strong>{workflow.name}</strong>
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
                        <div className="settings-eyebrow">Workflow Detail</div>
                        <h3>{workflowDraft.name || "Untitled Workflow"}</h3>
                        <p className="project-muted-copy">
                          Edit definition fields here. Save only becomes active after a change.
                        </p>
                      </div>
                      <div className="settings-inline-actions">
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
                          onChange={(event) =>
                            setWorkflowDraft((current) =>
                              current ? { ...current, description: event.target.value } : current,
                            )
                          }
                          value={workflowDraft.description}
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
                          value={workflowDraft.modelOverride ?? DEFAULT_MODEL}
                        >
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
