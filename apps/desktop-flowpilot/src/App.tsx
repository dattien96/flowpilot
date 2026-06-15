import { useEffect, useState } from "react";
import type {
  DesktopBootstrapState,
  SupabaseConfigInput,
  SupabaseConfigValidation,
  SupabaseRuntimeStatus,
} from "@flowpilot/client-core";
import { RunStatus } from "@/components/RunStatus";
import { RunnerStatusIndicator } from "@/components/RunnerStatusIndicator";
import { ChatWorkspace } from "@/components/ChatWorkspace";
import { LoginScreen } from "@/components/LoginScreen";
import { SettingsShell, type SettingsSection } from "@/components/SettingsShell";
import { resolveDesktopBootstrapState } from "@/app/bootstrapState";
import {
  loadDesktopBootstrapUseCase,
  loginUseCase,
  logoutUseCase,
  resetAdminUseCases,
  resetAuthSessionStateUseCase,
  saveSupabaseConfigUseCase,
  validateSupabaseConfigUseCase,
} from "@/clientCore";
import { runnerModeLabel } from "@/client/createRunnerClient";

type AppPhase = "loading" | "unauthenticated" | "authenticated";
type UnauthenticatedView = "login" | "settings";
type AuthenticatedView = "chat" | "settings";

const emptyRuntimeStatus: SupabaseRuntimeStatus = {
  mode: "demo",
  configured: false,
  apiUrl: null,
  anonKey: null,
  edgeFunctionUrl: null,
  hasServiceRoleKey: false,
  projectRef: null,
  runnerReachable: true,
  envAvailable: false,
  savedConfigAvailable: false,
  edgeFunctionsReady: false,
  lastError: null,
};

export function App(): React.ReactElement {
  const modeLabel = runnerModeLabel();
  const [phase, setPhase] = useState<AppPhase>("loading");
  const [busy, setBusy] = useState(false);
  const [leftSidebarVisible, setLeftSidebarVisible] = useState(true);
  const [rightSidebarVisible, setRightSidebarVisible] = useState(true);
  const [runtimeStatus, setRuntimeStatus] = useState<SupabaseRuntimeStatus>(emptyRuntimeStatus);
  const [unauthenticatedView, setUnauthenticatedView] =
    useState<UnauthenticatedView>("login");
  const [authenticatedView, setAuthenticatedView] =
    useState<AuthenticatedView>("chat");
  const [settingsSection, setSettingsSection] =
    useState<SettingsSection>("supabase");

  useEffect(() => {
    document.title = `FlowPilot Desktop (${modeLabel})`;
  }, [modeLabel]);

  useEffect(() => {
    void refreshBootstrap();
  }, []);

  const refreshBootstrap = async () => {
    setPhase("loading");
    try {
      const bootstrap = await loadDesktopBootstrapUseCase.execute();
      applyBootstrapState(bootstrap);
    } catch {
      setRuntimeStatus({
        ...emptyRuntimeStatus,
        runnerReachable: false,
        lastError: "Unable to load desktop bootstrap state from the local runner.",
      });
      setUnauthenticatedView("settings");
      setPhase("unauthenticated");
    }
  };

  const applyBootstrapState = (bootstrap: DesktopBootstrapState) => {
    setRuntimeStatus(bootstrap.runtimeStatus);
    const resolved = resolveDesktopBootstrapState(bootstrap);
    setSettingsSection(resolved.preferredSettingsSection);
    if (resolved.view === "settings") {
      setUnauthenticatedView("settings");
      setPhase("unauthenticated");
      return;
    }
    if (resolved.view === "authenticated-chat") {
      setAuthenticatedView("chat");
      setPhase("authenticated");
      return;
    }

    setUnauthenticatedView("login");
    setPhase("unauthenticated");
  };

  const handleLogin = async (email: string, password: string) => {
    setBusy(true);
    try {
      await loginUseCase.execute(email, password);
      resetAdminUseCases();
      setAuthenticatedView("chat");
      await refreshBootstrap();
    } finally {
      setBusy(false);
    }
  };

  const handleLogout = async () => {
    setBusy(true);
    try {
      await logoutUseCase.execute();
      resetAdminUseCases();
      setUnauthenticatedView("login");
      setPhase("unauthenticated");
      await refreshBootstrap();
    } finally {
      setBusy(false);
    }
  };

  const handleValidateSupabase = async (input: SupabaseConfigInput) => {
    setBusy(true);
    try {
      return await validateSupabaseConfigUseCase.execute(input);
    } finally {
      setBusy(false);
    }
  };

  const handleSaveSupabase = async (input: SupabaseConfigInput) => {
    setBusy(true);
    try {
      await saveSupabaseConfigUseCase.execute(input);
      await resetAuthSessionStateUseCase.execute();
      resetAdminUseCases();
      await refreshBootstrap();
    } finally {
      setBusy(false);
    }
  };

  if (phase === "loading") {
    return (
      <div className="status-shell">
        <div className="status-card">
          <div className="status-title">Bootstrapping desktop workspace…</div>
          <div className="status-copy">
            Checking local runner reachability, Supabase runtime config, and auth
            session state.
          </div>
        </div>
      </div>
    );
  }

  if (phase === "unauthenticated") {
    return unauthenticatedView === "login" ? (
      <LoginScreen
        busy={busy}
        onLogin={handleLogin}
        onOpenSettings={() => {
          setSettingsSection("supabase");
          setUnauthenticatedView("settings");
        }}
        runtimeStatus={runtimeStatus}
      />
    ) : (
      <SettingsShell
        activeSection={settingsSection}
        busy={busy}
        onBack={
          runtimeStatus.configured
            ? () => setUnauthenticatedView("login")
            : undefined
        }
        onSaveSupabase={handleSaveSupabase}
        onSelectSection={setSettingsSection}
        onValidateSupabase={handleValidateSupabase}
        runtimeStatus={runtimeStatus}
        visibleSections={["supabase"]}
      />
    );
  }

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-chrome">
          <button
            className={`sidebar-toggle ${leftSidebarVisible ? "active" : ""}`}
            onClick={() => setLeftSidebarVisible((value) => !value)}
            type="button"
            aria-pressed={leftSidebarVisible}
            title={leftSidebarVisible ? "Hide left sidebar" : "Show left sidebar"}
            aria-label={leftSidebarVisible ? "Hide left sidebar" : "Show left sidebar"}
          >
            <span className="sidebar-toggle-icon" aria-hidden="true">
              ◧
            </span>
          </button>
          <button
            className={`sidebar-toggle ${rightSidebarVisible ? "active" : ""}`}
            onClick={() => setRightSidebarVisible((value) => !value)}
            type="button"
            aria-pressed={rightSidebarVisible}
            title={rightSidebarVisible ? "Hide right sidebar" : "Show right sidebar"}
            aria-label={rightSidebarVisible ? "Hide right sidebar" : "Show right sidebar"}
          >
            <span className="sidebar-toggle-icon" aria-hidden="true">
              ◨
            </span>
          </button>
        </div>
        <div className="brand">
          FlowPilot <span className="brand-sub">desktop · {modeLabel}</span>
        </div>

        <div className="header-actions">
          <div className="header-tabs" role="tablist" aria-label="Desktop mode">
            <button
              className={`header-tab ${authenticatedView === "chat" ? "active" : ""}`}
              disabled={!runtimeStatus.runnerReachable}
              onClick={() => setAuthenticatedView("chat")}
              role="tab"
              type="button"
            >
              Chat
            </button>
            <button
              className={`header-tab ${authenticatedView === "settings" ? "active" : ""}`}
              onClick={() => {
                setSettingsSection("supabase");
                setAuthenticatedView("settings");
              }}
              role="tab"
              type="button"
            >
              Settings
            </button>
          </div>

          <RunnerStatusIndicator />
          <RunStatus />

          <button className="ghost-btn" disabled={busy} onClick={() => void handleLogout()} type="button">
            Sign out
          </button>
        </div>
      </header>

        {authenticatedView === "chat" ? (
          runtimeStatus.runnerReachable ? (
          <ChatWorkspace
            leftSidebarVisible={leftSidebarVisible}
            rightSidebarVisible={rightSidebarVisible}
          />
          ) : (
            <SettingsShell
              activeSection="runner"
            busy={busy}
            onSaveSupabase={handleSaveSupabase}
            onSelectSection={setSettingsSection}
            onValidateSupabase={handleValidateSupabase}
            runtimeStatus={runtimeStatus}
          />
        )
      ) : (
        <SettingsShell
          activeSection={settingsSection}
          busy={busy}
          onSaveSupabase={handleSaveSupabase}
          onSelectSection={setSettingsSection}
          onValidateSupabase={handleValidateSupabase}
          runtimeStatus={runtimeStatus}
        />
      )}
    </div>
  );
}
