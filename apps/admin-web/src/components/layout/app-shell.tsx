import { Bot, FolderKanban, LayoutDashboard, Settings2, Sparkles } from "lucide-react";
import { Link, useLocation } from "@tanstack/react-router";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useAuth } from "@/features/auth/auth-provider";
import { cn } from "@/lib/utils/cn";

const primaryNavItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/projects", label: "Projects", icon: FolderKanban },
  { to: "/ai-runs", label: "AI Runs", icon: Sparkles },
];

const settingsNavItems = [
  { to: "/settings/integrations", label: "Integrations", icon: Settings2 },
  { to: "/settings/prompt-templates", label: "Prompt Templates", icon: Bot },
];

function NavSection({
  items,
  title,
}: {
  items: Array<{ to: string; label: string; icon: typeof LayoutDashboard }>;
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
          const active =
            location.pathname === item.to || location.pathname.startsWith(`${item.to}/`);

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
      </div>
    </div>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const { loading, session, signOut } = useAuth();

  return (
    <div className="noise-bg min-h-screen">
      <div className="mx-auto grid min-h-screen max-w-[1500px] grid-cols-1 gap-4 px-4 py-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur">
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
            <NavSection items={settingsNavItems} title="Settings" />
          </div>

          {session?.mode === "supabase" ? (
            <Button className="mt-6 w-full justify-center" onClick={() => void signOut()} variant="secondary">
              Sign out
            </Button>
          ) : null}
        </aside>

        <main className="panel-shadow rounded-[2rem] border border-border/80 bg-card/90 p-5 backdrop-blur lg:p-7">
          {children}
        </main>
      </div>
    </div>
  );
}
