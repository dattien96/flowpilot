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
}

export function SettingsShell({
  activeSection,
  busy,
  runtimeStatus,
  onBack,
  onSelectSection,
  onValidateSupabase,
  onSaveSupabase,
}: SettingsShellProps): React.ReactElement {
  const runnerOffline = !runtimeStatus.runnerReachable;
  const sectionDisabled = (section: SettingsSection) =>
    runnerOffline && section !== "supabase" && section !== "runner";

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
          <button
            className={`settings-nav-item ${activeSection === "supabase" ? "active" : ""}`}
            onClick={() => onSelectSection("supabase")}
            type="button"
          >
            Supabase
          </button>
          <button
            className={`settings-nav-item ${activeSection === "projects" ? "active" : ""} ${sectionDisabled("projects") ? "disabled" : ""}`}
            disabled={sectionDisabled("projects")}
            onClick={() => onSelectSection("projects")}
            type="button"
          >
            Projects
          </button>
          <button
            className={`settings-nav-item ${activeSection === "workflows" ? "active" : ""} ${sectionDisabled("workflows") ? "disabled" : ""}`}
            disabled={sectionDisabled("workflows")}
            onClick={() => onSelectSection("workflows")}
            type="button"
          >
            Workflows
          </button>
          <button
            className={`settings-nav-item ${activeSection === "teams" ? "active" : ""} ${sectionDisabled("teams") ? "disabled" : ""}`}
            disabled={sectionDisabled("teams")}
            onClick={() => onSelectSection("teams")}
            type="button"
          >
            Teams
          </button>
          <button
            className={`settings-nav-item ${activeSection === "artifacts" ? "active" : ""} ${sectionDisabled("artifacts") ? "disabled" : ""}`}
            disabled={sectionDisabled("artifacts")}
            onClick={() => onSelectSection("artifacts")}
            type="button"
          >
            Artifacts
          </button>
          <button
            className={`settings-nav-item ${activeSection === "ai-providers" ? "active" : ""} ${sectionDisabled("ai-providers") ? "disabled" : ""}`}
            disabled={sectionDisabled("ai-providers")}
            onClick={() => onSelectSection("ai-providers")}
            type="button"
          >
            AI Providers
          </button>
          <button
            className={`settings-nav-item ${activeSection === "google-drive" ? "active" : ""} ${sectionDisabled("google-drive") ? "disabled" : ""}`}
            disabled={sectionDisabled("google-drive")}
            onClick={() => onSelectSection("google-drive")}
            type="button"
          >
            Google Drive
          </button>
          <button
            className={`settings-nav-item ${activeSection === "jira-mcp" ? "active" : ""} ${sectionDisabled("jira-mcp") ? "disabled" : ""}`}
            disabled={sectionDisabled("jira-mcp")}
            onClick={() => onSelectSection("jira-mcp")}
            type="button"
          >
            Jira MCP
          </button>
          <button
            className={`settings-nav-item ${activeSection === "runner" ? "active" : ""}`}
            onClick={() => onSelectSection("runner")}
            type="button"
          >
            Runner
          </button>
        </nav>
      </aside>

      <main className="settings-main">
        {runnerOffline && activeSection !== "supabase" && activeSection !== "runner" ? (
          <div className="settings-feedback error">
            Local runner is offline. Reconnect the runner from the Runner panel before
            using settings that call runner APIs.
          </div>
        ) : null}
        {activeSection === "supabase" ? (
          <SupabaseSetupScreen
            busy={busy}
            onBack={onBack}
            onSave={onSaveSupabase}
            onValidate={onValidateSupabase}
            runtimeStatus={runtimeStatus}
          />
        ) : activeSection === "projects" ? (
          <ProjectsSettings />
        ) : activeSection === "workflows" ? (
          <WorkflowsSettings />
        ) : activeSection === "teams" ? (
          <TeamsSettings />
        ) : activeSection === "artifacts" ? (
          <ArtifactsSettings />
        ) : activeSection === "ai-providers" ? (
          <AiProvidersSettings />
        ) : activeSection === "google-drive" ? (
          <McpSettings mode="google-drive" />
        ) : activeSection === "jira-mcp" ? (
          <McpSettings mode="jira" />
        ) : (
          <RunnerHealthPanel />
        )}
      </main>
    </div>
  );
}
