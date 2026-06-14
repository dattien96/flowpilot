import { useEffect, useMemo, useState } from "react";
import type { Project, StepDefinition, SupportedModel, Workflow, WorkflowStep } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";

type Tab = "workflows" | "steps";

export function WorkflowsSettings(): React.ReactElement {
  const [tab, setTab] = useState<Tab>("workflows");
  const [projects, setProjects] = useState<Project[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [stepDefinitions, setStepDefinitions] = useState<StepDefinition[]>([]);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState("");
  const [selectedStepType, setSelectedStepType] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const selectedWorkflow = useMemo(() => workflows.find((item) => item.id === selectedWorkflowId) ?? null, [workflows, selectedWorkflowId]);
  const selectedStep = useMemo(() => stepDefinitions.find((item) => item.stepType === selectedStepType) ?? null, [stepDefinitions, selectedStepType]);

  const [workflowDraft, setWorkflowDraft] = useState<Partial<Workflow>>({});
  const [stepDraft, setStepDraft] = useState<StepDefinition | null>(null);

  const refresh = async (workflowId?: string, stepType?: string) => {
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextWorkflows, nextSteps, nextModels] = await Promise.all([
        admin.projects.listProjects(),
        admin.workflows.listWorkflows(),
        admin.workflows.listStepDefinitions(),
        admin.providers.listSupportedModels(),
      ]);
      setProjects(nextProjects);
      setWorkflows(nextWorkflows);
      setStepDefinitions(nextSteps);
      setModels(nextModels.filter((model) => model.isEnabled));
      const nextWorkflowId = workflowId ?? nextWorkflows[0]?.id ?? "";
      setSelectedWorkflowId(nextWorkflowId);
      setSelectedStepType(stepType ?? nextSteps[0]?.stepType ?? "");
      setWorkflowSteps(nextWorkflowId ? await admin.workflows.listWorkflowSteps(nextWorkflowId) : []);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load workflows."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedWorkflow) return;
    setWorkflowDraft({ ...selectedWorkflow });
    void getAdminUseCases().then((admin) => admin.workflows.listWorkflowSteps(selectedWorkflow.id)).then(setWorkflowSteps);
  }, [selectedWorkflow]);

  useEffect(() => {
    if (selectedStep) setStepDraft({ ...selectedStep });
  }, [selectedStep]);

  const createWorkflow = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const workflow = await admin.workflows.saveWorkflow({
        name: "New Workflow",
        description: "",
        projectId: projects[0]?.id ?? null,
        isTemplate: false,
        providerOverride: "codex",
        modelOverride: "gpt-5.4",
        reasoningEffortOverride: "medium",
        yoloMode: false,
        steps: [],
      });
      await refresh(workflow.id, selectedStepType);
      setMessage("Workflow created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create workflow."));
    } finally {
      setBusy(false);
    }
  };

  const saveWorkflow = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const workflow = await admin.workflows.saveWorkflow({
        ...workflowDraft,
        steps: workflowSteps,
      });
      await refresh(workflow.id, selectedStepType);
      setMessage("Workflow saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save workflow."));
    } finally {
      setBusy(false);
    }
  };

  const addWorkflowStep = () => {
    const stepType = stepDefinitions.find((step) => !workflowSteps.some((item) => item.stepType === step.stepType))?.stepType;
    if (!stepType) return;
    setWorkflowSteps((current) => [...current, {
      id: crypto.randomUUID(),
      workflowId: selectedWorkflowId,
      stepType,
      orderIndex: current.length,
      isEnabled: true,
      providerOverride: null,
      modelOverride: null,
      reasoningEffortOverride: null,
      requiresApproval: true,
      createdAt: "",
      updatedAt: "",
    }]);
  };

  const createStepDefinition = async () => {
    const stepType = `custom_${Date.now()}`;
    setStepDraft({
      stepType,
      name: "New Step",
      description: "",
      promptBase: null,
      requiredMcps: [],
      mcpAccessMode: "read_only",
      requiredSkills: [],
      teamRole: null,
      subagent: null,
      model: "gpt-5.4",
      reasoningEffort: "medium",
      yoloMode: false,
      agentType: "standard",
      inputArtifactDefinitions: [],
      outputArtifactDefinitions: [],
      createdAt: "",
      updatedAt: "",
    });
    setSelectedStepType(stepType);
  };

  const saveStepDefinition = async () => {
    if (!stepDraft) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const step = await admin.workflows.saveStepDefinition(stepDraft);
      await refresh(selectedWorkflowId, step.stepType);
      setMessage("Step definition saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save step definition."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div><div className="settings-eyebrow">Workflows</div><h2>Workflows</h2><p>Manage workflow definitions and reusable step definitions.</p></div>
        <div className="header-tabs"><button className={`header-tab ${tab === "workflows" ? "active" : ""}`} onClick={() => setTab("workflows")} type="button">Workflows</button><button className={`header-tab ${tab === "steps" ? "active" : ""}`} onClick={() => setTab("steps")} type="button">Steps</button></div>
      </div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      {tab === "workflows" ? (
        <div className="settings-two-column">
          <div className="settings-subpanel"><div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void createWorkflow()} type="button">Create Workflow</button></div><div className="settings-list">{workflows.map((workflow) => <button className={`settings-list-item ${workflow.id === selectedWorkflowId ? "active" : ""}`} key={workflow.id} onClick={() => setSelectedWorkflowId(workflow.id)} type="button"><strong>{workflow.name}</strong><span>{workflow.projectId ?? "global"} / {formatTimestamp(workflow.updatedAt)}</span></button>)}</div></div>
          <div className="settings-subpanel">
            <h3>Edit Workflow</h3>
            <div className="settings-grid">
              <label className="settings-field"><span>Name</span><input value={workflowDraft.name ?? ""} onChange={(event) => setWorkflowDraft((current) => ({ ...current, name: event.target.value }))} /></label>
              <label className="settings-field"><span>Project</span><select value={workflowDraft.projectId ?? ""} onChange={(event) => setWorkflowDraft((current) => ({ ...current, projectId: event.target.value || null }))}><option value="">Global</option>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
              <label className="settings-field"><span>Model</span><select value={workflowDraft.modelOverride ?? ""} onChange={(event) => setWorkflowDraft((current) => ({ ...current, modelOverride: event.target.value || null }))}><option value="">Project default</option>{models.map((model) => <option key={model.id} value={model.modelId}>{model.displayName}</option>)}</select></label>
              <label className="settings-field settings-field-full"><span>Description</span><textarea value={workflowDraft.description ?? ""} onChange={(event) => setWorkflowDraft((current) => ({ ...current, description: event.target.value }))} /></label>
            </div>
            <div className="settings-list">{workflowSteps.map((step, index) => <div className="settings-list-item static" key={step.id}><strong>{index + 1}. {step.stepType}</strong><button className="ghost-btn" onClick={() => setWorkflowSteps((current) => current.filter((item) => item.id !== step.id))} type="button">Remove</button></div>)}</div>
            <div className="settings-actions"><button className="secondary-btn" onClick={addWorkflowStep} type="button">Add Step</button><button className="primary-btn" disabled={busy} onClick={() => void saveWorkflow()} type="button">Save Workflow</button></div>
          </div>
        </div>
      ) : (
        <div className="settings-two-column">
          <div className="settings-subpanel"><div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void createStepDefinition()} type="button">Create Step</button></div><div className="settings-list">{stepDefinitions.map((step) => <button className={`settings-list-item ${step.stepType === selectedStepType ? "active" : ""}`} key={step.stepType} onClick={() => setSelectedStepType(step.stepType)} type="button"><strong>{step.name}</strong><span>{step.stepType} / {step.model}</span></button>)}</div></div>
          <div className="settings-subpanel">
            <h3>Edit Step</h3>
            {stepDraft ? <div className="settings-grid">
              <label className="settings-field"><span>Step Type</span><input value={stepDraft.stepType} onChange={(event) => setStepDraft((current) => current ? { ...current, stepType: event.target.value } : current)} /></label>
              <label className="settings-field"><span>Name</span><input value={stepDraft.name} onChange={(event) => setStepDraft((current) => current ? { ...current, name: event.target.value } : current)} /></label>
              <label className="settings-field settings-field-full"><span>Description</span><textarea value={stepDraft.description} onChange={(event) => setStepDraft((current) => current ? { ...current, description: event.target.value } : current)} /></label>
              <label className="settings-field"><span>Model</span><select value={stepDraft.model} onChange={(event) => setStepDraft((current) => current ? { ...current, model: event.target.value } : current)}>{models.map((model) => <option key={model.id} value={model.modelId}>{model.displayName}</option>)}</select></label>
              <label className="settings-checkbox"><input checked={stepDraft.yoloMode} onChange={(event) => setStepDraft((current) => current ? { ...current, yoloMode: event.target.checked } : current)} type="checkbox" /><span>YOLO mode</span></label>
            </div> : <div className="settings-empty">Select or create a step.</div>}
            <div className="settings-actions"><button className="primary-btn" disabled={busy || !stepDraft} onClick={() => void saveStepDefinition()} type="button">Save Step</button></div>
          </div>
        </div>
      )}
    </section>
  );
}
