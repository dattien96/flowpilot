import { createPortal } from "react-dom";
import { useEffect, useRef, useState } from "react";
import { buildToastGroupSummary } from "@/app/runToastGrouping";
import { useStore } from "@/state/store";
import type { RunStatus } from "@/types/contract";

interface Toast {
  id: number;
  message: string;
  kind: "done" | "approval" | "question";
}

let seq = 0;
const ACTIVE_STATUSES: RunStatus[] = ["starting", "running", "waiting_approval", "waiting_question", "completed", "failed"];

function fireNative(title: string, body: string) {
  console.log("[RunToast] fireNative", title, body);
  void window.flowpilot?.showNotification(title, body);
}

export function RunToast(): React.ReactElement | null {
  const status = useStore((s) => s.status);
  const prevRef = useRef<RunStatus>(status);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [modalOpen, setModalOpen] = useState(false);

  useEffect(() => {
    if (toasts.length === 0) {
      setModalOpen(false);
    }
  }, [toasts.length]);

  useEffect(() => {
    const prev = prevRef.current;
    prevRef.current = status;

    // Suppress the completion toast while a history replay drives status running→completed:
    // opening an old chat is not a fresh AI response. (BUG-118)
    if (useStore.getState()._historyReplaying) {
      return;
    }

    if (status === "completed" && ACTIVE_STATUSES.includes(prev) && prev !== "completed") {
      const toast: Toast = { id: ++seq, message: "AI response complete", kind: "done" };
      setToasts((ts) => [...ts, toast]);
      fireNative("FlowPilot", "AI response complete");
      const tid = setTimeout(() => setToasts((ts) => ts.filter((t) => t.id !== toast.id)), 4000);
      return () => clearTimeout(tid);
    }

    if (status === "waiting_approval" && prev !== "waiting_approval") {
      const toast: Toast = { id: ++seq, message: "Approval required — AI is waiting for you", kind: "approval" };
      setToasts((ts) => [...ts, toast]);
      fireNative("FlowPilot", "Approval required — AI is waiting for your input");
      const tid = setTimeout(() => setToasts((ts) => ts.filter((t) => t.id !== toast.id)), 8000);
      return () => clearTimeout(tid);
    }

    if (status === "waiting_question" && prev !== "waiting_question") {
      const toast: Toast = { id: ++seq, message: "AI has a question for you", kind: "question" };
      setToasts((ts) => [...ts, toast]);
      fireNative("FlowPilot", "AI has a question for you");
      const tid = setTimeout(() => setToasts((ts) => ts.filter((t) => t.id !== toast.id)), 8000);
      return () => clearTimeout(tid);
    }
  }, [status]);

  if (toasts.length === 0) return null;

  const groupSummary = buildToastGroupSummary(toasts);

  return (
    <>
      <div className="run-toast-stack" role="status" aria-live="polite" aria-label="Run notifications">
        <div className="run-toast run-toast--grouped">
          <button
            type="button"
            className="run-toast-group-toggle"
            onClick={() => setModalOpen(true)}
            aria-haspopup="dialog"
            aria-label={`${toasts.length} notification${toasts.length !== 1 ? "s" : ""} — click to view`}
          >
            <span className="run-toast-icon" aria-hidden="true">⚑</span>
            <span className="run-toast-group-copy">
              <span className="run-toast-msg">
                {toasts.length} notification{toasts.length !== 1 ? "s" : ""}
              </span>
              <span className="run-toast-group-summary">{groupSummary}</span>
            </span>
            <span className="run-toast-group-caret" aria-hidden="true">▸</span>
          </button>
          <button
            type="button"
            className="run-toast-close"
            onClick={() => setToasts([])}
            aria-label="Dismiss all notifications"
          >
            ✕
          </button>
        </div>
      </div>

      {modalOpen &&
        createPortal(
          <div
            className="run-toast-modal-backdrop"
            onClick={() => setModalOpen(false)}
          >
            <div
              className="run-toast-modal"
              role="dialog"
              aria-label="Notification list"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="run-toast-modal-head">
                <span>Notifications ({toasts.length})</span>
                <button
                  type="button"
                  className="run-toast-close"
                  onClick={() => setModalOpen(false)}
                  aria-label="Close notification list"
                >
                  ✕
                </button>
              </div>
              <div className="run-toast-modal-list">
                {toasts.map((toast) => (
                  <div
                    key={toast.id}
                    className={`run-toast-modal-item run-toast-modal-item--${toast.kind}`}
                  >
                    <span
                      className={`run-toast-group-dot run-toast-group-dot--${toast.kind}`}
                      aria-hidden="true"
                    />
                    <span className="run-toast-msg">{toast.message}</span>
                    <button
                      type="button"
                      className="run-toast-close"
                      onClick={() => setToasts((ts) => ts.filter((t) => t.id !== toast.id))}
                      aria-label={`Dismiss ${toast.message}`}
                    >
                      ✕
                    </button>
                  </div>
                ))}
              </div>
              <div className="run-toast-modal-footer">
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => {
                    setToasts([]);
                    setModalOpen(false);
                  }}
                >
                  Dismiss all
                </button>
              </div>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}
