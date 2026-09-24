import { useEffect, useRef, useState } from "react";
import type {
  DesktopBootstrapState,
  SupabaseConfigInput,
  SupabaseSchemaApplyInput,
  SupabaseSchemaApplyResult,
  SupabaseConfigValidation,
  SupabaseRuntimeStatus,
} from "@flowpilot/client-core";
import { PanelLeftIcon, PanelRightIcon, TerminalIcon, BoardIcon } from "@/components/icons";
import { SessionsBoard } from "@/components/SessionsBoard";
import { RunStatus } from "@/components/RunStatus";
import { RunnerStatusIndicator } from "@/components/RunnerStatusIndicator";
import { RunToast } from "@/components/RunToast";
import { AttentionInbox } from "@/components/AttentionInbox";
import { ChatWorkspace } from "@/components/ChatWorkspace";
import { LoginScreen } from "@/components/LoginScreen";
import { SettingsShell, type SettingsSection } from "@/components/SettingsShell";
import { resolveDesktopBootstrapState } from "@/app/bootstrapState";
import {
  applySupabaseMigrationsUseCase,
  getAdminUseCases,
  loadDesktopBootstrapUseCase,
  loginUseCase,
  logoutUseCase,
  resetAdminUseCases,
  resetAuthSessionStateUseCase,
  saveSupabaseConfigUseCase,
  validateSupabaseConfigUseCase,
} from "@/clientCore";
import { runnerModeLabel } from "@/client/createRunnerClient";
import { autoInitProjectEngine } from "@/components/settings/projectEngine";
import { useStore } from "@/state/store";

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
  const terminalOpen = useStore((s) => s.terminalOpen);
  const [boardOpen, setBoardOpen] = useState(false);

  // Auto-collapse side rails at narrow widths so columns never overlap or
  // force a horizontal scrollbar. Remembers the user's choice and restores it
  // when the window widens again; manual toggles still work while narrow.
  const sidebarMemory = useRef<{ left?: boolean; right?: boolean }>({});
  const leftVisRef = useRef(leftSidebarVisible);
  const rightVisRef = useRef(rightSidebarVisible);
  leftVisRef.current = leftSidebarVisible;
  rightVisRef.current = rightSidebarVisible;
  useEffect(() => {
    const rightMq = window.matchMedia("(max-width: 1240px)");
    const leftMq = window.matchMedia("(max-width: 880px)");
    const syncRight = () => {
      if (rightMq.matches) {
        if (sidebarMemory.current.right === undefined) sidebarMemory.current.right = rightVisRef.current;
        setRightSidebarVisible(false);
      } else if (sidebarMemory.current.right !== undefined) {
        setRightSidebarVisible(sidebarMemory.current.right);
        sidebarMemory.current.right = undefined;
      }
    };
    const syncLeft = () => {
      if (leftMq.matches) {
        if (sidebarMemory.current.left === undefined) sidebarMemory.current.left = leftVisRef.current;
        setLeftSidebarVisible(false);
      } else if (sidebarMemory.current.left !== undefined) {
        setLeftSidebarVisible(sidebarMemory.current.left);
        sidebarMemory.current.left = undefined;
      }
    };
    syncRight();
    syncLeft();
    rightMq.addEventListener("change", syncRight);
    leftMq.addEventListener("change", syncLeft);
    return () => {
      rightMq.removeEventListener("change", syncRight);
      leftMq.removeEventListener("change", syncLeft);
    };
  }, []);
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

  // Task-428: Ctrl+` toggles the bottom terminal dock (VS Code convention).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey && !e.shiftKey && !e.altKey && e.key === "`") {
        e.preventDefault();
        useStore.getState().toggleTerminal();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    void refreshBootstrap();
  }, []);

  // CP-81 Task-418 T-5: subscribe to the lifecycle status pushed by Electron
  // main (shared clients, active work, idle deadline, update pending,
  // reconnecting). No-op outside Electron (plain browser/e2e).
  useEffect(() => {
    const bridge = window.flowpilot?.lifecycle;
    if (!bridge?.onStatus) return;
    const unsubscribe = bridge.onStatus((status) => {
      useStore.getState().setLifecycleStatus(status);
    });
    return unsubscribe;
  }, []);

  // CP-84 (Task-431 T-5): notification click deep-links into the originating
  // run — resolve the attention item for project/chat context, then open.
  useEffect(() => {
    const subscribe = window.flowpilot?.onNotificationClick;
    if (!subscribe) return;
    return subscribe((runId) => {
      const st = useStore.getState();
      const item = st.attentionItems.find((i) => i.runId === runId);
      void st.openRunAtAttention(runId, item?.chatId ?? runId, item?.projectId);
    });
  }, []);

  // Kick the workspace data pipeline the moment we're authenticated — it runs
  // in the background while the user sits in Settings, and the chat tab shows
  // its boot overlay until the data lands. Navigator's own mount call dedupes
  // via loadProjectsInFlight.
  useEffect(() => {
    if (phase !== "authenticated") return;
    void useStore.getState().loadProjects();
  }, [phase]);

  // On every authenticated boot, run a bind-time engine init for all projects so
  // the change ledger picks up commits made since the last session — without the
  // user having to click "Re-init" or save project settings.
  useEffect(() => {
    if (phase !== "authenticated") return;
    void (async () => {
      try {
        const admin = await getAdminUseCases();
        const projects = await admin.projects.listProjects();
        await Promise.allSettled(
          projects.map(async (project) => {
            const bindings = await admin.projects.listBindings(project.id);
            if (bindings.length > 0) {
              await autoInitProjectEngine(project.id, bindings, project.platform);
            }
          }),
        );
      } catch {
        // best-effort: a startup bind failure must never crash the app
      }
    })();
  }, [phase]);

  const refreshBootstrap = async () => {
    setPhase("loading");
    // On first `just dev` start the runner binary has to compile before it can
    // serve.  Retry for up to 30 s (15 × 2 s) so the desktop does not get
    // permanently stuck on the Supabase tab because the single initial probe
    // fired before the runner was ready.
    const MAX_ATTEMPTS = 15;
    const RETRY_DELAY_MS = 2000;
    for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt++) {
      try {
        const bootstrap = await loadDesktopBootstrapUseCase.execute();
        if (bootstrap.runtimeStatus.runnerReachable || attempt === MAX_ATTEMPTS - 1) {
          applyBootstrapState(bootstrap);
          return;
        }
      } catch {
        if (attempt === MAX_ATTEMPTS - 1) {
          setRuntimeStatus({
            ...emptyRuntimeStatus,
            runnerReachable: false,
            lastError: "Unable to load desktop bootstrap state from the local runner.",
          });
          setSettingsSection("runner");
          setUnauthenticatedView("settings");
          setPhase("unauthenticated");
          return;
        }
      }
      await new Promise<void>((resolve) => setTimeout(resolve, RETRY_DELAY_MS));
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

  const handleApplySupabaseMigrations = async (
    input: SupabaseSchemaApplyInput,
  ): Promise<SupabaseSchemaApplyResult> => {
    setBusy(true);
    try {
      return await applySupabaseMigrationsUseCase.execute(input);
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
        onApplySupabaseMigrations={handleApplySupabaseMigrations}
        onSaveSupabase={handleSaveSupabase}
        onSelectSection={setSettingsSection}
        onValidateSupabase={handleValidateSupabase}
        runtimeStatus={runtimeStatus}
        visibleSections={
          runtimeStatus.runnerReachable ? ["supabase"] : ["supabase", "runner"]
        }
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
            <PanelLeftIcon size={15} />
          </button>
          <button
            className={`sidebar-toggle ${rightSidebarVisible ? "active" : ""}`}
            onClick={() => setRightSidebarVisible((value) => !value)}
            type="button"
            aria-pressed={rightSidebarVisible}
            title={rightSidebarVisible ? "Hide right sidebar" : "Show right sidebar"}
            aria-label={rightSidebarVisible ? "Hide right sidebar" : "Show right sidebar"}
          >
            <PanelRightIcon size={15} />
          </button>
          <button
            className={`sidebar-toggle ${terminalOpen ? "active" : ""}`}
            onClick={() => useStore.getState().toggleTerminal()}
            type="button"
            aria-pressed={terminalOpen}
            title="Toggle terminal (Ctrl+`)"
            aria-label="Toggle terminal panel"
          >
            <TerminalIcon size={15} />
          </button>
        </div>
        <div className="brand">
          FlowPilot <span className="brand-sub">desktop · {modeLabel}</span>
        </div>

        <div className="header-actions">
          <button
            type="button"
            className="icon-btn"
            aria-label="Sessions monitor"
            title="All runs across projects"
            onClick={() => setBoardOpen(true)}
          >
            <BoardIcon size={15} />
          </button>
          <AttentionInbox />
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

        <RunToast />
      {boardOpen && <SessionsBoard onClose={() => setBoardOpen(false)} />}
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
            onApplySupabaseMigrations={handleApplySupabaseMigrations}
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
          onApplySupabaseMigrations={handleApplySupabaseMigrations}
          onSaveSupabase={handleSaveSupabase}
          onSelectSection={setSettingsSection}
          onValidateSupabase={handleValidateSupabase}
          runtimeStatus={runtimeStatus}
        />
      )}
    </div>
  );
}
