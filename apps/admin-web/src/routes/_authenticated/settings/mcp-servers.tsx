import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { Cloud, FileText } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { loadMcpSettingsData } from "@/features/mcp/mcp-settings-loader";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/settings/mcp-servers")({
  loader: loadMcpSettingsData,
  component: McpServersOverviewPage,
});

export function McpServersOverviewPage() {
  const location = useLocation();
  const { allIntegrations, backends, health } = Route.useLoaderData();
  const connectedCount = allIntegrations.filter((integration) => integration.status === "connected").length;

  if (location.pathname !== "/settings/mcp-servers") {
    return <Outlet />;
  }

  return (
    <PageFrame
      description="MCP settings are split into dedicated child pages for reusable instances, Google Drive setup, and Jira link management."
      title="MCP Servers"
    >
      <div className="space-y-6">
        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Parent Menu
              </p>
              <h2 className="mt-3 text-2xl font-semibold tracking-tight">
                MCP settings now live in child pages
              </h2>
              <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
                Use the MCP submenu in Settings to work on one concern at a time: reusable MCP
                instances, Google Console plus Google Drive MCP, or the Jira MCP link.
              </p>
            </div>
            <Badge tone={health.status === "online" ? "success" : "danger"}>{health.status}</Badge>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-3">
            <SummaryCard label="Runner" value={health.baseUrl} />
            <SummaryCard label="Backends" value={String(backends.length)} />
            <SummaryCard label="Connected / Total" value={`${connectedCount}/${allIntegrations.length}`} />
          </div>
        </section>

        <div className="grid gap-4 xl:grid-cols-2">
          <ChildCard
            badge="Google Console + Drive MCP"
            description="Manage the Google Cloud setup flow and the Google Drive MCP step in one place."
            icon={Cloud}
            title="Google Console - Driver"
            to="/settings/google-drive-setup"
          />
          <ChildCard
            badge="Atlassian backend"
            description="Enable, verify, and inspect the runner-side Jira MCP link separately from instance CRUD."
            icon={FileText}
            title="Jira"
            to="/settings/mcp-servers/jira-link"
          />
        </div>
      </div>
    </PageFrame>
  );
}

function ChildCard({
  badge,
  description,
  icon,
  title,
  to,
}: {
  badge: string;
  description: string;
  icon: typeof Cloud | typeof FileText;
  title: string;
  to: string;
}) {
  const Icon = icon;

  return (
    <Link
      className="rounded-[1.6rem] border border-border bg-background/70 p-6 transition-colors hover:bg-muted/40"
      to={to}
    >
      <p className="font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">{badge}</p>
      <div className="mt-3 flex items-center gap-3">
        <Icon className="size-5 text-foreground" />
        <h3 className="text-xl font-semibold tracking-tight">{title}</h3>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      <p className="mt-4 text-sm font-semibold text-accent">Open page</p>
    </Link>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}
