import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration, IntegrationType } from "@/domain/model/entity/integration";
import type {
  LocalRunnerHealth,
  LocalRunnerIntegrationConnectionResult,
  LocalRunnerMcpBackend,
  LocalRunnerMcpTestResult,
  LocalRunnerMcpTestRunSummary,
} from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalMcpBackendsUseCase } from "@/domain/usecase/local-runner/list-local-mcp-backends-usecase";
import {
  buildConfigFromDraft,
  buildJiraConnectionRequestFields,
  createEmptyDraftConfig,
  formatTimestamp,
  findDuplicateJiraIntegration,
  jiraMcpGuideLinks,
  integrationTypes,
  providerFields,
  normalizeJiraSiteUrl,
  toTitleCase,
} from "@/features/mcp/integration-config";
import { Badge } from "@/presentation/components/ui/badge";
import { cn } from "@/lib/utils/cn";

const secondaryLinkButtonClass =
  "inline-flex items-center justify-center rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold text-card-foreground transition-colors hover:bg-muted";

function backendTone(state: LocalRunnerMcpBackend["state"]) {
  switch (state) {
    case "installed":
      return "success";
    case "launcher_available":
      return "warning";
    case "missing":
      return "danger";
  }
}

function backendStateLabel(state: LocalRunnerMcpBackend["state"]) {
  switch (state) {
    case "launcher_available":
      return "Launcher Available";
    default:
      return state.replace("_", " ");
  }
}

function integrationTone(status: Integration["status"]) {
  switch (status) {
    case "connected":
      return "success";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

function mcpTypeTone(enabled: boolean) {
  return enabled ? "success" : "neutral";
}

function mcpTypeLabel(enabled: boolean) {
  return enabled ? "enabled" : "disabled";
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

function detailText(health: LocalRunnerHealth) {
  return (
    health.errorMessage ??
    "The runner can answer MCP backend inventory checks and expose install state to admin-web."
  );
}

function defaultLabel(providerType: IntegrationType) {
  return `${toTitleCase(providerType)} MCP`;
}

function defaultMcpTestPrompt(providerType: string) {
  switch (providerType) {
    case "jira":
      return "Find all open bugs in Project Alpha and summarize the top three results.";
    case "google_drive":
      return "List the main files available through this connection and summarize one of them.";
    default:
      return "List the capabilities available through this MCP connection and perform a short smoke test.";
  }
}

type McpTestTemplate = {
  key: string;
  label: string;
  prompt: string;
  requiresWrite: boolean;
};

function templatesForProvider(providerType: string): McpTestTemplate[] {
  switch (providerType) {
    case "jira":
      return [
        {
          key: "jira_find_open_bugs",
          label: "Find Open Bugs",
          prompt: "Find all open bugs in Project Alpha and summarize the top three results.",
          requiresWrite: false,
        },
        {
          key: "confluence_list_spaces",
          label: "List Confluence Spaces",
          prompt: "What spaces do I have access to?",
          requiresWrite: false,
        },
        {
          key: "jira_create_story",
          label: "Create Jira Story",
          prompt: "Create a story titled 'Redesign onboarding'.",
          requiresWrite: true,
        },
      ];
    default:
      return [];
  }
}

function resultTone(status: "success" | "failed") {
  return status === "success" ? "success" : "danger";
}

function preferredBackendKey(backends: LocalRunnerMcpBackend[]) {
  return backends.find((backend) => backend.providerType === "jira")?.key ?? backends[0]?.key ?? "";
}

function DetailRow({
  label,
  value,
}: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
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

export const Route = createFileRoute("/_authenticated/settings/mcp-servers")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [health, backends, projects, allIntegrations] = await Promise.all([
      new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
      new ListLocalMcpBackendsUseCase(gateways.localRunnerGateway).execute(),
      new ListProjectsUseCase(gateways.projectGateway).execute(),
      gateways.integrationGateway.listAllIntegrations(),
    ]);

    return { backends, health, projects, allIntegrations };
  },
  component: McpServersPage,
});

function McpServersPage() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const { backends, health, projects, allIntegrations } = Route.useLoaderData();

  if (pathname !== "/settings/mcp-servers") {
    return <Outlet />;
  }

  return (
    <McpServersPageContent
      allIntegrations={allIntegrations}
      backends={backends}
      health={health}
      projects={projects}
    />
  );
}

export function McpServersPageContent({
  allIntegrations,
  backends,
  health,
  projects,
}: {
  allIntegrations: Integration[];
  backends: LocalRunnerMcpBackend[];
  health: LocalRunnerHealth;
  projects: Project[];
}) {
  const router = useRouter();
  const runnerOnline = health.status === "online";
  const [editorError, setEditorError] = useState<string | null>(null);
  const [selectedProjectId, setSelectedProjectId] = useState(projects[0]?.id ?? "");
  const [selectedBackendKey, setSelectedBackendKey] = useState(preferredBackendKey(backends));
  const [draftType, setDraftType] = useState<IntegrationType>("jira");
  const [draftLabel, setDraftLabel] = useState(defaultLabel("jira"));
  const [draftConfigValues, setDraftConfigValues] = useState<Record<string, string>>(
    createEmptyDraftConfig("jira"),
  );
  const [editingIntegrationId, setEditingIntegrationId] = useState<string | null>(null);
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [selectedIntegrationId, setSelectedIntegrationId] = useState("");
  const initialTemplates = templatesForProvider(
    backends.find((backend) => backend.key === preferredBackendKey(backends))?.providerType ?? "jira",
  );
  const [selectedTemplateKey, setSelectedTemplateKey] = useState(initialTemplates[0]?.key ?? "");
  const [confirmWriteTest, setConfirmWriteTest] = useState(false);
  const [mcpTestPrompt, setMcpTestPrompt] = useState(
    defaultMcpTestPrompt(backends.find((backend) => backend.key === preferredBackendKey(backends))?.providerType ?? "jira"),
  );
  const [mcpTestError, setMcpTestError] = useState<string | null>(null);
  const [mcpTestResult, setMcpTestResult] = useState<LocalRunnerMcpTestResult | null>(null);
  const [mcpTestRuns, setMcpTestRuns] = useState<LocalRunnerMcpTestRunSummary[]>([]);
  const [mcpTestRunsLoading, setMcpTestRunsLoading] = useState(false);

  const selectedBackend = backends.find((backend) => backend.key === selectedBackendKey) ?? null;
  const availableTemplates = templatesForProvider(selectedBackend?.providerType ?? "");
  const selectedTemplate =
    availableTemplates.find((template) => template.key === selectedTemplateKey) ?? null;
  const matchingProjectIntegrations = allIntegrations.filter(
    (integration) => integration.type === selectedBackend?.providerType,
  );
  const connectedIntegrations = matchingProjectIntegrations.filter(
    (integration) => integration.status === "connected",
  );
  const selectedIntegration =
    connectedIntegrations.find((integration) => integration.id === selectedIntegrationId) ?? null;
  const testConsoleDisabledReason = !runnerOnline
    ? `The local runner is unreachable at ${health.baseUrl}.`
    : !selectedBackend
      ? "Select an MCP backend to test."
      : selectedBackend.providerType !== "jira"
        ? "Only Jira-backed MCP tests are implemented in this MVP."
      : connectedIntegrations.length === 0
            ? `No connected ${selectedBackend.label} MCP instance is available to test.`
            : !selectedIntegration
              ? "Select a connected integration to run the MCP test."
              : selectedTemplate?.requiresWrite && !confirmWriteTest
                ? "Confirm the write test before creating Jira data."
              : !mcpTestPrompt.trim()
                ? "Prompt is required."
                : null;

  const saveIntegration = useMutation({
    mutationFn: async () => {
      if (!runnerOnline) {
        throw new Error(`The local runner is unreachable at ${health.baseUrl}.`);
      }
      if (!selectedProjectId) {
        throw new Error("Project is required.");
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
        const integrations = await gateways.integrationGateway.listAllIntegrations();
        const duplicateIntegration = findDuplicateJiraIntegration(
          integrations,
          draftConfigValues,
          editingIntegrationId ?? undefined,
        );
        if (duplicateIntegration) {
          throw new Error(
            `A Jira MCP for ${normalizeJiraSiteUrl(draftConfigValues.workspaceUrl)} already exists.`,
          );
        }
      }
      const parsedConfig = buildConfigFromDraft(draftType, draftConfigValues);
      const integration = editingIntegrationId
        ? await gateways.integrationGateway.updateIntegration(editingIntegrationId, {
            type: draftType,
            label,
            configEncrypted: parsedConfig,
            mcpTypeEnabled: true,
          })
        : await gateways.integrationGateway.createIntegration({
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

      await persistIntegrationConnectionResult(gateways, integration.id, result);
      if (result.requestStatus === "rejected" || result.integrationStatus === "failed") {
        throw new Error(result.message ?? "Unable to save MCP.");
      }
    },
    onMutate: () => {
      setEditorError(null);
    },
    onSuccess: async () => {
      resetEditor();
      await router.invalidate();
    },
    onError: (error) => {
      setEditorError(error instanceof Error ? error.message : "Unable to save MCP.");
    },
  });

  const runBackendAction = useMutation({
    mutationFn: async (backend: LocalRunnerMcpBackend) => {
      if (!runnerOnline) {
        throw new Error(`The local runner is unreachable at ${health.baseUrl}.`);
      }

      const gateways = await createGatewayBundle();
      if (backend.action === "install") {
        return gateways.localRunnerGateway.installMcpBackend(backend.key);
      }
      if (!selectedProjectId) {
        throw new Error("Select a project before verifying a backend.");
      }
      return gateways.localRunnerGateway.triggerMcpBackendAction(backend.key, {
        projectId: selectedProjectId,
        ...(backend.providerType === "jira" && selectedIntegrationId
          ? { integrationId: selectedIntegrationId }
          : {}),
        action: backend.action,
      });
    },
    onMutate: () => {
      setEditorError(null);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
    onError: (error) => {
      setEditorError(error instanceof Error ? error.message : "Unable to manage backend.");
    },
  });

  const toggleMcpType = useMutation({
    mutationFn: async ({
      enabled,
      providerType,
    }: {
      enabled: boolean;
      providerType: IntegrationType;
    }) => {
      const gateways = await createGatewayBundle();
      const providerIntegrations = allIntegrations.filter(
        (integration) => integration.type === providerType,
      );
      await Promise.all(
        providerIntegrations.map((integration) =>
          gateways.integrationGateway.updateIntegration(integration.id, {
            type: integration.type,
            label: integration.label,
            configEncrypted: integration.configEncrypted,
            status: integration.status,
            lastSyncedAt: integration.lastSyncedAt,
            lastError: integration.lastError,
            mcpTypeEnabled: enabled,
          }),
        ),
      );
    },
    onMutate: () => {
      setEditorError(null);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
    onError: (error) => {
      setEditorError(error instanceof Error ? error.message : "Unable to update MCP type.");
    },
  });

  const removeIntegration = useMutation({
    mutationFn: async (integration: Integration) => {
      const gateways = await createGatewayBundle();
      if (integration.type === "jira") {
        await gateways.localRunnerGateway.deleteIntegrationConnection(integration.id);
      }
      await gateways.integrationGateway.deleteIntegration(integration.id);
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const runMcpTest = useMutation({
    mutationFn: async () => {
      if (testConsoleDisabledReason) {
        throw new Error(testConsoleDisabledReason);
      }

      const gateways = await createGatewayBundle();
      return gateways.localRunnerGateway.runMcpTest({
        backendKey: selectedBackendKey,
        providerType: selectedBackend?.providerType ?? "",
        projectId: selectedProjectId,
        integrationId: selectedIntegrationId,
        templateKey: selectedTemplate?.key ?? "",
        allowWrite: selectedTemplate?.requiresWrite ?? false,
        prompt: mcpTestPrompt.trim(),
        timeoutMs: 60_000,
      });
    },
    onMutate: () => {
      setMcpTestError(null);
    },
    onSuccess: async (result) => {
      setMcpTestResult(result);
      if (!selectedBackendKey) {
        return;
      }
      setMcpTestRunsLoading(true);
      try {
        const gateways = await createGatewayBundle();
        const runs = await gateways.localRunnerGateway.listMcpTestRuns(
          selectedBackendKey,
          selectedProjectId,
          selectedIntegrationId || undefined,
          5,
        );
        setMcpTestRuns(Array.isArray(runs) ? runs : []);
      } catch {
        setMcpTestRuns([]);
      } finally {
        setMcpTestRunsLoading(false);
      }
    },
    onError: (error) => {
      setMcpTestError(error instanceof Error ? error.message : "Unable to run MCP test.");
    },
  });

  async function persistIntegrationConnectionResult(
    gateways: ReturnType<typeof createGatewayBundle>,
    integrationId: string,
    result: LocalRunnerIntegrationConnectionResult,
  ) {
    await gateways.integrationGateway.updateIntegration(integrationId, {
      status: result.integrationStatus,
      lastError: result.integrationStatus === "failed" ? result.message ?? null : null,
      lastSyncedAt: result.integrationStatus === "connected" ? new Date().toISOString() : null,
    });
  }

  function resetEditor() {
    setEditorError(null);
    setEditingIntegrationId(null);
    setDraftType("jira");
    setDraftLabel(defaultLabel("jira"));
    setDraftConfigValues(createEmptyDraftConfig("jira"));
    setIsEditorOpen(false);
  }

  function enableRemoteMcpType(backend: LocalRunnerMcpBackend) {
    const providerType = backend.providerType as IntegrationType;
    const providerIntegrations = allIntegrations.filter(
      (integration) => integration.type === providerType,
    );
    if (providerIntegrations.length === 0) {
      router.navigate({
        to: "/settings/mcp-servers/create",
        search: { provider: providerType },
      });
      return;
    }
    toggleMcpType.mutate({ enabled: true, providerType });
  }

  function disableRemoteMcpType(backend: LocalRunnerMcpBackend) {
    toggleMcpType.mutate({
      enabled: false,
      providerType: backend.providerType as IntegrationType,
    });
  }

  function openEditEditor(integration: Integration) {
    setEditorError(null);
    setEditingIntegrationId(integration.id);
    setDraftType(integration.type);
    setDraftLabel(integration.label);
    setDraftConfigValues(
      Object.fromEntries(
        Object.entries(integration.configEncrypted).map(([key, value]) => [key, String(value ?? "")]),
      ),
    );
    setSelectedProjectId(integration.projectId);
    setIsEditorOpen(true);
  }

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

  useEffect(() => {
    if (!selectedBackend) {
      setSelectedIntegrationId("");
      setSelectedTemplateKey("");
      setConfirmWriteTest(false);
      setMcpTestPrompt(defaultMcpTestPrompt("jira"));
      return;
    }

    const nextTemplates = templatesForProvider(selectedBackend.providerType);
    const nextTemplate = nextTemplates[0] ?? null;
    setSelectedTemplateKey(nextTemplate?.key ?? "");
    setConfirmWriteTest(false);
    setMcpTestPrompt(nextTemplate?.prompt ?? defaultMcpTestPrompt(selectedBackend.providerType));
  }, [selectedBackendKey, selectedBackend]);

  useEffect(() => {
    setMcpTestResult(null);
    setMcpTestError(null);
  }, [selectedProjectId, selectedBackendKey, selectedIntegrationId, selectedTemplateKey]);

  useEffect(() => {
    if (!selectedBackend) {
      setSelectedIntegrationId("");
      return;
    }

    const nextIntegrationId = connectedIntegrations[0]?.id ?? "";
    if (
      selectedIntegrationId &&
      connectedIntegrations.some((integration) => integration.id === selectedIntegrationId)
    ) {
      return;
    }
    setSelectedIntegrationId(nextIntegrationId);
  }, [connectedIntegrations, selectedBackend, selectedIntegrationId]);

  useEffect(() => {
    if (!runnerOnline || !selectedBackendKey) {
      setMcpTestRuns([]);
      return;
    }

    let cancelled = false;

    async function loadMcpTestRuns() {
      setMcpTestRunsLoading(true);
      try {
        const gateways = await createGatewayBundle();
        const runs = await gateways.localRunnerGateway.listMcpTestRuns(
          selectedBackendKey,
          selectedProjectId,
          selectedIntegrationId || undefined,
          5,
        );
        if (!cancelled) {
          setMcpTestRuns(Array.isArray(runs) ? runs : []);
        }
      } catch {
        if (!cancelled) {
          setMcpTestRuns([]);
        }
      } finally {
        if (!cancelled) {
          setMcpTestRunsLoading(false);
        }
      }
    }

    void loadMcpTestRuns();

    return () => {
      cancelled = true;
    };
  }, [runnerOnline, selectedBackendKey, selectedIntegrationId, selectedProjectId]);

  return (
    <PageFrame
      description="Runner-wide MCP type inventory plus reusable MCP instance management."
      title="MCP Servers"
    >
      <div className="space-y-6">
        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Runner Reachability
              </p>
              <h3 className="mt-3 text-2xl font-semibold tracking-tight">
                Local MCP control plane
              </h3>
            </div>
            <Badge tone={runnerOnline ? "success" : "danger"}>{health.status}</Badge>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <DetailRow label="Base URL" value={health.baseUrl} />
            <DetailRow label="Version" value={health.runnerVersion ?? "Unavailable"} />
            <DetailRow label="Workspace" value={health.cwd ?? "Unavailable"} />
            <DetailRow label="Platform" value={health.os ?? "Unavailable"} />
          </div>

          <p className="mt-4 text-sm text-muted-foreground">{detailText(health)}</p>

          {!runnerOnline ? (
            <div className="mt-4 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
              The runner is offline, so install, verify, and connect actions are blocked here.
              Start the local runner before creating a connected MCP.
            </div>
          ) : null}

          <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Available Backends
              </p>
              <p className="mt-2 text-sm text-muted-foreground">
                Allowlisted MCP inventory and machine-level enablement state.
              </p>
            </div>
            <Link
              className={secondaryLinkButtonClass}
              to="/settings/mcp-servers/mcp-connect-test"
            >
              Open Test Console
            </Link>
          </div>

          {backends.length === 0 ? (
            <div className="mt-4 rounded-[1.6rem] border border-dashed border-border bg-background/60 p-6 text-sm text-muted-foreground">
              No MCP backends were returned by the local runner. Start the runner or verify the
              MCP backend registry if this should already be configured.
            </div>
          ) : (
            <div className="mt-4 grid gap-4 xl:grid-cols-2">
              {backends.map((backend) => {
                const providerType = backend.providerType as IntegrationType;
                const providerIntegrations = allIntegrations.filter(
                  (integration) => integration.type === providerType,
                );
                const remoteTypeEnabled = providerIntegrations.some(
                  (integration) => integration.mcpTypeEnabled === true,
                );
                const typeEnabled =
                  backend.transport === "remote" ? remoteTypeEnabled : backend.state === "installed";

                return (
                  <article
                    key={backend.key}
                    className={`relative overflow-hidden rounded-[1.6rem] border bg-background/70 p-6 ${
                      backend.key === selectedBackendKey
                        ? "border-primary shadow-[0_0_0_1px_hsl(var(--primary))]"
                        : "border-border"
                    } ${!typeEnabled ? "bg-muted/20" : ""}`}
                  >
                    {!typeEnabled ? (
                      <div className="pointer-events-none absolute inset-0 z-10 rounded-[1.6rem] bg-white/20 backdrop-blur-[1px] dark:bg-black/20" />
                    ) : null}
                    <div className={`relative z-20 flex flex-wrap items-start justify-between gap-3 ${!typeEnabled ? "opacity-75" : ""}`}>
                      <div>
                        <p className="font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          {backend.providerType}
                        </p>
                        <h4 className="mt-2 text-xl font-semibold tracking-tight">
                          {backend.label}
                        </h4>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Badge tone={backendTone(backend.state)}>
                          {backendStateLabel(backend.state)}
                        </Badge>
                        <Badge tone={mcpTypeTone(typeEnabled)}>{mcpTypeLabel(typeEnabled)}</Badge>
                        <Badge tone="neutral">{backend.transport}</Badge>
                      </div>
                    </div>

                    <div className={`relative z-20 mt-6 grid gap-3 sm:grid-cols-2 ${!typeEnabled ? "opacity-75" : ""}`}>
                      <DetailRow label="Launcher" value={backend.launcher} />
                      <DetailRow label="Binary Path" value={backend.binaryPath ?? "Not found"} />
                      <DetailRow label="Command" value={backend.command} />
                      <DetailRow
                        label="Type State"
                        value={typeEnabled ? "Enabled for MCP instances" : "Disabled until enabled"}
                      />
                      <DetailRow label="Last Checked" value={formatTimestamp(backend.lastCheckedAt)} />
                      <DetailRow
                        label="Install Hint"
                        value={backend.installHint ?? "No extra hint provided"}
                      />
                    </div>

                    {backend.lastError ? (
                      <p className="relative z-20 mt-4 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                        Last error: {backend.lastError}
                      </p>
                    ) : (
                      <p className={`relative z-20 mt-4 text-sm text-muted-foreground ${!typeEnabled ? "opacity-75" : ""}`}>
                        {backend.transport === "remote"
                          ? "Enable the MCP type first, then create reusable MCP instances for one or more projects."
                          : "Local MCP types stay on the runner install flow. Install or verify the launcher from here before using it elsewhere."}
                      </p>
                    )}

                    <div className="relative z-20 mt-4 flex flex-wrap gap-2">
                      <Button
                        disabled={
                          !runnerOnline ||
                          runBackendAction.isPending ||
                          (backend.action === "verify" &&
                            backend.providerType === "jira" &&
                            !selectedIntegrationId)
                        }
                        onClick={() => runBackendAction.mutate(backend)}
                        type="button"
                        variant="secondary"
                      >
                        {runBackendAction.isPending ? "Working..." : backend.actionLabel}
                      </Button>
                      {backend.transport === "remote" ? (
                        typeEnabled ? (
                          <Button
                            className="bg-danger text-white hover:bg-danger/90"
                            disabled={!runnerOnline || toggleMcpType.isPending}
                            onClick={() => disableRemoteMcpType(backend)}
                            type="button"
                            variant="secondary"
                          >
                            {toggleMcpType.isPending ? "Working..." : "Disable"}
                          </Button>
                        ) : (
                          <Button
                            disabled={!runnerOnline || toggleMcpType.isPending || projects.length === 0}
                            onClick={() => enableRemoteMcpType(backend)}
                            type="button"
                            variant="secondary"
                          >
                            {toggleMcpType.isPending ? "Working..." : "Enable"}
                          </Button>
                        )
                      ) : null}
                      {backend.transport === "remote" && typeEnabled ? (
                        <Link
                          className={cn(
                            secondaryLinkButtonClass,
                            (!runnerOnline || projects.length === 0) &&
                              "pointer-events-none opacity-50",
                          )}
                          search={{ provider: providerType }}
                          to="/settings/mcp-servers/create"
                        >
                          Create MCP
                        </Link>
                      ) : null}
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </section>

        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                MCP Instances
              </p>
              <h3 className="mt-3 text-2xl font-semibold tracking-tight">
                Create and manage reusable MCP instances
              </h3>
              <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
                Pick an owner project, configure a provider instance, then let the runner verify
                and connect it here. Project settings only link instances from this list.
              </p>
            </div>
            <Link
              className={cn(
                secondaryLinkButtonClass,
                (!runnerOnline || projects.length === 0) && "pointer-events-none opacity-50",
              )}
              to="/settings/mcp-servers/create"
            >
              Create MCP
            </Link>
          </div>

          {projects.length === 0 ? (
            <div className="mt-4 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
              No projects are available yet. Create a project before linking an MCP from this page.
            </div>
          ) : null}

          {isEditorOpen ? (
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

              <label className="text-sm font-medium">
                Provider
                <select
                  className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                  onChange={(event) => updateDraftType(event.target.value as IntegrationType)}
                  value={draftType}
                >
                  {integrationTypes.map((type) => (
                    <option key={type} value={type}>
                      {toTitleCase(type)}
                    </option>
                  ))}
                </select>
              </label>
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
                    type={field.inputType ?? "text"}
                    onChange={(event) => updateDraftConfigValue(field.key, event.target.value)}
                    placeholder={field.placeholder}
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
                Supported providers will hand off authentication to the local runner. For Jira,
                the runner must be online so it can start the Atlassian OAuth flow.
              </p>

              {editorError ? <p className="text-sm text-danger">{editorError}</p> : null}

              <div className="flex flex-wrap gap-2">
                <Button
                  disabled={!runnerOnline || projects.length === 0 || saveIntegration.isPending}
                  onClick={() => saveIntegration.mutate()}
                  type="button"
                >
                  {saveIntegration.isPending
                    ? editingIntegrationId
                      ? "Saving..."
                      : "Creating..."
                    : editingIntegrationId
                      ? "Save MCP"
                      : "Create and Connect"}
                </Button>
                <Button onClick={resetEditor} type="button" variant="secondary">
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <div className="mt-4 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
              Create MCP now opens a dedicated setup route. Edit stays here for existing instances.
            </div>
          )}

          <div className="mt-6 space-y-3">
            {allIntegrations.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
                No MCP instances have been created yet.
              </div>
            ) : (
              allIntegrations.map((integration) => (
                <div
                  key={integration.id}
                  className="rounded-2xl border border-border bg-background/60 px-4 py-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="font-medium">{integration.label}</p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {toTitleCase(integration.type)} / owner project {integration.projectId}
                      </p>
                    </div>
                    <Badge tone={integrationTone(integration.status)}>
                      {toTitleCase(integration.status)}
                    </Badge>
                  </div>
                  <div className="mt-3 grid gap-2 text-sm text-muted-foreground">
                    <p>Last sync: {formatTimestamp(integration.lastSyncedAt)}</p>
                    <p>Last error: {integration.lastError?.trim() ? integration.lastError : "None"}</p>
                  </div>
                  <div className="mt-4 flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => openEditEditor(integration)}
                    >
                      Edit
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => {
                        setSelectedBackendKey(
                          backends.find((backend) => backend.providerType === integration.type)?.key ?? selectedBackendKey,
                        );
                        setSelectedIntegrationId(integration.id);
                      }}
                    >
                      Use in Test Console
                    </Button>
                    <Button
                      className="bg-danger text-white hover:bg-danger/90"
                      disabled={removeIntegration.isPending}
                      type="button"
                      variant="secondary"
                      onClick={() => removeIntegration.mutate(integration)}
                    >
                      {removeIntegration.isPending ? "Removing..." : "Remove"}
                    </Button>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>

        {false ? (
          <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                  MCP Test Console
                </p>
                <h3 className="mt-3 text-2xl font-semibold tracking-tight">
                  Run a runner-backed smoke test
                </h3>
                <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
                  Select a project, choose a connected integration, and run one of the supported
                  Jira-first MVP test templates against the selected MCP backend.
                </p>
              </div>
            {selectedBackend ? (
              <Badge tone="neutral">{selectedBackend.label}</Badge>
            ) : (
              <Badge tone="warning">No backend selected</Badge>
            )}
          </div>

          {backends.length === 0 ? (
            <div className="mt-4 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
              No MCP backends are available yet, so the test console cannot run.
            </div>
          ) : (
            <div className="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
              <div className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="text-sm font-medium">
                    Test Project
                    <select
                      className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                      disabled={runMcpTest.isPending}
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

                  <label className="text-sm font-medium">
                    Test Backend
                    <select
                      className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                      disabled={runMcpTest.isPending}
                      onChange={(event) => setSelectedBackendKey(event.target.value)}
                      value={selectedBackendKey}
                    >
                      {backends.map((backend) => (
                        <option key={backend.key} value={backend.key}>
                          {backend.label}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>

                <label className="text-sm font-medium">
                  Saved Integration
                  <select
                    className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                    disabled={connectedIntegrations.length === 0 || runMcpTest.isPending}
                    onChange={(event) => setSelectedIntegrationId(event.target.value)}
                    value={selectedIntegrationId}
                  >
                    {connectedIntegrations.length === 0 ? (
                      <option value="">No connected MCP instances available</option>
                    ) : null}
                    {connectedIntegrations.map((integration) => (
                      <option key={integration.id} value={integration.id}>
                        {integration.label}
                      </option>
                    ))}
                  </select>
                </label>

                {matchingProjectIntegrations.length > 0 && connectedIntegrations.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    This project has {matchingProjectIntegrations.length} saved{" "}
                    {selectedBackend?.providerType} integration
                    {matchingProjectIntegrations.length === 1 ? "" : "s"}, but none are currently
                    connected.
                  </p>
                ) : null}

                <label className="text-sm font-medium">
                  Test Template
                  <select
                    className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                    disabled={availableTemplates.length === 0 || runMcpTest.isPending}
                    onChange={(event) => {
                      const nextTemplate =
                        availableTemplates.find((template) => template.key === event.target.value) ?? null;
                      setSelectedTemplateKey(event.target.value);
                      setConfirmWriteTest(false);
                      setMcpTestPrompt(nextTemplate?.prompt ?? "");
                    }}
                    value={selectedTemplateKey}
                  >
                    {availableTemplates.length === 0 ? (
                      <option value="">No templates available</option>
                    ) : null}
                    {availableTemplates.map((template) => (
                      <option key={template.key} value={template.key}>
                        {template.label}
                        {template.requiresWrite ? " (Creates data)" : ""}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="text-sm font-medium">
                  Template Prompt
                  <textarea
                    className="mt-2 min-h-36 w-full rounded-2xl border border-border bg-card px-4 py-3"
                    disabled={runMcpTest.isPending}
                    onChange={(event) => setMcpTestPrompt(event.target.value)}
                    placeholder="The selected template controls the runner behavior for this MVP."
                    value={mcpTestPrompt}
                  />
                </label>

                {selectedTemplate?.requiresWrite ? (
                  <label className="flex items-start gap-3 rounded-2xl border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-foreground">
                    <input
                      checked={confirmWriteTest}
                      className="mt-1"
                      disabled={runMcpTest.isPending}
                      onChange={(event) => setConfirmWriteTest(event.target.checked)}
                      type="checkbox"
                    />
                    <span>
                      I understand this test will create Jira data in the selected project.
                    </span>
                  </label>
                ) : null}

                {testConsoleDisabledReason ? (
                  <div className="rounded-2xl border border-dashed border-border bg-background/60 px-4 py-3 text-sm text-muted-foreground">
                    {testConsoleDisabledReason}
                  </div>
                ) : (
                  <div className="rounded-2xl border border-border bg-card/60 px-4 py-3 text-sm text-muted-foreground">
                    The runner will execute the selected template through the saved MCP connection
                    and save artifacts for the run under `.flowpilot/mcp-tests`.
                  </div>
                )}

                {mcpTestError ? <p className="text-sm text-danger">{mcpTestError}</p> : null}

                <div className="flex flex-wrap gap-2">
                  <Button
                    disabled={Boolean(testConsoleDisabledReason) || runMcpTest.isPending}
                    onClick={() => runMcpTest.mutate()}
                    type="button"
                  >
                    {runMcpTest.isPending ? "Running..." : "Run MCP Test"}
                  </Button>
                  {mcpTestResult ? (
                    <Button
                      onClick={() => {
                        setMcpTestResult(null);
                        setMcpTestError(null);
                      }}
                      type="button"
                      variant="secondary"
                    >
                      Clear Result
                    </Button>
                  ) : null}
                </div>
              </div>

              <div className="space-y-4">
                <div className="rounded-2xl border border-border bg-card/60 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-medium">Recent runs</p>
                    <Badge tone="neutral">{selectedBackend?.providerType ?? "none"}</Badge>
                  </div>
                  {mcpTestRunsLoading ? (
                    <p className="mt-3 text-sm text-muted-foreground">Loading recent test runs...</p>
                  ) : mcpTestRuns.length === 0 ? (
                    <p className="mt-3 text-sm text-muted-foreground">
                      No test runs recorded yet for the selected backend.
                    </p>
                  ) : (
                    <div className="mt-3 space-y-3">
                      {mcpTestRuns.map((run) => (
                        <div
                          className="rounded-2xl border border-border bg-background/70 p-3"
                          key={run.runId}
                        >
                          <div className="flex items-center justify-between gap-3">
                            <p className="text-sm font-medium">{run.runId}</p>
                            <Badge tone={resultTone(run.status)}>{run.status}</Badge>
                          </div>
                          <p className="mt-2 text-sm text-muted-foreground">
                            Started {formatTimestamp(run.startedAt)}
                          </p>
                          <p className="mt-1 break-words text-xs text-muted-foreground">
                            {run.artifactDir}
                          </p>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="rounded-2xl border border-border bg-card/60 p-4">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-medium">Latest result</p>
                    {mcpTestResult ? (
                      <Badge tone={resultTone(mcpTestResult.status)}>{mcpTestResult.status}</Badge>
                    ) : (
                      <Badge tone="neutral">Idle</Badge>
                    )}
                  </div>

                  {!mcpTestResult ? (
                    <p className="mt-3 text-sm text-muted-foreground">
                      Run a test to inspect the runner command, summaries, rendered markdown output,
                      and saved artifact paths.
                    </p>
                  ) : (
                    <div className="mt-3 space-y-3 text-sm">
                      <DetailRow label="Run ID" value={mcpTestResult.runId} />
                      <DetailRow label="Command" value={mcpTestResult.command} />
                      <DetailRow
                        label="Started"
                        value={formatTimestamp(mcpTestResult.startedAt)}
                      />
                      <DetailRow
                        label="Completed"
                        value={formatTimestamp(mcpTestResult.completedAt)}
                      />

                      {mcpTestResult.errorMessage ? (
                        <p className="rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                          {mcpTestResult.errorMessage}
                        </p>
                      ) : null}

                      <div className="rounded-2xl border border-border bg-background/70 p-4">
                        <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          Stdout Summary
                        </p>
                        <pre className="mt-2 whitespace-pre-wrap text-sm">
                          {mcpTestResult.stdoutSummary || "No stdout summary returned."}
                        </pre>
                      </div>

                      <div className="rounded-2xl border border-border bg-background/70 p-4">
                        <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          Stderr Summary
                        </p>
                        <pre className="mt-2 whitespace-pre-wrap text-sm">
                          {mcpTestResult.stderrSummary || "No stderr summary returned."}
                        </pre>
                      </div>

                      <div className="rounded-2xl border border-border bg-background/70 p-4">
                        <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          Output
                        </p>
                        <pre className="mt-2 whitespace-pre-wrap text-sm">
                          {mcpTestResult.outputMarkdown || "No markdown output returned."}
                        </pre>
                      </div>

                      <div className="rounded-2xl border border-border bg-background/70 p-4">
                        <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          Artifacts
                        </p>
                        {mcpTestResult.artifactPaths.length === 0 ? (
                          <p className="mt-2 text-sm text-muted-foreground">
                            No artifact paths were returned for this run.
                          </p>
                        ) : (
                          <ul className="mt-2 space-y-2">
                            {mcpTestResult.artifactPaths.map((artifactPath) => (
                              <li className="break-words font-mono text-xs" key={artifactPath}>
                                {artifactPath}
                              </li>
                            ))}
                          </ul>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
          </section>
        ) : null}

      </div>
    </PageFrame>
  );
}
