'use client';

import { useMutation } from '@tanstack/react-query';
import { AlertCircle, CheckCircle, Clock, Loader2, TriangleAlert } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { createGatewayBundle } from '@/data/repository/browser-factory';
import type {
  GoogleDriveWorkspaceConfigResponse,
  GoogleDriveMcpProviderConfigStatus,
} from '@/domain/model/entity/local-runner';
import { Badge } from '@/presentation/components/ui/badge';

interface GoogleDriveProviderConfigCardProps {
  googleDriveStatus: GoogleDriveWorkspaceConfigResponse | null;
  onStatusRefresh?: () => Promise<unknown>;
  embedded?: boolean;
}

const PROVIDER_LABELS: Record<string, string> = {
  codex: 'Codex',
  gemini: 'Gemini',
  claude: 'Claude',
};

function getStatusBadgeTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' {
  switch (status) {
    case 'configured':
      return 'success';
    case 'config_stale':
      return 'warning';
    case 'failed':
      return 'danger';
    case 'not_started':
    default:
      return 'neutral';
  }
}

function getStatusIcon(status: string) {
  switch (status) {
    case 'configured':
      return <CheckCircle className="h-4 w-4" />;
    case 'config_stale':
      return <TriangleAlert className="h-4 w-4" />;
    case 'failed':
      return <AlertCircle className="h-4 w-4" />;
    case 'not_started':
    default:
      return <Clock className="h-4 w-4" />;
  }
}

function getStatusText(status: string): string {
  switch (status) {
    case 'configured':
      return 'Configured';
    case 'config_stale':
      return 'Stale Config';
    case 'failed':
      return 'Failed';
    case 'not_started':
    default:
      return 'Not Started';
  }
}

function getStatusTooltip(status: string): string {
  switch (status) {
    case 'configured':
      return 'Provider is configured and ready to use the FlowPilot proxy Google Drive MCP';
    case 'config_stale':
      return 'Configuration is outdated. Click Configure to refresh the proxy MCP provider config.';
    case 'failed':
      return 'Provider configuration failed. Check error message and reconfigure.';
    case 'not_started':
    default:
      return 'Provider has not been configured yet';
  }
}

function getConfigKindLabel(configKind?: string) {
  switch (configKind) {
    case 'proxy':
      return 'Proxy MCP';
    case 'legacy_raw':
      return 'Legacy Raw MCP';
    default:
      return 'Unknown MCP Shape';
  }
}

function getModeLabel(mode?: string) {
  switch (mode) {
    case 'read_write':
      return 'Read + write';
    case 'read_only':
      return 'Read only';
    default:
      return 'Mode unknown';
  }
}

export function GoogleDriveProviderConfigCard({
  googleDriveStatus,
  onStatusRefresh,
  embedded = false,
}: GoogleDriveProviderConfigCardProps) {
  const [configMessage, setConfigMessage] = useState<string | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);

  const providerConfigs = googleDriveStatus?.providerConfigs ?? [];
  const proxyMcpEnabled = Boolean(googleDriveStatus?.mcp.proxyMcpEnabled);
  const proxyMcpConfigured = Boolean(googleDriveStatus?.mcp.configured);
  const discoveredProviderConfigs = providerConfigs.filter(
    (config) => config.accountHomePath.trim().length > 0
  );
  const pendingProviderConfigs = providerConfigs.filter(
    (config) =>
      config.status === 'not_started' || config.status === 'config_stale' || config.status === 'failed'
  );
  const actionableProviderConfigs = discoveredProviderConfigs;
  const unresolvedProviderCount = providerConfigs.length - discoveredProviderConfigs.length;
  const proxyMissingScopes = googleDriveStatus?.mcp.missingScopes ?? [];
  const proxySelectionRequired = Boolean(googleDriveStatus?.mcp.accountSelectionRequired);
  const proxyReconnectRequired = Boolean(googleDriveStatus?.mcp.reconnectRequired);
  const proxyAccountLabel = googleDriveStatus?.mcp.accountEmail || googleDriveStatus?.mcp.accountId || 'the selected account';

  const canConfigureProviders =
    proxyMcpEnabled && proxyMcpConfigured && actionableProviderConfigs.length > 0;

  const configureProviders = useMutation({
    mutationFn: async () => {
      setConfigMessage(null);
      setConfigError(null);

      const gateways = await createGatewayBundle();
      const errors: string[] = [];

      // Configure each provider
      for (const config of actionableProviderConfigs) {
        try {
          // Call the ensure endpoint to configure the provider
          await gateways.localRunnerGateway.ensureGoogleDriveMcpProviderConfig({
            providerKey: config.providerKey,
            accountHomePath: config.accountHomePath,
            scope: 'account',
            mode: 'read_only',
          });
        } catch (error) {
          errors.push(
            `${PROVIDER_LABELS[config.providerKey] || config.providerKey}: ${error instanceof Error ? error.message : 'Unknown error'}`
          );
        }
      }

      if (errors.length > 0) {
        throw new Error(errors.join('\n'));
      }

      // Refresh status after configuration
      if (onStatusRefresh) {
        await onStatusRefresh();
      }
    },
    onSuccess: () => {
      if (unresolvedProviderCount > 0) {
        setConfigMessage(
          'Configured discovered provider accounts. Some providers still need a discovered account home before they can be configured.'
        );
        return;
      }

      const providerCount = actionableProviderConfigs.length;
      setConfigMessage(
        pendingProviderConfigs.length === 0
          ? providerCount === 1
            ? 'Refreshed 1 provider account successfully.'
            : `Refreshed ${providerCount} provider accounts successfully.`
          : providerCount === 1
            ? 'Applied configuration to 1 provider account successfully.'
            : `Applied configuration to ${providerCount} provider accounts successfully.`
      );
    },
    onError: (error) => {
      setConfigError(error instanceof Error ? error.message : 'Configuration failed');
    },
  });

  const isConfiguring = configureProviders.isPending;

  return (
    <section className={embedded ? "" : "rounded-[1.6rem] border border-border bg-background/80 p-6"}>
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          {embedded ? null : (
            <>
              <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Provider setup</p>
              <h2 className="mt-2 text-2xl font-semibold">Proxy MCP provider setup</h2>
            </>
          )}
          <p className={`${embedded ? "" : "mt-3 "}max-w-2xl text-sm text-muted-foreground`}>
            Configure Codex, Gemini, and Claude to use the FlowPilot proxy Google Drive MCP during workflow execution.
          </p>
          {proxyMcpEnabled && !proxyMcpConfigured ? (
            <p className="mt-3 rounded-2xl border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-warning">
              {proxySelectionRequired
                ? 'Select an active Google account in Google Drive setup before configuring provider MCPs.'
                : proxyReconnectRequired
                  ? `Reconnect ${proxyAccountLabel} in Google Drive setup before configuring provider MCPs.`
                  : proxyMissingScopes.length > 0
                    ? `Reconnect ${proxyAccountLabel} and grant these scopes before configuring provider MCPs: ${proxyMissingScopes.join(', ')}.`
                    : `Finish proxy MCP Google Drive auth for ${proxyAccountLabel} before configuring provider MCPs.`}
            </p>
          ) : null}
        </div>
        <Button
          disabled={!canConfigureProviders || isConfiguring}
          onClick={() => configureProviders.mutate()}
          variant="secondary"
        >
          {isConfiguring ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Configuring...
            </>
          ) : (
            'Configure AI Providers'
          )}
        </Button>
      </div>

      {!proxyMcpConfigured ? (
        <div className="mt-4 rounded-2xl border border-border/70 bg-muted/40 p-4 text-sm text-muted-foreground">
          <p className="font-medium text-foreground">
            {proxySelectionRequired
              ? 'Proxy MCP needs an active Google account selection.'
              : proxyReconnectRequired
                ? 'Proxy MCP needs the selected Google account to be reconnected.'
                : proxyMissingScopes.length > 0
                  ? 'Proxy MCP is missing required Google Drive scopes.'
                  : 'Proxy MCP is not ready yet.'}
          </p>
          <p className="mt-2">
            {proxySelectionRequired
              ? 'The proxy MCP uses the selected Google account token. Project artifact sync keeps a separate per-project Google account and folder binding.'
              : proxyReconnectRequired
                ? 'Reconnect the selected Google account in Google Drive setup so the proxy MCP can refresh its token again.'
                : proxyMissingScopes.length > 0
                  ? `Reconnect the selected Google account and grant the missing scopes: ${proxyMissingScopes.join(', ')}.`
                  : 'Complete Google Drive setup so the proxy MCP can supply the provider-side MCP configs.'}
          </p>
        </div>
      ) : (
        <div className="mt-4 rounded-2xl border border-success/20 bg-success/5 p-4 text-sm text-muted-foreground">
          <p className="font-medium text-foreground">FlowPilot proxy MCP is active in this runner</p>
          <p className="mt-2">
            The rows below now show the detected MCP type and command from each provider config file, so you can verify that the active config is using the FlowPilot proxy instead of the legacy third-party package.
          </p>
        </div>
      )}

      {configMessage && (
        <div className="mt-4 rounded-2xl border border-success/30 bg-success/10 px-4 py-3 text-sm text-success">
          {configMessage}
        </div>
      )}

      {configError && (
        <div className="mt-4 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger whitespace-pre-wrap">
          {configError}
        </div>
      )}

      <div className="mt-6 grid gap-4">
        {providerConfigs.map((config) => (
          <ProviderConfigRow
            key={`${config.providerKey}:${config.accountHomePath || config.configPath || 'unresolved'}`}
            config={config}
          />
        ))}

        {providerConfigs.length === 0 && (
          <div className="rounded-2xl border border-border/50 bg-background/50 p-4 text-center text-sm text-muted-foreground">
            No provider configurations available. Check if accounts are discovered.
          </div>
        )}
      </div>
    </section>
  );
}

function ProviderConfigRow({ config }: { config: GoogleDriveMcpProviderConfigStatus }) {
  const providerLabel = PROVIDER_LABELS[config.providerKey] || config.providerKey;
  const tone = getStatusBadgeTone(config.status);
  const statusText = getStatusText(config.status);
  const statusTooltip = getStatusTooltip(config.status);

  return (
    <div className="rounded-xl border border-border/50 bg-background/60 p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <p className="font-medium text-foreground">{providerLabel}</p>
            <div title={statusTooltip}>
              <Badge tone={tone}>
                <span className="inline-flex items-center gap-1.5">
                  {getStatusIcon(config.status)}
                  {statusText}
                </span>
              </Badge>
            </div>
          </div>
          {config.accountHomePath && (
            <p className="mt-2 text-xs text-muted-foreground break-all">
              Home: {config.accountHomePath}
            </p>
          )}
          {config.configPath && (
            <p className="mt-1 text-xs text-muted-foreground break-all">
              Config: {config.configPath}
            </p>
          )}
          {config.configKind ? (
            <p className="mt-1 text-xs text-muted-foreground">
              Type: {getConfigKindLabel(config.configKind)}
            </p>
          ) : null}
          {config.mode ? (
            <p className="mt-1 text-xs text-muted-foreground">
              Access: {getModeLabel(config.mode)}
              {config.approvalMode ? ` | Approval: ${config.approvalMode}` : ""}
            </p>
          ) : null}
          {config.command ? (
            <div className="mt-2 rounded-lg border border-border/50 bg-muted/30 px-3 py-2">
              <p className="text-[11px] uppercase tracking-[0.22em] text-muted-foreground">Detected command</p>
              <p className="mt-1 break-all font-mono text-xs text-foreground">
                {[config.command, ...(config.args ?? [])].join(' ')}
              </p>
            </div>
          ) : null}
          {config.lastError && (
            <p className="mt-2 text-xs text-danger break-all">
              Error: {config.lastError}
            </p>
          )}
          {config.lastCheckedAt && (
            <p className="mt-2 text-xs text-muted-foreground">
              Checked: {new Date(config.lastCheckedAt).toLocaleString()}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
