import { useMutation } from "@tanstack/react-query";
import {
  createFileRoute,
  Link,
  Outlet,
  useLocation,
  useNavigate,
  useRouter,
} from "@tanstack/react-router";
import { type FormEvent, useEffect, useMemo, useRef, useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/components/ui/button";
import { getTeamLinkDelta } from "./project-team-links";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import { ListWorkflowsUseCase } from "@/domain/usecase/workflow-engine/list-workflows-usecase";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-engine/start-workflow-run-usecase";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";
import type { StepDefinition, Workflow } from "@/domain/model/entity/workflow-engine";
import { ensureProjectHasUsableBinding } from "@/features/projects/project-binding-launch-guard";

export const Route = createFileRoute("/_authenticated/projects/$projectId")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const [project, workflowRuns, teams, allTeams, bindings] = await Promise.all([
      gateways.projectGateway.getProjectById(params.projectId),
      gateways.workflowGateway.listWorkflowRuns(),
      gateways.teamGateway.listTeamsByProject(params.projectId),
      gateways.teamGateway.listTeams(),
      gateways.projectGateway.listProjectWorkspaceBindings(params.projectId),
    ]);

    const members =
      teams.length === 0
        ? []
        : (await Promise.all(teams.map((team) => gateways.teamGateway.listMembersByTeam(team.id)))).flat();

    return {
      project,
      workflowRuns: workflowRuns.filter((run) => run.projectId === params.projectId),
      teams,
      allTeams,
      members,
      bindings,
    };
  },
  component: ProjectDetailPage,
});

function ProjectDetailPage() {
  const detail = Route.useLoaderData();
  if (!detail.project) {
    return null;
  }

  return <ProjectDetailContent detail={detail} key={detail.project.id} />;
}

type LaunchMode = "workflow-definition" | "single-step" | "new-workflow";

type BindingDraft = {
  id: string;
  localPath: string;
  label: string;
  persisted: boolean;
};

function toBindingDraft(binding: ProjectWorkspaceBinding): BindingDraft {
  return {
    id: binding.id,
    localPath: binding.localPath,
    label: binding.label ?? "",
    persisted: true,
  };
}

function getInitialBindingDrafts(
  bindings: ProjectWorkspaceBinding[],
  directoryPath: string | null,
): BindingDraft[] {
  if (bindings.length > 0) {
    return bindings.map(toBindingDraft);
  }

  return [
    {
      id: crypto.randomUUID(),
      localPath: directoryPath ?? "",
      label: "Primary",
      persisted: false,
    },
  ];
}

function ProjectExecutionLauncher({
  bindings,
  projectId,
}: {
  bindings: ProjectWorkspaceBinding[];
  projectId: string;
}) {
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const listWorkflowsUseCase = useRef(
    new ListWorkflowsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const startWorkflowRunUseCase = useRef(
    new StartWorkflowRunUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const [mode, setMode] = useState<LaunchMode>("workflow-definition");
  const [workflowDefinitions, setWorkflowDefinitions] = useState<Workflow[]>([]);
  const [stepDefinitions, setStepDefinitions] = useState<StepDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [launching, setLaunching] = useState(false);
  const [workflowId, setWorkflowId] = useState("");
  const [stepType, setStepType] = useState("");
  const [beginPrompt, setBeginPrompt] = useState("");

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [workflows, steps] = await Promise.all([
          listWorkflowsUseCase.current.execute(projectId),
          listStepDefinitionsUseCase.current.execute(),
        ]);
        setWorkflowDefinitions(workflows);
        setStepDefinitions(steps);
        setWorkflowId((current) => current || workflows[0]?.id || "");
        setStepType((current) => current || steps[0]?.stepType || "");
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [projectId]);

  const workflowOptions = useMemo(
    () => workflowDefinitions.map((workflow) => ({ id: workflow.id, label: workflow.name })),
    [workflowDefinitions],
  );

  const stepOptions = useMemo(
    () => stepDefinitions.map((step) => ({ id: step.stepType, label: step.name })),
    [stepDefinitions],
  );

  const startExecution = async () => {
    const prompt = beginPrompt.trim();
    if (!prompt) {
      window.alert("Begin prompt is required.");
      return;
    }

    setLaunching(true);
    try {
      await ensureProjectHasUsableBinding(projectId, bindings);

      if (mode === "new-workflow") {
        await navigate({
          to: "/workflows/create",
          search: { projectId },
        });
        return;
      }

      if (mode === "workflow-definition") {
        if (!workflowId) {
          window.alert("Select a workflow definition first.");
          return;
        }

        await startWorkflowRunUseCase.current.execute({
          workflowId,
          projectId,
          startMode: "workflow-definition",
          beginPrompt: prompt,
        });
        await navigate({ to: "/workflow-runs" });
        return;
      }

      if (!stepType) {
        window.alert("Select a step definition first.");
        return;
      }

      const selectedStep = stepDefinitions.find((step) => step.stepType === stepType);
      if (!selectedStep) {
        window.alert("The selected step definition could not be found.");
        return;
      }

      await startWorkflowRunUseCase.current.execute({
        projectId,
        startMode: "single-step",
        beginPrompt: prompt,
        stepType: selectedStep.stepType,
      });
      await navigate({ to: "/workflow-runs" });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to start execution.");
    } finally {
      setLaunching(false);
    }
  };

  return (
    <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold">Launch Execution</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Start a workflow from a definition, start one selected step, or jump into creating a new workflow.
          </p>
        </div>
        <Badge>{mode}</Badge>
      </div>

      <div className="mt-5 grid gap-4 md:grid-cols-3">
        <label className="flex items-start gap-3 rounded-2xl border border-border bg-card px-4 py-3">
          <input
            checked={mode === "workflow-definition"}
            className="mt-1"
            onChange={() => setMode("workflow-definition")}
            type="radio"
          />
          <span>
            <span className="block font-medium">Workflow definition</span>
            <span className="block text-sm text-muted-foreground">
              Start an existing workflow for this project.
            </span>
          </span>
        </label>
        <label className="flex items-start gap-3 rounded-2xl border border-border bg-card px-4 py-3">
          <input
            checked={mode === "single-step"}
            className="mt-1"
            onChange={() => setMode("single-step")}
            type="radio"
          />
          <span>
            <span className="block font-medium">Single step</span>
            <span className="block text-sm text-muted-foreground">
              Spin up one step as a temporary one-step workflow.
            </span>
          </span>
        </label>
        <label className="flex items-start gap-3 rounded-2xl border border-border bg-card px-4 py-3">
          <input
            checked={mode === "new-workflow"}
            className="mt-1"
            onChange={() => setMode("new-workflow")}
            type="radio"
          />
          <span>
            <span className="block font-medium">Create workflow</span>
            <span className="block text-sm text-muted-foreground">
              Jump to the workflow composer with this project preselected.
            </span>
          </span>
        </label>
      </div>

      <div className="mt-5 grid gap-4 md:grid-cols-2">
        <label className="space-y-2 text-sm md:col-span-2">
          <span className="font-medium">Begin prompt</span>
          <textarea
            className="min-h-28 w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="I want to implement the login feature."
            value={beginPrompt}
            onChange={(event) => setBeginPrompt(event.target.value)}
          />
        </label>
        {mode === "workflow-definition" ? (
          <label className="space-y-2 text-sm">
            <span className="font-medium">Workflow definition</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3"
              value={workflowId}
              onChange={(event) => setWorkflowId(event.target.value)}
            >
              {workflowOptions.length === 0 ? <option value="">No workflows available</option> : null}
              {workflowOptions.map((workflow) => (
                <option key={workflow.id} value={workflow.id}>
                  {workflow.label}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        {mode === "single-step" ? (
          <label className="space-y-2 text-sm">
            <span className="font-medium">Step definition</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3"
              value={stepType}
              onChange={(event) => setStepType(event.target.value)}
            >
              {stepOptions.length === 0 ? <option value="">No steps available</option> : null}
              {stepOptions.map((step) => (
                <option key={step.id} value={step.id}>
                  {step.label}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        {mode === "new-workflow" ? (
          <div className="rounded-2xl border border-border bg-card px-4 py-3 text-sm text-muted-foreground md:col-span-2">
            This mode opens the workflow composer. It does not launch immediately from here.
          </div>
        ) : null}
      </div>

      <div className="mt-5 flex flex-wrap items-center gap-3">
        <Button disabled={loading || launching} onClick={() => void startExecution()}>
          {mode === "new-workflow"
            ? "Create workflow"
            : launching
              ? "Starting..."
              : "Start execution"}
        </Button>
        {loading ? <p className="text-sm text-muted-foreground">Loading launch options...</p> : null}
      </div>
    </section>
  );
}

export function ProjectDetailContent({ detail }: { detail: ReturnType<typeof Route.useLoaderData> }) {
  const location = useLocation();
  const router = useRouter();
  const navigate = useNavigate();
  const [isEditExpanded, setIsEditExpanded] = useState(false);
  const [form, setForm] = useState({
    name: detail.project.name,
    description: detail.project.description,
    repositoryUrl: detail.project.repositoryUrl,
    status: detail.project.status,
    platform: detail.project.platform,
  });
  const [bindings, setBindings] = useState<BindingDraft[]>(() =>
    getInitialBindingDrafts(detail.bindings, detail.project.directoryPath),
  );
  const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>(
    detail.teams.map((team: any) => team.id)
  );

  const updateProject = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      const normalizedBindings = bindings
        .map((binding) => ({
          ...binding,
          localPath: binding.localPath.trim(),
          label: binding.label.trim(),
        }))
        .filter((binding) => binding.localPath.length > 0);

      if (normalizedBindings.length === 0) {
        throw new Error("Add at least one directory binding.");
      }

      const uniquePaths = new Set<string>();
      for (const binding of normalizedBindings) {
        if (uniquePaths.has(binding.localPath)) {
          throw new Error("Directory bindings must use unique paths.");
        }
        uniquePaths.add(binding.localPath);
      }

      const project = await gateways.projectGateway.updateProject(detail.project.id, {
        name: form.name,
        description: form.description,
        repositoryUrl: form.repositoryUrl,
        directoryPath: normalizedBindings[0]?.localPath ?? null,
        status: form.status,
        platform: form.platform,
      });

      const retainedPersistedBindingIds = new Set(
        normalizedBindings.filter((binding) => binding.persisted).map((binding) => binding.id),
      );
      const bindingsToDelete = detail.bindings.filter(
        (binding) => !retainedPersistedBindingIds.has(binding.id),
      );

      await Promise.all(
        bindingsToDelete.map((binding) =>
          gateways.projectGateway.deleteProjectWorkspaceBinding(binding.id),
        ),
      );

      for (const [index, binding] of normalizedBindings.entries()) {
        const label = binding.label || (index === 0 ? "Primary" : null);
        if (binding.persisted) {
          await gateways.projectGateway.updateProjectWorkspaceBinding(binding.id, {
            localPath: binding.localPath,
            label,
          });
          continue;
        }

        await gateways.projectGateway.createProjectWorkspaceBinding(detail.project.id, {
          localPath: binding.localPath,
          label,
        });
      }

      const { toLink, toUnlink } = getTeamLinkDelta(detail.teams, selectedTeamIds);
      await Promise.all([
        ...toLink.map((teamId) => gateways.teamGateway.linkTeamToProject(detail.project.id, teamId)),
        ...toUnlink.map((teamId) =>
          gateways.teamGateway.unlinkTeamFromProject(detail.project.id, teamId),
        ),
      ]);

      return project;
    },
    onSuccess: async () => {
      await router.invalidate();
    },
    onError: (error) => {
      window.alert(error instanceof Error ? error.message : "Unable to update project.");
    },
  });

  const deleteProject = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      await gateways.projectGateway.deleteProject(detail.project.id);
    },
    onSuccess: async () => {
      await router.invalidate();
      await navigate({ to: "/projects" });
    },
    onError: (error) => {
      window.alert(error instanceof Error ? error.message : "Unable to delete project.");
    },
  });

  const handleUpdate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    updateProject.mutate();
  };

  const toggleTeam = (teamId: string) => {
    setSelectedTeamIds((current) =>
      current.includes(teamId)
        ? current.filter((selectedTeamId) => selectedTeamId !== teamId)
        : [...current, teamId],
    );
  };

  const applyDirectorySelection = async (bindingId: string) => {
    try {
      const gateways = createGatewayBundle();
      const selection = await gateways.localRunnerGateway.pickDirectory();
      setBindings((current) =>
        current.map((binding) =>
          binding.id === bindingId ? { ...binding, localPath: selection.path } : binding,
        ),
      );
    } catch (error) {
      window.alert(
        error instanceof Error
          ? `${error.message}. Paste the project path manually if needed.`
          : "Unable to select a directory. Paste the project path manually.",
      );
    }
  };

  const addBinding = () => {
    setBindings((current) => [
      ...current,
      { id: crypto.randomUUID(), localPath: "", label: "", persisted: false },
    ]);
  };

  const updateBinding = (bindingId: string, patch: Partial<BindingDraft>) => {
    setBindings((current) =>
      current.map((binding) => (binding.id === bindingId ? { ...binding, ...patch } : binding)),
    );
  };

  const removeBinding = (bindingId: string) => {
    setBindings((current) =>
      current.length === 1 ? current : current.filter((binding) => binding.id !== bindingId),
    );
  };

  if (location.pathname !== `/projects/${detail.project.id}`) {
    return <Outlet />;
  }

  return (
    <PageFrame description={detail.project.description} title={detail.project.name}>
      <div className="space-y-6">
        <div className="flex flex-wrap gap-2">
          <Badge>{detail.project.platform}</Badge>
          <Badge>{detail.project.status}</Badge>
          <Link to="/projects/$projectId/workflows" params={{ projectId: detail.project.id }}>
            <Button variant="secondary">Trigger workflow</Button>
          </Link>
          <Link to="/workflows/create" search={{ projectId: detail.project.id }}>
            <Button variant="secondary">Create private workflow</Button>
          </Link>
        </div>

        <ProjectExecutionLauncher bindings={detail.bindings} projectId={detail.project.id} />

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold">Edit project</h2>
              <p className="text-sm text-muted-foreground">
                Update the project name, description, status, repository, and directory bindings.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="secondary"
                onClick={() => setIsEditExpanded((current) => !current)}
              >
                {isEditExpanded ? "Collapse" : "Expand"}
              </Button>
              <Button
                type="button"
                variant="secondary"
                className="text-red-600"
                disabled={deleteProject.isPending}
                onClick={() => {
                  if (window.confirm("Delete this project?")) {
                    deleteProject.mutate();
                  }
                }}
              >
                {deleteProject.isPending ? "Deleting..." : "Delete"}
              </Button>
            </div>
          </div>
          {isEditExpanded ? (
            <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handleUpdate}>
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={form.name}
                onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                required
              />
              <input
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={form.repositoryUrl}
                onChange={(event) =>
                  setForm((current) => ({ ...current, repositoryUrl: event.target.value }))
                }
                required
              />
              <textarea
                className="min-h-28 rounded-2xl border border-border bg-card px-4 py-3 md:col-span-2"
                value={form.description}
                onChange={(event) =>
                  setForm((current) => ({ ...current, description: event.target.value }))
                }
                required
              />
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={form.platform}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    platform: event.target.value as typeof form.platform,
                  }))
                }
              >
                <option value="android">android</option>
                <option value="ios">ios</option>
                <option value="web">web</option>
                <option value="multi">multi</option>
              </select>
              <select
                className="rounded-2xl border border-border bg-card px-4 py-3"
                value={form.status}
                onChange={(event) => setForm((current) => ({ ...current, status: event.target.value }))}
              >
                <option value="active">active</option>
                <option value="archived">archived</option>
              </select>
              <div className="rounded-[1.5rem] border border-border bg-card/80 p-4 md:col-span-2">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <p className="font-medium">Directory bindings</p>
                    <p className="text-sm text-muted-foreground">
                      Manage one or more local project folders. The first binding is saved as the
                      primary path for legacy compatibility.
                    </p>
                  </div>
                  <Button type="button" variant="secondary" onClick={addBinding}>
                    Add binding
                  </Button>
                </div>
                <div className="mt-4 space-y-3">
                  {bindings.map((binding, index) => {
                    const isPrimary = index === 0;

                    return (
                      <div
                        key={binding.id}
                        className="rounded-2xl border border-border bg-background px-4 py-3"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div>
                            <p className="font-medium">
                              {isPrimary ? "Primary binding" : `Binding ${index + 1}`}
                            </p>
                            <p className="text-xs text-muted-foreground">
                              {isPrimary ? "Used as the legacy project path." : "Optional extra binding."}
                            </p>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            <Button
                              type="button"
                              variant="secondary"
                              onClick={() => void applyDirectorySelection(binding.id)}
                            >
                              Browse folder
                            </Button>
                            <Button
                              type="button"
                              variant="secondary"
                              onClick={() => removeBinding(binding.id)}
                              disabled={bindings.length === 1}
                            >
                              Remove
                            </Button>
                          </div>
                        </div>
                        <div className="mt-3 grid gap-3 md:grid-cols-2">
                          <input
                            className="rounded-2xl border border-border bg-card px-4 py-3"
                            placeholder="/Users/tiendat/Desktop/flowpilot/backend-abc"
                            value={binding.localPath}
                            onChange={(event) =>
                              updateBinding(binding.id, { localPath: event.target.value })
                            }
                            required={isPrimary}
                          />
                          <input
                            className="rounded-2xl border border-border bg-card px-4 py-3"
                            placeholder={isPrimary ? "Primary" : "Optional label"}
                            value={binding.label}
                            onChange={(event) => updateBinding(binding.id, { label: event.target.value })}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
              <div className="rounded-[1.5rem] border border-border bg-card/70 p-4 md:col-span-2">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="font-medium">Assigned teams</p>
                    <p className="text-sm text-muted-foreground">
                      Update the teams linked to this project as part of the save flow.
                    </p>
                  </div>
                  <Badge>{selectedTeamIds.length} linked</Badge>
                </div>
                {detail.allTeams.length === 0 ? (
                  <p className="mt-4 text-sm text-muted-foreground">
                    No teams exist yet. Create one from the Members tab and it will appear here.
                  </p>
                ) : (
                  <div className="mt-4 grid gap-3 md:grid-cols-2">
                    {detail.allTeams.map((team: any) => {
                      const checked = selectedTeamIds.includes(team.id);

                      return (
                        <label
                          key={team.id}
                          className="flex items-start gap-3 rounded-2xl border border-border bg-background px-4 py-3"
                        >
                          <input
                            checked={checked}
                            className="mt-1"
                            onChange={() => toggleTeam(team.id)}
                            type="checkbox"
                          />
                          <span className="min-w-0">
                            <span className="block font-medium">{team.name}</span>
                            <span className="block text-sm text-muted-foreground">
                              Include this team in the project roster.
                            </span>
                          </span>
                        </label>
                      );
                    })}
                  </div>
                )}
              </div>
              <div className="flex items-center justify-end md:col-span-2">
                <Button disabled={updateProject.isPending} type="submit">
                  {updateProject.isPending ? "Saving..." : "Save project"}
                </Button>
              </div>
            </form>
          ) : null}
        </section>

        <ProjectSectionNav projectId={detail.project.id} />

        <section className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Linked Teams</h2>
            <div className="mt-4 space-y-3">
              {detail.teams.length === 0 ? (
                <p className="text-sm text-muted-foreground">No teams are linked yet.</p>
              ) : (
                detail.teams.map((team: any) => (
                  <Link
                    key={team.id}
                    className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                    search={{ teamId: team.id }}
                    to="/teams"
                  >
                    <span className="font-medium">{team.name}</span>
                    <Badge>linked</Badge>
                  </Link>
                ))
              )}
            </div>
          </div>

          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Member Snapshot</h2>
            <div className="mt-4 space-y-3">
              {detail.members.length === 0 ? (
                <p className="text-sm text-muted-foreground">No members are available yet.</p>
              ) : (
                detail.members.map((member: any) => (
                  <div
                    key={member.id}
                    className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                  >
                    <div>
                      <p className="font-medium">{member.name}</p>
                      <p className="text-sm text-muted-foreground">
                        {member.role} · {member.levelLabel}
                      </p>
                    </div>
                    <Badge>{member.weeklyCapacityHours}h</Badge>
                  </div>
                ))
              )}
            </div>
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">Workflow Runs</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                Workflow runs are the execution sections inside a project. Start them from the Workflows tab and review recent activity here.
              </p>
            </div>
            <Link to="/projects/$projectId/workflows" params={{ projectId: detail.project.id }}>
              <Button variant="secondary">Open workflows</Button>
            </Link>
          </div>
          <div className="mt-4 space-y-3">
            {detail.workflowRuns.length === 0 ? (
              <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
            ) : (
              detail.workflowRuns.map((run: any) => (
                <Link
                  key={run.id}
                  className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                  to="/workflow-runs"
                >
                  <span className="font-medium">{run.id}</span>
                  <Badge>{run.status}</Badge>
                </Link>
              ))
            )}
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
