import { useMutation } from "@tanstack/react-query";
import { createFileRoute, useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type {
  LocalRunnerHealth,
  LocalRunnerProvider,
  LocalRunnerProviderModel,
} from "@/domain/model/entity/local-runner";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalProvidersUseCase } from "@/domain/usecase/local-runner/list-local-providers-usecase";
import { Badge } from "@/presentation/components/ui/badge";

type ProviderInstallResponse = {
  providers: LocalRunnerProvider[];
};

export const Route = createFileRoute("/_authenticated/settings/ai-providers")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [health, providers] = await Promise.all([
      new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
      new ListLocalProvidersUseCase(gateways.localRunnerGateway).execute(),
    ]);
    return { health, providers };
  },
  component: AiProvidersPage,
});

function AiProvidersPage() {
  const { health, providers } = Route.useLoaderData();
  return <AiProvidersContent health={health} providers={providers} />;
}

export function AiProvidersContent({
  health,
  providers: initialProviders,
}: {
  health: LocalRunnerHealth;
  providers: LocalRunnerProvider[];
}) {
  const router = useRouter();
  const isRouterPending = useRouterState({ select: (state) => state.status === "pending" });

  // Local state is the source of truth for the UI.
  // It is initialised from the route loader and updated by direct gateway fetches,
  // so we never depend on router.invalidate() re-running the loader to update the view.
  const [providers, setProviders] = useState(initialProviders);

  // Keep in sync when the router loader re-runs (e.g. navigating away and back).
  useEffect(() => {
    setProviders(initialProviders);
  }, [initialProviders]);

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

  const anyPending =
    refreshInventory.isPending ||
    installProvider.isPending ||
    authenticateProvider.isPending ||
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

              <div className="mt-4">
                <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                  Models
                </p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {providerModels.length === 0 ? (
                    <span className="text-sm text-muted-foreground">
                      No models reported yet.
                    </span>
                  ) : (
                    providerModels.map((model) => <ModelChip key={model.id} model={model} />)
                  )}
                </div>
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
