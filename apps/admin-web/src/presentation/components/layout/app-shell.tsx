"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname } from "next/navigation";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { requestArtifactSyncBootstrap } from "@/features/artifacts/artifact-sync-bootstrap-client";
import type { AdminSession } from "@/data/auth/session";
import { cn } from "@/lib/utils/cn";
import { navItems } from "@/presentation/components/layout/app-nav";

interface AppShellProps {
  children: ReactNode;
  session: AdminSession;
}

export function AppShell({ children, session }: AppShellProps) {
  const pathname = usePathname();

  useEffect(() => {
    let cancelled = false;

    async function pollRunnerHealth() {
      try {
        const gateways = createGatewayBundle();
        const health = await new CheckLocalRunnerHealthUseCase(
          gateways.localRunnerGateway,
        ).execute();

        if (cancelled || health.status !== "online") {
          return;
        }

        await requestArtifactSyncBootstrap(health);
      } catch (error) {
        if (!cancelled) {
          console.warn("Unable to request artifact sync bootstrap:", error);
        }
      }
    }

    void pollRunnerHealth();
    const intervalId = window.setInterval(() => {
      void pollRunnerHealth();
    }, 30_000);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
    };
  }, []);

  return (
    <div className="noise-bg min-h-screen">
      <div className="mx-auto grid min-h-screen max-w-[1500px] grid-cols-1 gap-4 px-4 py-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur">
          <div className="mb-8">
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              FlowPilot
            </p>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight">
              Admin MVP
            </h1>
            <p className="mt-2 text-sm text-muted-foreground">
              Workflow-first operations shell for AI-assisted mobile engineering.
            </p>
            <p className="mt-4 rounded-2xl border border-border bg-background/70 px-3 py-2 text-xs text-muted-foreground">
              {session.mode === "demo" ? "Demo mode" : session.user.email ?? "Supabase user"}
            </p>
          </div>
          <nav className="space-y-2">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.href || pathname.startsWith(`${item.href}/`);

              return (
                <a
                  key={item.href}
                  className={cn(
                    "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm transition-colors",
                    active
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-muted/80 hover:text-foreground",
                  )}
                  href={item.href}
                >
                  <Icon className="size-4" />
                  <span>{item.label}</span>
                </a>
              );
            })}
          </nav>
        </aside>
        <main className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur lg:p-7">
          {children}
        </main>
      </div>
    </div>
  );
}
