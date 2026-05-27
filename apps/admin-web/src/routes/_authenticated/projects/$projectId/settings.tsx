import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration, IntegrationStatus, IntegrationType } from "@/domain/model/entity/integration";
import {
  REASONING_EFFORT_OPTIONS,
  STEP_MODEL_OPTIONS,
} from "@/domain/model/entity/workflow-engine";
import {
  formatTimestamp,
  integrationTypes,
  toTitleCase,
} from "@/features/mcp/integration-config";
import { Badge } from "@/presentation/components/ui/badge";

const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING_EFFORT = "medium";

function providerForModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (model.startsWith("gemini-")) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  return "";
}

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
  const [selectedLinks, setSelectedLinks] = useState<Partial<Record<IntegrationType, string>>>({});
  const [providerDefaults, setProviderDefaults] = useState({
    defaultModel: project.defaultModel ?? DEFAULT_MODEL,
    defaultReasoningEffort: project.defaultReasoningEffort ?? DEFAULT_REASONING_EFFORT,
  });
  const [sessionIdleTtlMinutes, setSessionIdleTtlMinutes] = useState(
    project.sessionIdleTtlMinutes ?? 120,
  );

  useEffect(() => {
    setProviderDefaults({
      defaultModel: project.defaultModel ?? DEFAULT_MODEL,
      defaultReasoningEffort: project.defaultReasoningEffort ?? DEFAULT_REASONING_EFFORT,
    });
  }, [project.defaultModel, project.defaultReasoningEffort]);

  useEffect(() => {
    setSessionIdleTtlMinutes(project.sessionIdleTtlMinutes ?? 120);
  }, [project.sessionIdleTtlMinutes]);

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

  const updateProjectDefaults = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      const defaultModel = providerDefaults.defaultModel.trim() || DEFAULT_MODEL;
      const defaultReasoningEffort =
        providerDefaults.defaultReasoningEffort.trim() || DEFAULT_REASONING_EFFORT;
      const resolvedProvider = providerForModel(defaultModel);
      if (!resolvedProvider) {
        throw new Error("Default model must start with gpt-, gemini-, or claude-.");
      }

      await gateways.projectGateway.updateProject(projectId, {
        defaultProvider: resolvedProvider,
        defaultModel,
        defaultReasoningEffort: defaultReasoningEffort || null,
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const updateSessionTtl = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      const ttl = Number(sessionIdleTtlMinutes);
      if (!Number.isFinite(ttl) || ttl < 1) {
        throw new Error("Idle timeout must be at least 1 minute.");
      }

      await gateways.projectGateway.updateProject(projectId, {
        sessionIdleTtlMinutes: ttl,
      });
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
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">AI Provider Defaults</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Model is the user-editable source of truth. Provider is derived automatically
                  from the selected model and stored only as a reference field.
                </p>
              </div>
              <Badge tone={providerDefaults.defaultModel ? "success" : "neutral"}>
                {providerForModel(providerDefaults.defaultModel) || "unknown"}
              </Badge>
            </div>

            <div className="mt-4 grid gap-3 md:grid-cols-2">
              <label className="space-y-2 text-sm">
                <span className="font-medium">Default model</span>
                <select
                  className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                  value={providerDefaults.defaultModel}
                  onChange={(event) =>
                    setProviderDefaults((current) => ({
                      ...current,
                      defaultModel: event.target.value,
                    }))
                  }
                >
                  {STEP_MODEL_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>

              <label className="space-y-2 text-sm">
                <span className="font-medium">Default reasoning effort</span>
                <select
                  className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                  value={providerDefaults.defaultReasoningEffort}
                  onChange={(event) =>
                    setProviderDefaults((current) => ({
                      ...current,
                      defaultReasoningEffort: event.target.value,
                    }))
                  }
                >
                  {REASONING_EFFORT_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm text-muted-foreground">
                Select a model and the provider is derived automatically. Reasoning effort is
                stored independently and defaults to medium when not overridden.
              </p>
              <Button
                disabled={updateProjectDefaults.isPending}
                type="button"
                onClick={() => updateProjectDefaults.mutate()}
              >
                {updateProjectDefaults.isPending ? "Saving..." : "Save defaults"}
              </Button>
            </div>
          </div>

          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-semibold">Artifact settings</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  Artifact storage and the catalog now live under the global Settings menu.
                </p>
              </div>
              <Badge>{project.status}</Badge>
            </div>
            <p className="mt-3 text-sm text-muted-foreground">
              Open the global artifact settings page to edit storage preferences and artifact
              definitions used by step editors and workflow runs.
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Badge>{project.name}</Badge>
            </div>
            <div className="mt-4 flex justify-end">
              <Link to="/artifacts">
                <Button type="button" variant="secondary">
                  Open artifacts settings
                </Button>
              </Link>
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
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold">Session idle timeout</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                Controls how long the local runner keeps an idle terminal session alive before it
                is swept and marked dead for the next use. The value is shared across machines
                through Supabase.
              </p>
            </div>
            <Badge tone="neutral">{sessionIdleTtlMinutes} min</Badge>
          </div>

          <div className="mt-4 max-w-sm space-y-2">
            <label className="space-y-2 text-sm">
              <span className="font-medium">Idle timeout in minutes</span>
              <input
                aria-label="Idle timeout in minutes"
                className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                min={1}
                name="sessionIdleTtlMinutes"
                type="number"
                value={sessionIdleTtlMinutes}
                onChange={(event) =>
                  setSessionIdleTtlMinutes(event.target.valueAsNumber || 120)
                }
              />
            </label>
          </div>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              Default is 120 minutes. This value is used by the runner when starting and
              sweeping live sessions.
            </p>
            <Button
              disabled={updateSessionTtl.isPending}
              type="button"
              onClick={() => updateSessionTtl.mutate()}
            >
              {updateSessionTtl.isPending ? "Saving..." : "Save timeout"}
            </Button>
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
