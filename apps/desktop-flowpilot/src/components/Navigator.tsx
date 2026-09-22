import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useStore } from "@/state/store";
import type { RemoteChatSessionSummary, RunHistoryItem } from "@/types/contract";
import { filterVisibleHistory, isAgentHistoryItem, isProjectSyncing, isSyncableRun } from "@/components/navigatorHistory";
import { flattenGroupedHistory, groupRunsByChatId } from "../state/chatHistory";

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

// Circular-arrow glyph for manual refresh buttons.
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
  const syncRuns = useStore((s) => s.syncRuns);
  const syncAllInProject = useStore((s) => s.syncAllInProject);
  const syncBatchProgress = useStore((s) => s.syncBatchProgress);
  const deleteHistoryRun = useStore((s) => s.deleteHistoryRun);
  const restoreRemoteChatSession = useStore((s) => s.restoreRemoteChatSession);
  const visibleRunHistory = useMemo(() => filterVisibleHistory(runHistory), [runHistory]);
  const gateBlockedRunIds = useStore((s) => s._gateBlockedRunIds);

  const runIdRef = useRef(runId);
  runIdRef.current = runId;
  const prevHistoryRef = useRef<Map<string, RunHistoryItem["status"]>>(new Map());

  const [newlyCompleted, setNewlyCompleted] = useState<Set<string>>(new Set());
  const [recentProjectIds, setRecentProjectIds] = useState<string[]>([]);
  const [projectHistoryById, setProjectHistoryById] = useState<Record<string, RunHistoryItem[]>>({});
  const [remoteChatSessionsByProjectId, setRemoteChatSessionsByProjectId] = useState<Record<string, RemoteChatSessionSummary[]>>({});
  const [expandedHistoryIds, setExpandedHistoryIds] = useState<Set<string>>(new Set());
  const [selectionModeProjectId, setSelectionModeProjectId] = useState<string | null>(null);
  const [selectedRunIds, setSelectedRunIds] = useState<Set<string>>(new Set());
  const [confirmAction, setConfirmAction] = useState<{ type: "delete" | "sync"; runIds: string[]; projectId: string } | null>(null);
  const longPressRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [restoringIds, setRestoringIds] = useState<Set<string>>(new Set());
  const [restoringAll, setRestoringAll] = useState(false);
  const [restoreProgress, setRestoreProgress] = useState<{ done: number; total: number } | null>(null);
  const [showAllRemoteChats, setShowAllRemoteChats] = useState(false);
  const [remoteSelectionMode, setRemoteSelectionMode] = useState(false);
  const [selectedRemoteKeys, setSelectedRemoteKeys] = useState<Set<string>>(new Set());
  const remoteLongPressRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const exitSelectionMode = useCallback(() => {
    setSelectionModeProjectId(null);
    setSelectedRunIds(new Set());
  }, []);

  const exitRemoteSelectionMode = useCallback(() => {
    setRemoteSelectionMode(false);
    setSelectedRemoteKeys(new Set());
  }, []);

  const toggleRemoteSelection = useCallback((key: string) => {
    setSelectedRemoteKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const startRemoteLongPress = useCallback((key: string) => {
    remoteLongPressRef.current = setTimeout(() => {
      remoteLongPressRef.current = null;
      setRemoteSelectionMode(true);
      setSelectedRemoteKeys(new Set([key]));
    }, 500);
  }, []);

  const cancelRemoteLongPress = useCallback(() => {
    if (remoteLongPressRef.current) {
      clearTimeout(remoteLongPressRef.current);
      remoteLongPressRef.current = null;
    }
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
    // A "sync" click must never be a silent dead click: even when the selection
    // filters down to zero syncable items (e.g. all already synced/unavailable),
    // still open the dialog so its "No syncable chats selected..." branch
    // renders instead of nothing happening at all. "delete" has no such empty
    // state to explain, so it still bails.
    if (runIds.length === 0 && type !== "sync") return;
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
    const targets = remoteChatSessions.filter((item) => !item.unavailableReason);
    setRestoringAll(true);
    setRestoreProgress({ done: 0, total: targets.length });
    try {
      for (const item of targets) {
        const key = `${item.sourceMachineId}:${item.sourceRunId}`;
        setRestoringIds((prev) => new Set(prev).add(key));
        try {
          // Skip the per-item history refresh + auto-open here: doing a full
          // refresh after every single item serializes N extra round trips into
          // the loop (each item waits on the previous one's refresh before it can
          // even start), which is why the spinner looked stuck until the whole
          // batch finished. One combined refresh after the loop below is enough,
          // and auto-opening every restored run in turn would hijack the active
          // chat panel N times over.
          await restoreRemoteChatSession(item, undefined, { refresh: false, open: false });
        } finally {
          setRestoringIds((prev) => { const next = new Set(prev); next.delete(key); return next; });
          setRestoreProgress((prev) => (prev ? { ...prev, done: prev.done + 1 } : prev));
        }
      }
      await Promise.all([loadRunHistory(), loadRemoteChatSessions()]);
    } finally {
      setRestoringAll(false);
      setRestoreProgress(null);
    }
  }, [remoteChatSessions, restoreRemoteChatSession, loadRunHistory, loadRemoteChatSessions]);

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
      await syncRuns(runIds, projectId);
    }
  }, [confirmAction, deleteHistoryRun, exitSelectionMode, syncRuns]);

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
    if (!selectedProjectId) return;
    // Scope to the selected project's own items (rather than gating on
    // historyLoading) so a mid-switch fetch never writes a different project's
    // stale runHistory into this cache, while still letting same-project
    // optimistic updates (sync-up's per-item syncStatus, pull-down's restored
    // rows) flow through immediately instead of freezing until whatever
    // background poll happens to be in flight resolves. Blanket-gating on
    // historyLoading previously made a multi-chat sync/restore batch look
    // "stuck" the whole time it overlapped a poll, only updating once that
    // poll's own fetch finally settled.
    const scoped = visibleRunHistory.filter((item) => item.projectId === selectedProjectId);
    setProjectHistoryById((current) => ({
      ...current,
      [selectedProjectId]: sortByRecent(scoped),
    }));
  }, [selectedProjectId, visibleRunHistory]);

  // Cache the remote (Drive-synced) chat list per project so switching back to an
  // already-visited project shows its last-known list instantly instead of
  // resetting to empty/"Loading" every time -- loadRemoteChatSessions still runs
  // in the background on every switch to keep it fresh (stale-while-revalidate),
  // but the visible list no longer has to wait on that round trip each time.
  // Gated on remoteHistoryLoading (unlike the runHistory cache above) because,
  // unlike sync-up's per-item optimistic updates, there is no same-project
  // incremental local mutation of remoteChatSessions to protect -- it only ever
  // changes via a full fetch completing, so waiting for that fetch to settle
  // before caching is exactly what avoids writing a mid-switch project's stale
  // list under the new project's cache key.
  useEffect(() => {
    if (!selectedProjectId || remoteHistoryLoading) return;
    setRemoteChatSessionsByProjectId((current) => ({
      ...current,
      [selectedProjectId]: remoteChatSessions,
    }));
  }, [selectedProjectId, remoteHistoryLoading, remoteChatSessions]);

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
    return flattenGroupedHistory(groupRunsByChatId(base));
  }, [selectedProjectId, projectHistoryById]);
  const showAllHistory = expandedHistoryIds.has(selectedProjectId ?? "");
  const visibleHistory = showAllHistory ? activeHistory : activeHistory.slice(0, HISTORY_LIMIT);
  const activeRemoteChatSessions = selectedProjectId ? (remoteChatSessionsByProjectId[selectedProjectId] ?? []) : [];
  const unsyncedCount = activeHistory.filter((item) => isSyncableRun(item, activeRemoteChatSessions)).length;
  const activeSyncProgress =
    syncBatchProgress && syncBatchProgress.projectId === selectedProjectId ? syncBatchProgress : undefined;
  // Drive the chip off the batch itself (active continuously from the first item to the last),
  // not off isProjectSyncing's per-row "is some row syncing at this exact instant" snapshot --
  // that flips false in the gap between one item finishing and the next one starting (the row
  // is briefly "synced" while the next row hasn't flipped to "syncing" yet), which made the
  // spinner and x/y counter flicker away and reappear between every item instead of holding
  // steady until the whole batch reaches total/total. isProjectSyncing stays as a fallback for
  // any row-level sync state that did not originate from a syncRuns-driven batch.
  const projectSyncing = Boolean(activeSyncProgress) || isProjectSyncing(activeHistory, selectedProjectId ?? "");
  const syncProgressLabel = activeSyncProgress ? `${activeSyncProgress.done}/${activeSyncProgress.total}` : "Syncing…";

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
          <label className="project-rail-head-label">
            Active Project
            <span className="project-count-badge-inline">{projects.length}</span>
          </label>
        </div>

        {orderedProjects.length === 0 ? (
          <div className="project-rail-empty">No projects loaded yet.</div>
        ) : (
          <div className="project-selector">
            <select
              className="project-selector-select"
              value={selectedProjectId ?? ""}
              onChange={(e) => selectProjectAndTrack(e.target.value)}
              aria-label="Active project"
            >
              {orderedProjects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
            {selectedProjectId && (
              <span className="project-selector-path">
                {orderedProjects.find((p) => p.id === selectedProjectId)?.path}
              </span>
            )}
          </div>
        )}
      </section>

      <section className="project-history-section">
        <div className="project-rail-head">
          <div>
            <label>History</label>
            {activeHistory.length > 0 && (
              <p className="project-history-chat-count">{activeHistory.length} chats</p>
            )}
          </div>
          <div className="project-rail-head-actions">
            {historyLoading && <span className="project-rail-state">Loading</span>}
            {selectedProjectId && (
              <button
                type="button"
                className="project-history-refresh"
                onClick={() => {
                  void loadRunHistory();
                  void loadRemoteChatSessions();
                }}
                disabled={historyLoading}
                title="Refresh history"
                aria-label="Refresh history"
              >
                {historyLoading ? <span className="history-status-spinner" aria-hidden="true" /> : <RefreshGlyph />}
              </button>
            )}
            {selectionModeProjectId === selectedProjectId ? (
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
              unsyncedCount > 0 && selectedProjectId && (
                <button
                  type="button"
                  className="project-history-sync-all"
                  onClick={() => void syncAllInProject(selectedProjectId)}
                  disabled={projectSyncing}
                  title={
                    projectSyncing
                      ? `Syncing chats to Drive (${syncProgressLabel})`
                      : `Sync ${unsyncedCount} chat${unsyncedCount > 1 ? "s" : ""} to Drive`
                  }
                  aria-label={
                    projectSyncing
                      ? `Syncing chats to Drive, ${syncProgressLabel} done`
                      : `Sync all ${unsyncedCount} unsynced chats to Drive`
                  }
                >
                  {projectSyncing ? <span className="history-status-spinner" aria-hidden="true" /> : <SyncGlyph />}
                  <span>{projectSyncing ? syncProgressLabel : unsyncedCount}</span>
                </button>
              )
            )}
          </div>
        </div>

        {historyLoadError && (
          <div className="project-history-error">
            <strong>History failed to load</strong>
            <span>{historyLoadError}</span>
          </div>
        )}

        {activeHistory.length === 0 ? (
          <div className="project-rail-empty">
            {selectedProjectId ? "Open a run to build project history." : "Select a project to load history."}
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
                  className="project-history-selection-action"
                  disabled={selectedRunIds.size === 0}
                  onClick={() => {
                    const syncable = [...selectedRunIds].filter((id) => {
                      const it = activeHistory.find((h) => h.runId === id);
                      return it && isSyncableRun(it, activeRemoteChatSessions);
                    });
                    requestConfirm("sync", syncable, selectedProjectId!);
                  }}
                  title="Sync selected chats to Drive"
                >
                  <SyncGlyph /> Sync
                </button>
                <button
                  type="button"
                  className="project-history-selection-action project-history-selection-action--delete"
                  disabled={selectedRunIds.size === 0}
                  onClick={() => requestConfirm("delete", [...selectedRunIds], selectedProjectId!)}
                  title="Delete selected chats"
                >
                  Delete
                </button>
                <button
                  type="button"
                  className="project-history-selection-action project-history-selection-action--delete"
                  disabled={activeHistory.length === 0}
                  onClick={() => requestConfirm("delete", activeHistory.map((h) => h.runId), selectedProjectId!)}
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
              const showSync = isSyncableRun(item, activeRemoteChatSessions);
              const isSyncing = item.syncStatus === "syncing";
              const syncFailed = item.syncStatus === "failed";
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
                      {showSync && (
                        <button
                          type="button"
                          className={`project-history-sync-icon${syncFailed ? " project-history-sync-icon--failed" : ""}`}
                          disabled={isSyncing}
                          onClick={(e) => { e.stopPropagation(); requestConfirm("sync", [item.runId], selectedProjectId!); }}
                          title={syncFailed ? "Sync failed — click to retry" : "Sync this chat to Drive"}
                          aria-label="Sync chat to Drive"
                        >
                          {isSyncing ? <span className="history-status-spinner" aria-hidden="true" /> : <SyncGlyph />}
                        </button>
                      )}
                      <button
                        type="button"
                        className="project-history-delete-icon"
                        onClick={(e) => { e.stopPropagation(); requestConfirm("delete", [item.runId], selectedProjectId!); }}
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
          </div>
        )}
      </section>

      <section className="project-history-section project-history-section--remote">
        <div className="project-rail-head">
          <div>
            <label>Remote Chats</label>
            <p>Drive-backed chat sessions available to restore into this project.</p>
          </div>
          <div className="project-rail-head-actions">
            {remoteHistoryLoading && <span className="project-rail-state">Loading</span>}
            {remoteSelectionMode ? (
              <button
                type="button"
                className="project-history-selection-exit"
                onClick={exitRemoteSelectionMode}
                title="Exit selection mode"
                aria-label="Exit selection mode"
              >
                ←
              </button>
            ) : (
              activeRemoteChatSessions.filter((item) => !item.unavailableReason).length > 0 && (
                <button
                  type="button"
                  className="project-history-sync-all"
                  onClick={() => void restoreAll()}
                  disabled={restoringAll || restoringIds.size > 0}
                  title={restoringAll && restoreProgress ? `Restoring ${restoreProgress.done}/${restoreProgress.total}…` : "Restore all remote chats"}
                  aria-label={restoringAll && restoreProgress ? `Restoring remote chats, ${restoreProgress.done}/${restoreProgress.total} done` : "Restore all remote chats"}
                >
                  {restoringAll ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                  {restoringAll && restoreProgress && <span>{restoreProgress.done}/{restoreProgress.total}</span>}
                </button>
              )
            )}
          </div>
        </div>

        {remoteHistoryLoadError && (
          <div className="project-history-error">
            <strong>Remote history failed to load</strong>
            <span>{remoteHistoryLoadError}</span>
          </div>
        )}

        {activeRemoteChatSessions.length === 0 ? (
          <div className="project-rail-empty">
            {selectedProjectId ? "No remote chats have been synced for this project yet." : "Select a project to load remote chats."}
          </div>
        ) : (
          <div className="project-history-list">
            {remoteSelectionMode && (
              <div className="project-history-selection-toolbar">
                <span className="project-history-selection-count">
                  {selectedRemoteKeys.size} selected
                </span>
                <button
                  type="button"
                  className="project-history-selection-action"
                  disabled={restoringAll || restoringIds.size > 0}
                  onClick={() => void restoreAll()}
                  title="Restore all remote chats"
                >
                  {restoringAll ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                  {restoringAll && restoreProgress ? `Restoring ${restoreProgress.done}/${restoreProgress.total}` : "Restore All"}
                </button>
              </div>
            )}
            {(showAllRemoteChats ? activeRemoteChatSessions : activeRemoteChatSessions.slice(0, REMOTE_CHATS_LIMIT)).map((item) => {
              const key = `${item.sourceMachineId}:${item.sourceRunId}`;
              const isRestoring = restoringIds.has(key);
              const isSelected = selectedRemoteKeys.has(key);

              if (remoteSelectionMode) {
                return (
                  <div
                    key={key}
                    className={`project-history-item-row project-history-item-row--selectable${isSelected ? " project-history-item-row--selected" : ""}${item.unavailableReason ? " project-history-item-row--disabled" : ""}`}
                    onClick={() => { if (!item.unavailableReason) toggleRemoteSelection(key); }}
                  >
                    <input
                      type="checkbox"
                      className="project-history-item-checkbox"
                      checked={isSelected}
                      disabled={Boolean(item.unavailableReason)}
                      onChange={() => toggleRemoteSelection(key)}
                      onClick={(e) => e.stopPropagation()}
                      aria-label={`Select ${runTitle(item.lastPrompt || item.lastMessage)}`}
                    />
                    <div className="project-history-item project-history-item--selectable">
                      <span className="project-history-item-top">
                        <span className="project-history-item-title">
                          {runTitle(item.lastPrompt || item.lastMessage)}
                        </span>
                      </span>
                      <span className="project-history-item-meta">
                        {item.providerKey} · {item.updatedAt ? RUN_TIME_FORMAT.format(new Date(item.updatedAt)) : "Remote"}
                      </span>
                    </div>
                    {!item.unavailableReason && (
                      <button
                        type="button"
                        className="project-history-sync-icon"
                        disabled={isRestoring || restoringAll}
                        onClick={(e) => { e.stopPropagation(); void restoreItem(item); }}
                        title="Restore this chat"
                        aria-label="Restore chat"
                      >
                        {isRestoring ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                      </button>
                    )}
                  </div>
                );
              }

              return (
                <div key={key} className="project-history-item-row">
                  <button
                    type="button"
                    className={`project-history-item${item.unavailableReason ? " project-history-item--disabled" : ""}`}
                    disabled={Boolean(item.unavailableReason) || isRestoring}
                    onPointerDown={() => startRemoteLongPress(key)}
                    onPointerUp={cancelRemoteLongPress}
                    onPointerLeave={cancelRemoteLongPress}
                    onClick={() => { if (!item.unavailableReason && !isRestoring) void restoreItem(item); }}
                    title={item.unavailableReason || `${item.sourceMachineId}/${item.sourceRunId}`}
                  >
                    <span className="project-history-item-top">
                      {isRestoring && <span className="history-status-spinner" aria-hidden="true" />}
                      <span className="project-history-item-title">
                        {runTitle(item.lastPrompt || item.lastMessage)}
                      </span>
                    </span>
                    <span className="project-history-item-meta">
                      {item.providerKey} · {item.updatedAt ? RUN_TIME_FORMAT.format(new Date(item.updatedAt)) : "Remote"}
                    </span>
                  </button>
                </div>
              );
            })}
            {activeRemoteChatSessions.length > REMOTE_CHATS_LIMIT && (
              <button
                type="button"
                className="project-history-more"
                onClick={() => setShowAllRemoteChats((v) => !v)}
              >
                {showAllRemoteChats
                  ? "Show less"
                  : `Show all (${activeRemoteChatSessions.length - REMOTE_CHATS_LIMIT} more)`}
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
