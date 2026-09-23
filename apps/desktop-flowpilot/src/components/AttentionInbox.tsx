import { useEffect, useRef, useState } from "react";
import { useStore } from "@/state/store";
import type { AttentionItem, AttentionKind } from "@/state/attentionQueue";
import { InboxIcon } from "@/components/icons";

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

function truncate(text: string, max = 72): string {
  const oneLine = text.replace(/\s+/g, " ").trim();
  return oneLine.length > max ? `${oneLine.slice(0, max - 1)}…` : oneLine;
}

function waitingLabel(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  const min = Math.floor(ms / 60_000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  return `${Math.floor(hr / 24)}d`;
}

/**
 * Header inbox affordance for the attention queue (replaces the always-visible
 * Navigator panel). Badge shows the pending count; the popover lists each
 * waiting run oldest-first and opens it in place.
 */
export function AttentionInbox(): React.ReactElement {
  const items = useStore((s) => s.attentionItems);
  const openRunAtAttention = useStore((s) => s.openRunAtAttention);
  const projects = useStore((s) => s.projects);
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("mousedown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  // Auto-close when the queue drains (e.g. last item approved from the chat).
  useEffect(() => {
    if (items.length === 0) setOpen(false);
  }, [items.length]);

  return (
    <div className="attention-inbox" ref={rootRef}>
      <button
        type="button"
        className={`icon-btn attention-inbox-btn${open ? " attention-inbox-btn-open" : ""}`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-label={items.length > 0 ? `${items.length} runs need attention` : "Attention inbox"}
        title={items.length > 0 ? `${items.length} run${items.length > 1 ? "s" : ""} need attention` : "Needs attention"}
      >
        <InboxIcon size={15} />
        {items.length > 0 && (
          <span className="attention-inbox-badge" aria-hidden="true">
            {items.length}
          </span>
        )}
      </button>
      {open && (
        <div className="attention-inbox-pop" role="menu" aria-label="Runs needing attention">
          <div className="attention-inbox-head">Needs attention</div>
          {items.length === 0 ? (
            <div className="attention-inbox-empty">Nothing is waiting on you.</div>
          ) : (
            <ul className="attention-inbox-list">
              {items.map((item: AttentionItem) => (
                <li key={item.runId}>
                  <button
                    type="button"
                    className="attention-inbox-item"
                    title={`${item.runTitle} — ${KIND_LABEL[item.kind]}`}
                    onClick={() => {
                      setOpen(false);
                      void openRunAtAttention(item.runId, item.chatId, item.projectId);
                    }}
                  >
                    <span className={`attention-kind attention-kind--${item.kind}`}>
                      {KIND_LABEL[item.kind]}
                    </span>
                    <span className="attention-inbox-title">{truncate(item.runTitle)}</span>
                    <span className="attention-inbox-project">
                      {projects.find((p) => p.id === item.projectId)?.name ?? ""}
                    </span>
                    <span className="attention-inbox-waiting">{waitingLabel(item.waitingSince)}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
