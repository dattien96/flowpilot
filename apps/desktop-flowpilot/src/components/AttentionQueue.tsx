import React from "react";
import { useStore } from "@/state/store";
import type { AttentionItem, AttentionKind } from "@/state/attentionQueue";

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

function truncate(text: string, max = 60): string {
  const oneLine = text.replace(/\s+/g, " ").trim();
  return oneLine.length > max ? `${oneLine.slice(0, max - 1)}…` : oneLine;
}

/**
 * Task-404: global attention queue — one row per run that is waiting on the
 * user (approval / question / gate / dispatch attention). Hidden when empty.
 * Renders at the top of the Navigator, above the history groups.
 */
export function AttentionQueue(props: {
  onOpenRun: (runId: string, chatId: string) => void;
}): React.ReactElement | null {
  const items = useStore((s) => s.attentionItems);
  if (items.length === 0) return null;
  return (
    <section className="attention-queue" aria-label="Attention queue">
      <div className="attention-queue-head">
        <label>Needs attention</label>
        <span className="attention-queue-badge" aria-label={`${items.length} runs waiting`}>
          {items.length}
        </span>
      </div>
      <ul className="attention-queue-list">
        {items.map((item: AttentionItem) => (
          <li key={item.runId}>
            <button
              type="button"
              className="attention-queue-item"
              title={`${item.runTitle} — ${KIND_LABEL[item.kind]}`}
              onClick={() => props.onOpenRun(item.runId, item.chatId)}
            >
              <span className={`attention-kind attention-kind--${item.kind}`}>
                {KIND_LABEL[item.kind]}
              </span>
              <span className="attention-queue-title">{truncate(item.runTitle)}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
