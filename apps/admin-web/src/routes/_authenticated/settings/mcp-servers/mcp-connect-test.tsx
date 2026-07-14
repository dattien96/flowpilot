import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";

/**
 * MCP Test Console UI removed (Jira Rovo MCP path). Backend REST helpers
 * (executeJiraMcpPrompt / jira_list_*) remain in the local-runner for
 * internal use only — not exposed here.
 */
export const Route = createFileRoute("/_authenticated/settings/mcp-servers/mcp-connect-test")({
  component: McpConnectTestRetiredPage,
});

export function McpConnectTestRetiredPage() {
  return (
    <PageFrame
      description="The MCP Test Console UI has been retired."
      title="MCP Test Console (retired)"
    >
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6 space-y-4">
        <p className="text-sm text-muted-foreground">
          FlowPilot now configures Atlassian <strong>Rovo remote MCP</strong> into AI provider configs using your
          connected email + API token (<code className="text-xs">Basic</code> auth). Live agents call MCP tools
          directly — a separate REST Test Console is no longer needed.
        </p>
        <p className="text-sm text-muted-foreground">
          Runner-side REST helpers are kept in code for diagnostics but are not exposed in this UI.
        </p>
        <div className="flex flex-wrap gap-3">
          <Link
            className="inline-flex items-center justify-center rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold"
            to="/settings/mcp-servers/jira-link"
          >
            Jira MCP settings
          </Link>
          <a
            className="inline-flex items-center justify-center rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold"
            href="https://support.atlassian.com/security-and-access-policies/docs/control-atlassian-rovo-mcp-server-settings/#Configure-authentication"
            rel="noreferrer"
            target="_blank"
          >
            Enable Rovo MCP API token (admin)
          </a>
        </div>
      </section>
    </PageFrame>
  );
}
