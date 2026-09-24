import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useStore } from "@/state/store";
import type { RunHistoryItem } from "@/types/contract";
import { filterVisibleHistory, isAgentHistoryItem, isSyncableRun } from "@/components/navigatorHistory";
import { flattenGroupedHistory, groupRunsByChatId } from "../state/chatHistory";
import { CloseIcon, DisclosureCaret, GlobeIcon, PlusIcon } from "@/components/icons";
import { RemoteSyncPanel } from "@/components/RemoteSyncPanel";

const HISTORY_LIMIT = 5;

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
  waiting_user_approval: "Waiting · your approval",
  waiting_question: "Waiting · question",
  // BUG-231: a persisted RunHistoryItem's status is sourced from the Go
  // runner's own RunStatus enum, which has no "blocked" value (only the
  // separate, live-only AgentLoopState can be "blocked") — this key exists
  // purely to satisfy the exhaustive Record since RunHistoryItem shares the
  // RunStatus type, and should never actually be hit at runtime.
  blocked: "Waiting · your input",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

function runTitle(text?: string): string {
  if (!text) return "Untitled run";
  return text.length > 68 ? `${text.slice(0, 65)}...` : text;
}

function runTypeLabel(item: RunHistoryItem, statusOverride?: RunHistoryItem["status"]): string {
  return isAgentHistoryItem(item) ? `Agent · ${item.agentName || item.role || "sub-agent"}` : RUN_LABEL[statusOverride ?? item.status];
}

// Circular-arrow glyph for the manual refresh button.
function RefreshGlyph(): React.ReactElement {
  return (
    <svg width="11" height="11" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <path
        d="M2.5 6a3.5 3.5 0 0 1 6-2.475M9.5 6a3.5 3.5 0 0 1-6 2.475"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
      />
      <polyline points="8.4,2.9 8.6,4.9 6.6,5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <polyline points="3.6,9.1 3.4,7.1 5.4,7" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
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

/**
 * CP-85: the Navigator is a local-only surface — it renders each project's
 * local chat history and nothing else. No remote (Drive-synced) session list
 * is shown or fetched here; that whole flow lives behind the "Open Sync"
 * button, which mounts RemoteSyncPanel and runs the fetch + sync logic only
 * while that screen is open.
 */
export function Navigator(): React.ReactElement {
  const projects = useStore((s) => s.projects);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const status = useStore((s) => s.status);
  const runHistory = useStore((s) => s.runHistory);
  const historyLoading = useStore((s) => s.historyLoading);
  const historyLoadError = useStore((s) => s.historyLoadError);
  const loadProjects = useStore((s) => s.loadProjects);
  const selectProject = useStore((s) => s.selectProject);
  const runId = useStore((s) => s.runId);
  const loadRunHistory = useStore((s) => s.loadRunHistory);
  const openHistoryRun = useStore((s) => s.openHistoryRun);
  const loadProjectHistory = useStore((s) => s.loadProjectHistory);
  const loadAllProjectHistories = useStore((s) => s.loadAllProjectHistories);
  const projectHistoryById = useStore((s) => s.projectHistoryById);
  const attentionItems = useStore((s) => s.attentionItems);
  const resetRun = useStore((s) => s.resetRun);
  const deleteHistoryRun = useStore((s) => s.deleteHistoryRun);
  const visibleRunHistory = useMemo(() => filterVisibleHistory(runHistory), [runHistory]);
  const gateBlockedRunIds = useStore((s) => s._gateBlockedRunIds);

  const runIdRef = useRef(runId);
  runIdRef.current = runId;
  const prevHistoryRef = useRef<Map<string, RunHistoryItem["status"]>>(new Map());

  const [newlyCompleted, setNewlyCompleted] = useState<Set<string>>(new Set());
  const [recentProjectIds, setRecentProjectIds] = useState<string[]>([]);
  const [expandedHistoryIds, setExpandedHistoryIds] = useState<Set<string>>(new Set());
  const [collapsedProjectIds, setCollapsedProjectIds] = useState<Set<string>>(new Set());
  // Per-project collapse state for the History sub-section inside a group body.
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(new Set());
  const [selectionModeProjectId, setSelectionModeProjectId] = useState<string | null>(null);
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(new Set());
  const [confirmAction, setConfirmAction] = useState<{ runIds: string[]; projectId: string } | null>(null);
  const longPressRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [syncPanelOpen, setSyncPanelOpen] = useState(false);

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

  const requestDeleteConfirm = useCallback((runIds: string[], projectId: string) => {
    if (runIds.length === 0) return;
    setConfirmAction({ runIds, projectId });
  }, []);

  const executeConfirm = useCallback(async () => {
    if (!confirmAction) return;
    const { runIds } = confirmAction;
    setConfirmAction(null);
    exitSelectionMode();
    for (const runId of runIds) {
      try { await deleteHistoryRun(runId); } catch { /* best effort */ }
    }
  }, [confirmAction, deleteHistoryRun, exitSelectionMode]);

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  useEffect(() => {
    if (!selectedProjectId) return;
    void loadRunHistory();
  }, [loadRunHistory, selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId) return;
    const active =
      status === "running" ||
      status === "starting" ||
      status === "waiting_approval" ||
      status === "waiting_question";
    const ms = active ? 3_000 : 10_000;
    // CP-85: silent poll — keeps the list fresh without toggling
    // historyLoading, so the header no longer flashes "Loading" every tick.
    const id = setInterval(() => void loadRunHistory({ silent: true }), ms);
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

  // Warm every project's history once so group chat counts + the header
  // attention inbox are populated before the user visits each project; the
  // 30s refresh keeps waiting-status badges fresh without hammering the runner
  // (the selected project keeps its own faster 3s/10s poll via loadRunHistory).
  useEffect(() => {
    if (projects.length === 0) return;
    void loadAllProjectHistories();
    const id = setInterval(() => void loadAllProjectHistories(), 30_000);
    return () => clearInterval(id);
  }, [projects, loadAllProjectHistories]);

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

  // CP-59 Task-316 (DOD-5): one row per logical chat — provider-switch legs
  // collapse under the chat head (latest leg) with a leg-count chip.
  const activeHistory = useMemo(() => {
    const base = selectedProjectId ? (projectHistoryById[selectedProjectId] ?? []) : [];
    return flattenGroupedHistory(groupRunsByChatId(filterVisibleHistory(sortByRecent(base))));
  }, [selectedProjectId, projectHistoryById]);
  const showAllHistory = expandedHistoryIds.has(selectedProjectId ?? "");
  const visibleHistory = showAllHistory ? activeHistory : activeHistory.slice(0, HISTORY_LIMIT);
  // CP-85: local-flag-only hint for the Open Sync badge — no remote list is
  // fetched here, so this can over-count vs the Drive index; the sync panel
  // recomputes against the fresh remote list once opened.
  const unsyncedCount = activeHistory.filter((item) => isSyncableRun(item)).length;

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
    setCollapsedProjectIds((current) => {
      if (!current.has(projectId)) return current;
      const next = new Set(current);
      next.delete(projectId);
      return next;
    });
    void selectProject(projectId);
  };

  // "+ New chat" on a project group: selecting a different project already
  // resets the run via selectProject(); same-project needs an explicit reset.
  const newChatInProject = (projectId: string) => {
    if (projectId !== selectedProjectId) {
      selectProjectAndTrack(projectId);
    } else {
      resetRun();
    }
  };

  const toggleSection = (key: string) => {
    setCollapsedSections((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleProjectCollapsed = (projectId: string) => {
    setCollapsedProjectIds((current) => {
      const next = new Set(current);
      if (next.has(projectId)) next.delete(projectId);
      else next.add(projectId);
      return next;
    });
    // Expanding a group lazy-loads its history if the warm pass hasn't run yet.
    if (collapsedProjectIds.has(projectId) && !projectHistoryById[projectId]) {
      void loadProjectHistory(projectId);
    }
  };

  // Clicking a chat inside any project group auto-selects that project first
  // (chats always open inside their owning workspace), then replays the run.
  const openChatInProject = (projectId: string, item: RunHistoryItem) => {
    if (projectId !== selectedProjectId) {
      selectProjectAndTrack(projectId);
    }
    void openHistoryRun(item.runId, item);
  };

  return (
    <div className="navigator project-rail">
      <section className="project-rail-section">
        <div className="project-rail-head">
          <div>
            <label className="project-rail-head-label">
              Projects
              <span className="project-count-badge-inline">{projects.length}</span>
            </label>
          </div>
        </div>

        {orderedProjects.length === 0 ? (
          <div className="project-rail-empty">No projects loaded yet.</div>
        ) : (
          <div className="project-groups">
            {orderedProjects.map((project) => {
              const isActiveProject = project.id === selectedProjectId;
              const projectCollapsed = collapsedProjectIds.has(project.id);
              const groupHistory = isActiveProject
                ? activeHistory
                : flattenGroupedHistory(
                    groupRunsByChatId(filterVisibleHistory(sortByRecent(projectHistoryById[project.id] ?? []))),
                  );
              const chatCount = groupHistory.length;
              const waitingCount = attentionItems.filter((item) => item.projectId === project.id).length;
              const showAllGroup = expandedHistoryIds.has(project.id);
              const visibleGroupHistory = showAllGroup ? groupHistory : groupHistory.slice(0, HISTORY_LIMIT);
              return (
                <div key={project.id} className={`project-group${isActiveProject ? " project-group-active" : ""}`}>
                  <div className="project-group-head">
                    <button
                      type="button"
                      className="project-group-caret"
                      aria-expanded={!projectCollapsed}
                      aria-label={projectCollapsed ? `Expand ${project.name}` : `Collapse ${project.name}`}
                      title={projectCollapsed ? "Expand" : "Collapse"}
                      onClick={() => toggleProjectCollapsed(project.id)}
                    >
                      <DisclosureCaret open={!projectCollapsed} />
                    </button>
                    <button
                      type="button"
                      className="project-group-toggle"
                      aria-label={`Switch to ${project.name}`}
                      title={project.path}
                      onClick={() => selectProjectAndTrack(project.id)}
                    >
                      <span className="project-group-name">{project.name}</span>
                      {waitingCount > 0 && (
                        <span className="project-group-waiting" title={`${waitingCount} run${waitingCount > 1 ? "s" : ""} need attention`}>
                          {waitingCount}
                        </span>
                      )}
                      {chatCount > 0 && <span className="project-group-count">{chatCount}</span>}
                    </button>
                    <button
                      type="button"
                      className="project-group-new"
                      title={`New chat in ${project.name}`}
                      aria-label={`New chat in ${project.name}`}
                      onClick={(e) => { e.stopPropagation(); newChatInProject(project.id); }}
                    >
                      <PlusIcon size={11} />
                    </button>
                  </div>
                  {!projectCollapsed ? (
                    <div className="project-group-body">
                      <div className="project-group-path" title={project.path}>{project.path}</div>
                      {isActiveProject ? (
                        <>

      <section className="project-history-section">
        <div className="project-rail-head">
          <button
            type="button"
            className="project-section-toggle"
            aria-expanded={!collapsedSections.has(`${project.id}:history`)}
            onClick={() => toggleSection(`${project.id}:history`)}
          >
            <DisclosureCaret open={!collapsedSections.has(`${project.id}:history`)} />
            <span className="project-section-toggle-label">History</span>
            {activeHistory.length > 0 && (
              <span className="project-history-chat-count">{activeHistory.length} chats</span>
            )}
          </button>
          <div className="project-rail-head-actions">
            {selectedProjectId && (
              <button
                type="button"
                className="project-history-refresh"
                onClick={() => {
                  void loadRunHistory();
                }}
                disabled={historyLoading}
                title="Refresh history"
                aria-label="Refresh history"
              >
                {historyLoading ? <span className="history-status-spinner" aria-hidden="true" /> : <RefreshGlyph />}
              </button>
            )}
            {selectionModeProjectId === selectedProjectId && (
              <button
                type="button"
                className="project-history-selection-exit"
                onClick={exitSelectionMode}
                title="Exit selection mode"
                aria-label="Exit selection mode"
              >
                ←
              </button>
            )}
          </div>
        </div>

        {!collapsedSections.has(`${project.id}:history`) && (<>
        {historyLoadError && (
          <div className="project-history-error">
            <strong>History failed to load</strong>
            <span>{historyLoadError}</span>
          </div>
        )}

        {activeHistory.length === 0 ? (
          <div className="project-rail-empty">
            {historyLoading
              ? "Loading chats…"
              : selectedProjectId
                ? "Open a run to build project history."
                : "Select a project to load history."}
          </div>
        ) : (
          <div className="project-history-list">
            {selectionModeProjectId === selectedProjectId && (
              <div className="project-history-selection-toolbar">
                <span className="project-history-selection-count">
                  {selectedRunIds.size} selected
                </span>
                <button
                  type="button"
                  className="project-history-selection-action project-history-selection-action--delete"
                  disabled={selectedRunIds.size === 0}
                  onClick={() => requestDeleteConfirm([...selectedRunIds], selectedProjectId!)}
                  title="Delete selected chats"
                >
                  Delete
                </button>
                <button
                  type="button"
                  className="project-history-selection-action project-history-selection-action--delete"
                  disabled={activeHistory.length === 0}
                  onClick={() => requestDeleteConfirm(activeHistory.map((h) => h.runId), selectedProjectId!)}
                  title="Delete all chats"
                >
                  Delete All
                </button>
              </div>
            )}

            {visibleHistory.map((item) => {
              const isNew = newlyCompleted.has(item.runId);
              const isActive = item.runId === runId;
              // A gate-blocked run is treated as "completed" whether or not it is the
              // active chat. The runner keeps such a run in "running" state until the user
              // re-prompts, so both the polled runHistory ("running") AND the live store
              // status after reopening a blocked chat (openHistoryRun sets status from the
              // resumed handle = "running") would otherwise show a stuck spinner. The
              // gate-blocked set is cleared on the next turn_started, restoring the live
              // spinner for a genuine new turn. (CP-35 BUG-137)
              // Otherwise: the active chat trusts the live store status (the polled snapshot
              // can lag a just-completed turn); inactive chats use the polled status.
              const effectiveStatus = gateBlockedRunIds[item.runId]
                ? "completed"
                : (isActive ? status : item.status);
              const hasIcon = isNew || effectiveStatus === "running" || effectiveStatus === "starting" ||
                effectiveStatus === "waiting_approval" || effectiveStatus === "waiting_question";
              const isUnavailable = Boolean(item.unavailableReason);
              const isSyncing = item.syncStatus === "syncing";
              const inSelectionMode = selectionModeProjectId === selectedProjectId;
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
                          {item.legsCount && item.legsCount > 1 ? (
                            <span className="project-history-legs-count">{item.legsCount} legs</span>
                          ) : null}
                          {item.worktreeState ? (
                            <span
                              className="project-history-worktree-badge"
                              title={`Isolated worktree (${item.worktreeState})${item.worktreeSlug ? ` — ${item.worktreeSlug}` : ""}`}
                            >
                              ⎇ {item.worktreeSlug ?? "worktree"}
                            </span>
                          ) : null}
                        </span>
                      </span>
                      <span className="project-history-item-meta">
                        {runTypeLabel(item)} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                      </span>
                    </div>
                    <div className="project-history-item-actions">
                      <button
                        type="button"
                        className="project-history-delete-icon"
                        onClick={(e) => { e.stopPropagation(); requestDeleteConfirm([item.runId], selectedProjectId!); }}
                        title="Delete this chat"
                        aria-label="Delete chat"
                      >
                        <CloseIcon size={11} />
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
                    onPointerDown={() => startLongPress(item, selectedProjectId!)}
                    onPointerUp={cancelLongPress}
                    onPointerLeave={cancelLongPress}
                    onClick={() => {
                      if (isNew) setNewlyCompleted((c) => { const n = new Set(c); n.delete(item.runId); return n; });
                      void openHistoryRun(item.runId);
                    }}
                  >
                    <span className="project-history-item-top">
                      {showRowSpinner ? <span className="history-status-spinner" aria-hidden="true" /> : <HistoryStatusIcon status={effectiveStatus} isNew={isNew} />}
                      <span className="project-history-item-title">
                        {runTitle(item.lastPrompt || item.lastMessage)}
                        {item.worktreeState ? (
                          <span
                            className="project-history-worktree-badge"
                            title={`Isolated worktree (${item.worktreeState})${item.worktreeSlug ? ` — ${item.worktreeSlug}` : ""}`}
                          >
                            ⎇ {item.worktreeSlug ?? "worktree"}
                          </span>
                        ) : null}
                      </span>
                    </span>
                    <span className="project-history-item-meta">
                      {isSyncing ? "Syncing to Drive…" : runTypeLabel(item, isActive ? status : undefined)} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                    </span>
                  </button>
                </div>
              );
            })}

            {activeHistory.length > HISTORY_LIMIT && (
              <button
                type="button"
                className="project-history-more"
                onClick={() => toggleShowAllHistory(selectedProjectId!)}
              >
                {showAllHistory ? "Show less" : `Show all (${activeHistory.length - HISTORY_LIMIT} more)`}
              </button>
            )}

            <button
              type="button"
              className="project-history-open-sync"
              onClick={() => setSyncPanelOpen(true)}
              title="Open the sync screen — fetch remote (Drive) chats, restore them here, or upload unsynced local chats"
              aria-label="Open chat sync screen"
            >
              <GlobeIcon size={12} />
              <span>Open Sync</span>
              {unsyncedCount > 0 && (
                <span className="project-history-open-sync-badge" title={`${unsyncedCount} local chat${unsyncedCount > 1 ? "s" : ""} not synced to Drive`}>
                  {unsyncedCount}
                </span>
              )}
            </button>
          </div>
        )}
        </>)}
      </section>
                        </>
                      ) : (
                        // Inactive group peek — read-only chat list; clicking a
                        // row switches to that project and opens the run.
                        visibleGroupHistory.length === 0 ? (
                          <div className="project-rail-empty">
                            {projectHistoryById[project.id] ? "No chats yet." : "Loading…"}
                          </div>
                        ) : (
                          <div className="project-history-list">
                            {visibleGroupHistory.map((item) => (
                              <div
                                key={item.runId}
                                className={`project-history-item-row${item.unavailableReason ? " project-history-item-row--disabled" : ""}`}
                                title={item.unavailableReason || item.runId}
                              >
                                <button
                                  type="button"
                                  className={`project-history-item${item.unavailableReason ? " project-history-item--disabled" : ""}`}
                                  disabled={Boolean(item.unavailableReason)}
                                  onClick={() => openChatInProject(project.id, item)}
                                >
                                  <span className="project-history-item-top">
                                    <HistoryStatusIcon status={item.status} />
                                    <span className="project-history-item-title">
                                      {runTitle(item.lastPrompt || item.lastMessage)}
                                      {item.legsCount && item.legsCount > 1 ? (
                                        <span className="project-history-legs-count">{item.legsCount} legs</span>
                                      ) : null}
                                    </span>
                                  </span>
                                  <span className="project-history-item-meta">
                                    {runTypeLabel(item)} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                                  </span>
                                </button>
                              </div>
                            ))}
                            {groupHistory.length > HISTORY_LIMIT && (
                              <button
                                type="button"
                                className="project-history-more"
                                onClick={() => toggleShowAllHistory(project.id)}
                              >
                                {showAllGroup
                                  ? "Show less"
                                  : `Show all (${groupHistory.length - HISTORY_LIMIT} more)`}
                              </button>
                            )}
                          </div>
                        )
                      )}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        )}
      </section>

      {confirmAction && (
        <div
          className="project-history-confirm-overlay"
          onClick={() => setConfirmAction(null)}
          role="dialog"
          aria-modal="true"
          aria-label="Confirm delete"
        >
          <div className="project-history-confirm-modal" onClick={(e) => e.stopPropagation()}>
            <p className="project-history-confirm-message">
              {`Delete ${confirmAction.runIds.length} chat${confirmAction.runIds.length > 1 ? "s" : ""}? This also removes the provider session file.`}
            </p>
            <div className="project-history-confirm-actions">
              <button type="button" className="project-history-confirm-cancel" onClick={() => setConfirmAction(null)}>
                Cancel
              </button>
              <button
                type="button"
                className="project-history-confirm-ok project-history-confirm-ok--delete"
                onClick={() => void executeConfirm()}
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {syncPanelOpen && <RemoteSyncPanel onClose={() => setSyncPanelOpen(false)} />}
    </div>
  );
}
