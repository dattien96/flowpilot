import { useEffect, useRef } from "react";
import { useStore } from "@/state/store";
import type { RunStatus as RunStatusValue } from "@/types/contract";

const LABEL: Record<RunStatusValue, string> = {
  idle: "Idle",
  starting: "Starting",
  running: "Running",
  waiting_approval: "Waiting · approval",
  waiting_question: "Waiting · question",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

const RUN_TIME_FORMAT = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function runTitle(text?: string): string {
  if (!text) return "Untitled run";
  return text.length > 72 ? `${text.slice(0, 69)}...` : text;
}

export function RunStatus(): React.ReactElement {
  const historyAnchorRef = useRef<HTMLDivElement>(null);
  const status = useStore((s) => s.status);
  const runId = useStore((s) => s.runId);
  const projects = useStore((s) => s.projects);
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const runHistory = useStore((s) => s.runHistory);
  const historyOpen = useStore((s) => s.historyOpen);
  const historyLoading = useStore((s) => s.historyLoading);
  const historyLoadError = useStore((s) => s.historyLoadError);
  const resetRun = useStore((s) => s.resetRun);
  const stop = useStore((s) => s.stop);
  const toggleRunHistory = useStore((s) => s.toggleRunHistory);
  const openHistoryRun = useStore((s) => s.openHistoryRun);

  useEffect(() => {
    if (!historyOpen) return;
    const closeIfOutside = (event: Event) => {
      const root = historyAnchorRef.current;
      if (root && !root.contains(event.target as Node)) {
        useStore.setState({ historyOpen: false });
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        useStore.setState({ historyOpen: false });
      }
    };
    document.addEventListener("pointerdown", closeIfOutside, true);
    document.addEventListener("focusin", closeIfOutside, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", closeIfOutside, true);
      document.removeEventListener("focusin", closeIfOutside, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [historyOpen]);

  const active = status === "running" || status === "waiting_approval" || status === "waiting_question";
  const ready = status === "idle" && !runId && projects.length > 0;
  const statusClass = ready ? "ready" : status;
  const statusLabel = ready ? "Run Ready" : LABEL[status];

  return (
    <div className="run-status">
      <span className={`status-dot status-${statusClass}`} />
      <span className="status-label">{statusLabel}</span>
      {runId && <span className="run-id">{runId}</span>}
      {active && (
        <button className="btn btn-ghost" onClick={() => void stop()}>
          Stop
        </button>
      )}
      <div className="run-actions">
        <div className="run-history-anchor" ref={historyAnchorRef}>
          <button className="btn btn-ghost" onClick={() => void toggleRunHistory()} disabled={!selectedProjectId}>
            History
          </button>
          {historyOpen && (
            <div className="run-history-popover" role="dialog" aria-label="Run history">
              {historyLoading ? (
                <div className="run-history-empty">Loading...</div>
              ) : historyLoadError ? (
                <div className="run-history-empty run-history-error">Failed to load history</div>
              ) : runHistory.length === 0 ? (
                <div className="run-history-empty">No runs for this project</div>
              ) : (
                <div className="run-history-list">
                  {runHistory.map((item) => (
                    <button
                      key={item.runId}
                      className="run-history-item"
                      onClick={() => void openHistoryRun(item.runId)}
                      title={item.runId}
                    >
                      <span className="run-history-main">
                        <span className={`status-dot status-${item.status}`} />
                        <span className="run-history-title">{runTitle(item.lastPrompt || item.lastMessage)}</span>
                      </span>
                      <span className="run-history-meta">
                        {LABEL[item.status]} · {RUN_TIME_FORMAT.format(new Date(item.updatedAt))}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
        {runId && (
          <button className="btn btn-ghost" onClick={resetRun}>
            New run
          </button>
        )}
      </div>
    </div>
  );
}
