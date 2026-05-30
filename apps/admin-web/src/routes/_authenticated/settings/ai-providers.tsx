import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { RefreshCw, Trash2, Plus } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type {
  LocalRunnerHealth,
  LocalRunnerProvider,
  LocalRunnerProviderModel,
} from "@/domain/model/entity/local-runner";
import type { SupportedModel } from "@/domain/model/entity/workflow-engine";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalProvidersUseCase } from "@/domain/usecase/local-runner/list-local-providers-usecase";
import { Badge } from "@/presentation/components/ui/badge";

type ProviderInstallResponse = {
  providers: LocalRunnerProvider[];
};

export const Route = createFileRoute("/_authenticated/settings/ai-providers")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [health, providers, supportedModels] = await Promise.all([
      new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
      new ListLocalProvidersUseCase(gateways.localRunnerGateway).execute(),
      gateways.workflowEngineGateway.listSupportedModels(),
    ]);
    return { health, providers, supportedModels };
  },
  component: AiProvidersPage,
});

function AiProvidersPage() {
  const { health, providers, supportedModels } = Route.useLoaderData();
  return (
    <AiProvidersContent
      health={health}
      providers={providers}
      supportedModels={supportedModels}
    />
  );
}

export function AiProvidersContent({
  health,
  providers: initialProviders,
  supportedModels: initialSupportedModels = [],
}: {
  health: LocalRunnerHealth;
  providers: LocalRunnerProvider[];
  supportedModels?: SupportedModel[];
}) {
  const router = useRouter();
  const isRouterPending = useRouterState({ select: (state) => state.status === "pending" });

  // Local state is the source of truth for the UI.
  // It is initialised from the route loader and updated by direct gateway fetches,
  // so we never depend on router.invalidate() re-running the loader to update the view.
  const [providers, setProviders] = useState(initialProviders);
  const [supportedModels, setSupportedModels] = useState(initialSupportedModels);

  // Keep in sync when the router loader re-runs (e.g. navigating away and back).
  useEffect(() => {
    setProviders(initialProviders);
  }, [initialProviders]);

  useEffect(() => {
    setSupportedModels(initialSupportedModels);
  }, [initialSupportedModels]);

  const providerCount = providers.length;
  const installedCount = providers.filter((provider) => provider.installed).length;

  // ── Refresh inventory (detect-only, no install) ──────────────────────────
  // Fetches the current provider list directly from the local runner via the
  // browser-side gateway, bypassing the Vite API proxy and router.invalidate().
  const refreshInventory = useMutation({
    mutationFn: async () => {
      const gateways = await createGatewayBundle();
      return new ListLocalProvidersUseCase(gateways.localRunnerGateway).execute();
    },
    onSuccess: (freshProviders) => {
      setProviders(freshProviders);
    },
  });

  // ── Install provider (triggers actual install via cobra CLI) ──────────────
  // Only called when the provider status is NOT_INSTALLED.
  const installProvider = useMutation({
    mutationFn: async (providerName: string) => {
      const response = await fetch("/api/local-runner/providers/install", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({ providerName }),
      });

      const payload = (await response.json()) as ProviderInstallResponse & { error?: string };
      if (!response.ok) {
        throw new Error(payload.error ?? "Provider install failed.");
      }

      return payload;
    },
    onSuccess: async (payload) => {
      // Update local state immediately from the response payload so the UI
      // reflects the new status without waiting for a router reload.
      setProviders(payload.providers);
      // Also invalidate router so that navigating away/back shows fresh data.
      void router.invalidate();
    },
  });

  const authenticateProvider = useMutation({
    mutationFn: async (providerName: string) => {
      const response = await fetch("/api/local-runner/providers/auth", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({ providerName }),
      });

      if (!response.ok) {
        const payload = (await response.json()) as { error?: string };
        throw new Error(payload.error ?? "Failed to trigger authentication.");
      }
    },
  });

  const queryClient = useQueryClient();

  const addSupportedModel = useMutation({
    mutationFn: async (input: { providerKey: "codex" | "claude" | "gemini"; modelId: string; displayName: string }) => {
      const gateways = await createGatewayBundle();
      const maxSortOrder = supportedModels.reduce((max, m) => Math.max(max, m.sortOrder), 0);
      return gateways.workflowEngineGateway.createSupportedModel({
        providerKey: input.providerKey,
        modelId: input.modelId,
        displayName: input.displayName,
        isEnabled: true,
        sortOrder: maxSortOrder + 1,
        source: "manual",
      });
    },
    onSuccess: (newModel) => {
      setSupportedModels((current) => [...current, newModel]);
      void queryClient.invalidateQueries({ queryKey: ["supportedModels"] });
      void router.invalidate();
    },
  });

  const deleteSupportedModel = useMutation({
    mutationFn: async (id: string) => {
      const gateways = await createGatewayBundle();
      await gateways.workflowEngineGateway.deleteSupportedModel(id);
      return id;
    },
    onSuccess: (deletedId) => {
      setSupportedModels((current) => current.filter((m) => m.id !== deletedId));
      void queryClient.invalidateQueries({ queryKey: ["supportedModels"] });
      void router.invalidate();
    },
  });

  const [importSummary, setImportSummary] = useState<Record<string, string>>({});
  const [importErrors, setImportErrors] = useState<Record<string, string>>({});

  const importMissingModels = useMutation({
    mutationFn: async (providerKey: string) => {
      setImportSummary((prev) => {
        const copy = { ...prev };
        delete copy[providerKey];
        return copy;
      });
      setImportErrors((prev) => {
        const copy = { ...prev };
        delete copy[providerKey];
        return copy;
      });

      const response = await fetch("/api/local-runner/providers/import", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({ providerKey }),
      });

      if (!response.ok) {
        const payload = (await response.json()) as { error?: string };
        throw new Error(payload.error ?? "Failed to import models.");
      }

      const payload = (await response.json()) as {
        providerKey: string;
        detectedAt: string;
        detectedCliVersion: string;
        detectionMethod: string;
        models: Array<{
          modelId: string;
          displayName: string;
          source?: string;
        }>;
        warnings: string[];
      };

      const gateways = await createGatewayBundle();
      const existingModelIds = new Set(
        supportedModels
          .filter((model) => model.providerKey === providerKey)
          .map((model) => model.modelId),
      );
      const maxSortOrder = supportedModels.reduce((max, model) => Math.max(max, model.sortOrder), 0);

      let importedCount = 0;
      let skippedCount = 0;

      for (const model of payload.models) {
        if (existingModelIds.has(model.modelId)) {
          skippedCount++;
          continue;
        }

        await gateways.workflowEngineGateway.createSupportedModel({
          providerKey: payload.providerKey as "codex" | "claude" | "gemini",
          modelId: model.modelId,
          displayName: model.displayName,
          isEnabled: true,
          sortOrder: maxSortOrder + importedCount + 1,
          source: "detected",
          detectionMethod: payload.detectionMethod,
          detectedCliVersion: payload.detectedCliVersion,
          lastDetectedAt: payload.detectedAt,
        });
        existingModelIds.add(model.modelId);
        importedCount++;
      }

      return {
        providerKey: payload.providerKey,
        detectedCount: payload.models.length,
        importedCount,
        skippedCount,
        warnings: payload.warnings,
      };
    },
    onError: (error, providerKey) => {
      setImportErrors((prev) => ({
        ...prev,
        [providerKey]: error instanceof Error ? error.message : "Failed to import models.",
      }));
    },
    onSuccess: async (data) => {
      const gateways = await createGatewayBundle();
      const freshSupported = await gateways.workflowEngineGateway.listSupportedModels();
      setSupportedModels(freshSupported);

      setImportSummary((prev) => ({
        ...prev,
        [data.providerKey]: `${data.importedCount} new models imported, ${data.skippedCount} already registered.`,
      }));

      void queryClient.invalidateQueries({ queryKey: ["supportedModels"] });
      void router.invalidate();
    },
  });

  const toggleSupportedModel = useMutation({
    mutationFn: async (input: { id: string; isEnabled: boolean }) => {
      const gateways = await createGatewayBundle();
      return gateways.workflowEngineGateway.updateSupportedModel(input.id, {
        isEnabled: input.isEnabled,
      });
    },
    onSuccess: (updatedModel) => {
      setSupportedModels((current) =>
        current.map((m) => (m.id === updatedModel.id ? updatedModel : m))
      );
      void queryClient.invalidateQueries({ queryKey: ["supportedModels"] });
      void router.invalidate();
    },
  });

  const anyPending =
    refreshInventory.isPending ||
    installProvider.isPending ||
    authenticateProvider.isPending ||
    addSupportedModel.isPending ||
    deleteSupportedModel.isPending ||
    toggleSupportedModel.isPending ||
    importMissingModels.isPending ||
    isRouterPending;

  const sortedProviders = useMemo(
    () => [...providers].sort((left, right) => left.key.localeCompare(right.key)),
    [providers],
  );

  return (
    <PageFrame
      description="Discover installed provider CLIs, verify auth readiness, and trigger OS-aware installs from the local runner."
      title="AI Providers"
    >
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Provider Inventory
            </p>
            <h3 className="mt-3 text-2xl font-semibold tracking-tight">
              Local provider readiness
            </h3>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              The runner publishes detected AI CLIs, auth readiness, and model catalogs. Install
              actions stay on the runner so the browser never shells out directly.
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={health.status === "online" ? "success" : "danger"}>{health.status}</Badge>
            <Badge tone="neutral">{installedCount} installed</Badge>
            <Badge tone="neutral">{providerCount} supported</Badge>
          </div>
        </div>

        <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <DetailRow label="Runner" value={health.baseUrl} />
          <DetailRow label="Version" value={health.runnerVersion ?? "Unavailable"} />
          <DetailRow label="Workspace" value={health.cwd ?? "Unavailable"} />
          <DetailRow label="Platform" value={health.os ?? "Unavailable"} />
        </div>

        <div className="mt-6 flex justify-end">
          <Button
            disabled={anyPending}
            type="button"
            variant="secondary"
            onClick={() => refreshInventory.mutate()}
          >
            {refreshInventory.isPending ? (
              <>
                <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                Refreshing...
              </>
            ) : (
              "Refresh inventory"
            )}
          </Button>
        </div>
      </section>

      <section className="mt-6 space-y-4">
        {sortedProviders.map((provider) => {
          const installStatus = provider.installStatus ?? (provider.installed ? "INSTALLED" : "NOT_INSTALLED");
          const authStatus = provider.authStatus ?? "UNKNOWN";
          const providerModels = provider.models ?? [];
          const detectedBinary = provider.detectedBinary ?? provider.binaryPath ?? "Binary not detected";
          const detectedVersion = provider.detectedVersion ?? provider.version ?? "Unavailable";
          const canInstall = provider.supported !== false && installStatus !== "UNSUPPORTED_OS";
          const isInstalled = installStatus === "INSTALLED";

          // Which mutation is active for this specific provider card
          const isThisProviderInstalling =
            installProvider.isPending && installProvider.variables === provider.key;
          const isRefreshingThis = refreshInventory.isPending;

          return (
            <article
              key={provider.key}
              className="rounded-[1.4rem] border border-border bg-card/80 p-5"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h4 className="text-lg font-semibold">{provider.label}</h4>
                    <Badge tone={providerTone(installStatus)}>{installStatus}</Badge>
                    <Badge tone={authTone(authStatus)}>{authStatus}</Badge>
                    {provider.supported === false ? <Badge tone="warning">unsupported</Badge> : null}
                  </div>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Provider key <span className="font-medium text-foreground">{provider.key}</span>
                  </p>
                </div>

                <div className="flex flex-wrap gap-2">
                  {/* Auth button (shown when installed but auth required) */}
                  {isInstalled && authStatus === "AUTH_REQUIRED" && (
                    <Button
                      disabled={anyPending}
                      type="button"
                      variant="default"
                      onClick={() => authenticateProvider.mutate(provider.key)}
                    >
                      {authenticateProvider.isPending && authenticateProvider.variables === provider.key ? (
                        <>
                          <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                          Authenticating...
                        </>
                      ) : (
                        "Auth"
                      )}
                    </Button>
                  )}

                  {/* Install button (shown when NOT installed) */}
                  {!isInstalled && (
                    <Button
                      disabled={anyPending || !canInstall}
                      type="button"
                      variant="secondary"
                      onClick={() => installProvider.mutate(provider.key)}
                    >
                      {isThisProviderInstalling ? (
                        <>
                          <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                          Installing...
                        </>
                      ) : (
                        "Install"
                      )}
                    </Button>
                  )}

                  {/* Refresh button (always shown) */}
                  <Button
                    disabled={anyPending}
                    type="button"
                    variant="secondary"
                    onClick={() => refreshInventory.mutate()}
                  >
                    {isRefreshingThis ? (
                      <>
                        <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                        Refreshing...
                      </>
                    ) : (
                      "Refresh"
                    )}
                  </Button>
                </div>
              </div>

              <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <DetailRow label="Binary" value={detectedBinary} />
                <DetailRow label="Version" value={detectedVersion} />
                <DetailRow label="Install Hint" value={provider.installHint ?? "Unavailable"} />
                <DetailRow
                  label="Last Error"
                  value={provider.lastError?.trim() ? provider.lastError : "None"}
                />
              </div>

              <div className="mt-6 border-t border-border pt-4">
                <div className="flex flex-wrap items-center justify-between gap-4">
                  <div>
                    <h5 className="text-xs uppercase tracking-[0.24em] text-muted-foreground font-mono">
                      Supported Models
                    </h5>
                    <p className="text-xs text-muted-foreground mt-1">
                      Configure models available in FlowPilot dropdowns and validated during runtime.
                    </p>
                  </div>
                  <Button
                    disabled={anyPending}
                    type="button"
                    variant="link"
                    className="text-emerald-600 hover:text-emerald-500 dark:text-emerald-400 dark:hover:text-emerald-300 hover:underline p-0 h-auto font-medium text-xs whitespace-nowrap self-center"
                    onClick={() => importMissingModels.mutate(provider.key)}
                  >
                    {importMissingModels.isPending && importMissingModels.variables === provider.key ? (
                      <>
                        <RefreshCw className="mr-1 h-3 w-3 animate-spin inline-block" />
                        Importing...
                      </>
                    ) : (
                      "Check and import missing models"
                    )}
                  </Button>
                </div>

                {importSummary[provider.key] && (
                  <div className="mt-3 text-xs text-emerald-600 bg-emerald-50 dark:text-emerald-400 dark:bg-emerald-950/20 px-3 py-1.5 rounded-lg border border-emerald-100 dark:border-emerald-900/30">
                    {importSummary[provider.key]}
                  </div>
                )}

                {importErrors[provider.key] && (
                  <div className="mt-3 text-xs text-destructive bg-destructive/10 px-3 py-1.5 rounded-lg border border-destructive/20 font-medium">
                    {importErrors[provider.key]}
                  </div>
                )}

                {/* List of supported models for this provider */}
                <div className="mt-3">
                  {supportedModels.filter((m) => m.providerKey === provider.key).length === 0 ? (
                    <p className="text-sm text-muted-foreground italic">No supported models configured.</p>
                  ) : (
                    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                      {supportedModels
                        .filter((m) => m.providerKey === provider.key)
                        .map((model) => (
                          <div
                            key={model.id}
                            className="flex items-center justify-between rounded-xl border border-border bg-background px-3 py-2 text-sm"
                          >
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5 min-w-0">
                                <p className="font-medium truncate">{model.displayName}</p>
                                {model.source === "detected" && (
                                  <span className="inline-flex items-center rounded-full bg-primary/10 px-1.5 py-0.5 text-[9px] font-mono font-medium text-primary border border-primary/20">
                                    detected
                                  </span>
                                )}
                              </div>
                              <p className="text-xs text-muted-foreground font-mono truncate">{model.modelId}</p>
                            </div>
                            <div className="ml-2 flex items-center gap-2">
                              <button
                                type="button"
                                className={`text-xs px-2 py-1 rounded-full border transition-colors ${model.isEnabled
                                  ? "bg-success/10 text-success border-success/30 hover:bg-success/20"
                                  : "bg-muted text-muted-foreground border-border hover:bg-muted/80"
                                  }`}
                                disabled={anyPending}
                                onClick={() =>
                                  toggleSupportedModel.mutate({ id: model.id, isEnabled: !model.isEnabled })
                                }
                              >
                                {model.isEnabled ? "Enabled" : "Disabled"}
                              </button>
                              <Button
                                disabled={anyPending}
                                size="icon"
                                variant="ghost"
                                className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10 flex items-center justify-center"
                                onClick={() => {
                                  if (confirm(`Are you sure you want to delete ${model.displayName}?`)) {
                                    deleteSupportedModel.mutate(model.id);
                                  }
                                }}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </div>
                          </div>
                        ))}
                    </div>
                  )}
                </div>

                {/* Add Supported Model Form */}
                <AddSupportedModelForm
                  providerKey={provider.key as any}
                  onAdd={async (modelId, displayName) => {
                    await addSupportedModel.mutateAsync({ providerKey: provider.key as any, modelId, displayName });
                  }}
                  disabled={anyPending}
                  existingModelIds={supportedModels.map((m) => m.modelId)}
                />
              </div>
            </article>
          );
        })}
      </section>
    </PageFrame>
  );
}

function providerTone(status: string) {
  switch (status) {
    case "INSTALLED":
      return "success";
    case "FAILED":
    case "UNSUPPORTED_OS":
      return "danger";
    default:
      return "neutral";
  }
}

function authTone(status: string) {
  switch (status) {
    case "READY":
      return "success";
    case "AUTH_REQUIRED":
      return "warning";
    default:
      return "neutral";
  }
}

function ModelChip({ model }: { model: LocalRunnerProviderModel }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1 text-xs font-medium">
      <span>{model.displayName}</span>
      <span className="text-muted-foreground">{model.available ? "ready" : "locked"}</span>
    </span>
  );
}

function DetailRow({
  label,
  value,
}: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

function AddSupportedModelForm({
  providerKey,
  onAdd,
  disabled,
  existingModelIds,
}: {
  providerKey: "codex" | "claude" | "gemini";
  onAdd: (modelId: string, displayName: string) => Promise<void>;
  disabled: boolean;
  existingModelIds: string[];
}) {
  const [modelId, setModelId] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    const trimmedModelId = modelId.trim();
    const trimmedDisplayName = displayName.trim();

    if (!trimmedModelId || !trimmedDisplayName) {
      setError("Both Model ID and Display Name are required.");
      return;
    }

    if (existingModelIds.includes(trimmedModelId)) {
      setError("This Model ID is already registered.");
      return;
    }

    try {
      setIsSubmitting(true);
      await onAdd(trimmedModelId, trimmedDisplayName);
      setModelId("");
      setDisplayName("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to register model.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-2 rounded-xl border border-dashed border-border p-4 bg-muted/20">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex-1 min-w-[200px]">
          <label className="text-[10px] uppercase font-mono tracking-wider text-muted-foreground block mb-1">
            Model ID (e.g. {providerKey === "gemini" ? "gemini-2.5-pro" : providerKey === "claude" ? "claude-3-5-sonnet" : "gpt-4o"})
          </label>
          <input
            type="text"
            className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            placeholder="model-id"
            value={modelId}
            onChange={(e) => setModelId(e.target.value)}
            disabled={disabled || isSubmitting}
          />
        </div>
        <div className="flex-1 min-w-[200px]">
          <label className="text-[10px] uppercase font-mono tracking-wider text-muted-foreground block mb-1">
            Display Name
          </label>
          <input
            type="text"
            className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            placeholder="Display Name"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            disabled={disabled || isSubmitting}
          />
        </div>
        <div className="self-end pt-1">
          <Button type="submit" disabled={disabled || isSubmitting} size="sm" className="h-9">
            <Plus className="h-4 w-4 mr-1" />
            {isSubmitting ? "Adding..." : "Add model"}
          </Button>
        </div>
      </div>
      {error && <p className="text-xs text-destructive mt-1 font-medium">{error}</p>}
    </form>
  );
}
