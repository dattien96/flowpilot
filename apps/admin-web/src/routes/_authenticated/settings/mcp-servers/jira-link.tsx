import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { useEffect, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { IntegrationType } from "@/domain/model/entity/integration";
import type { LocalRunnerMcpBackend } from "@/domain/model/entity/local-runner";
import { loadMcpSettingsData } from "@/features/mcp/mcp-settings-loader";
import { formatTimestamp } from "@/features/mcp/integration-config";
import { Badge } from "@/presentation/components/ui/badge";

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

function detailText(baseUrl: string, runnerOnline: boolean) {
  if (!runnerOnline) {
    return `The local runner is unreachable at ${baseUrl}. Start the runner before verifying or enabling the Jira MCP link.`;
  }
  return "Use this page to enable the Atlassian MCP type and verify the runner-side Jira link before creating or testing instances.";
}

export const Route = createFileRoute("/_authenticated/settings/mcp-servers/jira-link")({
  loader: loadMcpSettingsData,
  component: JiraMcpLinkPage,
});

export function JiraMcpLinkPage() {
  const { allIntegrations, backends, health, projects } = Route.useLoaderData();
  const router = useRouter();
  const runnerOnline = health.status === "online";
  const jiraBackend = backends.find((backend) => backend.providerType === "jira") ?? null;
  const jiraIntegrations = allIntegrations.filter((integration) => integration.type === "jira");
  const connectedJiraIntegrations = jiraIntegrations.filter(
    (integration) => integration.status === "connected",
  );
  const [selectedIntegrationId, setSelectedIntegrationId] = useState(
    connectedJiraIntegrations[0]?.id ?? "",
  );

  useEffect(() => {
    if (
      selectedIntegrationId &&
      connectedJiraIntegrations.some((integration) => integration.id === selectedIntegrationId)
    ) {
      return;
    }
    setSelectedIntegrationId(connectedJiraIntegrations[0]?.id ?? "");
  }, [connectedJiraIntegrations, selectedIntegrationId]);

  const runBackendAction = useMutation({
    mutationFn: async (backend: LocalRunnerMcpBackend) => {
      if (!runnerOnline) {
        throw new Error(`The local runner is unreachable at ${health.baseUrl}.`);
      }

      const gateways = await createGatewayBundle();
      if (backend.action === "install") {
        return gateways.localRunnerGateway.installMcpBackend(backend.key);
      }

      return gateways.localRunnerGateway.triggerMcpBackendAction(backend.key, {
        projectId: jiraIntegrations[0]?.projectId ?? projects[0]?.id ?? "",
        ...(selectedIntegrationId ? { integrationId: selectedIntegrationId } : {}),
        action: backend.action,
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const toggleMcpType = useMutation({
    mutationFn: async ({ enabled, providerType }: { enabled: boolean; providerType: IntegrationType }) => {
      if (enabled && jiraIntegrations.length === 0) {
        await router.navigate({
          to: "/settings/mcp-servers/create",
          search: { provider: providerType },
        });
        return;
      }

      const gateways = await createGatewayBundle();
      await Promise.all(
        jiraIntegrations.map((integration) =>
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

      if (!enabled || jiraIntegrations.length > 0) {
        return;
      }

      await router.navigate({
        to: "/settings/mcp-servers/create",
        search: { provider: providerType },
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
  });

  const typeEnabled = jiraIntegrations.some((integration) => integration.mcpTypeEnabled === true);

  return (
    <PageFrame
      description="Runner-side Atlassian MCP status and enablement for Jira-backed MCP links."
      title="Jira MCP Link"
    >
      <div className="space-y-6">
        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Atlassian MCP
              </p>
              <h3 className="mt-3 text-2xl font-semibold tracking-tight">
                Runner-side Jira MCP link
              </h3>
              <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
                Connect with email + API token (legacy form). FlowPilot writes Atlassian{" "}
                <strong>Rovo remote MCP</strong> into AI provider configs (
                <code className="text-xs">Basic</code> →{" "}
                <code className="text-xs">mcp.atlassian.com/v1/mcp</code>). Org admins must allow API token
                authentication for Rovo MCP. If Jira returns <code className="text-xs">403 Forbidden</code> from
                Teamwork Graph, reconnect with a modern scoped token; legacy tokens may be rejected. The old MCP
                Test Console UI was removed; REST helpers remain in the runner for internal use only.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Link
                className={secondaryLinkButtonClass}
                search={{ provider: "jira" }}
                to="/settings/mcp-servers/create"
              >
                Create MCP
              </Link>
              <a
                className={secondaryLinkButtonClass}
                href="https://support.atlassian.com/security-and-access-policies/docs/control-atlassian-rovo-mcp-server-settings/#Configure-authentication"
                rel="noreferrer"
                target="_blank"
              >
                Enable Rovo MCP API token (admin)
              </a>
              <a
                className={secondaryLinkButtonClass}
                href="https://id.atlassian.com/manage-profile/security/api-tokens?autofillToken&expiryDays=max&appId=mcp&selectedScopes=all"
                rel="noreferrer"
                target="_blank"
              >
                Create MCP-scoped API token
              </a>
            </div>
          </div>

          <p className="mt-4 text-sm text-muted-foreground">
            {detailText(health.baseUrl, runnerOnline)}
          </p>

          {jiraBackend ? (
            <article className="mt-6 rounded-[1.6rem] border border-border bg-background/70 p-6">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">
                    {jiraBackend.providerType}
                  </p>
                  <h4 className="mt-2 text-xl font-semibold tracking-tight">{jiraBackend.label}</h4>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Badge tone={backendTone(jiraBackend.state)}>{backendStateLabel(jiraBackend.state)}</Badge>
                  <Badge tone={typeEnabled ? "success" : "neutral"}>
                    {typeEnabled ? "enabled" : "disabled"}
                  </Badge>
                  <Badge tone="neutral">{jiraBackend.transport}</Badge>
                </div>
              </div>

              <div className="mt-6 grid gap-3 sm:grid-cols-2">
                <DetailCard label="Launcher" value={jiraBackend.launcher} />
                <DetailCard label="Command" value={jiraBackend.command} />
                <DetailCard
                  label="Type State"
                  value={typeEnabled ? "Enabled for Jira MCP instances" : "Disabled until enabled"}
                />
                <DetailCard label="Last Checked" value={formatTimestamp(jiraBackend.lastCheckedAt)} />
              </div>

              {jiraBackend.lastError ? (
                <p className="mt-4 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                  Last error: {jiraBackend.lastError}
                </p>
              ) : null}

              <div className="mt-4 flex flex-wrap gap-2">
                <Button
                  disabled={
                    !runnerOnline ||
                    runBackendAction.isPending ||
                    (jiraBackend.action === "verify" && !selectedIntegrationId)
                  }
                  onClick={() => runBackendAction.mutate(jiraBackend)}
                  type="button"
                  variant="secondary"
                >
                  {runBackendAction.isPending ? "Working..." : jiraBackend.actionLabel}
                </Button>
                {typeEnabled ? (
                  <Button
                    className="bg-danger text-white hover:bg-danger/90"
                    disabled={!runnerOnline || toggleMcpType.isPending}
                    onClick={() => toggleMcpType.mutate({ enabled: false, providerType: "jira" })}
                    type="button"
                    variant="secondary"
                  >
                    {toggleMcpType.isPending ? "Working..." : "Disable"}
                  </Button>
                ) : (
                  <Button
                    disabled={!runnerOnline || toggleMcpType.isPending}
                    onClick={() => toggleMcpType.mutate({ enabled: true, providerType: "jira" })}
                    type="button"
                    variant="secondary"
                  >
                    {toggleMcpType.isPending ? "Working..." : "Enable"}
                  </Button>
                )}
                {typeEnabled ? (
                  <Link
                    className={`${secondaryLinkButtonClass} ${!runnerOnline || projects.length === 0 ? "pointer-events-none opacity-50" : ""}`}
                    search={{ provider: "jira" }}
                    to="/settings/mcp-servers/create"
                  >
                    Create MCP
                  </Link>
                ) : null}
              </div>
            </article>
          ) : (
            <div className="mt-6 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
              No Atlassian MCP backend was returned by the local runner.
            </div>
          )}
        </section>

        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Jira Instances
              </p>
              <h3 className="mt-3 text-2xl font-semibold tracking-tight">Connected Jira MCP instances</h3>
            </div>
            <Badge tone={connectedJiraIntegrations.length > 0 ? "success" : "neutral"}>
              {connectedJiraIntegrations.length} connected
            </Badge>
          </div>

          {connectedJiraIntegrations.length > 0 ? (
            <label className="mt-4 block text-sm font-medium">
              Active Jira instance for verification
              <select
                className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) => setSelectedIntegrationId(event.target.value)}
                value={selectedIntegrationId}
              >
                {connectedJiraIntegrations.map((integration) => (
                  <option key={integration.id} value={integration.id}>
                    {integration.label}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <div className="mt-4 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
              No connected Jira MCP instances are available yet. Create one from Create MCP after
              enabling the Jira MCP type.
            </div>
          )}

          <div className="mt-6 grid gap-3">
            {jiraIntegrations.map((integration) => (
              <div key={integration.id} className="rounded-2xl border border-border bg-background/60 px-4 py-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="font-medium">{integration.label}</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      owner project {integration.projectId}
                    </p>
                  </div>
                  <Badge tone={integration.status === "connected" ? "success" : "neutral"}>
                    {integration.status}
                  </Badge>
                </div>
                <p className="mt-3 text-sm text-muted-foreground">
                  Last sync: {formatTimestamp(integration.lastSyncedAt)}
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Link
                    className={secondaryLinkButtonClass}
                    search={{ integrationId: integration.id, provider: "jira" }}
                    to="/settings/mcp-servers/create"
                  >
                    Edit / Rotate token
                  </Link>
                </div>
              </div>
            ))}
          </div>
        </section>
      </div>
    </PageFrame>
  );
}

function DetailCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}
