import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useStore } from "@/state/store";
import type { RunHistoryItem } from "@/types/contract";
import { filterVisibleHistory, isAgentHistoryItem, isProjectSyncing } from "@/components/navigatorHistory";

const PROJECT_LIMIT = 3;
const HISTORY_LIMIT = 5;
const REMOTE_CHATS_LIMIT = 4;

const RUN_TIME_FORMAT = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

const RUN_LABEL: Record<RunHistoryItem["status"], string> = {
  idle: "Idle",
  starting: "Starting",
  running: "Running",
  waiting_approval: "Waiting · approval",
  waiting_question: "Waiting · question",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

function runTitle(text?: string): string {
  if (!text) return "Untitled run";
  return text.length > 68 ? `${text.slice(0, 65)}...` : text;
}

function isUnsyncedChat(item: RunHistoryItem): boolean {
  return item.runKind === "chat" && !isAgentHistoryItem(item) && item.syncStatus !== "synced" && !item.unavailableReason;
}

function runTypeLabel(item: RunHistoryItem): string {
  return isAgentHistoryItem(item) ? `Agent · ${item.agentName || item.role || "sub-agent"}` : RUN_LABEL[item.status];
}

// Upload-arrow glyph for per-chat and per-project sync buttons (sync = upload to Drive).
function SyncGlyph(): React.ReactElement {
  return (
    <svg width="11" height="11" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <polyline points="4,5.5 6,3 8,5.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="6" y1="3" x2="6" y2="9" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
      <line x1="2.5" y1="10.5" x2="9.5" y2="10.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  );
}

// Download-arrow glyph for individual restore buttons.
function RestoreGlyph(): React.ReactElement {
  return (
    <svg width="11" height="11" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <polyline points="4,6.5 6,9 8,6.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="6" y1="9" x2="6" y2="3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
      <line x1="2.5" y1="10.5" x2="9.5" y2="10.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  );
}

function HistoryStatusIcon({ status, isNew }: { status: RunHistoryItem["status"]; isNew?: boolean }): React.ReactElement | null {
  if (isNew) {
    return (
      <svg className="history-status-icon history-status-icon--complete" width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
        <polyline points="1.5,5.5 3.8,7.8 8.5,2.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    );
  }
  if (status === "running" || status === "starting") {
    return <span className="history-status-spinner" aria-hidden="true" />;
  }
  if (status === "waiting_approval") {
    return (
      <svg className="history-status-icon history-status-icon--approval" width="9" height="11" viewBox="0 0 9 11" fill="currentColor" aria-hidden="true">
        <path d="M4.5 0L0 2v4c0 2.4 2 4 4.5 5C7 10 9 8.4 9 6V2L4.5 0z" />
      </svg>
    );
  }
  if (status === "waiting_question") {
    return (
      <svg className="history-status-icon history-status-icon--question" width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
        <circle cx="5" cy="5" r="4.2" stroke="currentColor" strokeWidth="1.2" />
        <path d="M3.5 3.8C3.5 2.6 6.5 2.6 6.5 4.2c0 .9-.8 1.2-1.5 1.8v.5" stroke="currentColor" strokeWidth="1.1" strokeLinecap="round" />
        <circle cx="5" cy="7.5" r=".5" fill="currentColor" />
      </svg>
    );
  }
  return null;
}

function sortByRecent(items: RunHistoryItem[]): RunHistoryItem[] {
  return [...items].sort((a, b) => {
    const delta = new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime();
    if (delta !== 0) return delta;
    return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
  });
}

function projectLabel(projectId: string, projects: { id: string; name: string; path: string }[]): string {
  return projects.find((project) => project.id === projectId)?.name ?? projectId;
}

export function Navigator(): React.ReactElement {
  const projects = useStore((s) => s.projects);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const status = useStore((s) => s.status);
  const runHistory = useStore((s) => s.runHistory);
  const historyLoading = useStore((s) => s.historyLoading);
  const historyLoadError = useStore((s) => s.historyLoadError);
  const remoteChatSessions = useStore((s) => s.remoteChatSessions);
  const remoteHistoryLoading = useStore((s) => s.remoteHistoryLoading);
  const remoteHistoryLoadError = useStore((s) => s.remoteHistoryLoadError);
  const loadProjects = useStore((s) => s.loadProjects);
  const selectProject = useStore((s) => s.selectProject);
  const runId = useStore((s) => s.runId);
  const loadRunHistory = useStore((s) => s.loadRunHistory);
  const loadRemoteChatSessions = useStore((s) => s.loadRemoteChatSessions);
  const openHistoryRun = useStore((s) => s.openHistoryRun);
  const syncHistoryRun = useStore((s) => s.syncHistoryRun);
  const syncAllInProject = useStore((s) => s.syncAllInProject);
  const deleteHistoryRun = useStore((s) => s.deleteHistoryRun);
  const restoreRemoteChatSession = useStore((s) => s.restoreRemoteChatSession);
  const visibleRunHistory = useMemo(() => filterVisibleHistory(runHistory), [runHistory]);

  const runIdRef = useRef(runId);
  runIdRef.current = runId;
  const prevHistoryRef = useRef<Map<string, RunHistoryItem["status"]>>(new Map());

  const [showAllProjects, setShowAllProjects] = useState(false);
  const [newlyCompleted, setNewlyCompleted] = useState<Set<string>>(new Set());
  const [recentProjectIds, setRecentProjectIds] = useState<string[]>([]);
  const [projectHistoryById, setProjectHistoryById] = useState<Record<string, RunHistoryItem[]>>({});
  const [openProjectIds, setOpenProjectIds] = useState<Record<string, boolean>>({});
  const [expandedHistoryIds, setExpandedHistoryIds] = useState<Set<string>>(new Set());
  const [selectionModeProjectId, setSelectionModeProjectId] = useState<string | null>(null);
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(new Set());
  const [confirmAction, setConfirmAction] = useState<{ type: "delete" | "sync"; runIds: string[]; projectId: string } | null>(null);
  const longPressRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [restoringIds, setRestoringIds] = useState<Set<string>>(new Set());
  const [restoringAll, setRestoringAll] = useState(false);
  const [showAllRemoteChats, setShowAllRemoteChats] = useState(false);

  const exitSelectionMode = useCallback(() => {
    setSelectionModeProjectId(null);
    setSelectedRunIds(new Set());
  }, []);

  const toggleItemSelection = useCallback((runId: string) => {
    setSelectedRunIds((prev) => {
      const next = new Set(prev);
      if (next.has(runId)) next.delete(runId);
      else next.add(runId);
      return next;
    });
  }, []);

  const startLongPress = useCallback((item: RunHistoryItem, projectId: string) => {
    longPressRef.current = setTimeout(() => {
      longPressRef.current = null;
      setSelectionModeProjectId(projectId);
      setSelectedRunIds(new Set([item.runId]));
    }, 500);
  }, []);

  const cancelLongPress = useCallback(() => {
    if (longPressRef.current) {
      clearTimeout(longPressRef.current);
      longPressRef.current = null;
    }
  }, []);

  const requestConfirm = useCallback((type: "delete" | "sync", runIds: string[], projectId: string) => {
    if (runIds.length === 0) return;
    setConfirmAction({ type, runIds, projectId });
  }, []);

  const restoreItem = useCallback(async (item: Parameters<typeof restoreRemoteChatSession>[0]) => {
    const key = `${item.sourceMachineId}:${item.sourceRunId}`;
    setRestoringIds((prev) => new Set(prev).add(key));
    try {
      await restoreRemoteChatSession(item);
    } finally {
      setRestoringIds((prev) => { const next = new Set(prev); next.delete(key); return next; });
    }
  }, [restoreRemoteChatSession]);

  const restoreAll = useCallback(async () => {
    setRestoringAll(true);
    try {
      for (const item of remoteChatSessions) {
        if (!item.unavailableReason) {
          const key = `${item.sourceMachineId}:${item.sourceRunId}`;
          setRestoringIds((prev) => new Set(prev).add(key));
          try {
            await restoreRemoteChatSession(item);
          } finally {
            setRestoringIds((prev) => { const next = new Set(prev); next.delete(key); return next; });
          }
        }
      }
    } finally {
      setRestoringAll(false);
    }
  }, [remoteChatSessions, restoreRemoteChatSession]);

  const executeConfirm = useCallback(async () => {
    if (!confirmAction) return;
    const { type, runIds, projectId } = confirmAction;
    setConfirmAction(null);
    exitSelectionMode();
    if (type === "delete") {
      for (const runId of runIds) {
        try { await deleteHistoryRun(runId); } catch { /* best effort */ }
      }
    } else {
      for (const runId of runIds) {
        try { await syncHistoryRun(runId, projectId); } catch { /* best effort */ }
      }
    }
  }, [confirmAction, deleteHistoryRun, exitSelectionMode, syncHistoryRun]);

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  useEffect(() => {
    if (!selectedProjectId) return;
    void loadRunHistory();
    void loadRemoteChatSessions();
  }, [loadRemoteChatSessions, loadRunHistory, selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId) return;
    const active =
      status === "running" ||
      status === "starting" ||
      status === "waiting_approval" ||
      status === "waiting_question";
    const ms = active ? 3_000 : 10_000;
    const id = setInterval(() => void loadRunHistory(), ms);
    return () => clearInterval(id);
  }, [selectedProjectId, status, loadRunHistory]);

  useEffect(() => {
    if (!selectedProjectId || historyLoading) return;
    const prev = prevHistoryRef.current;
    const currentRunId = runIdRef.current;
    const toAdd: string[] = [];
    for (const item of visibleRunHistory) {
      const prevStatus = prev.get(item.runId);
      if (
        prevStatus !== undefined &&
        prevStatus !== "completed" &&
        item.status === "completed" &&
        item.runId !== currentRunId
      ) {
        toAdd.push(item.runId);
      }
      prev.set(item.runId, item.status);
    }
    if (toAdd.length > 0) {
      setNewlyCompleted((current) => {
        const next = new Set(current);
        for (const id of toAdd) next.add(id);
        return next;
      });
    }
  }, [selectedProjectId, historyLoading, visibleRunHistory]);

  useEffect(() => {
    if (!selectedProjectId || historyLoading) return;
    setProjectHistoryById((current) => ({
      ...current,
      [selectedProjectId]: sortByRecent(visibleRunHistory),
    }));
    if (visibleRunHistory.length > 0) {
      setOpenProjectIds((current) => (current[selectedProjectId] ? current : { ...current, [selectedProjectId]: true }));
    }
  }, [historyLoading, selectedProjectId, visibleRunHistory]);

  useEffect(() => {
    setSelectionModeProjectId(null);
    setSelectedRunIds(new Set());
  }, [selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId) return;
    setRecentProjectIds((current) => {
      const next = [selectedProjectId, ...current.filter((id) => id !== selectedProjectId)];
      return next.slice(0, 8);
    });
  }, [selectedProjectId]);

  const orderedProjects = useMemo(() => {
    const recent = recentProjectIds.filter((projectId) => projects.some((project) => project.id === projectId));
    const remaining = projects
      .map((project) => project.id)
      .filter((projectId) => !recent.includes(projectId));
    return [...recent, ...remaining]
      .map((projectId) => projects.find((project) => project.id === projectId))
      .filter((project): project is (typeof projects)[number] => Boolean(project));
  }, [projects, recentProjectIds]);

  const visibleProjects = showAllProjects ? orderedProjects : orderedProjects.slice(0, PROJECT_LIMIT);
  const canShowMoreProjects = orderedProjects.length > PROJECT_LIMIT;

  const historyGroups = useMemo(() => {
    return Object.entries(projectHistoryById)
      .map(([projectId, history]) => ({ projectId, history }))
      .filter((group) => group.history.length > 0)
      .sort((a, b) => {
        const aTime = new Date(a.history[0]?.updatedAt ?? 0).getTime();
        const bTime = new Date(b.history[0]?.updatedAt ?? 0).getTime();
        return bTime - aTime;
      });
  }, [projectHistoryById]);

  const toggleProjectHistory = (projectId: string) => {
    setOpenProjectIds((current) => ({ ...current, [projectId]: !current[projectId] }));
  };

  const toggleShowAllHistory = (projectId: string) => {
    setExpandedHistoryIds((current) => {
      const next = new Set(current);
      next.has(projectId) ? next.delete(projectId) : next.add(projectId);
      return next;
    });
  };

  const selectProjectAndTrack = (projectId: string) => {
    setRecentProjectIds((current) => {
      const next = [projectId, ...current.filter((id) => id !== projectId)];
      return next.slice(0, 8);
    });
    void selectProject(projectId);
  };

  return (
    <div className="navigator project-rail">
      <section className="project-rail-section">
        <div className="project-rail-head">
          <div>
            <label>Projects</label>
            <p>Recent workspaces first, with the active project highlighted.</p>
          </div>
          <span className="project-count-badge">{projects.length}</span>
        </div>

        <div className="project-rail-list">
          {visibleProjects.length === 0 ? (
            <div className="project-rail-empty">No projects loaded yet.</div>
          ) : (
            visibleProjects.map((project) => {
              const active = project.id === selectedProjectId;
              const recentIndex = recentProjectIds.indexOf(project.id);
              return (
                <button
                  key={project.id}
                  type="button"
                  className={`project-rail-item ${active ? "active" : ""}`}
                  onClick={() => selectProjectAndTrack(project.id)}
                >
                  <span className="project-rail-item-top">
                    <strong>{project.name}</strong>
                    {active && <span className="project-rail-active">Active</span>}
                    {!active && recentIndex >= 0 && <span className="project-rail-recent">Recent</span>}
                  </span>
                  <span className="project-rail-path">{project.path}</span>
                </button>
              );
            })
          )}
        </div>

        {canShowMoreProjects && (
          <button
            type="button"
            className="project-rail-more"
            onClick={() => setShowAllProjects((value) => !value)}
          >
            {showAllProjects ? "Show fewer projects" : "Show more projects"}
          </button>
        )}
      </section>

      <section className="project-history-section">
        <div className="project-rail-head">
          <div>
            <label>History</label>
            <p>Grouped by project and sorted by latest activity.</p>
          </div>
          {historyLoading && <span className="project-rail-state">Loading</span>}
        </div>

        {historyLoadError && (
          <div className="project-history-error">
            <strong>History failed to load</strong>
            <span>{historyLoadError}</span>
          </div>
        )}

        {historyGroups.length === 0 ? (
          <div className="project-rail-empty">
            {selectedProjectId ? "Open a run to build project history." : "Select a project to load history."}
          </div>
        ) : (
          <div className="project-history-groups">
            {historyGroups.map(({ projectId, history }) => {
              const projectName = projectLabel(projectId, projects);
              const expanded = openProjectIds[projectId] ?? projectId === selectedProjectId;
              const showAllHistory = expandedHistoryIds.has(projectId);
              const visibleHistory = expanded ? (showAllHistory ? history : history.slice(0, HISTORY_LIMIT)) : [];
              const unsyncedCount = history.filter(isUnsyncedChat).length;
              const projectSyncing = isProjectSyncing(history, projectId);
              return (
                <section key={projectId} className="project-history-group">
                  <div className={`project-history-group-head ${expanded ? "active" : ""}${selectionModeProjectId === projectId ? " project-history-group-head--selecting" : ""}`}>
                    <button
                      type="button"
                      className="project-history-group-toggle"
                      onClick={() => { if (selectionModeProjectId !== projectId) toggleProjectHistory(projectId); }}
                    >
                      <span className="project-history-group-title">
                        <strong>{projectName}</strong>
                        <small>{history.length} chats</small>
                      </span>
                      {selectionModeProjectId !== projectId && (
                        <span className="project-history-group-chevron">{expanded ? "▾" : "▸"}</span>
                      )}
                    </button>
                    {selectionModeProjectId === projectId ? (
                      <button
                        type="button"
                        className="project-history-selection-exit"
                        onClick={exitSelectionMode}
                        title="Exit selection mode"
                        aria-label="Exit selection mode"
                      >
                        ←
                      </button>
                    ) : (
                      unsyncedCount > 0 && (
                        <button
                          type="button"
                          className="project-history-sync-all"
                          onClick={() => void syncAllInProject(projectId)}
                          disabled={projectSyncing}
                          title={
                            projectSyncing
                              ? `Syncing ${unsyncedCount} chat${unsyncedCount > 1 ? "s" : ""} to Drive`
                              : `Sync ${unsyncedCount} chat${unsyncedCount > 1 ? "s" : ""} to Drive`
                          }
                          aria-label={
                            projectSyncing
                              ? `Syncing all ${unsyncedCount} unsynced chats to Drive`
                              : `Sync all ${unsyncedCount} unsynced chats to Drive`
                          }
                        >
                          {projectSyncing ? <span className="history-status-spinner" aria-hidden="true" /> : <SyncGlyph />}
                          <span>{projectSyncing ? "Syncing…" : unsyncedCount}</span>
                        </button>
                      )
                    )}
                  </div>

                  {expanded && (
                    <div className="project-history-list">
                      {selectionModeProjectId === projectId && (
                        <div className="project-history-selection-toolbar">
                          <span className="project-history-selection-count">
                            {selectedRunIds.size} selected
                          </span>
                          <button
                            type="button"
                            className="project-history-selection-action"
                            disabled={selectedRunIds.size === 0}
                            onClick={() => {
                              const syncable = [...selectedRunIds].filter((id) => {
                                const it = history.find((h) => h.runId === id);
                                return it && isUnsyncedChat(it);
                              });
                              requestConfirm("sync", syncable, projectId);
                            }}
                            title="Sync selected chats to Drive"
                          >
                            <SyncGlyph /> Sync
                          </button>
                          <button
                            type="button"
                            className="project-history-selection-action project-history-selection-action--delete"
                            disabled={selectedRunIds.size === 0}
                            onClick={() => requestConfirm("delete", [...selectedRunIds], projectId)}
                            title="Delete selected chats"
                          >
                            Delete
                          </button>
                        </div>
                      )}

                      {visibleHistory.map((item) => {
                        const isNew = newlyCompleted.has(item.runId);
                        const hasIcon = isNew || item.status === "running" || item.status === "starting" ||
                          item.status === "waiting_approval" || item.status === "waiting_question";
                        const isUnavailable = Boolean(item.unavailableReason);
                        const isActive = item.runId === runId;
                        const showSync = isUnsyncedChat(item);
                        const isSyncing = item.syncStatus === "syncing";
                        const syncFailed = item.syncStatus === "failed";
                        const inSelectionMode = selectionModeProjectId === projectId;
                        const showRowSpinner = isSyncing && !inSelectionMode;
                        const isSelected = selectedRunIds.has(item.runId);

                        if (inSelectionMode) {
                          return (
                            <div
                              key={item.runId}
                              className={`project-history-item-row project-history-item-row--selectable${isSelected ? " project-history-item-row--selected" : ""}`}
                              onClick={() => toggleItemSelection(item.runId)}
                            >
                              <input
                                type="checkbox"
                                className="project-history-item-checkbox"
                                checked={isSelected}
                                onChange={() => toggleItemSelection(item.runId)}
                                onClick={(e) => e.stopPropagation()}
                                aria-label={`Select ${runTitle(item.lastPrompt || item.lastMessage)}`}
                              />
                              <div className="project-history-item project-history-item--selectable">
                                <span className="project-history-item-top">
                                  <HistoryStatusIcon status={item.status} isNew={isNew} />
                                  <span className="project-history-item-title">
                                    {runTitle(item.lastPrompt || item.lastMessage)}
                                  </span>
                                </span>
                                <span className="project-history-item-meta">
                                  {runTypeLabel(item)} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                                </span>
                              </div>
                              <div className="project-history-item-actions">
                                {showSync && (
                                  <button
                                    type="button"
                                    className={`project-history-sync-icon${syncFailed ? " project-history-sync-icon--failed" : ""}`}
                                    disabled={isSyncing}
                                    onClick={(e) => { e.stopPropagation(); requestConfirm("sync", [item.runId], projectId); }}
                                    title={syncFailed ? "Sync failed — click to retry" : "Sync this chat to Drive"}
                                    aria-label="Sync chat to Drive"
                                  >
                                    {isSyncing ? <span className="history-status-spinner" aria-hidden="true" /> : <SyncGlyph />}
                                  </button>
                                )}
                                <button
                                  type="button"
                                  className="project-history-delete-icon"
                                  onClick={(e) => { e.stopPropagation(); requestConfirm("delete", [item.runId], projectId); }}
                                  title="Delete this chat"
                                  aria-label="Delete chat"
                                >
                                  ×
                                </button>
                              </div>
                            </div>
                          );
                        }

                        return (
                          <div
                            key={item.runId}
                            className={`project-history-item-row${isUnavailable ? " project-history-item-row--disabled" : ""}`}
                            title={item.unavailableReason || item.runId}
                          >
                            <button
                              type="button"
                              className={`project-history-item${hasIcon ? " project-history-item--has-icon" : ""}${isUnavailable ? " project-history-item--disabled" : ""}${isActive ? " project-history-item--active" : ""}`}
                              disabled={isUnavailable}
                              aria-current={isActive ? "true" : undefined}
                              onPointerDown={() => startLongPress(item, projectId)}
                              onPointerUp={cancelLongPress}
                              onPointerLeave={cancelLongPress}
                              onClick={() => {
                                if (isNew) setNewlyCompleted((c) => { const n = new Set(c); n.delete(item.runId); return n; });
                                void openHistoryRun(item.runId);
                              }}
                            >
                              <span className="project-history-item-top">
                                {showRowSpinner ? <span className="history-status-spinner" aria-hidden="true" /> : <HistoryStatusIcon status={item.status} isNew={isNew} />}
                                <span className="project-history-item-title">
                                  {runTitle(item.lastPrompt || item.lastMessage)}
                                </span>
                              </span>
                              <span className="project-history-item-meta">
                                {isSyncing ? "Syncing to Drive…" : runTypeLabel(item)} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                              </span>
                            </button>
                          </div>
                        );
                      })}

                      {history.length > HISTORY_LIMIT && (
                        <button
                          type="button"
                          className="project-history-more"
                          onClick={() => toggleShowAllHistory(projectId)}
                        >
                          {showAllHistory ? "Show less" : `Show all (${history.length - HISTORY_LIMIT} more)`}
                        </button>
                      )}
                    </div>
                  )}
                </section>
              );
            })}
          </div>
        )}
      </section>

      <section className="project-history-section">
        <div className="project-rail-head">
          <div>
            <label>Remote Chats</label>
            <p>Drive-backed chat sessions available to restore into this project.</p>
          </div>
          <div className="project-rail-head-actions">
            {remoteHistoryLoading && <span className="project-rail-state">Loading</span>}
            {remoteChatSessions.filter((item) => !item.unavailableReason).length > 0 && (
              <button
                type="button"
                className="project-history-sync-all"
                onClick={() => void restoreAll()}
                disabled={restoringAll || restoringIds.size > 0}
                title="Restore all remote chats into this project"
                aria-label="Restore all remote chats"
              >
                {restoringAll ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                <span>{restoringAll ? "Restoring…" : "Restore All"}</span>
              </button>
            )}
          </div>
        </div>

        {remoteHistoryLoadError && (
          <div className="project-history-error">
            <strong>Remote history failed to load</strong>
            <span>{remoteHistoryLoadError}</span>
          </div>
        )}

        {remoteChatSessions.length === 0 ? (
          <div className="project-rail-empty">
            {selectedProjectId ? "No remote chats have been synced for this project yet." : "Select a project to load remote chats."}
          </div>
        ) : (
          <div className="project-history-list">
            {(showAllRemoteChats ? remoteChatSessions : remoteChatSessions.slice(0, REMOTE_CHATS_LIMIT)).map((item) => {
              const key = `${item.sourceMachineId}:${item.sourceRunId}`;
              const isRestoring = restoringIds.has(key);
              return (
                <div key={key} className="project-history-item-row">
                  <button
                    type="button"
                    className={`project-history-item${item.unavailableReason ? " project-history-item--disabled" : ""}`}
                    disabled={Boolean(item.unavailableReason) || isRestoring}
                    onClick={() => void restoreItem(item)}
                    title={item.unavailableReason || `${item.sourceMachineId}/${item.sourceRunId}`}
                  >
                    <span className="project-history-item-top">
                      <span className="project-history-item-title">
                        {runTitle(item.lastPrompt || item.lastMessage)}
                      </span>
                    </span>
                    <span className="project-history-item-meta">
                      {item.providerKey} · {item.updatedAt ? RUN_TIME_FORMAT.format(new Date(item.updatedAt)) : "Remote"}
                    </span>
                  </button>
                  {!item.unavailableReason && (
                    <button
                      type="button"
                      className="project-history-sync"
                      disabled={isRestoring || restoringAll}
                      onClick={() => void restoreItem(item)}
                      title="Restore this chat"
                      aria-label="Restore chat"
                    >
                      {isRestoring ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                    </button>
                  )}
                </div>
              );
            })}
            {remoteChatSessions.length > REMOTE_CHATS_LIMIT && (
              <button
                type="button"
                className="project-history-more"
                onClick={() => setShowAllRemoteChats((v) => !v)}
              >
                {showAllRemoteChats
                  ? "Show less"
                  : `Show all (${remoteChatSessions.length - REMOTE_CHATS_LIMIT} more)`}
              </button>
            )}
          </div>
        )}
      </section>

      {confirmAction && (
        <div
          className="project-history-confirm-overlay"
          onClick={() => setConfirmAction(null)}
          role="dialog"
          aria-modal="true"
          aria-label={confirmAction.type === "delete" ? "Confirm delete" : "Confirm sync"}
        >
          <div className="project-history-confirm-modal" onClick={(e) => e.stopPropagation()}>
            <p className="project-history-confirm-message">
              {confirmAction.type === "delete"
                ? `Delete ${confirmAction.runIds.length} chat${confirmAction.runIds.length > 1 ? "s" : ""}? This also removes the provider session file.`
                : confirmAction.runIds.length === 0
                  ? "No syncable chats selected (all may already be synced or unavailable)."
                  : `Sync ${confirmAction.runIds.length} chat${confirmAction.runIds.length > 1 ? "s" : ""} to Drive?`
              }
            </p>
            <div className="project-history-confirm-actions">
              <button type="button" className="project-history-confirm-cancel" onClick={() => setConfirmAction(null)}>
                Cancel
              </button>
              {(confirmAction.type === "delete" || confirmAction.runIds.length > 0) && (
                <button
                  type="button"
                  className={`project-history-confirm-ok${confirmAction.type === "delete" ? " project-history-confirm-ok--delete" : ""}`}
                  onClick={() => void executeConfirm()}
                >
                  {confirmAction.type === "delete" ? "Delete" : "Sync"}
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
