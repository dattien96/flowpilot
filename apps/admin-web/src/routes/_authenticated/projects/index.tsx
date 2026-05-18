import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { Button } from "@/components/ui/button";
import { PageFrame } from "@/components/common/page-frame";
import { Badge } from "@/presentation/components/ui/badge";
import { getTeamLinkDelta } from "./project-team-links";

export const Route = createFileRoute("/_authenticated/projects/")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [projects, allTeams] = await Promise.all([
      gateways.projectGateway.listProjects(),
      gateways.teamGateway.listTeams(),
    ]);

    return { projects, allTeams };
  },
  component: ProjectsPage,
});

function ProjectsPage() {
  const { projects, allTeams } = Route.useLoaderData();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [form, setForm] = useState({
    name: "",
    description: "",
    platform: "android" as "android" | "ios" | "web" | "multi",
    repositoryUrl: "",
    directoryPath: "",
  });
  const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>([]);

  const createProject = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      const project = await gateways.projectGateway.createProject({
        name: form.name,
        description: form.description,
        platform: form.platform,
        repositoryUrl: form.repositoryUrl,
        directoryPath: form.directoryPath || null,
        status: "active",
        artifactStoragePreference: "supabase",
      });

      const { toLink } = getTeamLinkDelta([], selectedTeamIds);
      await Promise.all(
        toLink.map((teamId) => gateways.teamGateway.linkTeamToProject(project.id, teamId)),
      );

      return project;
    },
    onSuccess: async () => {
      setForm({
        name: "",
        description: "",
        platform: "android",
        repositoryUrl: "",
        directoryPath: "",
      });
      setSelectedTeamIds([]);
      await queryClient.invalidateQueries();
      await router.invalidate();
    },
  });

  const toggleTeam = (teamId: string) => {
    setSelectedTeamIds((current) =>
      current.includes(teamId)
        ? current.filter((selectedTeamId) => selectedTeamId !== teamId)
        : [...current, teamId],
    );
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    createProject.mutate();
  };

  return (
    <PageFrame
      description="Project registry with management metadata and route entry points for team-oriented project work."
      title="Projects"
    >
      <section className="mb-6 rounded-[1.5rem] border border-border bg-background/60 p-5">
        <h2 className="text-xl font-semibold">Create project</h2>
        <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handleSubmit}>
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="Project name"
            value={form.name}
            onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
            required
          />
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="https://github.com/org/repo"
            type="url"
            value={form.repositoryUrl}
            onChange={(event) =>
              setForm((current) => ({ ...current, repositoryUrl: event.target.value }))
            }
            required
          />
          <textarea
            className="min-h-28 rounded-2xl border border-border bg-card px-4 py-3 md:col-span-2"
            placeholder="Short project description"
            value={form.description}
            onChange={(event) =>
              setForm((current) => ({ ...current, description: event.target.value }))
            }
            required
          />
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="Optional directory path"
            value={form.directoryPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, directoryPath: event.target.value }))
            }
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
          <div className="rounded-[1.5rem] border border-border bg-card/70 p-4 md:col-span-2">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="font-medium">Assigned teams</p>
                <p className="text-sm text-muted-foreground">
                  Select existing teams that should be linked as soon as the project is created.
                </p>
              </div>
              <Badge>{selectedTeamIds.length} selected</Badge>
            </div>
            {allTeams.length === 0 ? (
              <p className="mt-4 text-sm text-muted-foreground">
                No teams exist yet. Create one from a project Members screen, then assign it here.
              </p>
            ) : (
              <div className="mt-4 grid gap-3 md:grid-cols-2">
                {allTeams.map((team) => {
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
                          Link this team to the new project.
                        </span>
                      </span>
                    </label>
                  );
                })}
              </div>
            )}
          </div>
          <div className="flex items-center justify-end md:col-span-2">
            <Button type="submit">Create project</Button>
          </div>
        </form>
      </section>

      <div className="grid gap-4 xl:grid-cols-2">
        {projects.map((project) => (
          <Link
            key={project.id}
            className="rounded-[1.5rem] border border-border bg-background/60 p-5 transition-transform hover:-translate-y-0.5"
            params={{ projectId: project.id }}
            to="/projects/$projectId"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <h3 className="text-xl font-semibold">{project.name}</h3>
                <p className="mt-2 text-sm text-muted-foreground">{project.description}</p>
              </div>
              <div className="flex flex-col items-end gap-2">
                <Badge>{project.platform}</Badge>
                <Badge>{project.status}</Badge>
              </div>
            </div>
            <p className="mt-4 text-xs text-muted-foreground">{project.repositoryUrl}</p>
            {project.directoryPath ? (
              <p className="mt-2 text-xs text-muted-foreground">
                Directory: {project.directoryPath}
              </p>
            ) : null}
            <Button className="mt-4" variant="secondary">
              Open project
            </Button>
          </Link>
        ))}
      </div>
    </PageFrame>
  );
}
