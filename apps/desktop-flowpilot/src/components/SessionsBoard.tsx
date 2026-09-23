import React, { useEffect } from "react";
import { useStore } from "@/state/store";
import { deriveBoardSections } from "@/state/boardModel";
import type { AttentionKind } from "@/state/attentionQueue";
import { CloseIcon, EyeIcon } from "./icons";

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
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  return `${Math.floor(hr / 24)}d`;
}

function truncate(text: string, max = 96): string {
  const oneLine = text.replace(/\s+/g, " ").trim();
  return oneLine.length > max ? `${oneLine.slice(0, max - 1)}…` : oneLine;
}

/**
 * Task-422: Devin-style sessions monitor — overlay listing every known run
 * across all projects grouped by project. Read-only awareness surface: row
 * click routes through openRunAtAttention (same path as the inbox) and closes.
 */
export function SessionsBoard({ onClose }: { onClose: () => void }): React.ReactElement {
  const historyByProject = useStore((s) => s.projectHistoryById);
  const attention = useStore((s) => s.attentionItems);
  const projects = useStore((s) => s.projects);
  const focusedRunId = useStore((s) => s.runId);
  const openRunAtAttention = useStore((s) => s.openRunAtAttention);
  const openSpectator = useStore((s) => s.openSpectator);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const sections = deriveBoardSections(historyByProject, attention, projects, focusedRunId ?? null);
  const totalRuns = sections.reduce((n, s) => n + s.rows.length, 0);

  return (
    <div className="board-overlay" onClick={onClose} role="presentation">
      <div
        className="board-panel"
        role="dialog"
        aria-modal="true"
        aria-label="Sessions monitor"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="board-head">
          <span className="board-title">Sessions</span>
          <span className="board-count">{totalRuns}</span>
          <span className="term-tabs-spacer" />
          <button type="button" className="term-tab-new" aria-label="Close sessions board" onClick={onClose}>
            <CloseIcon size={13} />
          </button>
        </div>
        <div className="board-body">
          {sections.length === 0 ? (
            <div className="board-empty">
              No runs yet. Start a chat or workflow run and it will show up here.
            </div>
          ) : (
            sections.map((section) => (
              <div className="board-section" key={section.projectId}>
                <div className="board-section-head">
                  <span className="board-section-name">{section.projectName}</span>
                  <span className="board-section-count">{section.rows.length}</span>
                </div>
                <ul className="board-rows">
                  {section.rows.map((row) => (
                    <li key={row.runId}>
                      <button
                        type="button"
                        className={`board-row${row.isFocused ? " focused" : ""}`}
                        title={`${row.runTitle} — ${row.status}`}
                        onClick={() => {
                          onClose();
                          void openRunAtAttention(row.runId, row.chatId, row.projectId);
                        }}
                      >
                        {row.waitingKind ? (
                          <span className={`attention-kind attention-kind--${row.waitingKind}`}>
                            {KIND_LABEL[row.waitingKind]}
                          </span>
                        ) : (
                          <span className={`board-status board-status--${row.status}`}>
                            {STATUS_LABEL[row.status] ?? row.status}
                          </span>
                        )}
                        <span className="board-row-title">{truncate(row.runTitle)}</span>
                        {row.worktreeBound && (
                          <span className="project-history-worktree-badge" title="Isolated worktree">⎇</span>
                        )}
                        {row.isFocused && <span className="board-focused">focused</span>}
                        <span className="board-row-time">{relTime(row.updatedAt)}</span>
                      </button>
                      {!row.isFocused && (
                        <button
                          type="button"
                          className="board-row-watch"
                          aria-label={`Watch ${row.runTitle}`}
                          title="Watch in spectator pane"
                          onClick={(e) => {
                            e.stopPropagation();
                            openSpectator(row.runId, row.projectId);
                          }}
                        >
                          <EyeIcon size={13} />
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
