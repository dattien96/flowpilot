import {
  Bot,
  Database,
  FolderKanban,
  FileText,
  LayoutDashboard,
  PlugZap,
  Sparkles,
  Users,
  Activity,
  Cloud,
  Workflow,
  History,
  ListTree,
  BookOpen,
} from "lucide-react";

import type { ComponentType } from "react";

export type NavItem = {
  to?: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  statusKey?: string;
  children?: NavItem[];
};

export const primaryNavItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/projects", label: "Projects", icon: FolderKanban },
  { to: "/workflows", label: "Workflows", icon: Workflow },
  { to: "/workflow-steps", label: "Workflow Steps", icon: ListTree },
  { to: "/workflow-runs", label: "Workflow Runs", icon: History },
  { to: "/teams", label: "Teams", icon: Users },
  { to: "/guide", label: "FlowPilot Guide", icon: BookOpen },
] as const satisfies readonly NavItem[];

export const settingsNavItems = [
  { to: "/artifacts", label: "Artifacts", icon: Sparkles },
  { to: "/settings/ai-providers", label: "AI Providers", icon: Sparkles },
  { to: "/settings/accounts", label: "Accounts", icon: Users },
  { to: "/settings/supabase", label: "Supabase", icon: Database },
  {
    label: "MCP Servers",
    icon: PlugZap,
    statusKey: "/settings/mcp-servers",
    children: [
      { to: "/settings/google-drive-setup", label: "Google Console - Driver", icon: Cloud },
      { to: "/settings/mcp-servers/jira-link", label: "Jira", icon: FileText },
    ],
  },
  { to: "/settings/runner", label: "Runner", icon: Activity },
  { to: "/settings/prompt-templates", label: "Prompt Templates", icon: Bot },
] as const satisfies readonly NavItem[];
