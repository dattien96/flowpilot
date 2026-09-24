import { useCallback, useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import type { RemoteChatSessionSummary, RunHistoryItem } from "@/types/contract";
import { filterVisibleHistory, isSyncableRun } from "@/components/navigatorHistory";
import { flattenGroupedHistory, groupRunsByChatId } from "@/state/chatHistory";
import { CloseIcon } from "@/components/icons";

const REMOTE_LIST_LIMIT = 8;

const RUN_TIME_FORMAT = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function runTitle(text?: string): string {
  if (!text) return "Untitled run";
  return text.length > 68 ? `${text.slice(0, 65)}...` : text;
}

// Upload-arrow glyph (sync local chat up to Drive).
function SyncGlyph(): React.ReactElement {
  return (
    <svg width="11" height="11" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <polyline points="4,5.5 6,3 8,5.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="6" y1="3" x2="6" y2="9" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
      <line x1="2.5" y1="10.5" x2="9.5" y2="10.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  );
}

// Download-arrow glyph (restore a remote chat down to this machine).
function RestoreGlyph(): React.ReactElement {
  return (
    <svg width="11" height="11" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <polyline points="4,6.5 6,9 8,6.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="6" y1="9" x2="6" y2="3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
      <line x1="2.5" y1="10.5" x2="9.5" y2="10.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  );
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

function remoteKey(item: RemoteChatSessionSummary): string {
  return `${item.sourceMachineId}:${item.sourceRunId}`;
}

/**
 * CP-85: the on-demand sync screen. The Navigator no longer shows or fetches
 * remote (Drive-synced) chat sessions — everything remote lives behind this
 * overlay, opened via the History section's "Open Sync" button. Mounting the
 * panel is what triggers loadRemoteChatSessions; nothing remote runs while
 * the panel is closed.
 *
 * Two directions, one screen:
 *  - Download: remote session list with per-chat restore (click = restore +
 *    open) and a batch "Restore all" with x/y progress.
 *  - Upload: local chats whose sync flag isn't "synced" yet, with per-chat
 *    sync (confirmed) and "Sync all" driving the shared syncBatchProgress.
 */
export function RemoteSyncPanel({ onClose }: { onClose: () => void }): React.ReactElement {
  const projects = useStore((s) => s.projects);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const projectHistoryById = useStore((s) => s.projectHistoryById);
  const remoteChatSessions = useStore((s) => s.remoteChatSessions);
  const remoteHistoryLoading = useStore((s) => s.remoteHistoryLoading);
  const remoteHistoryLoadError = useStore((s) => s.remoteHistoryLoadError);
  const loadRemoteChatSessions = useStore((s) => s.loadRemoteChatSessions);
  const restoreRemoteChatSession = useStore((s) => s.restoreRemoteChatSession);
  const loadRunHistory = useStore((s) => s.loadRunHistory);
  const syncRuns = useStore((s) => s.syncRuns);
  const syncAllInProject = useStore((s) => s.syncAllInProject);
  const syncBatchProgress = useStore((s) => s.syncBatchProgress);

  const [restoringIds, setRestoringIds] = useState<Set<string>>(new Set());
  const [restoringAll, setRestoringAll] = useState(false);
  const [restoreProgress, setRestoreProgress] = useState<{ done: number; total: number } | null>(null);
  const [showAllRemote, setShowAllRemote] = useState(false);
  const [confirmSync, setConfirmSync] = useState<{ runIds: string[] } | null>(null);

  const project = projects.find((p) => p.id === selectedProjectId);

  // Opening the panel (or switching project while open) is the only trigger
  // for the remote fetch — CP-85 P-2.
  useEffect(() => {
    if (!selectedProjectId) return;
    void loadRemoteChatSessions();
  }, [loadRemoteChatSessions, selectedProjectId]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const localHistory = useMemo(() => {
    const base = selectedProjectId ? (projectHistoryById[selectedProjectId] ?? []) : [];
    return flattenGroupedHistory(
      groupRunsByChatId(
        filterVisibleHistory(
          [...base].sort((a, b) => {
            const delta = new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime();
            return delta !== 0 ? delta : new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime();
          }),
        ),
      ),
    );
  }, [selectedProjectId, projectHistoryById]);

  // Upload candidates: reconciled against the freshly fetched remote list so a
  // chat already in Drive never shows a stale "needs sync" row.
  const unsyncedLocal = useMemo(
    () => localHistory.filter((item) => isSyncableRun(item, remoteChatSessions)),
    [localHistory, remoteChatSessions],
  );

  const activeSyncProgress =
    syncBatchProgress && syncBatchProgress.projectId === selectedProjectId ? syncBatchProgress : undefined;

  const restoreItem = useCallback(async (item: RemoteChatSessionSummary) => {
    const key = remoteKey(item);
    setRestoringIds((prev) => new Set(prev).add(key));
    try {
      await restoreRemoteChatSession(item);
      // restoreRemoteChatSession opens the chat on success — close the panel
      // unless the failure path stamped the row unavailable (then stay open so
      // the user sees why, e.g. account_not_signed_in -> historyOpenError modal).
      const stamped = useStore
        .getState()
        .remoteChatSessions.find(
          (s) => s.sourceMachineId === item.sourceMachineId && s.sourceRunId === item.sourceRunId,
        );
      if (!stamped?.unavailableReason) onClose();
    } finally {
      setRestoringIds((prev) => {
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
    }
  }, [restoreRemoteChatSession, onClose]);

  const restoreAll = useCallback(async () => {
    const targets = remoteChatSessions.filter((item) => !item.unavailableReason);
    setRestoringAll(true);
    setRestoreProgress({ done: 0, total: targets.length });
    try {
      for (const item of targets) {
        const key = remoteKey(item);
        setRestoringIds((prev) => new Set(prev).add(key));
        try {
          // Skip the per-item refresh + auto-open in the batch loop — one
          // combined refresh after keeps the x/y counter honest and avoids
          // hijacking the chat panel N times.
          await restoreRemoteChatSession(item, undefined, { refresh: false, open: false });
        } finally {
          setRestoringIds((prev) => {
            const next = new Set(prev);
            next.delete(key);
            return next;
          });
          setRestoreProgress((prev) => (prev ? { ...prev, done: prev.done + 1 } : prev));
        }
      }
      await Promise.all([loadRunHistory(), loadRemoteChatSessions()]);
    } finally {
      setRestoringAll(false);
      setRestoreProgress(null);
    }
  }, [remoteChatSessions, restoreRemoteChatSession, loadRunHistory, loadRemoteChatSessions]);

  const executeSyncConfirm = useCallback(async () => {
    if (!confirmSync || !selectedProjectId) return;
    const { runIds } = confirmSync;
    setConfirmSync(null);
    await syncRuns(runIds, selectedProjectId);
  }, [confirmSync, selectedProjectId, syncRuns]);

  const visibleRemote = showAllRemote ? remoteChatSessions : remoteChatSessions.slice(0, REMOTE_LIST_LIMIT);

  return (
    <div className="board-overlay" onClick={onClose} role="presentation">
      <div
        className="board-panel sync-panel"
        role="dialog"
        aria-modal="true"
        aria-label="Chat sync"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="board-head">
          <span className="board-title">Chat Sync</span>
          {project && <span className="board-count">{project.name}</span>}
          {remoteHistoryLoading && <span className="history-status-spinner" aria-label="Loading remote chats" />}
          <span className="term-tabs-spacer" />
          <button
            type="button"
            className="project-history-refresh"
            onClick={() => void loadRemoteChatSessions()}
            disabled={remoteHistoryLoading || !selectedProjectId}
            title="Refresh remote chat list"
            aria-label="Refresh remote chat list"
          >
            <RefreshGlyph />
          </button>
          <button type="button" className="term-tab-new" aria-label="Close sync screen" onClick={onClose}>
            <CloseIcon size={13} />
          </button>
        </div>

        <div className="board-body">
          {!selectedProjectId ? (
            <div className="board-empty">Select a project first — sync runs per project.</div>
          ) : (
            <>
              <div className="board-section">
                <div className="board-section-head">
                  <span className="board-section-name">Remote chats (Drive)</span>
                  <span className="board-section-count">{remoteChatSessions.length}</span>
                  <span className="term-tabs-spacer" />
                  {remoteChatSessions.some((item) => !item.unavailableReason) && (
                    <button
                      type="button"
                      className="project-history-selection-action"
                      onClick={() => void restoreAll()}
                      disabled={restoringAll || restoringIds.size > 0}
                      title={
                        restoringAll && restoreProgress
                          ? `Restoring ${restoreProgress.done}/${restoreProgress.total}…`
                          : "Restore all remote chats to this machine"
                      }
                    >
                      {restoringAll ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                      {restoringAll && restoreProgress ? ` ${restoreProgress.done}/${restoreProgress.total}` : " Restore all"}
                    </button>
                  )}
                </div>

                {remoteHistoryLoadError && (
                  <div className="project-history-error">
                    <strong>Remote history failed to load</strong>
                    <span>{remoteHistoryLoadError}</span>
                  </div>
                )}

                {remoteChatSessions.length === 0 ? (
                  <div className="board-empty">
                    {remoteHistoryLoading
                      ? "Loading remote chats…"
                      : "No remote chats have been synced for this project yet."}
                  </div>
                ) : (
                  <div className="project-history-list sync-panel-list">
                    {visibleRemote.map((item) => {
                      const key = remoteKey(item);
                      const isRestoring = restoringIds.has(key);
                      return (
                        <div key={key} className="project-history-item-row">
                          <button
                            type="button"
                            className={`project-history-item${item.unavailableReason ? " project-history-item--disabled" : ""}`}
                            disabled={Boolean(item.unavailableReason) || isRestoring || restoringAll}
                            onClick={() => void restoreItem(item)}
                            title={
                              item.unavailableReason ||
                              `${item.sourceMachineId}/${item.sourceRunId} — click to restore and open`
                            }
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
                          {!item.unavailableReason && (
                            <span className="project-history-item-actions">
                              <button
                                type="button"
                                className="project-history-sync-icon"
                                disabled={isRestoring || restoringAll}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  void restoreItem(item);
                                }}
                                title="Restore this chat"
                                aria-label="Restore chat"
                              >
                                {isRestoring ? <span className="history-status-spinner" aria-hidden="true" /> : <RestoreGlyph />}
                              </button>
                            </span>
                          )}
                        </div>
                      );
                    })}
                    {remoteChatSessions.length > REMOTE_LIST_LIMIT && (
                      <button
                        type="button"
                        className="project-history-more"
                        onClick={() => setShowAllRemote((v) => !v)}
                      >
                        {showAllRemote ? "Show less" : `Show all (${remoteChatSessions.length - REMOTE_LIST_LIMIT} more)`}
                      </button>
                    )}
                  </div>
                )}
              </div>

              <div className="board-section">
                <div className="board-section-head">
                  <span className="board-section-name">Local chats not synced</span>
                  <span className="board-section-count">{unsyncedLocal.length}</span>
                  <span className="term-tabs-spacer" />
                  {unsyncedLocal.length > 0 && (
                    <button
                      type="button"
                      className="project-history-selection-action"
                      onClick={() => void syncAllInProject(selectedProjectId)}
                      disabled={Boolean(activeSyncProgress)}
                      title={
                        activeSyncProgress
                          ? `Syncing chats to Drive (${activeSyncProgress.done}/${activeSyncProgress.total})`
                          : `Sync ${unsyncedLocal.length} chat${unsyncedLocal.length > 1 ? "s" : ""} to Drive`
                      }
                    >
                      {activeSyncProgress ? (
                        <span className="history-status-spinner" aria-hidden="true" />
                      ) : (
                        <SyncGlyph />
                      )}
                      {activeSyncProgress ? ` ${activeSyncProgress.done}/${activeSyncProgress.total}` : " Sync all"}
                    </button>
                  )}
                </div>

                {unsyncedLocal.length === 0 ? (
                  <div className="board-empty">All local chats are synced.</div>
                ) : (
                  <div className="project-history-list sync-panel-list">
                    {unsyncedLocal.map((item: RunHistoryItem) => {
                      const isSyncing = item.syncStatus === "syncing";
                      const syncFailed = item.syncStatus === "failed";
                      return (
                        <div key={item.runId} className="project-history-item-row">
                          <div className="project-history-item project-history-item--static">
                            <span className="project-history-item-top">
                              {isSyncing && <span className="history-status-spinner" aria-hidden="true" />}
                              <span className="project-history-item-title">
                                {runTitle(item.lastPrompt || item.lastMessage)}
                              </span>
                            </span>
                            <span className="project-history-item-meta">
                              {isSyncing
                                ? "Syncing to Drive…"
                                : `${item.providerKey} · ${RUN_TIME_FORMAT.format(new Date(item.updatedAt))}`}
                            </span>
                          </div>
                          <div className="project-history-item-actions">
                            <button
                              type="button"
                              className={`project-history-sync-icon${syncFailed ? " project-history-sync-icon--failed" : ""}`}
                              disabled={isSyncing || Boolean(activeSyncProgress)}
                              onClick={() => setConfirmSync({ runIds: [item.runId] })}
                              title={syncFailed ? "Sync failed — click to retry" : "Sync this chat to Drive"}
                              aria-label="Sync chat to Drive"
                            >
                              {isSyncing ? <span className="history-status-spinner" aria-hidden="true" /> : <SyncGlyph />}
                            </button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {confirmSync && (
        <div
          className="project-history-confirm-overlay"
          onClick={() => setConfirmSync(null)}
          role="dialog"
          aria-modal="true"
          aria-label="Confirm sync"
        >
          <div className="project-history-confirm-modal" onClick={(e) => e.stopPropagation()}>
            <p className="project-history-confirm-message">
              {confirmSync.runIds.length === 0
                ? "No syncable chats selected (all may already be synced or unavailable)."
                : `Sync ${confirmSync.runIds.length} chat${confirmSync.runIds.length > 1 ? "s" : ""} to Drive?`}
            </p>
            <div className="project-history-confirm-actions">
              <button type="button" className="project-history-confirm-cancel" onClick={() => setConfirmSync(null)}>
                Cancel
              </button>
              {confirmSync.runIds.length > 0 && (
                <button type="button" className="project-history-confirm-ok" onClick={() => void executeSyncConfirm()}>
                  Sync
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
