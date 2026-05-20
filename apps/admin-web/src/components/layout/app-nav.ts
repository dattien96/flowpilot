import {
  Bot,
  FolderKanban,
  LayoutDashboard,
  PlugZap,
  Settings2,
  Sparkles,
  Users,
  Activity,
  Workflow,
  History,
  ListTree,
} from "lucide-react";

export const primaryNavItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/projects", label: "Projects", icon: FolderKanban },
  { to: "/workflows", label: "Workflows", icon: Workflow },
  { to: "/workflow-steps", label: "Workflow Steps", icon: ListTree },
  { to: "/workflow-runs", label: "Workflow Runs", icon: History },
  { to: "/teams", label: "Teams", icon: Users },
  { to: "/ai-runs", label: "AI Runs", icon: Sparkles },
] as const;

export const settingsNavItems = [
  { to: "/settings/mcp-servers", label: "MCP Servers", icon: PlugZap },
  { to: "/settings/runner", label: "Runner", icon: Activity },
  { to: "/settings/integrations", label: "Integrations", icon: Settings2 },
  { to: "/settings/prompt-templates", label: "Prompt Templates", icon: Bot },
] as const;
