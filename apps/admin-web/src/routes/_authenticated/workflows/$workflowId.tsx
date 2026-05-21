import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Project } from "@/domain/model/entity/project";
import type { StepDefinition, Workflow, WorkflowStep } from "@/domain/model/entity/workflow-engine";
import { GetWorkflowDetailUseCase } from "@/domain/usecase/workflow-engine/get-workflow-detail-usecase";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import { SaveWorkflowUseCase } from "@/domain/usecase/workflow-engine/save-workflow-usecase";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-engine/start-workflow-run-usecase";

export const Route = createFileRoute("/_authenticated/workflows/$workflowId")({
  validateSearch: (search: Record<string, unknown>) => ({
    projectId: typeof search.projectId === "string" ? search.projectId : undefined,
  }),
  component: WorkflowDetailPage,
});

export function WorkflowDetailPage() {
  const { workflowId } = Route.useParams();
  const { projectId: searchProjectId } = Route.useSearch();
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const getWorkflowDetailUseCase = useRef(
    new GetWorkflowDetailUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const saveWorkflowUseCase = useRef(
    new SaveWorkflowUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const startWorkflowRunUseCase = useRef(
    new StartWorkflowRunUseCase(gatewayBundle.current.workflowEngineGateway)
  );

  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [catalog, setCatalog] = useState<StepDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [runStarting, setRunStarting] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [providerOverride, setProviderOverride] = useState("");
  const [modelOverride, setModelOverride] = useState("");
  const [selectedStepType, setSelectedStepType] = useState("");
  const [steps, setSteps] = useState<Partial<WorkflowStep>[]>([]);
  const [runProjectId, setRunProjectId] = useState(searchProjectId ?? "");

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [detail, projectRows, stepRows] = await Promise.all([
          getWorkflowDetailUseCase.current.execute(workflowId),
          gatewayBundle.current.projectGateway.listProjects(),
          listStepDefinitionsUseCase.current.execute(),
        ]);

        setWorkflow(detail);
        setProjects(projectRows);
        setCatalog(stepRows);
        setSelectedStepType(stepRows[0]?.stepType ?? "");

        if (detail) {
          setName(detail.name);
          setDescription(detail.description);
          setProviderOverride(detail.providerOverride ?? "");
          setModelOverride(detail.modelOverride ?? "");
          setSteps(
            (detail.steps ?? [])
              .slice()
              .sort((left, right) => left.orderIndex - right.orderIndex)
              .map((step) => ({ ...step }))
          );

          const requestedProjectId = projectRows.some((project) => project.id === searchProjectId)
            ? searchProjectId
            : undefined;
          const defaultRunProjectId =
            detail.projectId ?? requestedProjectId ?? projectRows[0]?.id ?? "";
          setRunProjectId(defaultRunProjectId);
        }
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [searchProjectId, workflowId]);

  const stepNameByType = useMemo(
    () => new Map(catalog.map((step) => [step.stepType, step.name])),
    [catalog]
  );
  const projectNameById = useMemo(
    () => new Map(projects.map((project) => [project.id, project.name])),
    [projects]
  );
  const isPrivateWorkflow = workflow?.projectId != null;
  const canEdit = Boolean(workflow);
  const effectiveRunProjectId = isPrivateWorkflow ? workflow?.projectId ?? "" : runProjectId;

  const addStep = () => {
    if (!canEdit || !selectedStepType) return;
    setSteps((current) => [
      ...current,
      {
        stepType: selectedStepType,
        orderIndex: current.length,
        isEnabled: true,
        requiresApproval: true,
        providerOverride: null,
        modelOverride: null,
      },
    ]);
  };

  const moveStep = (index: number, direction: "up" | "down") => {
    if (!canEdit) return;
    const nextIndex = direction === "up" ? index - 1 : index + 1;
    if (nextIndex < 0 || nextIndex >= steps.length) return;

    const updated = [...steps];
    const temp = updated[index];
    updated[index] = updated[nextIndex];
    updated[nextIndex] = temp;
    setSteps(updated.map((step, orderIndex) => ({ ...step, orderIndex })));
  };

  const removeStep = (index: number) => {
    if (!canEdit) return;
    setSteps((current) =>
      current.filter((_, currentIndex) => currentIndex !== index).map((step, orderIndex) => ({
        ...step,
        orderIndex,
      }))
    );
  };

  const saveWorkflow = async () => {
    if (!workflow) {
      window.alert("Workflow not found.");
      return;
    }

    setSaving(true);
    try {
      const saved = await saveWorkflowUseCase.current.execute({
        id: workflow.id,
        projectId: workflow.projectId ?? null,
        name,
        description,
        isTemplate: workflow.isTemplate,
        providerOverride: providerOverride || null,
        modelOverride: modelOverride || null,
        steps,
      });
      setWorkflow(saved);
      if (saved.id !== workflow.id) {
        await navigate({
          to: "/workflows/$workflowId",
          params: { workflowId: saved.id },
          search: { projectId: saved.projectId ?? searchProjectId ?? undefined },
          replace: true,
        });
      }
      window.alert("Workflow saved.");
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save workflow.");
    } finally {
      setSaving(false);
    }
  };

  const startRun = async () => {
    if (!workflow || !effectiveRunProjectId) {
      window.alert("Please choose a project to run this workflow.");
      return;
    }

    setRunStarting(true);
    try {
      await startWorkflowRunUseCase.current.execute(workflow.id, effectiveRunProjectId);
      await navigate({
        to: "/projects/$projectId/workflows",
        params: { projectId: effectiveRunProjectId },
      });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to start workflow run.");
    } finally {
      setRunStarting(false);
    }
  };

  if (loading) {
    return (
      <PageFrame title="Workflow Builder" description="Loading workflow details..." />
    );
  }

  if (!workflow) {
    return (
      <PageFrame
        title="Workflow Builder"
        description="The requested workflow definition could not be found."
        actions={
          <Link to="/workflows" search={{ projectId: searchProjectId }}>
            <Button variant="secondary">Back to definitions</Button>
          </Link>
        }
      />
    );
  }

  return (
    <PageFrame
      title={workflow.name}
      description="Dedicated workflow detail page for composing steps, adjusting overrides, and launching runs."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflows" search={{ projectId: searchProjectId }}>
            <Button variant="secondary">Back to definitions</Button>
          </Link>
          <Link to="/workflow-steps">
            <Button variant="secondary">Step catalog</Button>
          </Link>
          <Button disabled={!canEdit || saving} onClick={() => void saveWorkflow()}>
            Save workflow
          </Button>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-3">
        <div className="rounded-2xl border border-border bg-card px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Scope
          </p>
          <p className="mt-2 text-sm font-medium">{isPrivateWorkflow ? "Private" : "Global"}</p>
        </div>
        <div className="rounded-2xl border border-border bg-card px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Owner project
          </p>
          <p className="mt-2 text-sm font-medium">
            {workflow.projectId ? projectNameById.get(workflow.projectId) ?? workflow.projectId : "Workspace global"}
          </p>
        </div>
        <div className="rounded-2xl border border-border bg-card px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Editing mode
          </p>
          <p className="mt-2 text-sm font-medium">
            {workflow.projectId ? "Editable private workflow" : "Editable global workflow"}
          </p>
        </div>
      </div>

      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-2">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Workflow name</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            disabled={!canEdit}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm md:col-span-2">
          <span className="font-medium">Description</span>
          <textarea
            className="min-h-28 w-full rounded-2xl border border-border bg-card px-4 py-3"
            disabled={!canEdit}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Provider override</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            disabled={!canEdit}
            value={providerOverride}
            onChange={(event) => setProviderOverride(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Model override</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            disabled={!canEdit}
            value={modelOverride}
            onChange={(event) => setModelOverride(event.target.value)}
          />
        </label>
      </div>

      <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
        <div className="flex flex-wrap items-end gap-3">
          <label className="min-w-[260px] flex-1 space-y-2 text-sm">
            <span className="font-medium">{isPrivateWorkflow ? "Run project" : "Launch in project"}</span>
            {isPrivateWorkflow ? (
              <div className="rounded-2xl border border-border bg-card px-4 py-3 text-sm">
                {workflow.projectId ? projectNameById.get(workflow.projectId) ?? workflow.projectId : ""}
              </div>
            ) : (
              <select
                className="w-full rounded-2xl border border-border bg-card px-4 py-3"
                value={runProjectId}
                onChange={(event) => setRunProjectId(event.target.value)}
              >
                <option value="">Select a project</option>
                {projects.map((project) => (
                  <option key={project.id} value={project.id}>
                    {project.name}
                  </option>
                ))}
              </select>
            )}
          </label>
          <Button disabled={!effectiveRunProjectId || runStarting || steps.length === 0} onClick={() => void startRun()}>
            Launch run
          </Button>
        </div>
        <p className="mt-3 text-sm text-muted-foreground">
          {isPrivateWorkflow
            ? "Private workflows always launch in their owner project."
            : "Global workflows stay reusable. Pick a project only when you want to launch a run."}
        </p>
      </div>

      <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
        <div className="flex flex-wrap items-end gap-3">
          <label className="min-w-[260px] flex-1 space-y-2 text-sm">
            <span className="font-medium">Add step</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3"
              disabled={!canEdit}
              value={selectedStepType}
              onChange={(event) => setSelectedStepType(event.target.value)}
            >
              {catalog.map((step) => (
                <option key={step.stepType} value={step.stepType}>
                  {step.name}
                </option>
              ))}
            </select>
          </label>
          <Button disabled={!canEdit || !selectedStepType} onClick={addStep}>
            Add step
          </Button>
        </div>

        <div className="mt-5 space-y-3">
          {steps.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No steps are attached to this workflow yet.
            </p>
          ) : null}
          {steps.map((step, index) => (
            <div
              key={`${step.stepType}-${index}`}
              className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3"
            >
              <div>
                <p className="font-medium">
                  {index + 1}. {stepNameByType.get(step.stepType ?? "") ?? step.stepType}
                </p>
                <p className="text-xs text-muted-foreground">{step.stepType}</p>
              </div>
              <div className="flex gap-2">
                <Button disabled={!canEdit} variant="secondary" onClick={() => moveStep(index, "up")}>
                  Up
                </Button>
                <Button disabled={!canEdit} variant="secondary" onClick={() => moveStep(index, "down")}>
                  Down
                </Button>
                <Button disabled={!canEdit} variant="secondary" onClick={() => removeStep(index)}>
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </PageFrame>
  );
}
