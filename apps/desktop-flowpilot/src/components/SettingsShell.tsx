import type { SupabaseConfigInput, SupabaseConfigValidation, SupabaseRuntimeStatus } from "@flowpilot/client-core";
import { AiProvidersSettings } from "@/components/settings/AiProvidersSettings";
import { ArtifactsSettings } from "@/components/settings/ArtifactsSettings";
import { McpSettings } from "@/components/settings/McpSettings";
import { ProjectsSettings } from "@/components/settings/ProjectsSettings";
import { TeamsSettings } from "@/components/settings/TeamsSettings";
import { WorkflowsSettings } from "@/components/settings/WorkflowsSettings";
import { RunnerHealthPanel } from "@/components/RunnerHealthPanel";
import { SupabaseSetupScreen } from "@/components/SupabaseSetupScreen";

export type SettingsSection =
  | "supabase"
  | "projects"
  | "workflows"
  | "teams"
  | "artifacts"
  | "ai-providers"
  | "google-drive"
  | "jira-mcp"
  | "runner";

interface SettingsShellProps {
  activeSection: SettingsSection;
  busy: boolean;
  runtimeStatus: SupabaseRuntimeStatus;
  onBack?: () => void;
  onSelectSection: (section: SettingsSection) => void;
  onValidateSupabase: (input: SupabaseConfigInput) => Promise<SupabaseConfigValidation>;
  onSaveSupabase: (input: SupabaseConfigInput) => Promise<void>;
  visibleSections?: readonly SettingsSection[];
}

export function SettingsShell({
  activeSection,
  busy,
  runtimeStatus,
  onBack,
  onSelectSection,
  onValidateSupabase,
  onSaveSupabase,
  visibleSections,
}: SettingsShellProps): React.ReactElement {
  const runnerOffline = !runtimeStatus.runnerReachable;
  const sectionDisabled = (section: SettingsSection) =>
    runnerOffline && section !== "supabase" && section !== "runner";
  const allowedSections = visibleSections ?? [
    "supabase",
    "projects",
    "workflows",
    "teams",
    "artifacts",
    "ai-providers",
    "google-drive",
    "jira-mcp",
    "runner",
  ];
  const currentSection = allowedSections.includes(activeSection)
    ? activeSection
    : allowedSections[0] ?? "supabase";
  const showSection = (section: SettingsSection) => allowedSections.includes(section);

  return (
    <div className="settings-shell">
      <aside className="settings-sidebar">
        <div className="settings-sidebar-head">
          <div className="settings-sidebar-title">Settings</div>
          <div className="settings-sidebar-copy">
            Desktop configuration surface for the Phase 1 migration.
          </div>
        </div>

        <nav className="settings-nav">
          {showSection("supabase") ? (
            <button
              className={`settings-nav-item ${currentSection === "supabase" ? "active" : ""}`}
              onClick={() => onSelectSection("supabase")}
              type="button"
            >
              Supabase
            </button>
          ) : null}
          {showSection("projects") ? (
            <button
              className={`settings-nav-item ${currentSection === "projects" ? "active" : ""} ${sectionDisabled("projects") ? "disabled" : ""}`}
              disabled={sectionDisabled("projects")}
              onClick={() => onSelectSection("projects")}
              type="button"
            >
              Projects
            </button>
          ) : null}
          {showSection("workflows") ? (
            <button
              className={`settings-nav-item ${currentSection === "workflows" ? "active" : ""} ${sectionDisabled("workflows") ? "disabled" : ""}`}
              disabled={sectionDisabled("workflows")}
              onClick={() => onSelectSection("workflows")}
              type="button"
            >
              Workflows
            </button>
          ) : null}
          {showSection("teams") ? (
            <button
              className={`settings-nav-item ${currentSection === "teams" ? "active" : ""} ${sectionDisabled("teams") ? "disabled" : ""}`}
              disabled={sectionDisabled("teams")}
              onClick={() => onSelectSection("teams")}
              type="button"
            >
              Teams
            </button>
          ) : null}
          {showSection("artifacts") ? (
            <button
              className={`settings-nav-item ${currentSection === "artifacts" ? "active" : ""} ${sectionDisabled("artifacts") ? "disabled" : ""}`}
              disabled={sectionDisabled("artifacts")}
              onClick={() => onSelectSection("artifacts")}
              type="button"
            >
              Artifacts
            </button>
          ) : null}
          {showSection("ai-providers") ? (
            <button
              className={`settings-nav-item ${currentSection === "ai-providers" ? "active" : ""} ${sectionDisabled("ai-providers") ? "disabled" : ""}`}
              disabled={sectionDisabled("ai-providers")}
              onClick={() => onSelectSection("ai-providers")}
              type="button"
            >
              AI Providers
            </button>
          ) : null}
          {showSection("google-drive") ? (
            <button
              className={`settings-nav-item ${currentSection === "google-drive" ? "active" : ""} ${sectionDisabled("google-drive") ? "disabled" : ""}`}
              disabled={sectionDisabled("google-drive")}
              onClick={() => onSelectSection("google-drive")}
              type="button"
            >
              Google Drive
            </button>
          ) : null}
          {showSection("jira-mcp") ? (
            <button
              className={`settings-nav-item ${currentSection === "jira-mcp" ? "active" : ""} ${sectionDisabled("jira-mcp") ? "disabled" : ""}`}
              disabled={sectionDisabled("jira-mcp")}
              onClick={() => onSelectSection("jira-mcp")}
              type="button"
            >
              Jira MCP
            </button>
          ) : null}
          {showSection("runner") ? (
            <button
              className={`settings-nav-item ${currentSection === "runner" ? "active" : ""}`}
              onClick={() => onSelectSection("runner")}
              type="button"
            >
              Runner
            </button>
          ) : null}
        </nav>
      </aside>

      <main className="settings-main">
        {runnerOffline && currentSection !== "supabase" && currentSection !== "runner" ? (
          <div className="settings-feedback error">
            Local runner is offline. Reconnect the runner from the Runner panel before
            using settings that call runner APIs.
          </div>
        ) : null}
        {currentSection === "supabase" ? (
          <SupabaseSetupScreen
            busy={busy}
            onBack={onBack}
            onSave={onSaveSupabase}
            onValidate={onValidateSupabase}
            runtimeStatus={runtimeStatus}
          />
        ) : currentSection === "projects" ? (
          <ProjectsSettings />
        ) : currentSection === "workflows" ? (
          <WorkflowsSettings />
        ) : currentSection === "teams" ? (
          <TeamsSettings />
        ) : currentSection === "artifacts" ? (
          <ArtifactsSettings />
        ) : currentSection === "ai-providers" ? (
          <AiProvidersSettings />
        ) : currentSection === "google-drive" ? (
          <McpSettings mode="google-drive" />
        ) : currentSection === "jira-mcp" ? (
          <McpSettings mode="jira" />
        ) : (
          <RunnerHealthPanel />
        )}
      </main>
    </div>
  );
}
