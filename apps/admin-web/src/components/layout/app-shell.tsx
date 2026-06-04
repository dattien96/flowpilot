import { Link, useLocation, useRouterState } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Power, RotateCw } from "lucide-react";

import { primaryNavItems, settingsNavItems, type NavItem } from "@/components/layout/app-nav";
import { buildMcpServerStatus, buildRunnerStatus } from "@/components/layout/app-shell-status";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration } from "@/domain/model/entity/integration";
import type { LocalRunnerHealth } from "@/domain/model/entity/local-runner";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { requestArtifactSyncBootstrap } from "@/features/artifacts/artifact-sync-bootstrap-client";
import { useAuth } from "@/features/auth/auth-provider";
import { ThemeSwitcher } from "@/features/theme/components/theme-switcher";
import { cn } from "@/lib/utils/cn";

type SettingsNavStatusMap = Partial<
  Record<
    string,
    {
      kind: "count" | "runner";
      label: string;
      toneClass?: string;
      textClass?: string;
      activeToneClass?: string;
      activeTextClass?: string;
    }
  >
>;

export const APP_SHELL_LAYOUT_CLASSES = {
  outer: "noise-bg min-h-screen lg:h-screen lg:overflow-hidden",
  grid:
    "mx-auto grid min-h-screen max-w-[1500px] grid-cols-1 gap-4 px-4 py-4 lg:h-full lg:min-h-0 lg:grid-cols-[280px_minmax(0,1fr)]",
  sidebar:
    "panel-shadow flex flex-col justify-between rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur lg:sticky lg:top-4 lg:h-[calc(100vh-2rem)] lg:self-start lg:overflow-y-auto",
  main:
    "panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur lg:h-[calc(100vh-2rem)] lg:min-h-0 lg:overflow-y-auto lg:p-7",
} as const;

function NavSection({
  items,
  itemStatuses,
  title,
}: {
  items: readonly NavItem[];
  itemStatuses?: SettingsNavStatusMap;
  title: string;
}) {
  const location = useLocation();

  function isActive(item: NavItem) {
    if (item.to) {
      return location.pathname === item.to || location.pathname.startsWith(`${item.to}/`);
    }
    return item.children?.some((child) => isActive(child)) ?? false;
  }

  return (
    <div>
      <p className="mb-3 font-mono text-[11px] uppercase tracking-[0.28em] text-muted-foreground">
        {title}
      </p>
      <div className="space-y-2">
        {items.map((item) => {
          const Icon = item.icon;
          const status = item.statusKey ? itemStatuses?.[item.statusKey] : item.to ? itemStatuses?.[item.to] : undefined;
          const active = isActive(item);

          if (item.children?.length) {
            return (
              <div className="space-y-2" key={item.label}>
                <div
                  className={cn(
                    "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm",
                    active ? "bg-accent/12 text-foreground" : "text-muted-foreground",
                  )}
                >
                  <Icon className="size-4" />
                  <span className="min-w-0 flex-1 font-medium">{item.label}</span>
                  {status?.kind === "count" ? (
                    <span className="inline-flex min-w-[3.25rem] items-center justify-center rounded-full border border-border/70 bg-background/80 px-2 py-1 font-mono text-[11px] font-semibold leading-none text-foreground/80">
                      {status.label}
                    </span>
                  ) : null}
                </div>

                <div className="space-y-2 pl-4">
                  {item.children.map((child) => {
                    const ChildIcon = child.icon;
                    const childActive = isActive(child);
                    return (
                      <Link
                        key={child.to ?? child.label}
                        className={cn(
                          "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm transition-colors",
                          childActive
                            ? "bg-accent text-white"
                            : "text-muted-foreground hover:bg-muted/80 hover:text-foreground",
                        )}
                        to={child.to ?? "/settings/mcp-servers"}
                      >
                        <ChildIcon className={cn("size-4", childActive ? "text-white" : undefined)} />
                        <span className={cn("min-w-0 flex-1", childActive ? "text-white" : undefined)}>
                          {child.label}
                        </span>
                      </Link>
                    );
                  })}
                </div>
              </div>
            );
          }

          return (
            <Link
              key={item.to ?? item.label}
              className={cn(
                "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm transition-colors",
                active
                  ? "bg-accent text-white"
                  : "text-muted-foreground hover:bg-muted/80 hover:text-foreground",
              )}
              to={item.to ?? "/"}
            >
              <Icon className={cn("size-4", active ? "text-white" : undefined)} />
              <span className={cn("min-w-0 flex-1", active ? "text-white" : undefined)}>
                {item.label}
              </span>
              {status?.kind === "count" ? (
                <span
                  className={cn(
                    "inline-flex min-w-[3.25rem] items-center justify-center rounded-full border px-2 py-1 font-mono text-[11px] font-semibold leading-none",
                    active
                      ? "border-white/20 bg-white/12 text-white"
                      : "border-border/70 bg-background/80 text-foreground/80",
                  )}
                >
                  {status.label}
                </span>
              ) : null}
              {status?.kind === "runner" ? (
                <span
                  className={cn(
                    "inline-flex min-w-[5.75rem] items-center justify-center gap-2 rounded-full border px-2 py-1 text-[11px] font-semibold leading-none",
                    active
                      ? "border-white/20 bg-white/12 text-white"
                      : "border-border/70 bg-background/80 text-foreground/80",
                  )}
                >
                  <span
                    className={cn(
                      "size-2.5 rounded-full shadow-sm",
                      active ? status.activeToneClass : status.toneClass,
                    )}
                  />
                  <span
                    className={cn(
                      "truncate",
                      active ? status.activeTextClass ?? "text-white" : status.textClass,
                    )}
                  >
                    {status.label}
                  </span>
                </span>
              ) : null}
            </Link>
          );
        })}
      </div>
    </div>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const { loading, session, signOut } = useAuth();
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  });
  const [settingsStatuses, setSettingsStatuses] = useState<SettingsNavStatusMap>({});
  const [runnerOnline, setRunnerOnline] = useState<boolean>(false);
  const [isPending, setIsPending] = useState<"shutdown" | "restart" | null>(null);


  useEffect(() => {
    let cancelled = false;

    async function loadStatuses() {
      try {
        const gateways = createGatewayBundle();
        const runnerHealth = await new CheckLocalRunnerHealthUseCase(
          gateways.localRunnerGateway,
        ).execute();
        const integrations = await gateways.integrationGateway
          .listAllIntegrations()
          .catch(() => [] as Integration[]);

        if (cancelled) {
          return;
        }

        if (runnerHealth.status === "online") {
          void requestArtifactSyncBootstrap(runnerHealth).catch((error) => {
            console.warn("Unable to request artifact sync bootstrap:", error);
          });
        }

        setSettingsStatuses(createSettingsNavStatuses(integrations, runnerHealth));
        setRunnerOnline(runnerHealth.status === "online");
      } catch {
        if (cancelled) {
          return;
        }

        setSettingsStatuses(
          createSettingsNavStatuses([], {
            status: "offline",
            runnerVersion: null,
            cwd: null,
            os: null,
            startedAt: null,
            baseUrl: "",
            errorMessage: null,
          }),
        );
        setRunnerOnline(false);
      }
    }

    void loadStatuses();
    const intervalId = window.setInterval(() => {
      void loadStatuses();
    }, 30_000);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
    };
  }, [pathname]);

  const handleShutdown = async () => {
    if (!window.confirm("Are you sure you want to shut down the dev stack?")) return;
    setIsPending("shutdown");
    try {
      const gateways = createGatewayBundle();
      await gateways.localRunnerGateway.shutdownStack();
    } catch (err) {
      console.error("Shutdown failed", err);
      setIsPending(null);
    }
  };

  const handleRestart = async () => {
    if (!window.confirm("Are you sure you want to restart the dev stack?")) return;
    setIsPending("restart");
    try {
      const gateways = createGatewayBundle();
      await gateways.localRunnerGateway.restartStack();
      
      // Poll runner health
      const interval = setInterval(async () => {
        try {
          const innerGateways = createGatewayBundle();
          const health = await new CheckLocalRunnerHealthUseCase(innerGateways.localRunnerGateway).execute();
          if (health.status === "online") {
            clearInterval(interval);
            void requestArtifactSyncBootstrap(health).catch((error) => {
              console.warn("Unable to request artifact sync bootstrap after restart:", error);
            });
            setIsPending(null);
            const integrations = await innerGateways.integrationGateway.listAllIntegrations();
            setSettingsStatuses(createSettingsNavStatuses(integrations, health));
            setRunnerOnline(true);
          }
        } catch (e) {
          // keep polling
        }
      }, 2000);
      
      setTimeout(() => {
        clearInterval(interval);
        setIsPending((current) => current === "restart" ? null : current);
      }, 30000);
    } catch (err) {
      console.error("Restart failed", err);
      setIsPending(null);
    }
  };


  return (
    <div className={APP_SHELL_LAYOUT_CLASSES.outer}>
      <div className={APP_SHELL_LAYOUT_CLASSES.grid}>
        <aside className={APP_SHELL_LAYOUT_CLASSES.sidebar}>
          <div>
            <div className="mb-8">
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                FlowPilot
              </p>
              <h1 className="mt-3 text-2xl font-semibold tracking-tight">Foundation Shell</h1>
              <p className="mt-2 text-sm text-muted-foreground">
                Vite, TanStack Router, Query, and browser-auth baseline for the admin app.
              </p>
              <p className="mt-4 rounded-2xl border border-border bg-background/70 px-3 py-2 text-xs text-muted-foreground">
                {loading
                  ? "Loading session..."
                  : session?.mode === "demo"
                    ? "Demo mode"
                    : session?.user.email ?? "Signed in"}
              </p>
            </div>

            <div className="space-y-6">
              <NavSection items={primaryNavItems} title="Workspace" />
              <NavSection itemStatuses={settingsStatuses} items={settingsNavItems} title="Settings" />
            </div>
          </div>

          <div className="mt-8 space-y-4">
            <div className="flex flex-col gap-2 rounded-2xl border border-border/60 bg-background/40 p-3">
              <p className="font-mono text-[9px] uppercase tracking-[0.28em] text-muted-foreground">
                Appearance
              </p>
              <ThemeSwitcher className="w-full" compact />
            </div>

            {runnerOnline && (
              <div className="flex flex-col gap-2 rounded-2xl border border-border/60 bg-background/40 p-3">
                <p className="font-mono text-[9px] uppercase tracking-[0.28em] text-muted-foreground">
                  Dev Stack
                </p>
                {isPending === "shutdown" ? (
                  <div className="flex h-9 w-full items-center justify-center rounded-[1.25rem] border border-border/80 bg-background/50 px-3 text-[10px] font-medium text-destructive backdrop-blur-sm">
                    Shutting down...
                  </div>
                ) : isPending === "restart" ? (
                  <div className="flex h-9 w-full items-center justify-center rounded-[1.25rem] border border-border/80 bg-background/50 px-3 text-[10px] font-medium text-muted-foreground backdrop-blur-sm">
                    Restarting...
                  </div>
                ) : (
                  <div className="grid grid-cols-2 gap-2">
                    <button
                      onClick={handleShutdown}
                      className="flex min-w-0 items-center justify-center rounded-[1rem] border border-emerald-500/30 bg-emerald-500/5 py-2 text-muted-foreground hover:bg-destructive hover:border-destructive hover:text-white active:scale-95 transition-all duration-200 outline-none focus-visible:ring-1 focus-visible:ring-accent"
                      title="Shutdown Dev Stack"
                      type="button"
                    >
                      <Power className="size-4" />
                    </button>
                    <button
                      onClick={handleRestart}
                      className="flex min-w-0 items-center justify-center rounded-[1rem] border border-emerald-500/30 bg-emerald-500/5 py-2 text-muted-foreground hover:bg-muted/70 hover:text-foreground active:scale-95 transition-all duration-200 outline-none focus-visible:ring-1 focus-visible:ring-accent"
                      title="Restart Dev Stack"
                      type="button"
                    >
                      <RotateCw className="size-4" />
                    </button>
                  </div>
                )}
              </div>
            )}

            {session?.mode === "supabase" ? (
              <Button className="w-full justify-center" onClick={() => void signOut()} variant="secondary">
                Sign out
              </Button>
            ) : null}
          </div>

        </aside>

        <main className={APP_SHELL_LAYOUT_CLASSES.main}>
          {children}
        </main>
      </div>
    </div>
  );
}

export function createSettingsNavStatuses(
  integrations: Integration[],
  runnerHealth: LocalRunnerHealth | null,
): SettingsNavStatusMap {
  const mcpStatus = buildMcpServerStatus(integrations);
  const runnerStatus = buildRunnerStatus(runnerHealth);

  return {
    "/settings/mcp-servers": {
      kind: "count",
      label: mcpStatus.label,
    },
    "/settings/runner": {
      kind: "runner",
      label: runnerStatus.label,
      toneClass: runnerStatus.toneClass,
      textClass: runnerStatus.textClass,
      activeToneClass: runnerStatus.activeToneClass,
      activeTextClass: runnerStatus.activeTextClass,
    },
  };
}
