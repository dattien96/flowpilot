import { useMutation } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration, IntegrationStatus, IntegrationType } from "@/domain/model/entity/integration";
import {
  formatTimestamp,
  integrationTypes,
  toTitleCase,
} from "@/features/mcp/integration-config";
import { Badge } from "@/presentation/components/ui/badge";

function integrationTone(status: IntegrationStatus) {
  switch (status) {
    case "connected":
      return "success";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

function integrationStatusLabel(status: IntegrationStatus) {
  switch (status) {
    case "awaiting_oauth":
      return "Awaiting OAuth";
    default:
      return toTitleCase(status);
  }
}

export const Route = createFileRoute("/_authenticated/projects/$projectId/settings")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const [project, teams, allTeams, linkedIntegrations, availableIntegrations] = await Promise.all([
      gateways.projectGateway.getProjectById(params.projectId),
      gateways.teamGateway.listTeamsByProject(params.projectId),
      gateways.teamGateway.listTeams(),
      gateways.integrationGateway.listLinkedIntegrationsByProject(params.projectId),
      gateways.integrationGateway.listAllIntegrations(),
    ]);
    return {
      project,
      teams,
      allTeams,
      linkedIntegrations,
      availableIntegrations,
      projectId: params.projectId,
    };
  },
  component: ProjectSettingsPage,
});

function ProjectSettingsPage() {
  const { project, teams, allTeams, linkedIntegrations, availableIntegrations, projectId } =
    Route.useLoaderData();
  if (!project) {
    return null;
  }

  return (
    <ProjectSettingsContent
      allTeams={allTeams}
      availableIntegrations={availableIntegrations}
      linkedIntegrations={linkedIntegrations}
      key={project.id}
      project={project}
      projectId={projectId}
      teams={teams}
    />
  );
}

export function ProjectSettingsContent({
  project,
  teams,
  allTeams,
  linkedIntegrations,
  availableIntegrations,
  projectId,
}: {
  project: NonNullable<ReturnType<typeof Route.useLoaderData>["project"]>;
  teams: ReturnType<typeof Route.useLoaderData>["teams"];
  allTeams: ReturnType<typeof Route.useLoaderData>["allTeams"];
  linkedIntegrations: ReturnType<typeof Route.useLoaderData>["linkedIntegrations"];
  availableIntegrations: ReturnType<typeof Route.useLoaderData>["availableIntegrations"];
  projectId: string;
}) {
  const router = useRouter();
  const [artifactStoragePreference, setArtifactStoragePreference] = useState<"supabase" | "google_drive">(
    project.artifactStoragePreference ?? "supabase",
  );
  const [selectedLinks, setSelectedLinks] = useState<Partial<Record<IntegrationType, string>>>({});

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

  const linkIntegration = useMutation({
    mutationFn: async ({
      integrationType,
      integrationId,
    }: {
      integrationType: IntegrationType;
      integrationId: string;
    }) => {
      const gateways = await createGatewayBundle();
      await gateways.integrationGateway.linkIntegrationToProject(projectId, integrationId);
      setSelectedLinks((current) => ({ ...current, [integrationType]: integrationId }));
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const unlinkIntegration = useMutation({
    mutationFn: async (integration: Integration) => {
      const gateways = await createGatewayBundle();
      await gateways.integrationGateway.unlinkIntegrationFromProject(projectId, integration.id);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const linkedTeamIds = useMemo(() => new Set(teams.map((team) => team.id)), [teams]);
  const linkedByType = useMemo(
    () => new Map(linkedIntegrations.map((integration) => [integration.type, integration])),
    [linkedIntegrations],
  );

  const availableByType = useMemo(() => {
    return integrationTypes.reduce<Record<IntegrationType, Integration[]>>((accumulator, type) => {
      accumulator[type] = availableIntegrations.filter((integration) => integration.type === type);
      return accumulator;
    }, {
      jira: [],
      figma: [],
      google_drive: [],
      firebase: [],
      telegram: [],
    });
  }, [availableIntegrations]);

  useEffect(() => {
    setSelectedLinks((current) => {
      const next = { ...current };
      for (const type of integrationTypes) {
        if (next[type]) {
          continue;
        }
        next[type] = linkedByType.get(type)?.id ?? availableByType[type][0]?.id ?? "";
      }
      return next;
    });
  }, [availableByType, linkedByType]);

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
              Google Drive selection requires a connected Google Drive MCP context.
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
                <h2 className="text-xl font-semibold">Project MCP Links</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Link one saved MCP instance per type to this project. Create and edit MCP
                  instances from the global MCP Servers page.
                </p>
              </div>
              <Button
                type="button"
                variant="secondary"
                onClick={() => router.navigate({ to: "/settings/mcp-servers" })}
              >
                Create new MCP
              </Button>
            </div>

            <div className="mt-4 space-y-2">
              {integrationTypes.map((type) => {
                const linkedIntegration = linkedByType.get(type) ?? null;
                const availableInstances = availableByType[type];
                return (
                  <div
                    key={type}
                    className="rounded-2xl border border-border bg-card px-4 py-4"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="font-medium">{toTitleCase(type)} MCP</p>
                        <p className="mt-1 text-sm text-muted-foreground">
                          {linkedIntegration
                            ? `Linked to ${linkedIntegration.label}`
                            : "No MCP instance linked for this type yet."}
                        </p>
                      </div>
                      <Badge tone={linkedIntegration ? integrationTone(linkedIntegration.status) : "neutral"}>
                        {linkedIntegration ? integrationStatusLabel(linkedIntegration.status) : "unlinked"}
                      </Badge>
                    </div>

                    {linkedIntegration ? (
                      <div className="mt-3 grid gap-2 text-sm text-muted-foreground">
                        <p>Current MCP: {linkedIntegration.label}</p>
                        <p>Owner project: {linkedIntegration.projectId}</p>
                        <p>Last sync: {formatTimestamp(linkedIntegration.lastSyncedAt)}</p>
                        <p>
                          Last error: {linkedIntegration.lastError?.trim() ? linkedIntegration.lastError : "None"}
                        </p>
                      </div>
                    ) : null}

                    <div className="mt-4 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_auto]">
                      <select
                        className="w-full rounded-2xl border border-border bg-background px-4 py-3 text-sm"
                        value={selectedLinks[type] ?? ""}
                        onChange={(event) =>
                          setSelectedLinks((current) => ({
                            ...current,
                            [type]: event.target.value,
                          }))
                        }
                      >
                        {availableInstances.length === 0 ? (
                          <option value="">No saved MCP instances available</option>
                        ) : null}
                        {availableInstances.map((integration) => (
                          <option key={integration.id} value={integration.id}>
                            {integration.label} ({integration.status})
                          </option>
                        ))}
                      </select>
                      {!linkedIntegration ? (
                        <Button
                          disabled={linkIntegration.isPending || !selectedLinks[type]}
                          type="button"
                          variant="secondary"
                          onClick={() =>
                            linkIntegration.mutate({
                              integrationType: type,
                              integrationId: selectedLinks[type] ?? "",
                            })
                          }
                        >
                          {linkIntegration.isPending ? "Linking..." : "Link MCP"}
                        </Button>
                      ) : null}
                      {linkedIntegration ? (
                        <Button
                          className="bg-danger text-white hover:bg-danger/90"
                          disabled={unlinkIntegration.isPending}
                          type="button"
                          variant="secondary"
                          onClick={() => unlinkIntegration.mutate(linkedIntegration)}
                        >
                          {unlinkIntegration.isPending ? "Unlinking..." : "Unlink"}
                        </Button>
                      ) : null}
                    </div>
                  </div>
                );
              })}

              {linkedIntegrations.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-border bg-card/60 px-4 py-3 text-sm text-muted-foreground">
                  No MCP instances are linked to this project yet.
                </div>
              ) : null}
            </div>
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <h2 className="text-xl font-semibold">Project to Team Links</h2>
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
