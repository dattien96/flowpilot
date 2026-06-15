import type { SupabaseConfigInput, SupabaseConfigValidation, SupabaseRuntimeStatus } from "@flowpilot/client-core";
import { AiProvidersSettings } from "@/components/settings/AiProvidersSettings";
import { ArtifactsSettings } from "@/components/settings/ArtifactsSettings";
import { GoogleDriveSettings } from "@/components/settings/GoogleDriveSettings";
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

const defaultSectionOrder: readonly SettingsSection[] = [
  "projects",
  "workflows",
  "teams",
  "artifacts",
  "ai-providers",
  "google-drive",
  "jira-mcp",
  "supabase",
  "runner",
];

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
  const allowedSections = visibleSections ?? defaultSectionOrder;
  const currentSection = allowedSections.includes(activeSection)
    ? activeSection
    : allowedSections.includes("supabase")
      ? "supabase"
      : allowedSections[0] ?? "supabase";
  const showSection = (section: SettingsSection) => allowedSections.includes(section);
  const navSections = defaultSectionOrder.filter(showSection);

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
          {navSections.map((section) => {
            const disabled = sectionDisabled(section);
            const label =
              section === "ai-providers"
                ? "AI Providers"
                : section === "workflows"
                  ? "Workflows/Steps"
                : section === "google-drive"
                  ? "Google Drive"
                  : section === "jira-mcp"
                    ? "Jira MCP"
                    : section === "runner"
                      ? "Runner"
                      : section.charAt(0).toUpperCase() + section.slice(1);

            return (
              <button
                aria-label={
                  section === "runner"
                    ? `Runner ${runnerOffline ? "offline" : "online"}`
                    : undefined
                }
                className={`settings-nav-item ${currentSection === section ? "active" : ""} ${disabled ? "disabled" : ""}`}
                disabled={disabled}
                key={section}
                onClick={() => onSelectSection(section)}
                type="button"
              >
                {section === "runner" ? (
                  <span className="settings-nav-item-row">
                    <span>{label}</span>
                    <span
                      aria-hidden="true"
                      className={`status-dot status-${runnerOffline ? "offline" : "online"}`}
                    />
                  </span>
                ) : (
                  label
                )}
              </button>
            );
          })}
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
          <ProjectsSettings onNavigateSection={onSelectSection} />
        ) : currentSection === "workflows" ? (
          <WorkflowsSettings />
        ) : currentSection === "teams" ? (
          <TeamsSettings />
        ) : currentSection === "artifacts" ? (
          <ArtifactsSettings />
        ) : currentSection === "ai-providers" ? (
          <AiProvidersSettings />
        ) : currentSection === "google-drive" ? (
          <GoogleDriveSettings />
        ) : currentSection === "jira-mcp" ? (
          <McpSettings mode="jira" />
        ) : (
          <RunnerHealthPanel />
        )}
      </main>
    </div>
  );
}
