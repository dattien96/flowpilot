import { useMutation } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useMemo, useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/components/ui/button";

const mcpStatusOptions = ["PENDING", "CONNECTED", "FAILED"] as const;

export const Route = createFileRoute("/_authenticated/projects/$projectId/settings")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const [project, teams, allTeams] = await Promise.all([
      gateways.projectGateway.getProjectById(params.projectId),
      gateways.teamGateway.listTeamsByProject(params.projectId),
      gateways.teamGateway.listTeams(),
    ]);
    return { project, teams, allTeams, projectId: params.projectId };
  },
  component: ProjectSettingsPage,
});

function ProjectSettingsPage() {
  const { project, teams, allTeams, projectId } = Route.useLoaderData();
  if (!project) {
    return null;
  }

  return (
    <ProjectSettingsContent
      allTeams={allTeams}
      key={project.id}
      project={project}
      projectId={projectId}
      teams={teams}
    />
  );
}

function ProjectSettingsContent({
  project,
  teams,
  allTeams,
  projectId,
}: {
  project: NonNullable<ReturnType<typeof Route.useLoaderData>["project"]>;
  teams: ReturnType<typeof Route.useLoaderData>["teams"];
  allTeams: ReturnType<typeof Route.useLoaderData>["allTeams"];
  projectId: string;
}) {
  const router = useRouter();
  const [artifactStoragePreference, setArtifactStoragePreference] = useState<"supabase" | "google_drive">(
    project.artifactStoragePreference ?? "supabase",
  );
  const [mcpContexts] = useState([
    { id: "git", type: "Git", status: "CONNECTED" as (typeof mcpStatusOptions)[number] },
    { id: "jira", type: "Jira", status: "PENDING" as (typeof mcpStatusOptions)[number] },
  ]);
  const [newContextType, setNewContextType] = useState("Custom");

  const saveStoragePreference = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.projectGateway.updateProject(projectId, {
        artifactStoragePreference,
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const linkTeam = useMutation({
    mutationFn: async (teamId: string) => {
      const gateways = await createGatewayBundle();
      await gateways.teamGateway.linkTeamToProject(projectId, teamId);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const unlinkTeam = useMutation({
    mutationFn: async (teamId: string) => {
      const gateways = await createGatewayBundle();
      await gateways.teamGateway.unlinkTeamFromProject(projectId, teamId);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const linkedTeamIds = useMemo(() => new Set(teams.map((team) => team.id)), [teams]);

  return (
    <PageFrame description="Project settings and integration status." title="Settings">
      <div className="space-y-6">
        <ProjectSectionNav projectId={projectId} />

        <section className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">Artifact Storage Preference</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Persist the project storage target used for future artifact workflows.
                </p>
              </div>
              <Badge>{project.status}</Badge>
            </div>
            <p className="mt-3 text-sm text-muted-foreground">
              Select where generated artifacts should be stored for this project.
            </p>
            <label className="mt-4 block text-sm font-medium">Storage strategy</label>
            <select
              className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
              value={artifactStoragePreference}
              onChange={(event) =>
                setArtifactStoragePreference(event.target.value as "supabase" | "google_drive")
              }
            >
              <option value="supabase">Supabase</option>
              <option value="google_drive">Google Drive</option>
            </select>
            <p className="mt-3 text-sm text-muted-foreground">
              Google Drive selection requires the Google Drive MCP connection in a later phase.
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Badge>{artifactStoragePreference}</Badge>
              <Badge>{project.name}</Badge>
            </div>
            <div className="mt-4 flex justify-end">
              <Button
                disabled={saveStoragePreference.isPending}
                onClick={() => saveStoragePreference.mutate()}
                type="button"
              >
                {saveStoragePreference.isPending ? "Saving..." : "Save storage setting"}
              </Button>
            </div>
          </div>

          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">MCP Contexts</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Configured MCP contexts with their connection status.
                </p>
              </div>
              <Button type="button" variant="secondary">
                Add MCP
              </Button>
            </div>
            <div className="mt-4 flex gap-2">
              {mcpStatusOptions.map((status) => (
                <Badge key={status}>{status}</Badge>
              ))}
            </div>
            <div className="mt-4 space-y-2">
              {mcpContexts.map((context) => (
                <div
                  key={context.id}
                  className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                >
                  <div>
                    <p className="font-medium">{context.type}</p>
                    <p className="text-sm text-muted-foreground">Context id: {context.id}</p>
                  </div>
                  <Badge>{context.status}</Badge>
                </div>
              ))}
            </div>
            <div className="mt-4 rounded-2xl border border-dashed border-border bg-card/60 p-4">
              <p className="text-sm font-medium">Add MCP placeholder</p>
              <p className="mt-2 text-sm text-muted-foreground">
                Select the MCP type and then configure it per SD-04 §4. This step is intentionally not wired yet.
              </p>
              <div className="mt-4 flex items-center gap-3">
              <select
                className="flex-1 rounded-2xl border border-border bg-card px-4 py-3"
                value={newContextType}
                onChange={(event) => setNewContextType(event.target.value)}
              >
                <option value="Custom">Custom</option>
                <option value="Git">Git</option>
                <option value="Jira">Jira</option>
                <option value="Google Drive">Google Drive</option>
              </select>
              <Button
                type="button"
                variant="secondary"
                disabled
              >
                Configure
              </Button>
            </div>
            </div>
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <h2 className="text-xl font-semibold">Project ↔ Team Links</h2>
          <div className="mt-4 space-y-3">
            {allTeams.length === 0 ? (
              <p className="text-sm text-muted-foreground">Create a team first to link it here.</p>
            ) : (
              allTeams.map((team) => {
                const linked = linkedTeamIds.has(team.id);
                return (
                  <div
                    key={team.id}
                    className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                  >
                    <div>
                      <p className="font-medium">{team.name}</p>
                      <p className="text-sm text-muted-foreground">
                        {linked ? "linked to project" : "available to link"}
                      </p>
                    </div>
                    {linked ? (
                      <Button
                        className="bg-danger text-white hover:bg-danger/90"
                        type="button"
                        variant="secondary"
                        onClick={() => unlinkTeam.mutate(team.id)}
                      >
                        Unlink
                      </Button>
                    ) : (
                      <Button type="button" onClick={() => linkTeam.mutate(team.id)}>
                        Link
                      </Button>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
