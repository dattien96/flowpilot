import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  Play,
  X,
  ShieldAlert,
  Check,
  RefreshCw,
  ExternalLink,
  MoreVertical,
  FileText,
  Copy,
  ChevronUp,
  ChevronDown,
} from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GetWorkflowRunDetailUseCase } from "@/domain/usecase/workflow-runs/get-workflow-run-detail-usecase";
import { GetWorkflowRunDetailUseCase as GetWorkflowEngineRunDetailUseCase } from "@/domain/usecase/workflow-engine/get-workflow-run-detail-usecase";
import { ListArtifactRunsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-runs-usecase";
import { SubmitStepApprovalDecisionUseCase } from "@/domain/usecase/workflow-engine/submit-step-approval-decision-usecase";
import { getBrowserSupabaseClient } from "@/data/datasource/supabase/client";
import { applyOptimisticWorkflowFollowUp } from "@/features/workflow-engine/workflow-run-detail-optimistic";
import {
  buildWorkflowStepSessionGroups,
  groupOutputsByStep,
  mapArtifactsToWorkflowOutputs,
  mergeWorkflowOutputs,
  type WorkflowOutputRecord,
  type WorkflowStepSessionGroup,
  type WorkflowStepSessionItem,
  type WorkflowStepSessionStart,
} from "@/features/workflow-engine/workflow-run-detail-timeline";
import {
  canCancelWorkflowRun,
  canResumeWorkflowRun,
  getInterruptedWorkflowRunStepIds,
  INTERRUPTED_RUN_ERROR,
  isInterruptedWorkflowStep,
} from "@/features/workflow-engine/workflow-run-interruption";
import {
  getExpiredWorkflowRunSessionIds,
  markWorkflowRunSessionsCompleted,
} from "@/features/workflow-engine/workflow-run-session-timeout";
import { getMissingWorkflowRunSessionIds } from "@/features/workflow-engine/workflow-run-session-liveness";
import {
  buildFallbackApprovalDecisionsFromLogs,
  buildFallbackOutputsFromLogs,
  extractBeginPromptFromLogs,
} from "@/features/workflow-engine/workflow-run-log-fallback";
import {
  buildLiveThinkingLines,
  extractProviderStreamDisplayText,
  extractSkillAuditEntries,
  findNextPromptCreatedAt,
  getPromptWindowThinkingLines,
  isLiveThinkingPrompt,
  parseProviderStreamPayload,
} from "@/features/workflow-engine/workflow-run-thinking";
import {
  appendSyntheticResultSummarySteps,
  isResultSummaryStepType,
  RESULT_SUMMARY_STEP_NAME,
} from "@/features/workflow-engine/workflow-result-summary";
import type { ApprovalDecision } from "@/domain/model/entity/workflow";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { ArtifactRun, WorkflowRunSession } from "@/domain/model/entity/workflow-engine";
import { resolveArtifactRunForOutput } from "@/lib/workflow-run-artifact-match";
import { loadWorkflowRunPromptText } from "@/lib/workflow-run-prompt";
import {
  buildWorkflowRunArtifactOpenHref,
  loadWorkflowRunArtifactPromptFiles,
} from "@/lib/workflow-run-artifact-open";
import { openMarkdownPreviewInNewTab } from "@/lib/markdown-preview";

function summarizeRunPrompt(promptText?: string) {
  const normalized = promptText?.trim() ?? "";
  if (!normalized) {
    return null;
  }

  const beginPromptMarker = "## Begin Prompt";
  const inputArtifactsMarker = "## Input Artifacts";

  const beginPromptIndex = normalized.indexOf(beginPromptMarker);
  if (beginPromptIndex >= 0) {
    const afterMarker = normalized
      .slice(beginPromptIndex + beginPromptMarker.length)
      .trim();
    const nextSectionIndex = afterMarker.indexOf(inputArtifactsMarker);
    const beginPromptSection =
      nextSectionIndex >= 0
        ? afterMarker.slice(0, nextSectionIndex).trim()
        : afterMarker;
    const singleLinePrompt = beginPromptSection.replace(/\s+/g, " ").trim();
    if (singleLinePrompt) {
      return singleLinePrompt.length > 88
        ? `${singleLinePrompt.slice(0, 85).trimEnd()}...`
        : singleLinePrompt;
    }
  }

  const firstLine = normalized
    .split("\n")
    .map((line) => line.trim())
    .find(Boolean);
  if (!firstLine) {
    return null;
  }

  return firstLine.length > 88
    ? `${firstLine.slice(0, 85).trimEnd()}...`
    : firstLine;
}

function isGoogleDriveMcpApprovalMessage(message?: string | null) {
  const normalized = message?.trim().toLowerCase() ?? "";
  return normalized.includes("google drive mcp approval required") ||
    normalized.includes("google drive write approval required") ||
    normalized.includes("mcp_tool_approval_required") ||
    normalized.includes("mcp_write_approval_required");
}

function isWorkflowStepWaitingForApproval(status?: string | null) {
  const normalized = String(status ?? "").trim().toUpperCase();
  return normalized === "WAITING_USER_APPROVAL" || normalized === "WAITING_APPROVAL";
}

function normalizePromptDisplay(promptText?: string | null) {
  const normalized = promptText?.trim() ?? "";
  if (!normalized) {
    return "";
  }

  const sections = normalized.split(/\n(?=## )/);
  const cleanedSections = sections.filter((section, index) => {
    if (index === 0 && !section.startsWith("## ")) {
      return true;
    }

    const lines = section.trim().split("\n");
    const body = lines.slice(1).join("\n").trim();
    if (!body) {
      return false;
    }

    const compactBody = body.replace(/\s+/g, " ").trim().toLowerCase();
    return (
      compactBody !== "- none" &&
      compactBody !== "- no artifact output configured for this step."
    );
  });

  return cleanedSections.join("\n\n").trim();
}

function normalizeFollowUpComment(comment: string | null | undefined) {
  return (comment ?? "").trim();
}

function mergeApprovalDecisions(
  baseDecisions: ApprovalDecision[],
  overlayDecisions: ApprovalDecision[],
) {
  const merged = new Map<string, ApprovalDecision>();

  for (const decision of baseDecisions) {
    merged.set(decision.id, decision);
  }

  for (const decision of overlayDecisions) {
    const hasMatchingDecision = Array.from(merged.values()).some(
      (candidate) =>
        candidate.workflowStepId === decision.workflowStepId &&
        candidate.decision === decision.decision &&
        normalizeFollowUpComment(candidate.comment) ===
        normalizeFollowUpComment(decision.comment),
    );

    if (!hasMatchingDecision) {
      merged.set(decision.id, decision);
    }
  }

  return Array.from(merged.values()).sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt),
  );
}

function pruneResolvedOptimisticFollowUps(
  optimisticDecisions: ApprovalDecision[],
  persistedDecisions: ApprovalDecision[],
  outputs: WorkflowOutputRecord[],
) {
  return optimisticDecisions.filter((decision) => {
    const hasPersistedMatch = persistedDecisions.some(
      (candidate) =>
        candidate.workflowStepId === decision.workflowStepId &&
        candidate.decision === decision.decision &&
        normalizeFollowUpComment(candidate.comment) ===
        normalizeFollowUpComment(decision.comment),
    );
    if (hasPersistedMatch) {
      return false;
    }

    const hasNewerOutput = outputs.some(
      (output) =>
        output.workflowStepId === decision.workflowStepId &&
        output.createdAt >= decision.createdAt,
    );

    return !hasNewerOutput;
  });
}

function CollapsibleSection({
  title,
  subtitle,
  defaultOpen = false,
  children,
}: {
  title: string;
  subtitle?: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  return (
    <details
      className="rounded-[1.6rem] border border-border bg-background/50 shadow-md backdrop-blur-md"
      open={defaultOpen}
    >
      <summary className="flex cursor-pointer list-none items-center justify-between gap-4 px-6 py-5">
        <div>
          <h2 className="text-xl font-bold tracking-tight">{title}</h2>
          {subtitle ? (
            <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>
          ) : null}
        </div>
        <span className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">
          Expand
        </span>
      </summary>
      <div className="border-t border-border/60 px-6 py-5">{children}</div>
    </details>
  );
}

function CollapsibleTextBlock({
  title,
  value,
  emptyLabel,
  defaultOpen = false,
}: {
  title: string;
  value?: string | null;
  emptyLabel: string;
  defaultOpen?: boolean;
}) {
  const normalized = value?.trim() ?? "";

  return (
    <details
      className="rounded-2xl border border-border/60 bg-card/50"
      open={defaultOpen && normalized.length > 0}
    >
      <summary className="cursor-pointer list-none px-4 py-3 text-sm font-semibold text-foreground">
        {title}
      </summary>
      <div className="border-t border-border/60 px-4 py-3">
        {normalized ? (
          <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-foreground">
            {normalized}
          </pre>
        ) : (
          <p className="text-sm italic text-muted-foreground">{emptyLabel}</p>
        )}
      </div>
    </details>
  );
}

function CollapsibleChatBubble({
  title,
  time,
  content,
  isSecondary = false,
  metaNote,
}: {
  title: string;
  time?: string;
  content: string;
  isSecondary?: boolean;
  metaNote?: string;
}) {
  const [isExpanded, setIsExpanded] = useState(false);
  const TRUNCATE_LENGTH = 400;
  const isLong = content.length > TRUNCATE_LENGTH;
  const displayContent = (!isExpanded && isLong) ? content.slice(0, TRUNCATE_LENGTH) + "..." : content;

  return (
    <div className="flex max-w-[85%] flex-col items-end gap-2">
      <div className={`w-full rounded-[1.6rem] px-6 py-4 shadow-sm ${isSecondary ? "bg-accent/90 text-accent-foreground" : "bg-accent text-accent-foreground"
        }`}>
        <p className="text-[10px] opacity-70 mb-2 font-mono tracking-widest uppercase flex items-center justify-between gap-4">
          <span className="flex items-center gap-1.5">{title}</span>
          {time ? <span className="text-[9px]">{time}</span> : null}
        </p>
        <div className="whitespace-pre-wrap text-sm leading-relaxed break-words font-sans">
          {displayContent}
        </div>
        {isLong && (
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="mt-2 text-[10px] font-bold uppercase tracking-wider underline opacity-85 hover:opacity-100 transition-opacity cursor-pointer block"
          >
            {isExpanded ? "Show less" : "Show more"}
          </button>
        )}
      </div>
      {metaNote ? (
        <div className="max-w-full px-1 text-right text-[8px] font-semibold tracking-[0.12em] text-amber-300/95">
          {metaNote}
        </div>
      ) : null}
    </div>
  );
}

function CollapsibleAttemptPanel({
  title,
  time,
  defaultOpen = true,
  children,
}: {
  title: string;
  time?: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="rounded-[1.25rem] border border-border/25 bg-[#0f1119]/70 shadow-sm">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-center justify-between gap-3 px-6 py-4 text-left"
      >
        <div className="flex items-center gap-3">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {title}
          </span>
          {time ? (
            <span className="text-[9px] font-mono text-muted-foreground/60">
              {time}
            </span>
          ) : null}
        </div>
        <span className="text-muted-foreground transition-colors shrink-0">
          {open ? (
            <ChevronUp className="h-4 w-4" />
          ) : (
            <ChevronDown className="h-4 w-4" />
          )}
        </span>
      </button>

      {open ? <div className="border-t border-border/20 px-6 py-4">{children}</div> : null}
    </div>
  );
}

const ArtifactContentViewer = ({ content }: { content: string }) => {
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const TRUNCATE_LENGTH = 350;

  const isLong = content.length > TRUNCATE_LENGTH;
  const displayContent = (!expanded && isLong) ? content.substring(0, TRUNCATE_LENGTH) : content;

  const handleCopy = () => {
    navigator.clipboard.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative rounded-xl border border-border/45 bg-[#090a0f] p-6 overflow-hidden">
      <button
        onClick={handleCopy}
        className="absolute top-4 right-4 text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-muted/10 z-10"
        title="Copy content"
      >
        {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
      </button>

      <article className={`whitespace-pre-wrap text-[#d1d5db] font-mono text-xs leading-relaxed break-words pr-8 ${!expanded && isLong ? "pb-12" : ""}`}>
        {displayContent}
      </article>

      {isLong && (
        <div className={`absolute bottom-0 left-0 right-0 flex items-end justify-center pb-3 pt-10 ${!expanded ? "bg-gradient-to-t from-[#090a0f] via-[#090a0f]/90 to-transparent h-20" : "relative h-auto pt-4 bg-none"
          }`}>
          <Button
            variant="ghost"
            onClick={() => setExpanded(!expanded)}
            className="text-[10px] text-accent hover:text-accent/80 font-bold uppercase tracking-wider bg-[#090a0f]/90 hover:bg-[#090a0f] border border-border/20 rounded-lg px-4 shadow-sm"
            style={{ fontSize: "10px" }}
          >
            {expanded ? "Show less" : "Show more"}
          </Button>
        </div>
      )}
    </div>
  );
};

function StepOutputTabs({
  artifactRun,
  output,
}: {
  artifactRun?: ArtifactRun | null;
  output: WorkflowOutputRecord;
}) {
  const [activeTab, setActiveTab] = useState<"response" | "prompt" | "artifact">("response");
  const [remotePromptText, setRemotePromptText] = useState<string | null>(null);
  const [promptLoading, setPromptLoading] = useState(false);
  const [copiedTab, setCopiedTab] = useState<"response" | "prompt" | null>(null);

  useEffect(() => {
    setRemotePromptText(null);
    setPromptLoading(false);
  }, [artifactRun?.id]);

  const tabs = [
    { key: "response" as const, label: "RESPONSE" },
    { key: "prompt" as const, label: "PROMPT" },
    ...(artifactRun ? [{ key: "artifact" as const, label: "ARTIFACT" }] : []),
  ];

  const normalizedActualPrompt = normalizePromptDisplay(
    output.actualPromptText ?? output.promptText,
  );
  const promptDisplayText = normalizedActualPrompt || remotePromptText || "";

  useEffect(() => {
    if (
      activeTab !== "prompt" ||
      normalizedActualPrompt ||
      remotePromptText !== null ||
      !artifactRun?.remotePath.trim()
    ) {
      return;
    }

    let cancelled = false;
    setPromptLoading(true);

    void loadWorkflowRunArtifactPromptFiles(artifactRun).then((promptFiles) => {
      if (cancelled) {
        return;
      }

      setRemotePromptText(
        normalizePromptDisplay(promptFiles.actualPromptText || promptFiles.promptText),
      );
      setPromptLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [
    activeTab,
    artifactRun,
    normalizedActualPrompt,
    remotePromptText,
  ]);

  const handleCopyTabContent = (tab: "response" | "prompt") => {
    const content =
      tab === "response"
        ? (output.contentMarkdown || "No response captured.")
        : (promptDisplayText || "No prompt was captured for this output.");
    navigator.clipboard.writeText(content);
    setCopiedTab(tab);
    setTimeout(() => setCopiedTab((current) => (current === tab ? null : current)), 2000);
  };

  return (
    <div className="space-y-5">
      <div className="flex border border-border/30 bg-[#090a0f] rounded-lg p-0.5 gap-1.5 w-fit shrink-0">
        {tabs.map((tab) => {
          const isActive = activeTab === tab.key;
          return (
            <button
              key={tab.key}
              style={{ fontSize: "10px" }}
              className={`px-3 py-1 font-extrabold uppercase tracking-wider text-center transition-all rounded-md relative ${isActive
                ? "text-emerald-400 bg-emerald-500/5 shadow-sm"
                : "text-muted-foreground hover:text-foreground"
                }`}
              onClick={() => setActiveTab(tab.key)}
              type="button"
            >
              {tab.label}
              {isActive && (
                <span className="absolute bottom-0 left-1/2 -translate-x-1/2 w-6 h-[1.5px] bg-emerald-400 rounded-full" />
              )}
            </button>
          );
        })}
      </div>

      {activeTab === "response" ? (
        <ArtifactContentViewer content={output.contentMarkdown || "No response captured."} />
      ) : null}

      {activeTab === "prompt" ? (
        <div className="relative rounded-xl border border-border/45 bg-[#090a0f] p-6 overflow-auto max-h-[28rem]">
          <button
            onClick={() => handleCopyTabContent("prompt")}
            className="absolute top-4 right-4 text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-muted/10 z-10"
            title="Copy prompt"
            type="button"
          >
            {copiedTab === "prompt" ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
          </button>
          <p className="mb-4 text-[10px] font-bold uppercase tracking-[0.22em] text-muted-foreground">
            Prompt Sent
          </p>
          {promptDisplayText ? (
            <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[#d1d5db]">
              {promptDisplayText}
            </pre>
          ) : promptLoading ? (
            <p className="text-xs italic text-muted-foreground">
              Loading prompt...
            </p>
          ) : (
            <p className="text-xs italic text-muted-foreground">
              No prompt was captured for this output.
            </p>
          )}
        </div>
      ) : null}

      {activeTab === "artifact" && artifactRun ? (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/45 bg-[#090a0f] p-5">
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-foreground">
                {artifactRun.title}
              </p>
              <p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">
                {artifactRun.remotePath.trim() || artifactRun.localPath}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2 shrink-0">
              <a
                className="inline-flex h-8 items-center justify-center px-4 text-xs font-bold uppercase tracking-wider bg-accent/10 border border-accent/20 hover:bg-accent/20 text-accent rounded-lg shadow-sm"
                href={buildWorkflowRunArtifactOpenHref(artifactRun)}
                rel="noreferrer"
                target="_blank"
              >
                <ExternalLink className="mr-2 h-3.5 w-3.5 text-sm" />
                Open in new tab
              </a>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function formatBubbleTime(value?: string | null) {
  const normalized = value?.trim() ?? "";
  if (!normalized) {
    return null;
  }

  const timestamp = new Date(normalized);
  if (Number.isNaN(timestamp.getTime())) {
    return null;
  }

  return timestamp.toLocaleTimeString();
}

type SessionRecoveryDisplay = {
  label: string;
  title: string;
};

function parseSessionEventPayload(message: string | null | undefined) {
  const normalized = message?.trim() ?? "";
  if (!normalized.startsWith("session_event:")) {
    return null;
  }

  try {
    return JSON.parse(normalized.slice("session_event:".length)) as {
      event?: string;
      providerSessionId?: string | null;
      recoveredFromSessionId?: string | null;
      recoveredFromProviderSessionId?: string | null;
      command?: string | null;
    };
  } catch {
    return null;
  }
}

function resolveSessionRecoveryDisplay(session: any): SessionRecoveryDisplay | null {
  const recoveryMode = session?.metadataJson?.recovery?.mode;
  if (recoveryMode === "bootstrap_replay") {
    return {
      label: "New Session + Replay",
      title: "A new session was created and previous checkpoints were replayed into the prompt.",
    };
  }

  if (recoveryMode === "resumed_thread") {
    return {
      label: "New Session",
      title: "A new session was created for this step after the previous session ended.",
    };
  }

  return null;
}

function getSessionsForScope(
  sessions: any[] | null | undefined,
  selectedStepId: string | undefined,
  subagent: string | null,
) {
  if (!sessions || sessions.length === 0) {
    return [];
  }

  const scopedSessions = sessions.filter((session: any) => {
    if (subagent) {
      return session.metadataJson?.step_run_id === selectedStepId;
    }

    return (
      session.metadataJson?.is_main === true ||
      session.metadataJson?.is_main === "true"
    );
  });

  if (scopedSessions.length === 0) {
    return [];
  }

  return [...scopedSessions].sort((left: any, right: any) =>
    left.startedAt.localeCompare(right.startedAt),
  );
}

function getSessionScopeLabel(subagent: string | null) {
  return subagent ? `Isolated (${subagent})` : "Session Main";
}

function getSessionStartsForScope({
  logs,
  sessions,
  selectedStepId,
}: {
  logs: any[];
  sessions: any[];
  selectedStepId: string | undefined;
}): WorkflowStepSessionStart[] {
  const sessionStarts: WorkflowStepSessionStart[] = [];

  for (const log of logs) {
    if (selectedStepId && log.workflowRunStepId !== selectedStepId) {
      continue;
    }

    const payload = parseSessionEventPayload(log.message);
    if (!payload) {
      continue;
    }

    if (payload.event !== "session_created" && payload.event !== "session_reused") {
      continue;
    }

    const matchedSession =
      sessions.find(
        (session: any) =>
          payload.providerSessionId &&
          session.providerSessionId === payload.providerSessionId,
      ) ?? null;

    sessionStarts.push({
      createdAt: log.createdAt,
      providerSessionId: payload.providerSessionId ?? null,
      sessionId: matchedSession?.id ?? null,
    });
  }

  return sessionStarts.sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt),
  );
}

function DeveloperDiagnosticsSection({ output }: { output: WorkflowOutputRecord }) {
  const [open, setOpen] = useState(false);

  return (
    <div className="mt-4 border-t border-border/30 pt-4">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="text-[10px] inline-flex w-full cursor-pointer items-center justify-between font-bold uppercase tracking-[0.2em] text-muted-foreground transition-colors hover:text-foreground select-none"
      >
        <span className="text-[10px]">Developer Diagnostics</span>
        <span className="text-muted-foreground transition-colors shrink-0">
          {open ? (
            <ChevronUp className="h-4 w-4" />
          ) : (
            <ChevronDown className="h-4 w-4" />
          )}
        </span>
      </button>

      {open && (
        <div className="mt-4 space-y-3 border-l-2 border-border/50 pl-2">
          {output.localPath ? (
            <CollapsibleTextBlock title="Artifact Path" value={output.localPath} emptyLabel="" />
          ) : null}
          <CollapsibleTextBlock title="Planned Prompt" value={output.promptText} emptyLabel="Inherited from run prompt." />
          <CollapsibleTextBlock title="Actual Sent Prompt" value={output.actualPromptText ?? output.promptText} emptyLabel="No actual prompt captured." />
          <CollapsibleTextBlock title="Stdout" value={output.stdoutText} emptyLabel="No stdout." />
          <CollapsibleTextBlock title="Stderr" value={output.stderrText} emptyLabel="No stderr." />
          <CollapsibleTextBlock title="CLI Command" value={output.commandText} emptyLabel="No command captured." />
        </div>
      )}
    </div>
  );
}

function ThinkingPanel({
  lines,
  isLive,
}: {
  lines: string[];
  isLive: boolean;
}) {
  const [isExpanded, setIsExpanded] = useState(isLive);

  useEffect(() => {
    setIsExpanded(isLive);
  }, [isLive]);

  if (!isLive && lines.length === 0) {
    return null;
  }

  const visibleLines = isLive ? buildLiveThinkingLines(lines, 3) : lines;

  return (
    <div className="ml-auto flex w-full max-w-[85%] flex-col gap-2">
      <div className="rounded-[1.2rem] border border-border/35 bg-[#151924] px-5 py-4 shadow-sm">
        <button
          type="button"
          onClick={() => {
            if (!isLive) {
              setIsExpanded((current) => !current);
            }
          }}
          className={`flex w-full items-center justify-between gap-3 text-left ${isLive ? "cursor-default" : "cursor-pointer"}`}
        >
          <span className="text-[10px] font-bold uppercase tracking-[0.18em] text-muted-foreground">
            Thinking
          </span>
          {isLive ? (
            <span className="text-[10px] text-emerald-400">Live</span>
          ) : (
            <span className="text-muted-foreground">
              {isExpanded ? (
                <ChevronUp className="h-4 w-4" />
              ) : (
                <ChevronDown className="h-4 w-4" />
              )}
            </span>
          )}
        </button>

        {isExpanded ? (
          <div className="mt-3 space-y-2">
            {visibleLines.length > 0 ? (
              visibleLines.map((line, index) => (
                <p
                  key={`${index}-${line}`}
                  className="text-sm leading-6 text-[#d3dae6]"
                >
                  {`- ${line}`}
                </p>
              ))
            ) : (
              <p className="text-sm leading-6 text-muted-foreground">
                Waiting for stream...
              </p>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function SkillAuditPanel({
  skills,
  isLive,
}: {
  skills: string[];
  isLive: boolean;
}) {
  const [isExpanded, setIsExpanded] = useState(false);

  return (
    <div className="ml-auto flex w-full max-w-[85%] flex-col gap-2">
      <div className="rounded-[1.2rem] border border-amber-500/20 bg-[#171a20] px-5 py-4 shadow-sm">
        <button
          type="button"
          onClick={() => setIsExpanded((current) => !current)}
          className="flex w-full cursor-pointer items-center justify-between gap-3 text-left"
        >
          <span className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.18em] text-amber-300">
            Skill Audit
            <span className="rounded-full border border-amber-500/25 bg-amber-500/10 px-2 py-0.5 text-[9px] text-amber-200">
              {skills.length}
            </span>
          </span>
          <span className="text-muted-foreground">
            {isExpanded ? (
              <ChevronUp className="h-4 w-4" />
            ) : (
              <ChevronDown className="h-4 w-4" />
            )}
          </span>
        </button>

        {isExpanded ? (
          <div className="mt-3 space-y-2">
            {skills.length > 0 ? (
              skills.map((skill) => (
                <p
                  key={skill}
                  className="font-mono text-xs leading-5 text-[#e6d5a8]"
                >
                  {`- ${skill}`}
                </p>
              ))
            ) : (
              <p className="text-sm leading-6 text-muted-foreground">
                {isLive
                  ? "Waiting for skill calls..."
                  : "No skill calls detected for this prompt."}
              </p>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function CommandPanel({ commands }: { commands: string[] }) {
  const [isExpanded, setIsExpanded] = useState(commands.length > 0);
  const visibleCommands = commands.length > 0 ? commands : ["No provider command captured yet."];

  useEffect(() => {
    if (commands.length > 0) {
      setIsExpanded(true);
    }
  }, [commands.length]);

  return (
    <div className="ml-auto flex w-full max-w-[85%] flex-col gap-2">
      <div className="rounded-[1.2rem] border border-sky-500/20 bg-[#111923] px-5 py-4 shadow-sm">
        <button
          type="button"
          onClick={() => setIsExpanded((current) => !current)}
          className="flex w-full cursor-pointer items-center justify-between gap-3 text-left"
        >
          <span className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.18em] text-sky-300">
            Command
            <span className="rounded-full border border-sky-500/25 bg-sky-500/10 px-2 py-0.5 text-[9px] text-sky-200">
              {commands.length}
            </span>
          </span>
          <span className="text-muted-foreground">
            {isExpanded ? (
              <ChevronUp className="h-4 w-4" />
            ) : (
              <ChevronDown className="h-4 w-4" />
            )}
          </span>
        </button>

        {isExpanded ? (
          <div className="mt-3 space-y-2">
            {visibleCommands.map((command, index) => (
              <pre
                key={`${index}-${command}`}
                className="overflow-x-auto whitespace-pre-wrap break-words rounded-xl border border-sky-500/15 bg-black/25 px-3 py-2 font-mono text-xs leading-5 text-sky-100"
              >
                {command}
              </pre>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function commandLinesForPrompt({
  attempts,
  logs,
  promptCreatedAt,
  nextPromptCreatedAt,
}: {
  attempts: WorkflowStepSessionItem[];
  logs: Array<{ createdAt: string; message?: string | null }>;
  promptCreatedAt: string;
  nextPromptCreatedAt?: string | null;
}) {
  const seen = new Set<string>();
  const commands: string[] = [];
  const addCommand = (value: string | null | undefined) => {
    const command = value?.trim();
    if (!command || seen.has(command)) {
      return;
    }
    seen.add(command);
    commands.push(command);
  };

  for (const log of logs) {
    if (log.createdAt < promptCreatedAt) {
      continue;
    }
    if (nextPromptCreatedAt && log.createdAt >= nextPromptCreatedAt) {
      continue;
    }
    const payload = parseSessionEventPayload(log.message);
    if (payload?.event === "session_created" || payload?.event === "session_reused") {
      addCommand(payload.command);
    }
  }

  for (const attempt of attempts) {
    if (attempt.kind === "output") {
      addCommand(attempt.output.commandText);
    }
  }

  return commands;
}

function SessionGroupSection({
  group,
  subagent,
  stepOutputs,
  detail,
  gatewayBundle,
  onKillSession,
  stepLogs,
  stepCommandLogs,
  stepPromptCreatedAts,
  stepStatus,
}: {
  group: WorkflowStepSessionGroup;
  subagent: string | null;
  stepOutputs: WorkflowOutputRecord[];
  detail: any;
  gatewayBundle: React.MutableRefObject<ReturnType<typeof createGatewayBundle>>;
  onKillSession: (session: WorkflowRunSession) => Promise<void>;
  stepLogs: Array<{ createdAt: string; message?: string | null }>;
  stepCommandLogs: Array<{ createdAt: string; message?: string | null }>;
  stepPromptCreatedAts: string[];
  stepStatus: string;
}) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isKilling, setIsKilling] = useState(false);
  const session = group.session;
  const sessionRecovery = session ? resolveSessionRecoveryDisplay(session) : null;

  return (
    <section className="space-y-4">
      {session ? (
        <div
          onClick={() => setIsCollapsed(!isCollapsed)}
          className="group/session rounded-xl border border-border/40 bg-[#161d28]/35 px-5 py-4 cursor-pointer hover:bg-[#161d28]/55 transition-colors flex items-center justify-between gap-4 select-none shadow-sm"
        >
          <div className="flex flex-col gap-1.5">
            <p className="flex flex-wrap items-center gap-1.5 text-xs font-mono text-muted-foreground">
              <span className="font-bold text-foreground">Session:</span>
              <span className="bg-accent/40 text-accent-foreground px-1.5 py-0.5 rounded font-bold uppercase tracking-wider text-[9px]">
                {getSessionScopeLabel(subagent)}
              </span>
              <span>&bull;</span>
              <span className="text-foreground/90 font-medium font-sans">
                {session.provider} ({session.model})
              </span>
              <span>&bull;</span>
              <span className={`font-semibold ${session.status === "active" ? "text-success" : "text-muted-foreground"}`}>
                {session.status}
              </span>


              {sessionRecovery ? (
                <>
                  <span>&bull;</span>
                  <span
                    className="rounded-full border border-amber-500/20 bg-amber-500/10 px-2 py-0.5 text-[9px] font-bold uppercase tracking-wider text-amber-300"
                    title={sessionRecovery.title}
                  >
                    {sessionRecovery.label}
                  </span>
                </>
              ) : null}
            </p>
            {session.status === "active" && session.processKey != null ? (
              <p className="flex flex-wrap items-center gap-1.5 text-xs font-mono text-muted-foreground">
                <span className="font-mono text-[9px] text-foreground/80 flex items-center">
                  {session.processPid != null ? (
                    <>
                      <span className="text-muted-foreground/70 mr-1 font-semibold uppercase tracking-wider">PID</span>
                      {session.processPid}
                    </>
                  ) : (
                    <span className="text-muted-foreground/70 mr-1 font-semibold uppercase tracking-wider">Active Session</span>
                  )}
                  <Button
                    variant="secondary"
                    disabled={isKilling || !session.processKey}
                    className="h-5 px-2 hover:bg-destructive/10 text-[9px] font-bold uppercase tracking-wider text-destructive border border-destructive/40 hover:border-destructive/60 transition-colors ml-3"
                    onClick={async (e) => {
                      e.stopPropagation();
                      if (!session.processKey || isKilling) {
                        return;
                      }
                      const confirmMsg = session.processPid != null
                        ? `Are you sure you want to kill PID ${session.processPid}?`
                        : "Are you sure you want to terminate this active session?";
                      if (!window.confirm(confirmMsg)) {
                        return;
                      }
                      setIsKilling(true);
                      try {
                        await onKillSession(session);
                      } catch (error: any) {
                        console.error("Failed to manually kill workflow session:", error);
                        alert(error?.message ?? "Failed to kill the workflow session.");
                      } finally {
                        setIsKilling(false);
                      }
                    }}
                  >
                    {isKilling ? "Killing..." : "Kill Process"}
                  </Button>
                </span>
              </p>
            ) : null}

            {session.providerSessionId ? (
              <p className="flex flex-wrap items-center gap-1.5 text-xs font-mono text-muted-foreground">
                <span className="font-mono text-[9px] text-foreground/80">
                  <span className="text-muted-foreground/70 mr-1 font-semibold uppercase tracking-wider">Session ID</span>
                  {session.providerSessionId}
                </span>
              </p>
            ) : null}
          </div>
          <span className="text-muted-foreground group-hover/session:text-foreground transition-colors shrink-0">
            {isCollapsed ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronUp className="h-4 w-4" />
            )}
          </span>
        </div>
      ) : null}

      {!isCollapsed && (
        <div className="ml-4 space-y-6 border-l border-border/20 pl-5">
          {group.promptGroups.map((pg, index) => {
            const nextPromptCreatedAt = findNextPromptCreatedAt(
              pg.prompt.createdAt,
              stepPromptCreatedAts,
            );
            const thinkingLines = getPromptWindowThinkingLines({
              logs: stepLogs,
              promptCreatedAt: pg.prompt.createdAt,
              nextPromptCreatedAt,
            });
            const skillAuditEntries = extractSkillAuditEntries(thinkingLines);
            const commandLines = commandLinesForPrompt({
              attempts: pg.attempts,
              logs: stepCommandLogs,
              promptCreatedAt: pg.prompt.createdAt,
              nextPromptCreatedAt,
            });
            const isLiveThinking = isLiveThinkingPrompt({
              promptIndex: index,
              promptCount: group.promptGroups.length,
              sessionStatus: session?.status,
              stepStatus,
            });
            return (
              <div key={pg.key} className="space-y-5 bg-[#0f111a]/45 border border-border/10 p-6 rounded-2xl shadow-inner">
                {/* Prompt Chat Bubble */}
                <div className="flex justify-end">
                  <CollapsibleChatBubble
                    title={pg.prompt.title}
                    time={formatBubbleTime(pg.prompt.createdAt) ?? undefined}
                    content={pg.prompt.content}
                    isSecondary={pg.prompt.isSecondary}
                    metaNote={pg.prompt.metaNote}
                  />
                </div>

                <ThinkingPanel
                  lines={thinkingLines}
                  isLive={isLiveThinking}
                />

                <SkillAuditPanel
                  skills={skillAuditEntries}
                  isLive={isLiveThinking}
                />

                <CommandPanel commands={commandLines} />

                {/* Attempts/Outputs belonging to this Prompt */}
                {pg.attempts.length > 0 && (
                  <div className="space-y-4 pl-4 border-l border-emerald-500/25">
                    {pg.attempts.map((attemptItem) => {
                      const outputAttempt =
                        stepOutputs.findIndex((output) => output.id === attemptItem.output.id) + 1;
                      const itemArtifactRun = resolveArtifactRunForOutput(
                        attemptItem.output,
                        ((detail.artifactRuns ?? []) as ArtifactRun[]) ?? [],
                      );

                      return (
                        <div key={attemptItem.key} className="space-y-3">
                          <CollapsibleAttemptPanel
                            title={stepOutputs.length > 1 ? `Attempt ${outputAttempt}` : "Output"}
                            time={formatBubbleTime(attemptItem.output.createdAt) ?? undefined}
                          >
                            <div className="space-y-4">
                              <StepOutputTabs
                                artifactRun={itemArtifactRun}
                                output={attemptItem.output}
                              />

                              <DeveloperDiagnosticsSection output={attemptItem.output} />
                            </div>
                          </CollapsibleAttemptPanel>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}

export const Route = createFileRoute("/_authenticated/workflow-runs/$runId")({
  validateSearch: (search: Record<string, unknown>): { logView?: "session" } => ({
    logView: search.logView === "session" ? "session" : undefined,
  }),
  component: WorkflowRunDetailPage,
});

function WorkflowRunDetailPage() {
  const { runId } = Route.useParams();
  const { logView } = Route.useSearch();
  const gatewayBundle = useRef(createGatewayBundle());
  const getWorkflowRunDetailUseCase = useRef(
    new GetWorkflowRunDetailUseCase(gatewayBundle.current.workflowGateway),
  );
  const getWorkflowEngineRunDetailUseCase = useRef(
    new GetWorkflowEngineRunDetailUseCase(
      gatewayBundle.current.workflowEngineGateway,
    ),
  );
  const listArtifactRunsUseCase = useRef(
    new ListArtifactRunsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const submitStepApprovalDecisionUseCase = useRef(
    new SubmitStepApprovalDecisionUseCase(
      gatewayBundle.current.workflowEngineGateway,
    ),
  );

  const [detail, setDetail] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [decisionComment, setDecisionComment] = useState("");
  const [submittingDecision, setSubmittingDecision] = useState(false);
  const [processingAction, setProcessingAction] = useState(false);
  const [runPromptText, setRunPromptText] = useState<string | null>(null);
  const [optimisticFollowUps, setOptimisticFollowUps] = useState<
    ApprovalDecision[]
  >([]);
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  const [logsExpanded, setLogsExpanded] = useState(true);

  function getGatewayBundle() {
    return gatewayBundle.current;
  }

  const reconcileTimedOutSessions = async ({
    sessionIdleTtlMinutes,
    sessions,
  }: {
    sessionIdleTtlMinutes: number | null | undefined;
    sessions: WorkflowRunSession[];
  }) => {
    const expiredSessionIds = getExpiredWorkflowRunSessionIds({
      sessionIdleTtlMinutes,
      sessions,
    });
    if (expiredSessionIds.length === 0) {
      return sessions;
    }

    const completedAt = new Date().toISOString();
    const nextSessions = markWorkflowRunSessionsCompleted(
      sessions,
      expiredSessionIds,
      completedAt,
    );

    try {
      const supabase = await getBrowserSupabaseClient();
      const { error } = await supabase
        .from("workflow_run_sessions")
        .update({
          status: "completed",
          completed_at: completedAt,
          process_key: null,
        })
        .in("id", expiredSessionIds);
      if (error) {
        console.warn("Failed to reconcile timed-out workflow sessions:", error);
      }
    } catch (error) {
      console.warn("Failed to persist timed-out workflow session reconciliation:", error);
    }

    return nextSessions;
  };

  const reconcileLiveSessions = async ({
    sessions,
  }: {
    sessions: WorkflowRunSession[];
  }) => {
    if (sessions.length === 0) {
      return sessions;
    }

    let liveSessions: Array<{ processKey: string | null }> = [];
    try {
      liveSessions = await getGatewayBundle().localRunnerGateway.listSessions();
    } catch (error) {
      console.warn("Unable to load live runner sessions for detail reconciliation:", error);
      return sessions;
    }

    const staleSessionIds = getMissingWorkflowRunSessionIds({
      sessions,
      liveProcessKeys: liveSessions
        .map((session) => session.processKey ?? "")
        .filter((processKey) => processKey.length > 0),
    });
    if (staleSessionIds.length === 0) {
      return sessions;
    }

    const completedAt = new Date().toISOString();
    const staleSessions = sessions.filter((session) => staleSessionIds.includes(session.id));
    try {
      await Promise.allSettled(
        staleSessions.map((session) =>
          getGatewayBundle().localRunnerGateway.closeSession({
            transportType: session.transportType,
            providerSessionId: session.providerSessionId ?? "",
            processKey: session.processKey,
            processPid: session.processPid ?? null,
          }),
        ),
      );

      const supabase = await getBrowserSupabaseClient();
      const { error } = await supabase
        .from("workflow_run_sessions")
        .update({
          status: "completed",
          completed_at: completedAt,
          process_key: null,
          process_pid: null,
        })
        .in("id", staleSessionIds);
      if (error) {
        throw new Error(error.message);
      }
    } catch (error) {
      console.warn("Unable to reconcile stale workflow sessions in detail view:", error);
      return sessions;
    }

    return markWorkflowRunSessionsCompleted(
      sessions,
      staleSessionIds,
      completedAt,
    );
  };

  const handleKillSession = async (session: WorkflowRunSession) => {
    if (session.status !== "active") {
      return;
    }

    if (!session.processKey) {
      throw new Error("Session process key is missing. The runner cannot kill this session.");
    }

    const completedAt = new Date().toISOString();

    await getGatewayBundle().localRunnerGateway.closeSession({
      transportType: session.transportType,
      providerSessionId: session.providerSessionId ?? "",
      processKey: session.processKey,
      processPid: session.processPid ?? null,
    });

    const supabase = await getBrowserSupabaseClient();
    const { error } = await supabase
      .from("workflow_run_sessions")
      .update({
        status: "completed",
        completed_at: completedAt,
        process_key: null,
      })
      .eq("id", session.id);

    if (error) {
      throw new Error(`Failed to persist killed workflow session: ${error.message}`);
    }

    setDetail((previousDetail: any) =>
      previousDetail
        ? {
            ...previousDetail,
            sessions: markWorkflowRunSessionsCompleted(
              Array.isArray(previousDetail.sessions) ? previousDetail.sessions : [],
              [session.id],
              completedAt,
            ),
          }
        : previousDetail,
    );
  };

  const processDetailData = async (data: any) => {
    if (!data) return null;
    let mappedOutputs: WorkflowOutputRecord[] = [];
    try {
      const localArtifacts =
        await getGatewayBundle().localRunnerGateway.listArtifacts();
      const runArtifacts = localArtifacts.filter(
        (art) => art.workflowRunId === runId,
      );
      const artifactDetails = await Promise.all(
        runArtifacts.map(
          async (artifact) =>
            (await getGatewayBundle().localRunnerGateway.getArtifactById(
              artifact.artifactId,
            )) ?? artifact,
        ),
      );

      mappedOutputs = mapArtifactsToWorkflowOutputs(
        artifactDetails as LocalRunnerArtifact[],
        data.steps ?? [],
      );
    } catch (artifactErr) {
      console.warn(
        "Failed to fetch local runner artifacts, using DB outputs only:",
        artifactErr,
      );
    }

    const combinedOutputs = mergeWorkflowOutputs(
      (data.outputs ?? []) as WorkflowOutputRecord[],
      mappedOutputs,
    );

    return {
      ...data,
      outputs: combinedOutputs,
    };
  };

  const loadData = async () => {
    if (!runId) return;
    try {
      const [data, promptText, engineDetailRaw] = await Promise.all([
        getWorkflowRunDetailUseCase.current.execute(runId),
        loadWorkflowRunPromptText(
          getGatewayBundle().localRunnerGateway,
          runId,
        ),
        getWorkflowEngineRunDetailUseCase.current.execute(runId),
      ]);
      const engineDetail = engineDetailRaw as
        | { steps?: any[]; logs: any[]; sessions?: any[] | null }
        | null;
      if (data) {
        let processed = await processDetailData(data);
        const engineLogs = engineDetail?.logs ?? [];
        const promptFromLogs = extractBeginPromptFromLogs(engineLogs);
        const resolvedRunPromptText = promptFromLogs ?? promptText;
        let mergedSteps = appendSyntheticResultSummarySteps({
          baseSteps: processed.steps ?? [],
          engineSteps: (engineDetail?.steps ?? []) as any[],
          runId,
        });
        const artifactRuns = await listArtifactRunsUseCase.current.execute(
          processed.run.projectId,
        );
        const stepArtifactRuns = artifactRuns.filter(
          (artifactRun) => artifactRun.workflowRunId === runId,
        );
        const logBackedOutputs = processed
          ? buildFallbackOutputsFromLogs({
            existingOutputs:
              (processed.outputs ?? []) as WorkflowOutputRecord[],
            logs: engineLogs,
            projectId: processed.run.projectId,
            runId,
            steps: mergedSteps,
          })
          : [];
        const logBackedDecisions = buildFallbackApprovalDecisionsFromLogs(
          engineLogs,
        );
        const mergedApprovalDecisions = mergeApprovalDecisions(
          ((processed?.approvalDecisions ?? []) as ApprovalDecision[]) ?? [],
          logBackedDecisions,
        );
        const mergedOutputs = processed
          ? mergeWorkflowOutputs(
            (processed.outputs ?? []) as WorkflowOutputRecord[],
            logBackedOutputs,
          )
          : [];
        let mergedSessions = await reconcileTimedOutSessions({
          sessionIdleTtlMinutes: processed?.project?.sessionIdleTtlMinutes ?? null,
          sessions: (
            (engineDetail?.sessions ?? processed.sessions ?? []) as WorkflowRunSession[]
          ),
        });
        mergedSessions = await reconcileLiveSessions({
          sessions: mergedSessions,
        });

        const interruptedStepIds = getInterruptedWorkflowRunStepIds({
          run: processed?.run,
          steps: mergedSteps,
          sessions: mergedSessions,
        });
        if (processed && interruptedStepIds.length > 0) {
          const failedAt = new Date().toISOString();
          try {
            const supabase = await getBrowserSupabaseClient();
            const { error: stepsError } = await supabase
              .from("workflow_run_steps")
              .update({
                status: "FAILED",
                finished_at: failedAt,
                error_message: INTERRUPTED_RUN_ERROR,
              })
              .in("id", interruptedStepIds);
            if (stepsError) {
              throw new Error(stepsError.message);
            }

            const { error: runError } = await supabase
              .from("workflow_runs")
              .update({
                status: "FAILED",
                finished_at: failedAt,
                error_message: INTERRUPTED_RUN_ERROR,
              })
              .eq("id", processed.run.id);
            if (runError) {
              throw new Error(runError.message);
            }
          } catch (error) {
            console.warn("Unable to reconcile interrupted workflow run:", error);
          }

          mergedSteps = mergedSteps.map((step: any) =>
            interruptedStepIds.includes(step.id)
              ? {
                  ...step,
                  status: "FAILED",
                  finishedAt: failedAt,
                  errorMessage: INTERRUPTED_RUN_ERROR,
                }
              : step,
          );
          processed = {
            ...processed,
            run: {
              ...processed.run,
              status: "failed",
              completedAt: failedAt,
              errorSummary: INTERRUPTED_RUN_ERROR,
            },
          };
        }
        setOptimisticFollowUps((previous) =>
          pruneResolvedOptimisticFollowUps(
            previous,
            mergedApprovalDecisions,
            mergedOutputs,
          ),
        );
        setDetail(
          processed
            ? {
              ...processed,
              steps: mergedSteps,
              outputs: mergedOutputs,
              logs: engineDetail?.logs ?? [],
              sessions: mergedSessions,
              artifactRuns: stepArtifactRuns,
              approvalDecisions: mergedApprovalDecisions,
              runPromptText: resolvedRunPromptText,
            }
            : processed,
        );
        setRunPromptText(resolvedRunPromptText);
      }
    } catch (err: any) {
      console.error("Error loading run detail:", err);
      setError(err.message || "Failed to load execution run");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!runId) {
      setLoading(false);
      return;
    }

    void loadData();

    let runSubscription: any = null;
    let stepSubscription: any = null;
    void (async () => {
      try {
        const supabase = await getBrowserSupabaseClient();
        runSubscription = supabase
          .channel(`run-realtime-${runId}`)
          .on(
            "postgres_changes",
            {
              event: "*",
              schema: "public",
              table: "workflow_runs",
              filter: `id=eq.${runId}`,
            },
            () => {
              void loadData();
            },
          )
          .subscribe();

        stepSubscription = supabase
          .channel(`steps-realtime-${runId}`)
          .on(
            "postgres_changes",
            {
              event: "*",
              schema: "public",
              table: "workflow_run_steps",
              filter: `workflow_run_id=eq.${runId}`,
            },
            () => {
              void loadData();
            },
          )
          .subscribe();
      } catch (realtimeErr) {
        console.warn(
          "Supabase realtime not available. Falling back to polling.",
          realtimeErr,
        );
      }
    })();

    const interval = setInterval(() => {
      if (
        !detail?.run ||
        detail.run.status === "pending" ||
        detail.run.status === "running"
      ) {
        void loadData();
      }
    }, 2000);

    return () => {
      void getBrowserSupabaseClient()
        .then((supabase) => {
          if (runSubscription) void supabase.removeChannel(runSubscription);
          if (stepSubscription) void supabase.removeChannel(stepSubscription);
        })
        .catch((err) => {
          console.warn("Cleanup error:", err);
        });
      clearInterval(interval);
    };
  }, [runId, detail?.run?.status]);

  useEffect(() => {
    setSelectedStepId(null);
  }, [runId]);

  useEffect(() => {
    if (detail?.steps && detail.steps.length > 0 && !selectedStepId) {
      const activeStep = detail.steps.find(
        (s: any) =>
          s.status === "RUNNING" || isWorkflowStepWaitingForApproval(s.status),
      );
      const pendingStep = detail.steps.find((s: any) => s.status === "PENDING");
      if (activeStep) {
        setSelectedStepId(activeStep.id);
      } else if (pendingStep) {
        setSelectedStepId(pendingStep.id);
      } else {
        setSelectedStepId(detail.steps[0].id);
      }
    }
  }, [detail?.steps, selectedStepId]);

  const outputsByStepId = useMemo(() => {
    if (!detail?.outputs) return new Map();
    return groupOutputsByStep(detail.outputs as WorkflowOutputRecord[]);
  }, [detail?.outputs]);

  const allLogs = useMemo(
    () =>
      Array.isArray(detail?.logs)
        ? [...detail.logs].sort((left, right) => right.createdAt.localeCompare(left.createdAt))
        : [],
    [detail?.logs],
  );

  const sessionEventLogs = useMemo(
    () =>
      allLogs.filter(
        (log: any) =>
          typeof log?.message === "string" &&
          log.message.startsWith("session_event:"),
      ),
    [allLogs],
  );

  const visibleLogs = logView === "session" ? sessionEventLogs : allLogs;
  const isSessionLogView = logView === "session";
  const canResumeRun = useMemo(
    () =>
      canResumeWorkflowRun({
        run: detail?.run,
        steps: (detail?.steps as Array<{ id: string; status: string }> | undefined) ?? [],
      }),
    [detail?.run, detail?.steps],
  );
  const canCancelRun = useMemo(
    () => canCancelWorkflowRun(detail?.run),
    [detail?.run],
  );
  const googleDriveMcpWaitingStep = useMemo(
    () =>
      detail?.steps.find(
        (step: any) =>
          isWorkflowStepWaitingForApproval(step.status) &&
          isGoogleDriveMcpApprovalMessage(step.errorMessage),
      ),
    [detail?.steps],
  );
  const selectedStep =
    detail?.steps.find((step: any) => step.id === selectedStepId) ??
    googleDriveMcpWaitingStep ??
    detail?.steps[0];
  const selectedStepIndex =
    detail?.steps.findIndex((step: any) => step.id === selectedStep?.id) ?? -1;
  const isInterruptedFailedStep = isInterruptedWorkflowStep({
    status: selectedStep?.status,
    errorMessage: selectedStep?.errorMessage,
  });

  const runTitle = useMemo(
    () => summarizeRunPrompt(runPromptText ?? undefined),
    [runPromptText],
  );

  const pendingApproval = useMemo(() => {
    if (!detail?.approvals) return null;
    return detail.approvals.find(
      (approval: any) => approval.status === "pending",
    );
  }, [detail?.approvals]);
  const isGoogleDriveMcpApproval = useMemo(
    () =>
      isGoogleDriveMcpApprovalMessage(
        selectedStep?.errorMessage ?? pendingApproval?.comment ?? null,
      ),
    [pendingApproval?.comment, selectedStep?.errorMessage],
  );

  const timelineApprovalDecisions = useMemo(
    () =>
      mergeApprovalDecisions(
        ((detail?.approvalDecisions ?? []) as ApprovalDecision[]) ?? [],
        optimisticFollowUps,
      ),
    [detail?.approvalDecisions, optimisticFollowUps],
  );
  const stepThinkingLogs = useMemo(
    () =>
      allLogs.filter(
        (log: any) =>
          log.workflowRunStepId === selectedStep?.id &&
          typeof log?.message === "string" &&
          parseProviderStreamPayload(log.message),
      ),
    [allLogs, selectedStep?.id],
  );
  const stepCommandLogs = useMemo(
    () =>
      allLogs.filter(
        (log: any) =>
          log.workflowRunStepId === selectedStep?.id &&
          typeof log?.message === "string",
      ),
    [allLogs, selectedStep?.id],
  );

  const handleDecision = async (
    stepId: string,
    decision: "approved" | "changes_requested" | "rejected",
  ) => {
    const useGoogleDriveMcpApproval = isGoogleDriveMcpApproval;
    if (decision === "rejected" && !pendingApproval && !useGoogleDriveMcpApproval) {
      alert("Cannot reject an already completed step.");
      return;
    }

    setSubmittingDecision(true);
    const followUpComment = decisionComment.trim();
    const canOptimisticallyContinue =
      decision === "changes_requested" &&
      followUpComment.length > 0 &&
      detail;
    const previousDetail = detail;
    const createdAt = new Date().toISOString();
    const optimisticDecision: ApprovalDecision = {
      id: `optimistic-follow-up-${stepId}-${createdAt}`,
      approvalId: `approval_${stepId}`,
      workflowRunId: detail.run.id,
      workflowStepId: stepId,
      aiOutputId: null,
      decision: "changes_requested",
      reviewerId: null,
      comment: followUpComment,
      createdAt,
    };
    let submitted = false;
    try {
      if (decision === "changes_requested" && useGoogleDriveMcpApproval) {
        alert("Use Reject Request for Google Drive MCP approvals.");
        return;
      }

      if (canOptimisticallyContinue) {
        setOptimisticFollowUps((current) => [...current, optimisticDecision]);
        setDetail(
          applyOptimisticWorkflowFollowUp(
            detail,
            stepId,
            followUpComment,
            createdAt,
          ),
        );
        setDecisionComment("");
      }

      if (decision === "changes_requested") {
        await submitStepApprovalDecisionUseCase.current.execute(
          stepId,
          false,
          followUpComment || undefined,
        );
      } else if (useGoogleDriveMcpApproval) {
        await getGatewayBundle().workflowEngineGateway.submitGoogleDriveWriteApproval(
          stepId,
          decision,
          followUpComment || undefined,
        );
      } else if (decision === "approved") {
        await submitStepApprovalDecisionUseCase.current.execute(
          stepId,
          true,
          followUpComment || undefined,
        );
      } else {
        const now = new Date().toISOString();
        await getGatewayBundle().workflowGateway.createApprovalDecision({
          id: crypto.randomUUID(),
          approvalId: pendingApproval.id,
          workflowRunId: pendingApproval.workflowRunId,
          workflowStepId: pendingApproval.workflowStepId,
          aiOutputId: pendingApproval.aiOutputId,
          decision: "rejected",
          reviewerId: null,
          comment: decisionComment || null,
          createdAt: now,
        });
        await getGatewayBundle().workflowGateway.updateWorkflowStep(
          pendingApproval.workflowStepId,
          {
            status: "rejected",
            completedAt: now,
            errorMessage: decisionComment || "Rejected by reviewer.",
          },
        );
        await getGatewayBundle().workflowGateway.updateWorkflowRun(runId, {
          status: "rejected",
          completedAt: now,
          errorSummary: decisionComment || "Rejected by reviewer.",
        });
      }
      submitted = true;
      await loadData();
      if (!canOptimisticallyContinue) {
        setDecisionComment("");
      }
    } catch (err: any) {
      if (!submitted && canOptimisticallyContinue) {
        setDetail(previousDetail);
        setOptimisticFollowUps((current) =>
          current.filter((candidate) => candidate.id !== optimisticDecision.id),
        );
        setDecisionComment(followUpComment);
      }
      if (submitted) {
        void loadData();
        alert(`Follow-up started, but refreshing the run detail failed: ${err.message}`);
      } else {
        alert(`Decision submission failed: ${err.message}`);
      }
    } finally {
      setSubmittingDecision(false);
    }
  };

  const handleReplayInterruptedStep = async () => {
    if (!selectedStep) {
      return;
    }

    const replayPrompt =
      decisionComment.trim() ||
      runPromptText?.trim() ||
      "Replay the previous prompt in a new session.";

    setSubmittingDecision(true);
    try {
      await submitStepApprovalDecisionUseCase.current.execute(
        selectedStep.id,
        false,
        replayPrompt,
      );
      setDecisionComment("");
      await loadData();
    } catch (err: any) {
      alert(`Replay failed: ${err.message}`);
    } finally {
      setSubmittingDecision(false);
    }
  };

  const handleResume = async () => {
    if (!runId) return;
    setProcessingAction(true);
    try {
      await getGatewayBundle().workflowGateway.updateWorkflowRun(runId, {
        status: "running",
        currentStepKey: null,
      });
      await getGatewayBundle().workflowExecutor.executeUntilPause(runId);
      await loadData();
    } catch (err: any) {
      alert(`Resume failed: ${err.message}`);
    } finally {
      setProcessingAction(false);
    }
  };

  const handleCancel = async () => {
    if (!runId) return;
    setProcessingAction(true);
    try {
      await getGatewayBundle().workflowGateway.updateWorkflowRun(runId, {
        status: "rejected",
        completedAt: new Date().toISOString(),
        errorSummary: "Cancelled by admin.",
      });
      await loadData();
    } catch (err: any) {
      alert(`Cancel failed: ${err.message}`);
    } finally {
      setProcessingAction(false);
    }
  };

  if (loading) {
    return (
      <PageFrame
        title="Workflow Run Detail"
        description="View timeline, approvals, context and outputs of this execution."
      >
        <div className="flex min-h-[300px] items-center justify-center">
          <div className="flex flex-col items-center gap-3">
            <RefreshCw className="h-8 w-8 animate-spin text-accent" />
            <p className="text-sm text-muted-foreground animate-pulse">
              Loading execution details...
            </p>
          </div>
        </div>
      </PageFrame>
    );
  }

  if (error || !detail) {
    return (
      <PageFrame
        title="Execution Load Error"
        description="An error occurred while attempting to resolve this run."
      >
        <div className="rounded-[1.6rem] border border-destructive/20 bg-destructive/10 p-6 max-w-lg mx-auto mt-8 text-center">
          <ShieldAlert className="mx-auto h-12 w-12 text-destructive mb-4" />
          <h2 className="text-xl font-bold text-destructive">
            Unable to load run
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {error ||
              "We could not find any execution record matching the provided ID."}
          </p>
          <Link to="/workflow-runs" className="inline-block mt-6">
            <Button variant="secondary">
              <ArrowLeft className="mr-2 h-4 w-4" /> Back to runs list
            </Button>
          </Link>
        </div>
      </PageFrame>
    );
  }

  const isSummaryStep =
    isResultSummaryStepType(selectedStep?.stepKey) ||
    isResultSummaryStepType(selectedStep?.stepType);

  const stepOutputs = (outputsByStepId.get(selectedStep?.id) as WorkflowOutputRecord[] | undefined) ?? [];
  const summaryOutput = isSummaryStep
    ? [...stepOutputs]
      .sort((left, right) => left.createdAt.localeCompare(right.createdAt))
      .at(-1)
    : null;
  const stepArtifactRun = ((detail.artifactRuns ?? []) as ArtifactRun[]).find(
    (artifactRun) => artifactRun.workflowRunStepId === selectedStep?.id,
  ) ?? null;
  const stepDecisions = (timelineApprovalDecisions.filter(
    (d: any) => d.workflowStepId === selectedStep?.id,
  ) as any[]).sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt),
  );
  const definitionStep = detail.definition?.steps?.find((ds: any) => ds.key === selectedStep?.stepKey);
  const subagent = definitionStep?.subagent ?? null;
  const stepSessions = getSessionsForScope(
    detail.sessions as any[] | null | undefined,
    selectedStep?.id,
    subagent,
  );
  const stepSessionStarts = getSessionStartsForScope({
    logs: allLogs,
    sessions: stepSessions,
    selectedStepId: selectedStep?.id,
  });
  const stepSessionGroups = buildWorkflowStepSessionGroups({
    outputs: stepOutputs,
    decisions: stepDecisions,
    sessions: stepSessions,
    sessionStarts: stepSessionStarts,
    initialPrompt:
      selectedStepIndex === 0 && runPromptText
        ? {
          createdAt: detail.run.startedAt,
          content: runPromptText,
        }
        : null,
  });
  const stepPromptCreatedAts = stepSessionGroups
    .flatMap((group) => group.promptGroups)
    .map((promptGroup) => promptGroup.prompt.createdAt);

  return (
    <div className="fixed inset-0 z-50 bg-[#0c0d12] flex flex-col lg:flex-row overflow-hidden text-foreground">
      {/* Left Sidebar: Pipeline Steps */}
      <aside className="w-full lg:w-80 shrink-0 bg-[#0e1017] border-r border-border/10 flex flex-col h-72 lg:h-full">
        {/* Sidebar Header */}
        <div className="flex items-center justify-between px-6 py-5 border-b border-border/10 shrink-0">
          <div className="flex items-center gap-2">
            {/* Brand Logo/Mark */}
            <div className="flex h-6 w-6 items-center justify-center rounded-lg bg-emerald-500/20 text-emerald-400 font-bold text-xs border border-emerald-500/30">
              FP
            </div>
            <span className="font-bold tracking-tight text-lg text-foreground">FlowPilot</span>
          </div>
          <button className="text-muted-foreground hover:text-foreground transition-colors p-1 rounded-lg hover:bg-muted/10">
            <MoreVertical className="h-4 w-4" />
          </button>
        </div>

        {/* Section title */}
        <div className="px-6 pt-5 pb-2 shrink-0">
          <h2 className="text-[10px] font-bold tracking-widest text-muted-foreground/60 uppercase">
            PIPELINE STEPS
          </h2>
        </div>

        {/* Scrollable Step list */}
        <div className="flex-1 overflow-y-auto px-4 pb-6 space-y-2">
          {detail.steps.map((step: any, idx: number) => {
            const isSelected = step.id === selectedStepId;
            const status = step.status?.toUpperCase();
            const isSummaryItem =
              isResultSummaryStepType(step.stepKey) ||
              isResultSummaryStepType(step.stepType);

            let statusIcon = null;
            if (status === "DONE" || status === "COMPLETED") {
              statusIcon = <Check className="h-4 w-4 text-emerald-400 bg-emerald-950/40 rounded-full p-0.5 border border-emerald-500/20" />;
            } else if (status === "RUNNING") {
              statusIcon = <RefreshCw className="h-3.5 w-3.5 animate-spin text-accent" />;
            } else if (isWorkflowStepWaitingForApproval(status)) {
              statusIcon = <ShieldAlert className="h-4 w-4 text-amber-500 animate-pulse" />;
            } else if (status === "FAILED" || status === "REJECTED") {
              statusIcon = <X className="h-4 w-4 text-destructive bg-destructive/10 rounded-full p-0.5 border border-destructive/20" />;
            } else {
              statusIcon = <div className="h-3.5 w-3.5 rounded-full border-2 border-muted-foreground/30" />;
            }

            return (
              <button
                key={step.id}
                onClick={() => setSelectedStepId(step.id)}
                className={`w-full flex items-center justify-between p-4 rounded-xl border text-left transition-all ${isSelected
                  ? "border-emerald-500/30 bg-[#161d28] shadow-md"
                  : "border-transparent bg-transparent hover:bg-[#161d28]/30"
                  }`}
              >
                <div className="min-w-0 flex-1">
                  {isSummaryItem ? (
                    <h3 className={`text-sm font-semibold truncate mt-0.5 ${isSelected ? "text-accent font-bold" : "text-foreground"}`}>
                      Final Step
                    </h3>
                  ) : (
                    <>
                      <p className="text-[10px] font-mono text-muted-foreground uppercase tracking-wider">
                        Step {idx + 1}
                      </p>
                      <h3 className={`text-sm font-semibold truncate mt-0.5 ${isSelected ? "text-accent font-bold" : "text-foreground"}`}>
                        {step.stepName}
                      </h3>
                    </>
                  )}
                  <p className="text-[10px] text-muted-foreground uppercase mt-1">
                    {status}
                  </p>
                </div>
                <div className="ml-3 shrink-0">{statusIcon}</div>
              </button>
            );
          })}
        </div>

        {/* Sidebar Approval UI */}
        {(() => {
          if (!selectedStep) return null;
          const stepStatus = selectedStep.status?.toUpperCase();
          const isWaiting = isWorkflowStepWaitingForApproval(stepStatus);
          const runStatus = detail.run.status?.toUpperCase();

          if (isWaiting && runStatus !== "REJECTED" && runStatus !== "FAILED") {
            return (
              <div className="p-4 border-t border-border/45 bg-[#0e1017] shrink-0 space-y-3">
                <div className="rounded-xl border border-warning/30 bg-warning/10 p-3 text-xs text-warning">
                  <p className="font-semibold uppercase tracking-wider flex items-center gap-1.5 mb-1.5">
                    <ShieldAlert className="h-4 w-4 shrink-0 text-warning" />
                    Approval Required
                  </p>
                  <p className="text-[11px] leading-relaxed text-warning/90">
                    {selectedStep.errorMessage || "Permission required to run step command."}
                  </p>
                </div>

                <div className="flex items-center justify-end gap-2.5">
                  <Button
                    variant="secondary"
                    className="h-8 w-8 rounded-lg p-0 bg-destructive/10 border border-destructive/20 text-destructive hover:bg-destructive/25 flex items-center justify-center shrink-0"
                    disabled={submittingDecision}
                    onClick={() => handleDecision(selectedStep.id, isGoogleDriveMcpApproval ? "rejected" : "changes_requested")}
                    title={isGoogleDriveMcpApproval ? "Reject Request" : "Reject & Retry"}
                  >
                    {submittingDecision ? (
                      <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <X className="h-4 w-4" />
                    )}
                  </Button>
                  <Button
                    className="h-8 w-8 rounded-lg p-0 bg-emerald-600/20 border border-emerald-500/30 text-emerald-400 hover:bg-emerald-500/20 flex items-center justify-center shrink-0"
                    disabled={submittingDecision}
                    onClick={() => handleDecision(selectedStep.id, "approved")}
                    title={isGoogleDriveMcpApproval ? "Approve Request" : "Approve & Continue"}
                  >
                    {submittingDecision ? (
                      <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Check className="h-4 w-4 text-emerald-400" />
                    )}
                  </Button>
                </div>
              </div>
            );
          }
          return null;
        })()}
      </aside>

      {/* Right Details Workspace */}
      <main className="flex-grow flex flex-col h-full bg-[#0c0d12] overflow-y-auto p-6 lg:p-8">
        {selectedStep ? (
          isSummaryStep ? (
            <div className="max-w-4xl w-full mx-auto bg-[#11131c] border border-border/20 rounded-[1.6rem] p-8 shadow-2xl space-y-6 relative">
              <div className="absolute top-8 right-8 flex items-center gap-3">
                <button className="text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-muted/10">
                  <span className="text-sm font-semibold">?</span>
                </button>
                <Link to="/workflow-runs">
                  <button className="text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-muted/15">
                    <X className="h-5 w-5" />
                  </button>
                </Link>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border/10 pb-4 pr-20">
                <div>
                  <span className="text-[10px] font-bold text-accent bg-accent/10 border border-accent/20 px-2 py-0.5 rounded uppercase tracking-widest">
                    SUMMARY
                  </span>
                  <h2 className="mt-1 text-2xl font-bold tracking-tight text-foreground flex items-center gap-2.5 flex-wrap">
                    {RESULT_SUMMARY_STEP_NAME}
                    <span className={`px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wider ${selectedStep.status === "DONE" || selectedStep.status === "COMPLETED"
                      ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20"
                      : "bg-accent/10 text-accent border border-accent/20"
                      }`}>
                      {selectedStep.status}
                    </span>
                  </h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Final workflow-level summary generated from the completed steps in this run.
                  </p>
                </div>

                {summaryOutput?.contentMarkdown?.trim() ? (
                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      variant="secondary"
                      className="h-8 rounded-lg px-3 text-xs font-semibold"
                      onClick={() => {
                        navigator.clipboard.writeText(summaryOutput.contentMarkdown);
                      }}
                    >
                      <Copy className="mr-1.5 h-3.5 w-3.5" /> Copy
                    </Button>
                    <Button
                      variant="secondary"
                      className="h-8 rounded-lg px-3 text-xs font-semibold"
                      onClick={() =>
                        openMarkdownPreviewInNewTab(
                          summaryOutput.contentMarkdown,
                          `${detail.run.title || detail.run.id} Summary`,
                        )
                      }
                    >
                      <ExternalLink className="mr-1.5 h-3.5 w-3.5" /> Open
                    </Button>
                  </div>
                ) : null}
              </div>

              <div className="rounded-2xl border border-border/70 bg-[#0c0f16] p-6">
                {summaryOutput?.contentMarkdown?.trim() ? (
                  <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-7 text-foreground">
                    {summaryOutput.contentMarkdown}
                  </pre>
                ) : selectedStep.status === "PENDING" || selectedStep.status === "RUNNING" ? (
                  <p className="text-sm text-muted-foreground">
                    Summary is being generated...
                  </p>
                ) : selectedStep.errorMessage ? (
                  <div className="rounded-xl bg-destructive/10 border border-destructive/20 p-4 text-sm text-destructive">
                    {selectedStep.errorMessage}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    No summary text generated yet.
                  </p>
                )}
              </div>
            </div>
          ) : (
          <div className="max-w-4xl w-full mx-auto bg-[#11131c] border border-border/20 rounded-[1.6rem] p-8 shadow-2xl space-y-6 relative">

            {/* Top Right Close & Help buttons */}
            <div className="absolute top-8 right-8 flex items-center gap-3">
              <button className="text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-muted/10">
                <span className="text-sm font-semibold">?</span>
              </button>
              <Link to="/workflow-runs">
                <button className="text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-muted/15">
                  <X className="h-5 w-5" />
                </button>
              </Link>
            </div>

            {/* Header: Step Index, Title, and Action Buttons */}
            <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border/10 pb-4 pr-20">
              <div>
                <span className="text-[10px] font-bold text-accent bg-accent/10 border border-accent/20 px-2 py-0.5 rounded uppercase tracking-widest">
                  STEP {selectedStepIndex + 1}
                </span>
                <h2 className="text-2xl font-bold tracking-tight text-foreground flex items-center gap-2.5 flex-wrap mt-1">
                  {selectedStep.stepName}
                  {isWorkflowStepWaitingForApproval(selectedStep.status) ? (
                    <span className="bg-amber-500/10 text-amber-400 border border-amber-500/20 px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wider animate-pulse">
                      Awaiting Safe-Gate Approval
                    </span>
                  ) : (
                    <span className={`px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wider ${selectedStep.status === "DONE" || selectedStep.status === "COMPLETED"
                      ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20"
                      : "bg-accent/10 text-accent border border-accent/20"
                      }`}>
                      {selectedStep.status}
                    </span>
                  )}
                </h2>
              </div>

              {/* Action Buttons: Resume, Cancel, Workflow YOLO */}
              <div className="flex items-center gap-3 shrink-0">
                {canResumeRun || canCancelRun ? (
                  <>
                    {canResumeRun ? (
                      <Button
                        variant="secondary"
                        className="h-8 rounded-lg px-3 text-xs font-semibold"
                        disabled={processingAction}
                        onClick={handleResume}
                      >
                        <Play className="mr-1.5 h-3.5 w-3.5 fill-current" /> Resume
                      </Button>
                    ) : null}
                    {canCancelRun ? (
                      <Button
                        variant="ghost"
                        className="h-8 rounded-lg px-3 text-xs"
                        disabled={processingAction}
                        onClick={handleCancel}
                      >
                        <X className="mr-1.5 h-3.5 w-3.5" /> Cancel
                      </Button>
                    ) : null}
                  </>
                ) : null}

                {canResumeRun || canCancelRun ? (
                  <div className="h-4 w-[1px] bg-border/20" />
                ) : null}

                <div className="flex items-center gap-1.5 rounded-full border border-border bg-card/60 px-3 py-1 text-[11px] font-bold uppercase tracking-wider">
                  <span className="text-muted-foreground">YOLO</span>
                  <span className={detail.run.yoloMode ? "text-emerald-400" : "text-muted-foreground/60"}>
                    {detail.run.yoloMode ? "ENABLED" : "DISABLED"}
                  </span>
                </div>
              </div>
            </div>

            <div className="space-y-3">
              {stepArtifactRun || stepOutputs.length > 0 ? (
                <div>
                  <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full border text-[10px] font-semibold uppercase tracking-wider ${stepArtifactRun
                    ? "bg-emerald-500/5 border-emerald-500/20 text-emerald-400"
                    : "bg-accent/5 border-accent/20 text-accent"
                    }`}>
                    <FileText className="h-3.5 w-3.5" />
                    {stepArtifactRun ? "Artifact Generated" : "Response Captured"}
                  </span>
                </div>
              ) : null}
            </div>

            {/* Step contents timeline */}
            <div className="pt-4">
              {stepSessionGroups.length === 0 ? (
                <div className="rounded-xl border border-border/80 bg-card/25 p-6">
                  <p className="text-sm text-muted-foreground text-center py-4">
                    {selectedStep.status === "PENDING" || selectedStep.status === "RUNNING"
                      ? "Step is processing..."
                      : "No outputs generated for this step yet."}
                  </p>
                  {selectedStep.errorMessage ? (
                    <div className="mt-4 rounded-xl bg-destructive/10 border border-destructive/20 p-4 text-sm text-destructive">
                      {selectedStep.errorMessage}
                    </div>
                  ) : null}
                </div>
              ) : (
                <div className="space-y-5">
                  {stepSessionGroups.map((group) => (
                    <SessionGroupSection
                      key={group.key}
                      detail={detail}
                      gatewayBundle={gatewayBundle}
                      group={group}
                      onKillSession={handleKillSession}
                      stepOutputs={stepOutputs}
                      stepCommandLogs={stepCommandLogs}
                      stepLogs={stepThinkingLogs}
                      stepPromptCreatedAts={stepPromptCreatedAts}
                      stepStatus={selectedStep.status}
                      subagent={subagent}
                    />
                  ))}
                </div>
              )}
            </div>

            {/* Collapsible Run Logs & Safe Gate & Follow-up Chat */}
            <div className="pt-4 border-t border-border/30">

              {/* Safe Gate / Follow-up Section inside the card footer */}
              {(() => {
                const stepStatus = selectedStep.status?.toUpperCase();
                const runStatus = detail.run.status?.toUpperCase();
                const isWaiting = isWorkflowStepWaitingForApproval(stepStatus);
                const isDone = stepStatus === "DONE" || stepStatus === "COMPLETED";
                const canContinue =
                  ((isWaiting || isDone) &&
                    runStatus !== "REJECTED" &&
                    runStatus !== "FAILED") ||
                  isInterruptedFailedStep;

                if (!canContinue || isWaiting || subagent) return null;

                return (
                  <div className="p-6 bg-[#11131c] border-t border-border/10">
                    <div className="relative border border-border/60 bg-[#090a0f] rounded-xl p-3 focus-within:border-emerald-500/50 transition-colors">
                      <textarea
                        className="w-full min-h-[60px] pb-12 resize-none bg-transparent text-sm text-foreground focus:outline-none placeholder:text-muted-foreground/60 leading-relaxed"
                        placeholder={
                          isInterruptedFailedStep
                            ? "Replay the prompt in a new session, or edit it before sending..."
                            : "Type chat for follow-up interactions..."
                        }
                        value={decisionComment}
                        onChange={(e) => {
                          setDecisionComment(e.target.value);
                        }}
                        rows={2}
                      />
                      <div className="absolute bottom-3 right-3">
                        <Button
                          className="rounded-lg px-4 h-8 bg-emerald-600 hover:bg-emerald-700 text-white font-semibold text-xs transition-colors"
                          disabled={
                            submittingDecision ||
                            (!isInterruptedFailedStep && !decisionComment.trim())
                          }
                          onClick={() =>
                            isInterruptedFailedStep
                              ? handleReplayInterruptedStep()
                              : handleDecision(selectedStep.id, "changes_requested")
                          }
                        >
                          {submittingDecision ? (
                            <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                          ) : isInterruptedFailedStep ? (
                            "Replay"
                          ) : (
                            "Send"
                          )}
                        </Button>
                      </div>
                    </div>
                  </div>
                );
              })()}

              {/* Custom Run Logs Collapsible Card */}
              <div className="border border-border/20 bg-[#11131c] rounded-[1.6rem] overflow-hidden shadow-lg">
                <button
                  type="button"
                  onClick={() => setLogsExpanded(!logsExpanded)}
                  className="w-full flex items-center justify-between px-6 py-4 bg-[#0e1017]/60 hover:bg-[#0e1017] transition-all border-b border-border/10 animate-fade-in"
                >
                  <div className="flex items-center gap-2.5">
                    <FileText className="h-4 w-4 text-emerald-400" />
                    <span className="text-sm font-bold text-foreground">Run Logs</span>
                  </div>
                  {logsExpanded ? (
                    <ChevronUp className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  )}
                </button>

                {logsExpanded && (
                  <div className="p-6 pb-2 border-b border-border/10 space-y-4">
                    <div>
                      <p className="text-xs text-muted-foreground mb-4">
                        Use the filter to isolate runner session lifecycle events while testing provider reuse and restart recovery.
                      </p>

                      <div className="flex flex-wrap items-start justify-between gap-4">
                        <div className="flex flex-wrap gap-2">
                          <Link
                            to="/workflow-runs/$runId"
                            params={{ runId }}
                            search={{ logView: undefined }}
                            className={`rounded-full border px-4 py-2 text-[10px] font-bold uppercase tracking-wider transition-colors ${!isSessionLogView
                              ? "border-emerald-500/20 bg-emerald-500/10 text-emerald-400"
                              : "border-border/70 bg-card text-muted-foreground hover:text-foreground"
                              }`}
                          >
                            All logs
                            <span className="ml-2 text-[10px] font-bold normal-case tracking-normal opacity-70">
                              {allLogs.length}
                            </span>
                          </Link>
                          <Link
                            to="/workflow-runs/$runId"
                            params={{ runId }}
                            search={{ logView: "session" }}
                            className={`rounded-full border px-4 py-2 text-[10px] font-bold uppercase tracking-wider transition-colors ${isSessionLogView
                              ? "border-emerald-500/20 bg-emerald-500/10 text-emerald-400"
                              : "border-border/70 bg-card text-muted-foreground hover:text-foreground"
                              }`}
                          >
                            Session events
                            <span className="ml-2 text-[10px] font-bold normal-case tracking-normal opacity-70">
                              {sessionEventLogs.length}
                            </span>
                          </Link>
                        </div>
                      </div>
                    </div>

                    <div className="space-y-3 max-h-72 overflow-y-auto pr-1">
                      {visibleLogs.length === 0 ? (
                        <p className="text-sm text-muted-foreground">
                          {isSessionLogView
                            ? "No session_event logs yet."
                            : "No logs yet."}
                        </p>
                      ) : (
                        visibleLogs.map((log: any) => {
                          const isSessionEvent =
                            typeof log?.message === "string" &&
                            log.message.startsWith("session_event:");
                          const providerStreamPayload = parseProviderStreamPayload(log.message);
                          const isProviderStream = Boolean(providerStreamPayload);
                          let logLabel = log.logLevel;
                          let badgeLabel = log.logLevel;
                          if (isSessionEvent) {
                            logLabel = "session_event";
                            badgeLabel = "session";
                          } else if (isProviderStream) {
                            logLabel = "provider_stream";
                            badgeLabel = providerStreamPayload?.stream || "stream";
                          }
                          const logMessage =
                            providerStreamPayload
                              ? extractProviderStreamDisplayText(
                                  providerStreamPayload.text ?? "",
                                )
                              : log.message;
                          return (
                            <div
                              key={log.id}
                              className="rounded-2xl border border-border bg-card p-4 text-sm"
                            >
                              <div className="flex flex-wrap items-start justify-between gap-3">
                                <div className="space-y-1">
                                  <div className="flex flex-wrap items-center gap-2">
                                    <p className="font-semibold">
                                      {logLabel}
                                    </p>
                                    <Badge>
                                      {badgeLabel}
                                    </Badge>
                                  </div>
                                  <p className="font-mono text-[11px] text-muted-foreground">
                                    {new Date(log.createdAt).toLocaleString()}
                                  </p>
                                </div>
                                <p className="break-all font-mono text-[11px] text-muted-foreground">
                                  {log.workflowRunStepId}
                                </p>
                              </div>
                              <pre className="mt-3 whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-foreground">
                                {logMessage}
                              </pre>
                            </div>
                          );
                        })
                      )}
                    </div>
                  </div>
                )}


              </div>
            </div>

          </div>
          )
        ) : (
          <div className="rounded-[1.6rem] border border-border bg-background/50 p-6 text-center text-muted-foreground">
            Select a step from the pipeline steps list to view details.
          </div>
        )}
      </main>
    </div>
  );
}
