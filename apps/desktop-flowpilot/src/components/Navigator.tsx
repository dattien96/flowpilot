import { useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "@/state/store";
import type { RunHistoryItem } from "@/types/contract";

const PROJECT_LIMIT = 3;
const HISTORY_LIMIT = 10;

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
  const loadProjects = useStore((s) => s.loadProjects);
  const selectProject = useStore((s) => s.selectProject);
  const runId = useStore((s) => s.runId);
  const loadRunHistory = useStore((s) => s.loadRunHistory);
  const openHistoryRun = useStore((s) => s.openHistoryRun);

  const runIdRef = useRef(runId);
  runIdRef.current = runId;
  const prevHistoryRef = useRef<Map<string, RunHistoryItem["status"]>>(new Map());

  const [showAllProjects, setShowAllProjects] = useState(false);
  const [newlyCompleted, setNewlyCompleted] = useState<Set<string>>(new Set());
  const [recentProjectIds, setRecentProjectIds] = useState<string[]>([]);
  const [projectHistoryById, setProjectHistoryById] = useState<Record<string, RunHistoryItem[]>>({});
  const [openProjectIds, setOpenProjectIds] = useState<Record<string, boolean>>({});
  const [visibleHistoryCounts, setVisibleHistoryCounts] = useState<Record<string, number>>({});

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
    const id = setInterval(() => void loadRunHistory(), ms);
    return () => clearInterval(id);
  }, [selectedProjectId, status, loadRunHistory]);

  useEffect(() => {
    if (!selectedProjectId || historyLoading) return;
    const prev = prevHistoryRef.current;
    const currentRunId = runIdRef.current;
    const toAdd: string[] = [];
    for (const item of runHistory) {
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
  }, [selectedProjectId, historyLoading, runHistory]);

  useEffect(() => {
    if (!selectedProjectId || historyLoading || runHistory.length === 0) return;
    setProjectHistoryById((current) => ({
      ...current,
      [selectedProjectId]: sortByRecent(runHistory),
    }));
    setOpenProjectIds((current) => (current[selectedProjectId] ? current : { ...current, [selectedProjectId]: true }));
    setVisibleHistoryCounts((current) => (current[selectedProjectId] ? current : { ...current, [selectedProjectId]: HISTORY_LIMIT }));
  }, [historyLoading, runHistory, selectedProjectId]);

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

  const visibleHistoryFor = (projectId: string, history: RunHistoryItem[]): RunHistoryItem[] => {
    const count = visibleHistoryCounts[projectId] ?? HISTORY_LIMIT;
    return history.slice(0, count);
  };

  const toggleProjectHistory = (projectId: string) => {
    setOpenProjectIds((current) => ({ ...current, [projectId]: !current[projectId] }));
  };

  const showMoreHistory = (projectId: string, total: number) => {
    setVisibleHistoryCounts((current) => ({ ...current, [projectId]: total }));
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
              const visibleHistory = expanded ? visibleHistoryFor(projectId, history) : [];
              const hiddenCount = history.length - visibleHistory.length;
              return (
                <section key={projectId} className="project-history-group">
                  <button
                    type="button"
                    className={`project-history-group-head ${expanded ? "active" : ""}`}
                    onClick={() => toggleProjectHistory(projectId)}
                  >
                    <span className="project-history-group-title">
                      <strong>{projectName}</strong>
                      <small>{history.length} chats</small>
                    </span>
                    <span className="project-history-group-chevron">{expanded ? "▾" : "▸"}</span>
                  </button>

                  {expanded && (
                    <div className="project-history-list">
                      {visibleHistory.map((item) => {
                        const isNew = newlyCompleted.has(item.runId);
                        const hasIcon = isNew || item.status === "running" || item.status === "starting" ||
                          item.status === "waiting_approval" || item.status === "waiting_question";
                        return (
                          <button
                            key={item.runId}
                            type="button"
                            className={`project-history-item${hasIcon ? " project-history-item--has-icon" : ""}`}
                            onClick={() => {
                              if (isNew) setNewlyCompleted((c) => { const n = new Set(c); n.delete(item.runId); return n; });
                              void openHistoryRun(item.runId);
                            }}
                            title={item.runId}
                          >
                            <span className="project-history-item-top">
                              <HistoryStatusIcon status={item.status} isNew={isNew} />
                              <span className="project-history-item-title">
                                {runTitle(item.lastPrompt || item.lastMessage)}
                              </span>
                            </span>
                            <span className="project-history-item-meta">
                              {RUN_LABEL[item.status]} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                            </span>
                          </button>
                        );
                      })}

                      {hiddenCount > 0 && (
                        <button
                          type="button"
                          className="project-history-more"
                          onClick={() => showMoreHistory(projectId, history.length)}
                        >
                          Show more ({hiddenCount} more)
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
    </div>
  );
}
