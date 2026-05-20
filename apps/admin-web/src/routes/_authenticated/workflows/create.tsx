import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import { SaveWorkflowUseCase } from "@/domain/usecase/workflow-engine/save-workflow-usecase";
import type { Project } from "@/domain/model/entity/project";
import type { StepDefinition, WorkflowStep } from "@/domain/model/entity/workflow-engine";

export const Route = createFileRoute("/_authenticated/workflows/create")({
  validateSearch: (search: Record<string, unknown>) => ({
    projectId: typeof search.projectId === "string" ? search.projectId : undefined,
  }),
  component: CreateWorkflowPage,
});

function CreateWorkflowPage() {
  const navigate = useNavigate();
  const { projectId: searchProjectId } = Route.useSearch();
  const gatewayBundle = useRef(createGatewayBundle());
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const saveWorkflowUseCase = useRef(
    new SaveWorkflowUseCase(gatewayBundle.current.workflowEngineGateway)
  );

  const [projects, setProjects] = useState<Project[]>([]);
  const [catalog, setCatalog] = useState<StepDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [projectId, setProjectId] = useState("");
  const [name, setName] = useState("New Private Workflow");
  const [description, setDescription] = useState("");
  const [providerOverride, setProviderOverride] = useState("");
  const [modelOverride, setModelOverride] = useState("");
  const [selectedStepType, setSelectedStepType] = useState("");
  const [steps, setSteps] = useState<Partial<WorkflowStep>[]>([]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [projectRows, stepRows] = await Promise.all([
          gatewayBundle.current.projectGateway.listProjects(),
          listStepDefinitionsUseCase.current.execute(),
        ]);
        setProjects(projectRows);
        setCatalog(stepRows);
        if (projectRows.length > 0) {
          const requestedProjectId = projectRows.some((project) => project.id === searchProjectId)
            ? searchProjectId
            : projectRows[0].id;
          setProjectId(requestedProjectId ?? "");
        }
        if (stepRows.length > 0) {
          setSelectedStepType(stepRows[0].stepType);
        }
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [searchProjectId]);

  const stepNameByType = useMemo(
    () => new Map(catalog.map((step) => [step.stepType, step.name])),
    [catalog]
  );

  const addStep = () => {
    if (!selectedStepType) return;
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
    const nextIndex = direction === "up" ? index - 1 : index + 1;
    if (nextIndex < 0 || nextIndex >= steps.length) return;

    const updated = [...steps];
    const temp = updated[index];
    updated[index] = updated[nextIndex];
    updated[nextIndex] = temp;
    setSteps(updated.map((step, orderIndex) => ({ ...step, orderIndex })));
  };

  const removeStep = (index: number) => {
    setSteps((current) =>
      current.filter((_, currentIndex) => currentIndex !== index).map((step, orderIndex) => ({
        ...step,
        orderIndex,
      }))
    );
  };

  const saveWorkflow = async () => {
    if (!projectId) {
      window.alert("Please choose a project owner for this workflow.");
      return;
    }

    setSaving(true);
    try {
      const saved = await saveWorkflowUseCase.current.execute({
        projectId,
        name,
        description,
        isTemplate: false,
        providerOverride: providerOverride || null,
        modelOverride: modelOverride || null,
        steps,
      });
      await navigate({
        to: "/workflows/$workflowId",
        params: { workflowId: saved.id },
        search: { projectId },
      });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save workflow.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageFrame
      title="Create Workflow"
      description="Create a private workflow definition by choosing an owning project, selecting reusable steps, and ordering them."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflow-steps">
            <Button variant="secondary">Browse steps</Button>
          </Link>
          <Link to="/workflow-steps/create">
            <Button variant="secondary">Create step</Button>
          </Link>
          <Button disabled={saving || loading} onClick={() => void saveWorkflow()}>
            Save workflow
          </Button>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-2">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Owner project</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Workflow name</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm md:col-span-2">
          <span className="font-medium">Description</span>
          <textarea
            className="min-h-28 w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Provider override</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={providerOverride}
            onChange={(event) => setProviderOverride(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Model override</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={modelOverride}
            onChange={(event) => setModelOverride(event.target.value)}
          />
        </label>
      </div>

      <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
        <div className="flex flex-wrap items-end gap-3">
          <label className="min-w-[260px] flex-1 space-y-2 text-sm">
            <span className="font-medium">Add step</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3"
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
          <Button disabled={!selectedStepType} onClick={addStep}>
            Add step
          </Button>
        </div>

        <div className="mt-5 space-y-3">
          {steps.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No steps added yet. Choose a definition from the catalog and add it to this workflow.
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
                <Button variant="secondary" onClick={() => moveStep(index, "up")}>
                  Up
                </Button>
                <Button variant="secondary" onClick={() => moveStep(index, "down")}>
                  Down
                </Button>
                <Button variant="secondary" onClick={() => removeStep(index)}>
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
