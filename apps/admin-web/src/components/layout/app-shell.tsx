import { Link, useLocation, useRouterState } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";

import { primaryNavItems, settingsNavItems } from "@/components/layout/app-nav";
import { buildMcpServerStatus, buildRunnerStatus } from "@/components/layout/app-shell-status";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration } from "@/domain/model/entity/integration";
import type { LocalRunnerHealth } from "@/domain/model/entity/local-runner";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { useAuth } from "@/features/auth/auth-provider";
import { ThemeSwitcher } from "@/features/theme/components/theme-switcher";
import { cn } from "@/lib/utils/cn";

type NavItem = {
  to: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
};

type SettingsNavStatusMap = Partial<
  Record<
    string,
    {
      kind: "count" | "runner";
      label: string;
      toneClass?: string;
      textClass?: string;
    }
  >
>;

function NavSection({
  items,
  itemStatuses,
  title,
}: {
  items: NavItem[];
  itemStatuses?: SettingsNavStatusMap;
  title: string;
}) {
  const location = useLocation();

  return (
    <div>
      <p className="mb-3 font-mono text-[11px] uppercase tracking-[0.28em] text-muted-foreground">
        {title}
      </p>
      <div className="space-y-2">
        {items.map((item) => {
          const Icon = item.icon;
          const status = itemStatuses?.[item.to];
          const active =
            location.pathname === item.to || location.pathname.startsWith(`${item.to}/`);

          return (
            <Link
              key={item.to}
              className={cn(
                "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm transition-colors",
                active
                  ? "bg-accent text-white"
                  : "text-muted-foreground hover:bg-muted/80 hover:text-foreground",
              )}
              to={item.to}
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
                      active ? "bg-emerald-300" : status.toneClass,
                    )}
                  />
                  <span className={cn("truncate", active ? "text-white" : status.textClass)}>
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

  useEffect(() => {
    let cancelled = false;

    async function loadStatuses() {
      try {
        const gateways = createGatewayBundle();
        const [integrations, runnerHealth] = await Promise.all([
          gateways.integrationGateway.listAllIntegrations(),
          new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
        ]);

        if (cancelled) {
          return;
        }

        setSettingsStatuses(createSettingsNavStatuses(integrations, runnerHealth));
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

  return (
    <div className="noise-bg min-h-screen">
      <div className="mx-auto grid min-h-screen max-w-[1500px] grid-cols-1 gap-4 px-4 py-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="panel-shadow flex flex-col justify-between rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur">
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

            {session?.mode === "supabase" ? (
              <Button className="w-full justify-center" onClick={() => void signOut()} variant="secondary">
                Sign out
              </Button>
            ) : null}
          </div>
        </aside>

        <main className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur lg:p-7">
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
    },
  };
}
