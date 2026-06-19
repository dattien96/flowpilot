import { useEffect, useRef, useState } from "react";
import { useStore, type TimelineItem } from "@/state/store";

const TIMELINE_PAGE_SIZE = 6;

function countPrompts(timeline: TimelineItem[]): number {
  return timeline.filter((item) => item.kind === "prompt").length;
}

function sliceTimelineFromPrompt(timeline: TimelineItem[], visiblePromptCount: number): TimelineItem[] {
  const totalPrompts = countPrompts(timeline);
  if (visiblePromptCount >= totalPrompts) {
    return timeline;
  }

  const promptsToSkip = totalPrompts - visiblePromptCount;
  let promptsSeen = 0;

  for (let i = 0; i < timeline.length; i++) {
    if (timeline[i].kind === "prompt") {
      promptsSeen++;
      if (promptsSeen > promptsToSkip) {
        return timeline.slice(i);
      }
    }
  }

  return timeline;
}
import { ApprovalCard } from "./ApprovalCard";
import { QuestionCard } from "./QuestionCard";

const shortName = (path: string): string => path.split("/").pop() ?? path;

const TOOL_ICON: Record<string, string> = { running: "⏳", success: "✓", failed: "✕", cancelled: "⊘" };
const FILE_ICON: Record<string, string> = { created: "＋", modified: "✎", deleted: "－", renamed: "→" };

type ToolItem = Extract<TimelineItem, { kind: "tool" }>;
type TimelineGroup = TimelineItem | { kind: "tool-group"; id: string; tools: ToolItem[] };

export function shouldShowAgentTimelineHeader(activeAgentRunId: string | undefined, mainRunId: string | undefined, agentRunCount: number): boolean {
  return Boolean(activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId) || agentRunCount > 0;
}

function previewValue(value: unknown): string | undefined {
  if (value === undefined || value === null) return undefined;
  if (typeof value === "string") return value;
  if (Array.isArray(value)) return value.map((part) => (typeof part === "string" ? part : JSON.stringify(part))).join(" ");
  if (typeof value === "object") {
    const record = value as Record<string, unknown>;
    const command = record.command ?? record.cmd ?? record.name ?? record.path;
    if (typeof command === "string") return command;
    if (Array.isArray(command)) return command.map((part) => String(part)).join(" ");
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  return String(value);
}

function toolLabel(tool: ToolItem): string {
  const name = typeof tool.toolName === "string" ? tool.toolName.trim() : "";
  if (name && name !== "command" && name !== "shell") return name;
  return previewValue(tool.input) ?? previewValue(tool.output) ?? (name || `tool ${tool.id || "call"}`);
}

function CopyBubble({
  text,
  className,
  children,
}: {
  text: string;
  className: string;
  children: React.ReactNode;
}): React.ReactElement {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] copy failed:", err);
    }
  };

  return (
    <div className={`${className} copyable-bubble`}>
      <button
        type="button"
        className="bubble-copy"
        onClick={copy}
        aria-label={copied ? "Copied" : "Copy message"}
        title={copied ? "Copied" : "Copy"}
      >
        {copied ? "✓" : "⧉"}
      </button>
      {children}
    </div>
  );
}

function PromptSkillsSummary({ skills }: { skills: string[] }): React.ReactElement {
  const [open, setOpen] = useState(false);
  const summary = `${skills.length} skill${skills.length === 1 ? "" : "s"} selected`;
  const preview = skills.slice(0, 2).map((name) => `/${name}`).join(", ");
  const remainder = skills.length - 2;

  return (
    <>
      <button
        type="button"
        className={`prompt-skills-summary ${open ? "prompt-skills-summary-open" : ""}`}
        aria-expanded={open}
        aria-label={summary}
        title={summary}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="prompt-skills-caret" aria-hidden="true">{open ? "▾" : "▸"}</span>
        <span className="prompt-skills-title">{summary}</span>
        <span className="prompt-skills-preview">
          {preview}
          {remainder > 0 ? ` +${remainder}` : ""}
        </span>
      </button>
      {open && (
        <div className="prompt-skills-body">
          {skills.map((name) => (
            <div key={name} className="prompt-skill-chip">
              <span className="prompt-skill-name">/{name}</span>
            </div>
          ))}
        </div>
      )}
    </>
  );
}

function ToolRow({ it }: { it: Extract<TimelineItem, { kind: "tool" }> }): React.ReactElement {
  const label = toolLabel(it);
  const output = previewValue(it.output);
  const status = it.status || "success";
  return (
    <div className={`row tool-row tool-${status}`}>
      <span className="row-icon">{TOOL_ICON[status] ?? "•"}</span>
      <span className="row-main">
        <code className="tool-label">{label}</code>
        {status !== "running" && <span className="row-status">{status}</span>}
        {output && output !== label && <span className="row-output">{output}</span>}
      </span>
    </div>
  );
}

function toolGroupLabel(tools: ToolItem[]): string {
  if (tools.some((tool) => tool.status === "running")) return `${tools.length} tool call${tools.length === 1 ? "" : "s"} running`;
  if (tools.some((tool) => tool.status === "failed")) return `${tools.length} tool call${tools.length === 1 ? "" : "s"} · failed`;
  if (tools.some((tool) => tool.status === "cancelled")) return `${tools.length} tool call${tools.length === 1 ? "" : "s"} · cancelled`;
  return `${tools.length} tool call${tools.length === 1 ? "" : "s"} completed`;
}

function ToolGroup({ tools }: { tools: ToolItem[] }): React.ReactElement {
  const [open, setOpen] = useState(false);
  const label = toolGroupLabel(tools);
  return (
    <>
      <button
        type="button"
        className={`tool-group-summary ${open ? "tool-group-summary-open" : ""}`}
        aria-expanded={open}
        aria-label={label}
        title={label}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="tool-group-caret">{open ? "▾" : "▸"}</span>
        <span className="tool-group-title">{label}</span>
      </button>
      {open && (
        <div className="tool-group-body">
          {tools.length === 0 ? (
            <div className="tool-empty">No tool calls captured.</div>
          ) : (
            tools.map((tool, index) => <ToolRow key={tool.id || `tool-${index}`} it={tool} />)
          )}
        </div>
      )}
    </>
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

function buildTimelineGroups(timeline: TimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let pendingTools: ToolItem[] = [];

  const flushTools = () => {
    if (pendingTools.length === 0) return;
    const firstToolId = pendingTools[0].id || `${pendingTools[0].toolName}-${groups.length}`;
    groups.push({ kind: "tool-group", id: `tool-group-${firstToolId}`, tools: pendingTools });
    pendingTools = [];
  };

  for (const item of timeline) {
    if (item.kind === "tool") {
      pendingTools.push(item);
    } else {
      flushTools();
      groups.push(item);
    }
  }
  flushTools();

  return groups;
}

function Item({ it }: { it: TimelineGroup }): React.ReactElement | null {
  switch (it.kind) {
    case "assistant":
      return (
        <CopyBubble text={it.text} className={`bubble assistant ${it.finalized ? "final" : "streaming"}`}>
          {it.text}
          {!it.finalized && <span className="caret">▌</span>}
        </CopyBubble>
      );
    case "prompt":
      return (
        <div className="prompt-stack">
          {it.attachments && it.attachments.length > 0 && (
            <div className="prompt-attachments" aria-label={`${it.attachments.length} image attachment(s)`}>
              {it.attachments.map((att) =>
                att.previewUrl ? (
                  <img
                    key={att.id}
                    className="prompt-attachment-thumb"
                    src={att.previewUrl}
                    alt={att.originalName}
                    title={att.originalName}
                  />
                ) : (
                  <span key={att.id} className="prompt-attachment-chip" title={att.originalName}>
                    🖼 {att.originalName}
                  </span>
                ),
              )}
            </div>
          )}
          <CopyBubble text={it.text} className="bubble prompt">
            {it.text}
          </CopyBubble>
          {it.selectedSkills && it.selectedSkills.length > 0 && (
            <PromptSkillsSummary skills={it.selectedSkills} />
          )}
        </div>
      );
    case "thinking":
      return <div className="system-line thinking">{it.text}</div>;
    case "tool":
      return <ToolRow it={it} />;
    case "tool-group":
      return <ToolGroup tools={it.tools} />;
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
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const activeAgentRunId = useStore((s) => s.activeAgentRunId);
  const agentRuns = useStore((s) => s.agentRuns);
  const backToMainRun = useStore((s) => s.backToMainRun);
  const endRef = useRef<HTMLDivElement>(null);

  const totalPromptCount = countPrompts(timeline);
  const [visiblePromptCount, setVisiblePromptCount] = useState(TIMELINE_PAGE_SIZE);

  useEffect(() => {
    setVisiblePromptCount((current) => {
      if (totalPromptCount <= TIMELINE_PAGE_SIZE) {
        return totalPromptCount;
      }
      return Math.min(Math.max(current, TIMELINE_PAGE_SIZE), totalPromptCount);
    });
  }, [totalPromptCount]);

  const hiddenPromptCount = Math.max(totalPromptCount - visiblePromptCount, 0);
  const visibleTimeline = sliceTimelineFromPrompt(timeline, visiblePromptCount);
  const timelineGroups = buildTimelineGroups(visibleTimeline);
  const runningAgentCount = agentRuns.filter(
    (run) => run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question",
  ).length;
  const showAgentHeader = shouldShowAgentTimelineHeader(activeAgentRunId, mainRunId, agentRuns.length);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [timeline]);

  return (
    <div className={`timeline ${activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? "timeline-agent-focused" : ""}`}>
      {showAgentHeader && (
        <div className="timeline-head">
          {activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? (
            <div className="timeline-breadcrumb">
              <button type="button" className="timeline-back-btn" onClick={backToMainRun}>
                ← Back to main
              </button>
              <span className="timeline-breadcrumb-sep">/</span>
              <span className="timeline-breadcrumb-current">{activeAgentRunId}</span>
            </div>
          ) : (
            <div className="timeline-breadcrumb">
              <span className="timeline-breadcrumb-current">{runningAgentCount > 0 ? `${runningAgentCount} agents running` : "Main run"}</span>
            </div>
          )}
          {runningAgentCount > 0 && (
            <div className="timeline-agent-summary">
              <span>{runningAgentCount} agents running</span>
            </div>
          )}
        </div>
      )}
      {timeline.length === 0 && <div className="empty">Select a project, choose your chat controls, and send a prompt to begin.</div>}
      {hiddenPromptCount > 0 && (
        <button
          type="button"
          className="load-earlier-btn"
          onClick={() =>
            setVisiblePromptCount((current) =>
              Math.min(totalPromptCount, current + TIMELINE_PAGE_SIZE),
            )
          }
        >
          ↑ Load earlier prompts ({hiddenPromptCount})
        </button>
      )}
      {timelineGroups.map((it) => (
        <Item key={it.id} it={it} />
      ))}

      <div ref={endRef} />
    </div>
  );
}
