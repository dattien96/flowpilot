import { useEffect, useMemo, useRef, useState } from "react";
import { providerLabel, useStore, type TimelineItem } from "@/state/store";
import { MentionText } from "@/components/MentionText";
import type { AgentRunSummary } from "@/types/contract";

// ---------------------------------------------------------------------------
// Lightweight Markdown renderer — no external dependency
// Handles: headings, fenced code, HR, tables, unordered lists, paragraphs,
// and inline bold / italic / inline-code.
// Copy button keeps the raw Markdown string via CopyBubble's `text` prop.
// ---------------------------------------------------------------------------

type MdBlock =
  | { kind: "code"; lang: string; content: string }
  | { kind: "heading"; level: number; text: string }
  | { kind: "hr" }
  | { kind: "table"; headers: string[]; rows: string[][] }
  | { kind: "list"; items: string[] }
  | { kind: "paragraph"; text: string };

const HEADING_TAGS = ["h1", "h2", "h3", "h4", "h5", "h6"] as const;

function renderInline(text: string, keyPrefix: string): React.ReactNode {
  const parts: React.ReactNode[] = [];
  const re = /(`[^`]+`|\*\*[^*\n]+\*\*|__[^_\n]+__|(?<!\*)\*(?!\*)([^*\n]+)(?<!\*)\*(?!\*)|(?<!_)_(?!_)([^_\n]+)(?<!_)_(?!_))/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let idx = 0;

  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    const raw = m[0];
    const key = `${keyPrefix}-${idx++}`;
    if (raw.startsWith("`")) {
      parts.push(<code key={key} className="md-icode">{raw.slice(1, -1)}</code>);
    } else if (raw.startsWith("**") || raw.startsWith("__")) {
      parts.push(<strong key={key}>{raw.slice(2, -2)}</strong>);
    } else {
      parts.push(<em key={key}>{raw.slice(1, -1)}</em>);
    }
    last = m.index + raw.length;
  }

  if (last < text.length) parts.push(text.slice(last));
  return parts.length === 0 ? "" : parts.length === 1 && typeof parts[0] === "string" ? parts[0] : <>{parts}</>;
}

function parseMdBlocks(text: string): MdBlock[] {
  const blocks: MdBlock[] = [];
  const lines = text.split("\n");
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block
    const fenceMatch = /^(`{3,}|~{3,})(\S*)/.exec(line);
    if (fenceMatch) {
      const fence = fenceMatch[1];
      const lang = fenceMatch[2] ?? "";
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith(fence)) {
        codeLines.push(lines[i]);
        i++;
      }
      blocks.push({ kind: "code", lang, content: codeLines.join("\n") });
      i++;
      continue;
    }

    // ATX heading
    const hm = /^(#{1,6})\s+(.*)$/.exec(line);
    if (hm) {
      blocks.push({ kind: "heading", level: hm[1].length, text: hm[2] });
      i++;
      continue;
    }

    // Horizontal rule (---, ***, ___)
    if (/^[-*_]{3,}\s*$/.test(line) && new Set(line.trim().split("")).size === 1) {
      blocks.push({ kind: "hr" });
      i++;
      continue;
    }

    // Table: first line starts with |
    if (line.trimStart().startsWith("|")) {
      const tableLines: string[] = [];
      while (i < lines.length && lines[i].trimStart().startsWith("|")) {
        tableLines.push(lines[i]);
        i++;
      }
      const parseRow = (r: string): string[] =>
        r.split("|").slice(1, -1).map((c) => c.trim());
      const isSeparator = (r: string): boolean => /^[\s|:-]+$/.test(r);
      const hasHeader = tableLines.length >= 2 && isSeparator(tableLines[1]);
      const headers = parseRow(tableLines[0]);
      const dataRows = (hasHeader ? tableLines.slice(2) : tableLines.slice(1)).map(parseRow);
      blocks.push({ kind: "table", headers, rows: dataRows });
      continue;
    }

    // Unordered list
    if (/^[ \t]*[-*+] /.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^[ \t]*[-*+] /.test(lines[i])) {
        items.push(lines[i].replace(/^[ \t]*[-*+] /, ""));
        i++;
      }
      blocks.push({ kind: "list", items });
      continue;
    }

    // Empty line
    if (line.trim() === "") {
      i++;
      continue;
    }

    // Paragraph — collect until blank or special line
    const paraLines: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !/^(`{3,}|~{3,})/.test(lines[i]) &&
      !lines[i].trimStart().startsWith("|") &&
      !/^#{1,6}\s/.test(lines[i]) &&
      !/^[ \t]*[-*+] /.test(lines[i]) &&
      !/^[-*_]{3,}\s*$/.test(lines[i])
    ) {
      paraLines.push(lines[i]);
      i++;
    }
    if (paraLines.length > 0) {
      blocks.push({ kind: "paragraph", text: paraLines.join("\n") });
    }
  }

  return blocks;
}

function MarkdownContent({ text }: { text: string }): React.ReactElement {
  const blocks = parseMdBlocks(text);
  return (
    <div className="md-body">
      {blocks.map((block, bi) => {
        const key = `b${bi}`;
        if (block.kind === "code") {
          return (
            <pre key={key} className="md-pre">
              <code>{block.content}</code>
            </pre>
          );
        }
        if (block.kind === "heading") {
          const Tag = HEADING_TAGS[Math.min(block.level - 1, 5)];
          return <Tag key={key} className={`md-h md-h${block.level}`}>{renderInline(block.text, key)}</Tag>;
        }
        if (block.kind === "hr") {
          return <hr key={key} className="md-hr" />;
        }
        if (block.kind === "table") {
          return (
            <div key={key} className="md-table-wrap">
              <table className="md-table">
                {block.headers.length > 0 && (
                  <thead>
                    <tr>{block.headers.map((h, hi) => <th key={hi}>{renderInline(h, `${key}-h${hi}`)}</th>)}</tr>
                  </thead>
                )}
                <tbody>
                  {block.rows.map((row, ri) => (
                    <tr key={ri}>
                      {row.map((cell, ci) => <td key={ci}>{renderInline(cell, `${key}-r${ri}c${ci}`)}</td>)}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          );
        }
        if (block.kind === "list") {
          return (
            <ul key={key} className="md-ul">
              {block.items.map((item, ii) => <li key={ii}>{renderInline(item, `${key}-i${ii}`)}</li>)}
            </ul>
          );
        }
        if (block.kind === "paragraph") {
          return <p key={key} className="md-p">{renderInline(block.text, key)}</p>;
        }
        return null;
      })}
    </div>
  );
}

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
import { DecisionCard } from "./DecisionCard";
import { TranslatePopup } from "./TranslatePopup";
import {
  ArrowRightIcon,
  BanIcon,
  CheckIcon,
  CloseIcon,
  CopyIcon,
  DisclosureCaret,
  ExternalIcon,
  GearIcon,
  MinusIcon,
  PaperclipIcon,
  PencilIcon,
  PlusIcon,
  SpinnerIcon,
} from "@/components/icons";

const shortName = (path: string): string => path.split("/").pop() ?? path;

const TOOL_ICON: Record<string, React.ReactElement> = {
  running: <SpinnerIcon size={11} className="spin-icon" />,
  success: <CheckIcon size={11} />,
  failed: <CloseIcon size={11} />,
  cancelled: <BanIcon size={11} />,
};
const FILE_ICON: Record<string, React.ReactElement> = {
  created: <PlusIcon size={11} />,
  modified: <PencilIcon size={11} />,
  deleted: <MinusIcon size={11} />,
  renamed: <ArrowRightIcon size={11} />,
};

import { buildTimelineGroups, type ApprovalItem, type QuestionItem, type TimelineGroup, type ToolItem } from "./timelineGrouping";

export function shouldShowAgentTimelineHeader(activeAgentRunId: string | undefined, mainRunId: string | undefined, agentRunCount: number): boolean {
  return Boolean(activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId) || agentRunCount > 0;
}

/**
 * Whether the "back to main agent" crumb should show the green pulsing "live
 * child run" badge. Mirrors AgentsPanel's own active/closed split (status !==
 * completed/failed/cancelled) so the crumb never disagrees with the sidebar's
 * own static "done" card for the same run — before this, the crumb pulsed
 * green forever regardless of the focused child's actual status.
 */
export function isFocusedChildLive(focusedRun: AgentRunSummary | undefined): boolean {
  return !focusedRun || (focusedRun.status !== "completed" && focusedRun.status !== "failed" && focusedRun.status !== "cancelled");
}

/**
 * A child with a persisted lifecycle card is already represented in the visible
 * timeline. Keep the live banner for children whose card is paged out, but do
 * not render the same live child twice in the current view.
 */
export function liveAgentRunsWithoutVisibleCard(
  agentRuns: AgentRunSummary[],
  visibleTimeline: TimelineItem[],
): AgentRunSummary[] {
  const representedRunIDs = new Set(
    visibleTimeline
      .filter((item): item is Extract<TimelineItem, { kind: "agent" }> => item.kind === "agent")
      .map((item) => item.childRunId),
  );
  return agentRuns.filter(
    (run) =>
      (run.status === "running" || run.status === "waiting_approval" || run.status === "waiting_question") &&
      !representedRunIDs.has(run.runId),
  );
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
        {copied ? <CheckIcon size={12} /> : <CopyIcon size={12} />}
      </button>
      {children}
    </div>
  );
}

const PROMPT_MAX_LINES = 4;
const PROMPT_ELLIPSIS = "....";

function PromptCard({ it }: { it: Extract<TimelineItem, { kind: "prompt" }> }): React.ReactElement {
  const [expanded, setExpanded] = useState(false);
  const [truncated, setTruncated] = useState(false);
  const textRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = textRef.current;
    if (!el) return;
    const check = () => setTruncated(el.scrollHeight > el.clientHeight + 1);
    check();
    const ro = new ResizeObserver(check);
    ro.observe(el);
    return () => ro.disconnect();
  }, [it.text]);

  const toggle = (event: React.MouseEvent) => {
    const target = event.target as HTMLElement;
    if (target.closest(".bubble-copy") || target.closest(".prompt-skills-summary")) return;
    setExpanded((value) => !value);
  };

  const clamped = truncated && !expanded;

  return (
    <div
      className={`prompt-stack ${truncated ? "prompt-truncatable" : ""} ${expanded ? "prompt-expanded" : ""}`}
      onClick={toggle}
    >
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
                <PaperclipIcon size={10} /> {att.originalName}
              </span>
            ),
          )}
        </div>
      )}
      <CopyBubble text={it.text} className="bubble prompt">
        <div
          ref={textRef}
          className={`prompt-text ${clamped ? "prompt-text-clamped" : ""}`}
          style={clamped ? { WebkitLineClamp: PROMPT_MAX_LINES } : undefined}
        >
          <MentionText text={it.text} skillNames={it.selectedSkills} />
        </div>
        {clamped && <span className="prompt-ellipsis">{PROMPT_ELLIPSIS}</span>}
      </CopyBubble>
      {it.selectedSkills && it.selectedSkills.length > 0 && (
        <PromptSkillsSummary skills={it.selectedSkills} />
      )}
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
        <span className="prompt-skills-caret" aria-hidden="true"><DisclosureCaret open={open} /></span>
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
  const isSpawn = it.toolName === "spawn_agent";
  if (isSpawn) {
    return (
      <div className="toolrow spawn">
        <GearIcon size={11} /> <b>spawn_agent</b>({label})
      </div>
    );
  }
  return (
    <div className={`row tool-row tool-${status}`}>
      <span className="row-icon">{TOOL_ICON[status] ?? null}</span>
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
        <span className="tool-group-caret"><DisclosureCaret open={open} /></span>
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

// Grouped ask UI: when several approvals are outstanding at once, show one collapsible
// group with a header bulk action ("Approve all" / "Deny all") plus every individual
// card still expandable and independently actionable underneath.
function ApprovalGroup({ items }: { items: ApprovalItem[] }): React.ReactElement {
  const approve = useStore((s) => s.approve);
  const [open, setOpen] = useState(false);
  // BUG-182: once every approval in the group has been decided, hide the bulk
  // action buttons and switch the label to a resolved state — leaving "Approve
  // all / Deny all" visible after the user already approved all read as if the
  // action didn't take.
  const unresolved = items.filter((item) => item.decision === undefined);
  const label =
    unresolved.length > 0 ? `${unresolved.length} approvals required` : `${items.length} approvals resolved`;

  const bulkDecide = (decision: "approve" | "deny") => {
    for (const item of items) {
      if (item.decision === undefined && item.details.decisions.some((d) => d.value === decision)) {
        void approve(item.approvalId, decision);
      }
    }
  };

  const resolved = unresolved.length === 0;
  return (
    <div className={`card-group approval-group ${resolved ? "approval-group-resolved" : "approval-group-pending"}`}>
      <div className="card-group-head">
        <button
          type="button"
          className={`card-group-summary approval-group-summary ${open ? "card-group-summary-open" : ""}`}
          aria-expanded={open}
          aria-label={label}
          title={label}
          onClick={() => setOpen((value) => !value)}
        >
          <span className="card-group-caret" aria-hidden="true"><DisclosureCaret open={open} /></span>
          <span className={`approval-group-label ${resolved ? "is-resolved" : "is-pending"}`}>{label}</span>
        </button>
        {unresolved.length > 0 && (
          <div className="card-group-bulk-actions">
            <button type="button" className="btn btn-primary" onClick={() => bulkDecide("approve")}>
              Approve all
            </button>
            <button type="button" className="btn btn-danger" onClick={() => bulkDecide("deny")}>
              Deny all
            </button>
          </div>
        )}
      </div>
      {open && (
        <div className="card-group-body">
          {items.map((item) => (
            <ApprovalCard key={item.approvalId} approvalId={item.approvalId} details={item.details} decision={item.decision} />
          ))}
        </div>
      )}
    </div>
  );
}

// Grouped ask UI for questions. Options can differ per question (arbitrary choice
// sets), so — unlike approvals — there is no generic single-click bulk action; the
// group only folds the cards visually while keeping each one individually answerable.
function QuestionGroup({ items }: { items: QuestionItem[] }): React.ReactElement {
  const [open, setOpen] = useState(false);
  const label = `${items.length} questions pending`;

  return (
    <div className="card-group question-group">
      <button
        type="button"
        className={`card-group-summary ${open ? "card-group-summary-open" : ""}`}
        aria-expanded={open}
        aria-label={label}
        title={label}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="card-group-caret"><DisclosureCaret open={open} /></span>
        <span className="badge badge-ask">{label}</span>
      </button>
      {open && (
        <div className="card-group-body">
          {items.map((item) => (
            <QuestionCard
              key={item.questionId}
              questionId={item.questionId}
              prompt={item.prompt}
              options={item.options}
              multiSelect={item.multiSelect}
              answer={item.answer}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function FileRow({ it }: { it: Extract<TimelineItem, { kind: "file" }> }): React.ReactElement {
  const openInIde = useStore((s) => s.openInIde);
  return (
    <button className="row file-row" title={it.path} onClick={() => openInIde(it.path)}>
      <span className="row-icon">{FILE_ICON[it.changeType ?? "modified"] ?? <PencilIcon size={11} />}</span>
      <span className="row-main">
        <span className="file-name">{shortName(it.path)}</span>
        <span className="file-change">{it.changeType ?? "modified"}</span>
      </span>
      <span className="row-hint">open in IDE <ExternalIcon size={9} /></span>
    </button>
  );
}

function AgentTimelineCard({ it }: { it: Extract<TimelineItem, { kind: "agent" }> }): React.ReactElement {
  const agentRuns = useStore((s) => s.agentRuns);
  const focusAgentRun = useStore((s) => s.focusAgentRun);
  const run = agentRuns.find((candidate) => candidate.runId === it.childRunId);
  // Prefer finalMessage for *this activation's* card: reinvoke reuses childRunId so
  // agentRuns.status flips back to running on round 2, but the round-1 card must
  // stay "completed" (run-9034 multi-activation cards share one run summary).
  const status = it.finalMessage
    ? "completed"
    : (run?.status ?? "running");
  const lowerName = it.agentName.toLowerCase();
  const roleClass = lowerName.includes("coder") ? "coder" : lowerName.includes("review") ? "reviewer" : lowerName.includes("test") ? "tester" : "";
  const provider = providerLabel(run?.providerKey ?? "");

  return (
    <div className={`abanner ${roleClass} agent-timeline-card`}>
      <span className={`pulse ${status === "completed" ? "done" : ""}`} />
      <span>
        <b>{run?.label ?? it.agentName}</b> · {provider} · {status}
        {run?.modelName && <span style={{ color: "var(--text-dim)" }}> · {run.modelName}</span>}
        {it.finalMessage && <span className="agent-timeline-result"> — completed</span>}
      </span>
      <button type="button" className="abanner-open-btn" onClick={() => void focusAgentRun(it.childRunId)}>
        Open <ExternalIcon size={9} />
      </button>
    </div>
  );
}

function Item({ it }: { it: TimelineGroup }): React.ReactElement | null {
  switch (it.kind) {
    case "assistant":
      return (
        <CopyBubble text={it.text} className={`bubble assistant ${it.finalized ? "final" : "streaming"}`}>
          <MarkdownContent text={it.text} />
          {!it.finalized && <span className="caret">▌</span>}
        </CopyBubble>
      );
    case "prompt":
      return <PromptCard it={it} />;
    case "thinking":
      return <div className="system-line thinking">{it.text}</div>;
    case "tool":
      return <ToolRow it={it} />;
    case "tool-group":
      return <ToolGroup tools={it.tools} />;
    case "approval-group":
      return <ApprovalGroup items={it.items} />;
    case "question-group":
      return <QuestionGroup items={it.items} />;
    case "file":
      return <FileRow it={it} />;
    case "agent":
      return <AgentTimelineCard it={it} />;
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
    case "decision_card":
      return <DecisionCard itemId={it.id} card={it.card} chosenOptionId={it.chosenOptionId} />;
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
  const focusAgentRun = useStore((s) => s.focusAgentRun);
  const endRef = useRef<HTMLDivElement>(null);
  const timelineRef = useRef<HTMLDivElement>(null);

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
  const timelineHasOlder = useStore((s) => s.timelineHasOlder);
  const timelineLoadingEarlier = useStore((s) => s._timelineLoadingEarlier);
  const loadEarlierTimeline = useStore((s) => s.loadEarlierTimeline);
  const visibleTimeline = useMemo(
    () => sliceTimelineFromPrompt(timeline, visiblePromptCount),
    [timeline, visiblePromptCount],
  );
  const timelineGroups = useMemo(() => buildTimelineGroups(visibleTimeline), [visibleTimeline]);
  const liveAgentRuns = liveAgentRunsWithoutVisibleCard(agentRuns, visibleTimeline);
  const showAgentHeader = shouldShowAgentTimelineHeader(activeAgentRunId, mainRunId, agentRuns.length);

  // Long-chat autoscroll: only pin to the bottom while the user is already
  // near it — a smooth scrollIntoView on every streamed token both thrashes
  // layout and yanks the view away from anyone reading earlier output.
  // Sending a new prompt re-engages the pin so the user's own message is seen.
  const stickToBottomRef = useRef(true);
  const handleTimelineScroll = () => {
    const el = timelineRef.current;
    if (!el) return;
    stickToBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };
  useEffect(() => {
    if (timeline[timeline.length - 1]?.kind === "prompt") {
      stickToBottomRef.current = true;
    }
    if (!stickToBottomRef.current) return;
    // Defer scroll one rAF so any layout shift from pagination (e.g. "Load earlier"
    // button inserted at the top when a gate reprompt pushes totalPromptCount over
    // TIMELINE_PAGE_SIZE) is fully committed before we measure the scroll target. (BUG-146)
    const id = requestAnimationFrame(() => {
      endRef.current?.scrollIntoView({ behavior: "auto", block: "end" });
    });
    return () => cancelAnimationFrame(id);
  }, [timeline]);

  return (
    <div
      ref={timelineRef}
      className={`timeline ${activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? "timeline-agent-focused" : ""}`}
      onScroll={handleTimelineScroll}
    >
      <TranslatePopup containerRef={timelineRef} />
      {activeAgentRunId && mainRunId && activeAgentRunId !== mainRunId ? (
        <div className="crumb ring">
          <button type="button" className="crumb-backbtn" onClick={backToMainRun}>
            ← Back to main agent
          </button>
          <span className="crumb-path">
            <b>main</b> <span style={{ opacity: 0.5 }}>›</span> <span className="here">{agentRuns.find(r => r.runId === activeAgentRunId)?.agentName ?? activeAgentRunId}</span>
          </span>
          <span className="crumb-path" style={{ marginLeft: "auto", display: "inline-flex", alignItems: "center", gap: "6px" }}>
            {(() => {
              const focusedRun = agentRuns.find((r) => r.runId === activeAgentRunId);
              if (isFocusedChildLive(focusedRun)) {
                return (
                  <>
                    <span className="pulse" /> live child run
                  </>
                );
              }
              return (
                <>
                  <span className={`sd ${focusedRun!.status === "failed" ? "fail" : "closed"}`} /> {focusedRun!.status}
                </>
              );
            })()}
          </span>
        </div>
      ) : null}
      {timeline.length === 0 && <div className="empty">Select a project, choose your chat controls, and send a prompt to begin.</div>}
      {(hiddenPromptCount > 0 || timelineHasOlder) && (
        <button
          type="button"
          className="load-earlier-btn"
          disabled={timelineLoadingEarlier}
          onClick={() => {
            // In-memory prompts first; when the window's retained prompts are
            // all visible, fall through to server-side backward paging (T-421).
            if (hiddenPromptCount > 0) {
              setVisiblePromptCount((current) =>
                Math.min(totalPromptCount, current + TIMELINE_PAGE_SIZE),
              );
            } else {
              void loadEarlierTimeline();
            }
          }}
        >
          {timelineLoadingEarlier
            ? "Loading…"
            : hiddenPromptCount > 0
              ? `↑ Load earlier prompts (${hiddenPromptCount})`
              : "↑ Load earlier history"}
        </button>
      )}
      {timelineGroups.map((it) => (
        <Item key={it.id} it={it} />
      ))}

      {(!activeAgentRunId || activeAgentRunId === mainRunId) &&
        liveAgentRuns
          .map((run) => {
            const lowerName = run.agentName.toLowerCase();
            const roleClass = lowerName.includes("coder") ? "coder" : lowerName.includes("review") ? "reviewer" : lowerName.includes("test") ? "tester" : "";
            const roleColor = roleClass === "coder" ? "var(--role-coder)" : roleClass === "reviewer" ? "var(--role-reviewer)" : roleClass === "tester" ? "var(--role-tester)" : "var(--text)";
            const isWaiting = run.status === "waiting_approval" || run.status === "waiting_question";
            const providerName = providerLabel(run.providerKey ?? "");
            return (
              <div key={run.runId} className={`abanner ${roleClass}`}>
                <span className={`pulse ${isWaiting ? "amber" : ""}`} />
                <span>
                  <b style={{ color: roleColor }}>{run.agentName}</b> · {providerName} · {run.status}
                  {run.agentStatus && <span style={{ color: "var(--text-dim)" }}> — {run.agentStatus}</span>}
                </span>
                <button type="button" className="abanner-open-btn" onClick={() => void focusAgentRun(run.runId)}>
                  Open <ExternalIcon size={9} />
                </button>
              </div>
            );
          })}

      <div ref={endRef} />
    </div>
  );
}
