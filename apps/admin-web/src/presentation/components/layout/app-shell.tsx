 "use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";

import { Bot, FileStack, FolderKanban, LayoutDashboard, Logs, Settings2, ShieldCheck, Workflow } from "lucide-react";

import { cn } from "@/lib/utils/cn";

const navItems = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/projects", label: "Projects", icon: FolderKanban },
  { href: "/features", label: "Features", icon: Bot },
  { href: "/workflow-runs", label: "Workflow Runs", icon: Workflow },
  { href: "/approvals", label: "Approval Center", icon: ShieldCheck },
  { href: "/outputs", label: "Outputs", icon: FileStack },
  { href: "/logs", label: "Logs", icon: Logs },
  { href: "/settings", label: "Settings", icon: Settings2 },
];

interface AppShellProps {
  children: ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  const pathname = usePathname();

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
