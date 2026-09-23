import React from "react";
import { useStore } from "@/state/store";
import { deriveSpectatorView } from "@/state/boardModel";
import type { AttentionKind } from "@/state/attentionQueue";
import { CloseIcon } from "./icons";

const KIND_LABEL: Record<AttentionKind, string> = {
  approval: "Approval",
  question: "Question",
  gate: "Gate",
  ss_lock: "SS Lock",
  cp_lock: "CP Lock",
  r_requirement: "Requirement",
  decision: "Decision",
  dispatch_attention: "Dispatch",
};

const STATUS_LABEL: Record<string, string> = {
  running: "Running",
  idle: "Idle",
  waiting_approval: "Waiting",
  waiting_user_approval: "Waiting",
  waiting_user_confirm: "SS Lock",
  waiting_question: "Waiting",
  blocked: "Blocked",
  completed: "Done",
  failed: "Failed",
  stopped: "Stopped",
};

function relTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  const min = Math.floor(ms / 60_000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}

function truncate(text: string, max = 120): string {
  const oneLine = text.replace(/\s+/g, " ").trim();
  return oneLine.length > max ? `${oneLine.slice(0, max - 1)}…` : oneLine;
}

/**
 * Task-425: read-only glance at ONE non-focused run — derived from the warmed
 * history slices + attention queue (poll-fresh, explicitly stale-tolerant).
 * No stream attach, no run-state writes. "Open" promotes via the same
 * openRunAtAttention path as inbox/board.
 */
export function SpectatorPane(): React.ReactElement | null {
  const spectatorRunId = useStore((s) => s.spectatorRunId);
  const spectatorProjectId = useStore((s) => s.spectatorProjectId);
  const focusedRunId = useStore((s) => s.runId);
  const historyByProject = useStore((s) => s.projectHistoryById);
  const attention = useStore((s) => s.attentionItems);
  const projects = useStore((s) => s.projects);
  const openRunAtAttention = useStore((s) => s.openRunAtAttention);
  const closeSpectator = useStore((s) => s.closeSpectator);

  // T-3: the focused run is never spectated.
  if (!spectatorRunId || spectatorRunId === focusedRunId) return null;

  const view = deriveSpectatorView(historyByProject, attention, projects, spectatorRunId, spectatorProjectId);
  if (!view) {
    return (
      <aside className="spectator-pane" aria-label="Watched run">
        <div className="spectator-head">
          <span className="spectator-label">Watching</span>
          <button type="button" className="term-tab-new" aria-label="Stop watching" onClick={closeSpectator}>
            <CloseIcon size={12} />
          </button>
        </div>
        <div className="spectator-missing">Run data not loaded yet — it appears on the next refresh.</div>
      </aside>
    );
  }

  return (
    <aside className="spectator-pane" aria-label="Watched run">
      <div className="spectator-head">
        <span className="spectator-label">Watching · {view.projectName}</span>
        <span className="spectator-time">{relTime(view.updatedAt)}</span>
        <button type="button" className="term-tab-new" aria-label="Stop watching" onClick={closeSpectator}>
          <CloseIcon size={12} />
        </button>
      </div>
      <button
        type="button"
        className="spectator-body"
        title={`${view.runTitle} — open this run`}
        onClick={() => void openRunAtAttention(view.runId, view.chatId, view.projectId)}
      >
        <div className="spectator-status-row">
          {view.waitingKind ? (
            <span className={`attention-kind attention-kind--${view.waitingKind}`}>
              {KIND_LABEL[view.waitingKind]}
            </span>
          ) : (
            <span className={`board-status board-status--${view.status}`}>
              {STATUS_LABEL[view.status] ?? view.status}
            </span>
          )}
        </div>
        <div className="spectator-title">{truncate(view.runTitle, 80)}</div>
        {view.lastLine && <div className="spectator-line">{truncate(view.lastLine)}</div>}
      </button>
    </aside>
  );
}
