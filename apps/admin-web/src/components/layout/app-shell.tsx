import type { ReactNode } from "react";

import { Bot, FileStack, FolderKanban, LayoutDashboard, Logs, Settings2, ShieldCheck, Workflow } from "lucide-react";
import { Link, useLocation } from "@tanstack/react-router";

import { useAuth } from "@/features/auth/auth-provider";
import { cn } from "@/lib/utils/cn";

const navItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/projects", label: "Projects", icon: FolderKanban },
  { to: "/features", label: "Features", icon: Bot },
  { to: "/workflow-definitions", label: "Definitions", icon: Workflow },
  { to: "/workflow-runs", label: "Workflow Runs", icon: Workflow },
  { to: "/approvals", label: "Approval Center", icon: ShieldCheck },
  { to: "/outputs", label: "Outputs", icon: FileStack },
  { to: "/ai-runs", label: "AI Runs", icon: Logs },
  { to: "/settings", label: "Settings", icon: Settings2 },
];

export function AppShell({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const { session, loading, signOut } = useAuth();

  return (
    <div className="noise-bg min-h-screen">
      <div className="mx-auto grid min-h-screen max-w-[1500px] grid-cols-1 gap-4 px-4 py-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur">
          <div className="mb-8">
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              FlowPilot
            </p>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight">
              Admin Shell
            </h1>
            <p className="mt-2 text-sm text-muted-foreground">
              Workflow-first operations shell for AI-assisted mobile engineering.
            </p>
            <p className="mt-4 rounded-2xl border border-border bg-background/70 px-3 py-2 text-xs text-muted-foreground">
              {loading
                ? "Loading session..."
                : session?.mode === "demo"
                  ? "Demo mode"
                  : session?.user.email ?? "Supabase user"}
            </p>
          </div>
          <nav className="space-y-2">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.to || pathname.startsWith(`${item.to}/`);

              return (
                <Link
                  key={item.to}
                  className={cn(
                    "flex items-center gap-3 rounded-2xl px-4 py-3 text-sm transition-colors",
                    active
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-muted/80 hover:text-foreground",
                  )}
                  to={item.to}
                >
                  <Icon className="size-4" />
                  <span>{item.label}</span>
                </Link>
              );
            })}
          </nav>
          {session?.mode === "supabase" ? (
            <button
              className="mt-6 w-full rounded-2xl border border-border px-4 py-3 text-sm text-muted-foreground hover:bg-muted/80"
              onClick={() => void signOut()}
              type="button"
            >
              Sign out
            </button>
          ) : null}
        </aside>
        <main className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur lg:p-7">
          {children}
        </main>
      </div>
    </div>
  );
}

