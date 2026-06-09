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
      return 'Provider is configured and ready to use Google Drive MCP';
    case 'config_stale':
      return 'Configuration is outdated. Click Configure to update.';
    case 'failed':
      return 'Provider configuration failed. Check error message and reconfigure.';
    case 'not_started':
    default:
      return 'Provider has not been configured yet';
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
  const mcpConfigured = googleDriveStatus?.mcp.status === 'configured';
  const pendingProviderConfigs = providerConfigs.filter(
    (config) =>
      config.status === 'not_started' || config.status === 'config_stale' || config.status === 'failed'
  );
  const actionableProviderConfigs = pendingProviderConfigs.filter(
    (config) => config.accountHomePath.trim().length > 0
  );
  const unresolvedProviderCount = pendingProviderConfigs.length - actionableProviderConfigs.length;

  // Determine if any provider needs configuration
  const needsConfiguration = actionableProviderConfigs.length > 0;

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
        providerCount === 1
          ? 'Configured 1 provider account successfully.'
          : `Configured ${providerCount} provider accounts successfully.`
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
              <h2 className="mt-2 text-2xl font-semibold">MCP Provider setup</h2>
            </>
          )}
          <p className={`${embedded ? "" : "mt-3 "}max-w-2xl text-sm text-muted-foreground`}>
            Configure Codex, Gemini, and Claude to use Google Drive MCP tools during workflow execution.
          </p>
        </div>
        <Button
          disabled={!mcpConfigured || isConfiguring || !needsConfiguration}
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

      {!mcpConfigured && (
        <div className="mt-4 rounded-2xl border border-border/70 bg-muted/40 p-4 text-sm text-muted-foreground">
          <p className="font-medium text-foreground">Google Drive MCP must be configured first</p>
          <p className="mt-2">Complete Step 6 (Upload Desktop OAuth JSON for MCP) to enable provider configuration.</p>
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
