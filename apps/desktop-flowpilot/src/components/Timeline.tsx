import { useEffect, useRef } from "react";
import { useStore, type TimelineItem } from "@/state/store";
import { ApprovalCard } from "./ApprovalCard";
import { QuestionCard } from "./QuestionCard";

const shortName = (path: string): string => path.split("/").pop() ?? path;

const TOOL_ICON: Record<string, string> = { running: "⏳", success: "✓", failed: "✗", cancelled: "⊘" };
const FILE_ICON: Record<string, string> = { created: "＋", modified: "✎", deleted: "－", renamed: "→" };

function ToolRow({ it }: { it: Extract<TimelineItem, { kind: "tool" }> }): React.ReactElement {
  return (
    <div className={`row tool-row tool-${it.status}`}>
      <span className="row-icon">{TOOL_ICON[it.status]}</span>
      <span className="row-main">
        <code>{it.toolName}</code>
        {it.status !== "running" && <span className="row-status">{it.status}</span>}
      </span>
    </div>
  );
}

function FileRow({ it }: { it: Extract<TimelineItem, { kind: "file" }> }): React.ReactElement {
  const openInIde = useStore((s) => s.openInIde);
  return (
    <button className="row file-row" title={it.path} onClick={() => openInIde(it.path)}>
      <span className="row-icon">{FILE_ICON[it.changeType ?? "modified"] ?? "✎"}</span>
      <span className="row-main">
        <span className="file-name">{shortName(it.path)}</span>
        <span className="file-change">{it.changeType ?? "modified"}</span>
      </span>
      <span className="row-hint">open in IDE ↗</span>
    </button>
  );
}

function Item({ it }: { it: TimelineItem }): React.ReactElement | null {
  switch (it.kind) {
    case "assistant":
      return (
        <div className={`bubble assistant ${it.finalized ? "final" : "streaming"}`}>
          {it.text}
          {!it.finalized && <span className="caret">▌</span>}
        </div>
      );
    case "tool":
      return <ToolRow it={it} />;
    case "file":
      return <FileRow it={it} />;
    case "approval":
      return <ApprovalCard approvalId={it.approvalId} details={it.details} decision={it.decision} />;
    case "question":
      return (
        <QuestionCard
          questionId={it.questionId}
          prompt={it.prompt}
          options={it.options}
          multiSelect={it.multiSelect}
          answer={it.answer}
        />
      );
    case "system":
      return <div className={`system-line ${it.tone}`}>{it.text}</div>;
    default:
      return null;
  }
}

export function Timeline(): React.ReactElement {
  const timeline = useStore((s) => s.timeline);
  const recoverable = useStore((s) => s.recoverable);
  const reconnect = useStore((s) => s.reconnect);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [timeline]);

  return (
    <div className="timeline">
      {timeline.length === 0 && <div className="empty">Select a workflow/step and send a prompt to begin.</div>}
      {timeline.map((it) => (
        <Item key={it.id} it={it} />
      ))}
      {recoverable && (
        <div className="reconnect-bar">
          <span>Stream interrupted.</span>
          <button className="btn btn-primary" onClick={() => void reconnect()}>
            Reconnect &amp; replay
          </button>
        </div>
      )}
      <div ref={endRef} />
    </div>
  );
}
