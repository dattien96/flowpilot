import { useMutation } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration, IntegrationType } from "@/domain/model/entity/integration";
import type {
  LocalRunnerHealth,
  LocalRunnerIntegrationConnectionResult,
  LocalRunnerMcpBackend,
} from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalMcpBackendsUseCase } from "@/domain/usecase/local-runner/list-local-mcp-backends-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import {
  buildConfigFromDraft,
  buildJiraConnectionRequestFields,
  createEmptyDraftConfig,
  findDuplicateJiraIntegration,
  jiraMcpGuideLinks,
  integrationTypes,
  normalizeJiraSiteUrl,
  providerFields,
  toTitleCase,
} from "@/features/mcp/integration-config";
import { Badge } from "@/presentation/components/ui/badge";

function defaultLabel(providerType: IntegrationType) {
  return `${toTitleCase(providerType)} MCP`;
}

function isMcpTypeEnabled(
  backends: LocalRunnerMcpBackend[],
  integrations: Integration[],
  providerType: IntegrationType,
) {
  const backend = backends.find((item) => item.providerType === providerType);
  if (!backend) {
    return false;
  }
  if (backend.transport === "remote") {
    return integrations.some(
      (integration) => integration.type === providerType && integration.mcpTypeEnabled === true,
    );
  }
  return backend.state === "installed";
}

function JiraMcpGuide() {
  return (
    <div className="rounded-2xl border border-border bg-card/70 p-4">
      <p className="text-sm font-medium">Jira setup references</p>
      <div className="mt-2 flex flex-col gap-2 text-sm">
        {jiraMcpGuideLinks.map((link) => (
          <a
            key={link.href}
            className="text-primary underline-offset-4 hover:underline"
            href={link.href}
            rel="noreferrer"
            target="_blank"
          >
            {link.label}
          </a>
        ))}
      </div>
    </div>
  );
}

export const Route = createFileRoute("/_authenticated/settings/mcp-servers/create")({
  validateSearch: (search: Record<string, unknown>) => ({
    provider: typeof search.provider === "string" ? search.provider : undefined,
  }),
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [health, backends, projects, allIntegrations] = await Promise.all([
      new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
      new ListLocalMcpBackendsUseCase(gateways.localRunnerGateway).execute(),
      new ListProjectsUseCase(gateways.projectGateway).execute(),
      gateways.integrationGateway.listAllIntegrations(),
    ]);

    return { allIntegrations, backends, health, projects };
  },
  component: McpCreatePage,
});

function McpCreatePage() {
  const { allIntegrations, backends, health, projects } = Route.useLoaderData();
  const search = Route.useSearch();
  const router = useRouter();
  const runnerOnline = health.status === "online";
  const requestedProvider = (
    search.provider && integrationTypes.includes(search.provider as IntegrationType)
      ? (search.provider as IntegrationType)
      : undefined
  );

  const enabledTypes = useMemo(
    () =>
      integrationTypes.filter((type) => isMcpTypeEnabled(backends, allIntegrations, type)),
    [allIntegrations, backends],
  );

  const forcedProvider = requestedProvider && !enabledTypes.includes(requestedProvider)
    ? requestedProvider
    : null;
  const providerOptions = forcedProvider ? [forcedProvider] : enabledTypes;

  const [selectedProjectId, setSelectedProjectId] = useState(projects[0]?.id ?? "");
  const [draftType, setDraftType] = useState<IntegrationType>(providerOptions[0] ?? "jira");
  const [draftLabel, setDraftLabel] = useState(defaultLabel(providerOptions[0] ?? "jira"));
  const [draftConfigValues, setDraftConfigValues] = useState<Record<string, string>>(
    createEmptyDraftConfig(providerOptions[0] ?? "jira"),
  );
  const [editorError, setEditorError] = useState<string | null>(null);

  useEffect(() => {
    const nextType = providerOptions[0] ?? "jira";
    if (providerOptions.length === 0) {
      return;
    }
    if (providerOptions.includes(draftType)) {
      return;
    }
    setDraftType(nextType);
    setDraftLabel(defaultLabel(nextType));
    setDraftConfigValues(createEmptyDraftConfig(nextType));
  }, [draftType, providerOptions]);

  const saveIntegration = useMutation({
    mutationFn: async () => {
      if (!runnerOnline) {
        throw new Error(`The local runner is unreachable at ${health.baseUrl}.`);
      }
      if (!selectedProjectId) {
        throw new Error("Project is required.");
      }
      if (providerOptions.length === 0) {
        throw new Error("Enable at least one MCP type before creating an MCP instance.");
      }

      const label = draftLabel.trim();
      if (!label) {
        throw new Error("Label is required.");
      }

      if (draftType === "jira") {
        const workspaceUrl = draftConfigValues.workspaceUrl.trim();
        const projectKey = draftConfigValues.projectKey.trim();
        const email = draftConfigValues.email.trim();
        const apiToken = draftConfigValues.apiToken.trim();

        if (!workspaceUrl) {
          throw new Error("Workspace URL is required.");
        }
        if (!projectKey) {
          throw new Error("Project Key is required.");
        }
        if (!email) {
          throw new Error("Atlassian Email is required.");
        }
        if (!apiToken) {
          throw new Error("API Token is required.");
        }
      }

      const gateways = await createGatewayBundle();
      if (draftType === "jira") {
        const duplicateIntegration = findDuplicateJiraIntegration(allIntegrations, draftConfigValues);
        if (duplicateIntegration) {
          throw new Error(
            `A Jira MCP for ${normalizeJiraSiteUrl(draftConfigValues.workspaceUrl)} already exists.`,
          );
        }
      }

      const parsedConfig = buildConfigFromDraft(draftType, draftConfigValues);
      const integration = await gateways.integrationGateway.createIntegration({
        projectId: selectedProjectId,
        type: draftType,
        label,
        configEncrypted: parsedConfig,
        mcpTypeEnabled: true,
      });

      const result = await gateways.localRunnerGateway.triggerIntegrationConnection({
        projectId: integration.projectId,
        integrationId: integration.id,
        providerType: integration.type,
        action: "test",
        ...(integration.type === "jira"
          ? buildJiraConnectionRequestFields(draftConfigValues)
          : {}),
      });

      await gateways.integrationGateway.updateIntegration(integration.id, {
        type: integration.type,
        label: integration.label,
        configEncrypted: integration.configEncrypted,
        status: result.integrationStatus,
        lastError: result.integrationStatus === "failed" ? result.message ?? null : null,
        lastSyncedAt: result.integrationStatus === "connected" ? new Date().toISOString() : null,
        mcpTypeEnabled: true,
      });

      if (result.requestStatus === "rejected" || result.integrationStatus === "failed") {
        throw new Error(result.message ?? "Unable to save MCP.");
      }
    },
    onMutate: () => {
      setEditorError(null);
    },
    onSuccess: async () => {
      await router.invalidate();
      await router.navigate({ to: "/settings/mcp-servers" });
    },
    onError: (error) => {
      setEditorError(error instanceof Error ? error.message : "Unable to save MCP.");
    },
  });

  function updateDraftType(nextType: IntegrationType) {
    setDraftType(nextType);
    setDraftLabel(defaultLabel(nextType));
    setDraftConfigValues(createEmptyDraftConfig(nextType));
  }

  function updateDraftConfigValue(key: string, value: string) {
    setDraftConfigValues((current) => ({
      ...current,
      [key]: value,
    }));
  }

  return (
    <PageFrame
      description="Create a reusable MCP instance from the MCP types that are currently enabled."
      title="Create MCP"
    >
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              MCP Instance Setup
            </p>
            <h3 className="mt-3 text-2xl font-semibold tracking-tight">
              Create and connect an MCP instance
            </h3>
            <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
              Choose an owner project, select an enabled MCP type, and let the runner verify the
              connection after creation.
            </p>
          </div>
          <Badge tone={runnerOnline ? "success" : "danger"}>
            {runnerOnline ? "runner online" : "runner offline"}
          </Badge>
        </div>

        {forcedProvider ? (
          <div className="mt-4 rounded-2xl border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-foreground">
            Creating the first {toTitleCase(forcedProvider)} MCP instance will also enable that MCP type.
          </div>
        ) : null}

        {providerOptions.length === 0 ? (
          <div className="mt-4 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
            No enabled MCP types are available yet. Enable an MCP type from the global MCP Servers page first.
          </div>
        ) : (
          <div className="mt-4 grid gap-3 rounded-2xl border border-dashed border-border bg-card/60 p-4">
            <label className="text-sm font-medium">
              Project
              <select
                className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) => setSelectedProjectId(event.target.value)}
                value={selectedProjectId}
              >
                {projects.map((project) => (
                  <option key={project.id} value={project.id}>
                    {project.name}
                  </option>
                ))}
              </select>
            </label>

            {forcedProvider ? (
              <div className="text-sm font-medium">
                Provider
                <div className="mt-2 rounded-2xl border border-border bg-card px-4 py-3">
                  {toTitleCase(forcedProvider)}
                </div>
              </div>
            ) : (
              <label className="text-sm font-medium">
                Provider
                <select
                  className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                  onChange={(event) => updateDraftType(event.target.value as IntegrationType)}
                  value={draftType}
                >
                  {providerOptions.map((type) => (
                    <option key={type} value={type}>
                      {toTitleCase(type)}
                    </option>
                  ))}
                </select>
              </label>
            )}

            {draftType === "jira" ? <JiraMcpGuide /> : null}

            <label className="text-sm font-medium">
              Label
              <input
                className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) => setDraftLabel(event.target.value)}
                placeholder="FlowPilot Jira"
                value={draftLabel}
              />
            </label>

            {providerFields[draftType].map((field) => (
              <label className="text-sm font-medium" key={field.key}>
                {field.label}
                <input
                  autoComplete={field.autoComplete}
                  className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                  onChange={(event) => updateDraftConfigValue(field.key, event.target.value)}
                  placeholder={field.placeholder}
                  type={field.inputType ?? "text"}
                  value={draftConfigValues[field.key] ?? ""}
                />
                {field.helpText ? (
                  <span className="mt-2 block text-sm font-normal text-muted-foreground">
                    {field.helpText}
                  </span>
                ) : null}
              </label>
            ))}

            <p className="text-sm text-muted-foreground">
              Supported providers hand off authentication to the local runner. Jira still requires the runner to be online so it can validate the stored connection on this machine.
            </p>

            {editorError ? <p className="text-sm text-danger">{editorError}</p> : null}

            <div className="flex flex-wrap gap-2">
              <Button
                disabled={!runnerOnline || projects.length === 0 || providerOptions.length === 0 || saveIntegration.isPending}
                onClick={() => saveIntegration.mutate()}
                type="button"
              >
                {saveIntegration.isPending ? "Creating..." : "Create and Connect"}
              </Button>
              <Button
                onClick={() => router.navigate({ to: "/settings/mcp-servers" })}
                type="button"
                variant="secondary"
              >
                Cancel
              </Button>
            </div>
          </div>
        )}
      </section>
    </PageFrame>
  );
}
