import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, useLocation, useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/components/ui/button";
import { getTeamLinkDelta } from "./project-team-links";

export const Route = createFileRoute("/_authenticated/projects/$projectId")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const [project, features, workflowRuns, teams, allTeams] = await Promise.all([
      gateways.projectGateway.getProjectById(params.projectId),
      gateways.featureGateway.listFeaturesByProject(params.projectId),
      gateways.workflowGateway.listWorkflowRuns(),
      gateways.teamGateway.listTeamsByProject(params.projectId),
      gateways.teamGateway.listTeams(),
    ]);

    const members =
      teams.length === 0
        ? []
        : (await Promise.all(teams.map((team) => gateways.teamGateway.listMembersByTeam(team.id)))).flat();

    return {
      project,
      features,
      workflowRuns: workflowRuns.filter((run) => run.projectId === params.projectId),
      teams,
      allTeams,
      members,
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

function ProjectDetailContent({ detail }: { detail: ReturnType<typeof Route.useLoaderData> }) {
  const location = useLocation();
  const router = useRouter();
  const navigate = useNavigate();
  const [form, setForm] = useState({
    name: detail.project.name,
    description: detail.project.description,
    repositoryUrl: detail.project.repositoryUrl,
    directoryPath: detail.project.directoryPath ?? "",
    status: detail.project.status,
    platform: detail.project.platform,
  });
  const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>(detail.teams.map((team) => team.id));

  const updateProject = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      const project = await gateways.projectGateway.updateProject(detail.project.id, {
        name: form.name,
        description: form.description,
        repositoryUrl: form.repositoryUrl,
        directoryPath: form.directoryPath || null,
        status: form.status,
        platform: form.platform,
      });

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

  if (location.pathname !== `/projects/${detail.project.id}`) {
    return <Outlet />;
  }

  return (
    <PageFrame description={detail.project.description} title={detail.project.name}>
      <div className="space-y-6">
        <div className="flex flex-wrap gap-2">
          <Badge>{detail.project.platform}</Badge>
          <Badge>{detail.project.status}</Badge>
        </div>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold">Edit project</h2>
              <p className="text-sm text-muted-foreground">
                Update the project name, description, status, repository, and directory path.
              </p>
            </div>
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                if (window.confirm("Delete this project?")) {
                  deleteProject.mutate();
                }
              }}
            >
              Delete
            </Button>
          </div>
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
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              value={form.directoryPath}
              onChange={(event) =>
                setForm((current) => ({ ...current, directoryPath: event.target.value }))
              }
              placeholder="Optional directory path"
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
                  {detail.allTeams.map((team) => {
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
              <Button type="submit">Save project</Button>
            </div>
          </form>
        </section>

        <ProjectSectionNav projectId={detail.project.id} />

        <section className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Linked Teams</h2>
            <div className="mt-4 space-y-3">
              {detail.teams.length === 0 ? (
                <p className="text-sm text-muted-foreground">No teams are linked yet.</p>
              ) : (
                detail.teams.map((team) => (
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
                detail.members.map((member) => (
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

        <section className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Features</h2>
            <div className="mt-4 space-y-3">
              {detail.features.map((feature) => (
                <div
                  key={feature.id}
                  className="rounded-2xl border border-border bg-card px-4 py-3"
                >
                  <p className="font-medium">{feature.title}</p>
                  <p className="text-sm text-muted-foreground">{feature.status}</p>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Workflow Runs</h2>
            <div className="mt-4 space-y-3">
              {detail.workflowRuns.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
              ) : (
                detail.workflowRuns.map((run) => (
                  <Link
                    key={run.id}
                    className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                    params={{ runId: run.id }}
                    to="/workflow-runs/$runId"
                  >
                    <span className="font-medium">{run.id}</span>
                    <Badge>{run.status}</Badge>
                  </Link>
                ))
              )}
            </div>
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
