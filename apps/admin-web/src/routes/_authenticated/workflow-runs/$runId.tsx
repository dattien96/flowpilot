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
import { WorkflowTimeline } from "@/presentation/components/workflow-runs/workflow-timeline";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GetWorkflowRunDetailUseCase } from "@/domain/usecase/workflow-runs/get-workflow-run-detail-usecase";
import { GetWorkflowRunDetailUseCase as GetWorkflowEngineRunDetailUseCase } from "@/domain/usecase/workflow-engine/get-workflow-run-detail-usecase";
import { ListArtifactRunsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-runs-usecase";
import { SubmitStepApprovalDecisionUseCase } from "@/domain/usecase/workflow-engine/submit-step-approval-decision-usecase";
import { createSupabaseBrowserClient } from "@/data/datasource/supabase/client";
import { applyOptimisticWorkflowFollowUp } from "@/features/workflow-engine/workflow-run-detail-optimistic";
import {
  buildWorkflowStepTimeline,
  groupOutputsByStep,
  mapArtifactsToWorkflowOutputs,
  mergeWorkflowOutputs,
  type WorkflowOutputRecord,
} from "@/features/workflow-engine/workflow-run-detail-timeline";
import {
  buildFallbackApprovalDecisionsFromLogs,
  buildFallbackOutputsFromLogs,
  extractBeginPromptFromLogs,
} from "@/features/workflow-engine/workflow-run-log-fallback";
import { statusTone } from "@/presentation/view-models/factories";
import type { ApprovalDecision } from "@/domain/model/entity/workflow";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";
import { loadWorkflowRunPromptText } from "@/lib/workflow-run-prompt";
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
}: {
  title: string;
  time?: string;
  content: string;
  isSecondary?: boolean;
}) {
  const [isExpanded, setIsExpanded] = useState(false);
  const TRUNCATE_LENGTH = 400;
  const isLong = content.length > TRUNCATE_LENGTH;
  const displayContent = (!isExpanded && isLong) ? content.slice(0, TRUNCATE_LENGTH) + "..." : content;

  return (
    <div className={`max-w-[85%] rounded-[1.6rem] px-6 py-4 shadow-sm ${isSecondary ? "bg-accent/90 text-accent-foreground" : "bg-accent text-accent-foreground"
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
            className="text-xs h-8 text-accent hover:text-accent/80 font-bold uppercase tracking-wider bg-[#090a0f]/90 hover:bg-[#090a0f] border border-border/20 rounded-lg px-4 shadow-sm"
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
  localRunnerGateway,
}: {
  artifactRun?: ArtifactRun | null;
  output: WorkflowOutputRecord;
  localRunnerGateway: {
    readFile(path: string): Promise<string>;
  };
}) {
  const [activeTab, setActiveTab] = useState<"response" | "prompt" | "artifact">("response");
  const [artifactContent, setArtifactContent] = useState<string | null>(null);
  const [artifactError, setArtifactError] = useState<string | null>(null);
  const [artifactLoading, setArtifactLoading] = useState(false);

  const loadArtifactContent = async () => {
    if (!artifactRun) {
      throw new Error("Artifact metadata is unavailable.");
    }

    setArtifactLoading(true);
    setArtifactError(null);

    try {
      const content = await localRunnerGateway.readFile(artifactRun.localPath);
      setArtifactContent(content);
      return content;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Unable to load artifact content.";
      setArtifactError(message);
      throw error instanceof Error ? error : new Error(message);
    } finally {
      setArtifactLoading(false);
    }
  };

  useEffect(() => {
    setArtifactContent(null);
    setArtifactError(null);
    setArtifactLoading(false);
  }, [artifactRun?.id]);

  const tabs = [
    { key: "response" as const, label: "RESPONSE" },
    { key: "prompt" as const, label: "PROMPT" },
    ...(artifactRun ? [{ key: "artifact" as const, label: "ARTIFACT" }] : []),
  ];

  const normalizedPrompt = normalizePromptDisplay(output.promptText);

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
          {normalizedPrompt ? (
            <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[#d1d5db]">
              {normalizedPrompt}
            </pre>
          ) : (
            <p className="text-xs italic text-muted-foreground">
              No prompt file captured for this output.
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
                {artifactRun.localPath}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2 shrink-0">
              <Button
                className="h-8 px-4 text-xs font-bold uppercase tracking-wider bg-accent/10 border border-accent/20 hover:bg-accent/20 text-accent rounded-lg shadow-sm"
                disabled={artifactLoading}
                onClick={() => {
                  const openPreview = (content: string) => {
                    const opened = openMarkdownPreviewInNewTab(
                      content.trim() || content,
                      artifactRun.title,
                    );
                    if (!opened) {
                      window.alert(
                        "The browser blocked the preview tab. Allow popups for FlowPilot and try again.",
                      );
                    }
                  };

                  if (artifactContent) {
                    openPreview(artifactContent);
                    return;
                  }

                  void loadArtifactContent()
                    .then((content) => {
                      openPreview(content);
                    })
                    .catch(() => {
                      // Error state is already surfaced in the artifact panel.
                    });
                }}
                variant="secondary"
              >
                <ExternalLink className="mr-2 h-3.5 w-3.5 text-sm" />
                {artifactLoading ? "Opening..." : "Open in new tab"}
              </Button>
            </div>
          </div>

          {artifactError ? (
            <p className="text-sm text-destructive">{artifactError}</p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

export const Route = createFileRoute("/_authenticated/workflow-runs/$runId")({
  validateSearch: (search: Record<string, unknown>) => ({
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
  const [togglingYolo, setTogglingYolo] = useState(false);
  const [processingAction, setProcessingAction] = useState(false);
  const [runPromptText, setRunPromptText] = useState<string | null>(null);
  const [optimisticFollowUps, setOptimisticFollowUps] = useState<
    ApprovalDecision[]
  >([]);
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  const [logsExpanded, setLogsExpanded] = useState(true);

  const processDetailData = async (data: any) => {
    if (!data) return null;
    let mappedOutputs: WorkflowOutputRecord[] = [];
    try {
      const localArtifacts =
        await gatewayBundle.current.localRunnerGateway.listArtifacts();
      const runArtifacts = localArtifacts.filter(
        (art) => art.workflowRunId === runId,
      );
      const artifactDetails = await Promise.all(
        runArtifacts.map(
          async (artifact) =>
            (await gatewayBundle.current.localRunnerGateway.getArtifactById(
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
          gatewayBundle.current.localRunnerGateway,
          runId,
        ),
        getWorkflowEngineRunDetailUseCase.current.execute(runId),
      ]);
      const engineDetail = engineDetailRaw as
        | { logs: any[]; sessions?: any[] | null }
        | null;
      if (data) {
        const processed = await processDetailData(data);
        const engineLogs = engineDetail?.logs ?? [];
        const promptFromLogs = extractBeginPromptFromLogs(engineLogs);
        const resolvedRunPromptText = promptFromLogs ?? promptText;
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
            steps: processed.steps ?? [],
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
              outputs: mergedOutputs,
              logs: engineDetail?.logs ?? [],
              sessions: engineDetail?.sessions ?? processed.sessions ?? [],
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
    try {
      const supabase = createSupabaseBrowserClient();
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
      try {
        const supabase = createSupabaseBrowserClient();
        if (runSubscription) void supabase.removeChannel(runSubscription);
        if (stepSubscription) void supabase.removeChannel(stepSubscription);
      } catch (err) {
        console.warn("Cleanup error:", err);
      }
      clearInterval(interval);
    };
  }, [runId, detail?.run?.status]);

  useEffect(() => {
    if (detail?.steps && detail.steps.length > 0 && !selectedStepId) {
      const activeStep = detail.steps.find(
        (s: any) =>
          s.status === "RUNNING" || s.status === "WAITING_USER_APPROVAL",
      );
      const pendingStep = detail.steps.find((s: any) => s.status === "PENDING");
      if (activeStep) {
        setSelectedStepId(activeStep.id);
      } else if (pendingStep) {
        setSelectedStepId(pendingStep.id);
      } else {
        setSelectedStepId(detail.steps[detail.steps.length - 1].id);
      }
    }
  }, [detail?.steps, selectedStepId]);

  const outputsByStepId = useMemo(() => {
    if (!detail?.outputs) return new Map();
    return groupOutputsByStep(detail.outputs as WorkflowOutputRecord[]);
  }, [detail?.outputs]);

  const allLogs = useMemo(
    () => (Array.isArray(detail?.logs) ? detail.logs : []),
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

  const runTitle = useMemo(
    () => summarizeRunPrompt(runPromptText),
    [runPromptText],
  );

  const pendingApproval = useMemo(() => {
    if (!detail?.approvals) return null;
    return detail.approvals.find(
      (approval: any) => approval.status === "pending",
    );
  }, [detail?.approvals]);

  const timelineApprovalDecisions = useMemo(
    () =>
      mergeApprovalDecisions(
        ((detail?.approvalDecisions ?? []) as ApprovalDecision[]) ?? [],
        optimisticFollowUps,
      ),
    [detail?.approvalDecisions, optimisticFollowUps],
  );

  const handleDecision = async (
    stepId: string,
    decision: "approved" | "changes_requested" | "rejected",
  ) => {
    if (decision === "rejected" && !pendingApproval) {
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

      if (decision === "approved" || decision === "changes_requested") {
        await submitStepApprovalDecisionUseCase.current.execute(
          stepId,
          decision === "approved",
          followUpComment || undefined,
        );
      } else {
        const now = new Date().toISOString();
        await gatewayBundle.current.workflowGateway.createApprovalDecision({
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
        await gatewayBundle.current.workflowGateway.updateWorkflowStep(
          pendingApproval.workflowStepId,
          {
            status: "rejected",
            completedAt: now,
            errorMessage: decisionComment || "Rejected by reviewer.",
          },
        );
        await gatewayBundle.current.workflowGateway.updateWorkflowRun(runId, {
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

  const handleToggleYolo = async () => {
    if (!detail?.run) return;
    setTogglingYolo(true);
    try {
      const updatedRun =
        await gatewayBundle.current.workflowEngineGateway.toggleYoloMode(
          detail.run.id,
          !detail.run.yoloMode,
        );
      setDetail((prev: any) => (prev ? { ...prev, run: updatedRun } : null));
    } catch (err: any) {
      alert(`YOLO toggle failed: ${err.message}`);
    } finally {
      setTogglingYolo(false);
    }
  };

  const handleResume = async () => {
    if (!runId) return;
    setProcessingAction(true);
    try {
      await gatewayBundle.current.workflowGateway.updateWorkflowRun(runId, {
        status: "running",
        currentStepKey: null,
      });
      await gatewayBundle.current.workflowExecutor.executeUntilPause(runId);
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
      await gatewayBundle.current.workflowGateway.updateWorkflowRun(runId, {
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

  const selectedStep = detail.steps.find((s: any) => s.id === selectedStepId) || detail.steps[0];
  const selectedStepIndex = detail.steps.findIndex((s: any) => s.id === selectedStepId);

  const stepOutputs = (outputsByStepId.get(selectedStep?.id) as WorkflowOutputRecord[] | undefined) ?? [];
  const stepArtifactRun = ((detail.artifactRuns ?? []) as ArtifactRun[]).find(
    (artifactRun) => artifactRun.workflowRunStepId === selectedStep?.id,
  ) ?? null;
  const stepDecisions = (timelineApprovalDecisions.filter(
    (d: any) => d.workflowStepId === selectedStep?.id,
  ) as any[]).sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt),
  );
  const stepTimelineItems = buildWorkflowStepTimeline(stepOutputs, stepDecisions);
  const definitionStep = detail.definition?.steps?.find((ds: any) => ds.key === selectedStep?.stepKey);
  const subagent = definitionStep?.subagent ?? null;
  const stepSession = (() => {
    if (!selectedStep || !detail.sessions) return null;
    if (subagent) {
      return detail.sessions.find((s: any) => s.metadataJson?.step_run_id === selectedStep.id);
    } else {
      return detail.sessions.find((s: any) => s.metadataJson?.is_main === true || s.metadataJson?.is_main === "true");
    }
  })();

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

            let statusIcon = null;
            if (status === "DONE" || status === "COMPLETED") {
              statusIcon = <Check className="h-4 w-4 text-emerald-400 bg-emerald-950/40 rounded-full p-0.5 border border-emerald-500/20" />;
            } else if (status === "RUNNING") {
              statusIcon = <RefreshCw className="h-3.5 w-3.5 animate-spin text-accent" />;
            } else if (status === "WAITING_USER_APPROVAL") {
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
                  <p className="text-[10px] font-mono text-muted-foreground uppercase tracking-wider">
                    Step {idx + 1}
                  </p>
                  <h3 className={`text-sm font-semibold truncate mt-0.5 ${isSelected ? "text-accent font-bold" : "text-foreground"
                    }`}>
                    {step.stepName}
                  </h3>
                  <p className="text-[10px] text-muted-foreground uppercase mt-1">
                    {status}
                  </p>
                </div>
                <div className="ml-3 shrink-0">{statusIcon}</div>
              </button>
            );
          })}
        </div>
      </aside>

      {/* Right Details Workspace */}
      <main className="flex-grow flex flex-col h-full bg-[#0c0d12] overflow-y-auto p-6 lg:p-8">
        {selectedStep ? (
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
                  {selectedStep.status === "WAITING_USER_APPROVAL" ? (
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

              {/* Action Buttons: Resume, Cancel, YOLO */}
              <div className="flex items-center gap-3 shrink-0">
                {detail.run.status !== "completed" && detail.run.status !== "rejected" ? (
                  <>
                    <Button
                      variant="secondary"
                      size="sm"
                      className="h-8 rounded-lg px-3 text-xs font-semibold"
                      disabled={processingAction || detail.run.status === "running"}
                      onClick={handleResume}
                    >
                      <Play className="mr-1.5 h-3.5 w-3.5 fill-current" /> Resume
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-8 rounded-lg px-3 text-xs"
                      disabled={processingAction}
                      onClick={handleCancel}
                    >
                      <X className="mr-1.5 h-3.5 w-3.5" /> Cancel
                    </Button>
                  </>
                ) : null}

                <div className="h-4 w-[1px] bg-border/20" />

                <div className="flex items-center gap-2 rounded-full border border-border bg-card/60 px-3 py-1 text-[11px]">
                  <span className="font-medium text-muted-foreground uppercase tracking-wider">
                    YOLO
                  </span>
                  <button
                    disabled={togglingYolo}
                    onClick={handleToggleYolo}
                    className={`relative inline-flex h-4 w-7 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${detail.run.yoloMode ? "bg-accent" : "bg-muted"
                      }`}
                  >
                    <span
                      className={`pointer-events-none inline-block h-3 w-3 transform rounded-full bg-background shadow ring-0 transition duration-200 ease-in-out ${detail.run.yoloMode ? "translate-x-3" : "translate-x-0"
                        }`}
                    />
                  </button>
                </div>
              </div>
            </div>

            {/* Initial Run Prompt (only show on step 1 details) */}
            {selectedStepIndex === 0 && runPromptText && (
              <div className="flex justify-end mb-4">
                <div className="bg-[#1b2b24] border border-emerald-500/20 p-4 rounded-xl space-y-1 max-w-[85%] text-left">
                  <span className="text-[9px] font-bold text-emerald-400 tracking-wider uppercase font-mono">
                    Initial Prompt
                  </span>
                  <p className="text-sm text-foreground leading-relaxed break-words font-sans">
                    {runPromptText}
                  </p>
                </div>
              </div>
            )}

            {/* Session & Output Badges */}
            <div className="space-y-3">
              {stepSession && (
                <p className="text-xs font-mono text-muted-foreground flex items-center gap-1.5 flex-wrap">
                  <span>Session:</span>
                  <span className="bg-accent/40 text-accent-foreground px-1.5 py-0.5 rounded font-bold uppercase tracking-wider text-[9px]">
                    {subagent ? `Isolated (${subagent})` : "Shared Main"}
                  </span>
                  <span>&bull;</span>
                  <span className="text-foreground/90 font-medium font-sans">
                    {stepSession.provider} ({stepSession.model})
                  </span>
                  <span>&bull;</span>
                  <span className={`font-semibold ${stepSession.status === "active" ? "text-success" : "text-muted-foreground"}`}>
                    {stepSession.status}
                  </span>
                  {stepSession.providerSessionId ? (
                    <>
                      <span>&bull;</span>
                      <span className="font-mono text-[9px] text-foreground/80">
                        {stepSession.providerSessionId}
                      </span>
                    </>
                  ) : null}
                </p>
              )}

              <div>
                <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full border text-[10px] font-semibold uppercase tracking-wider ${stepArtifactRun
                  ? "bg-emerald-500/5 border-emerald-500/20 text-emerald-400"
                  : "bg-accent/5 border-accent/20 text-accent"
                  }`}>
                  <FileText className="h-3.5 w-3.5" />
                  {stepArtifactRun ? "Artifact Generated" : "Response Captured"}
                </span>
              </div>
            </div>

            {/* Step contents timeline */}
            <div className="pt-4">
              {stepTimelineItems.length === 0 ? (
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
                stepTimelineItems.map((item: any) => {
                  const outputAttempt =
                    item.kind === "output"
                      ? stepOutputs.findIndex((output) => output.id === item.output.id) + 1
                      : 0;

                  return item.kind === "decision" ? (
                    <div key={item.key} className="flex justify-end mt-4">
                      <CollapsibleChatBubble
                        title="Follow-up"
                        time={new Date(item.decision.createdAt).toLocaleTimeString()}
                        content={item.decision.comment || `Decision: ${item.decision.decision}`}
                        isSecondary={true}
                      />
                    </div>
                  ) : (
                    <div key={item.key} className="space-y-4">
                      {stepOutputs.length > 1 && (
                        <div className="flex items-center justify-between pb-2 border-b border-border/30">
                          <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                            Attempt {outputAttempt}
                          </span>
                        </div>
                      )}

                      <StepOutputTabs
                        artifactRun={stepArtifactRun}
                        localRunnerGateway={
                          gatewayBundle.current.localRunnerGateway
                        }
                        output={item.output}
                      />

                      <details className="mt-4 pt-4 border-t border-border/30">
                        <summary className="text-[11px] font-bold uppercase tracking-[0.2em] text-muted-foreground cursor-pointer hover:text-foreground transition-colors inline-flex items-center justify-between w-full">
                          <span>DEVELOPER DIAGNOSTICS</span>
                          <span className="text-[9px] font-mono opacity-60">Expand</span>
                        </summary>
                        <div className="mt-4 space-y-3 pl-2 border-l-2 border-border/50">
                          {item.output.localPath && (
                            <CollapsibleTextBlock title="Artifact Path" value={item.output.localPath} emptyLabel="" />
                          )}
                          <CollapsibleTextBlock title="Prompt Override" value={item.output.promptText} emptyLabel="Inherited from run prompt." />
                          <CollapsibleTextBlock title="Stdout" value={item.output.stdoutText} emptyLabel="No stdout." />
                          <CollapsibleTextBlock title="Stderr" value={item.output.stderrText} emptyLabel="No stderr." />
                          <CollapsibleTextBlock title="CLI Command" value={item.output.commandText} emptyLabel="No command captured." />
                        </div>
                      </details>
                    </div>
                  );
                })
              )}
            </div>

            {/* Collapsible Run Logs & Safe Gate & Follow-up Chat */}
            <div className="pt-4 border-t border-border/30">

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
                          return (
                            <div
                              key={log.id}
                              className="rounded-2xl border border-border bg-card p-4 text-sm"
                            >
                              <div className="flex flex-wrap items-start justify-between gap-3">
                                <div className="space-y-1">
                                  <div className="flex flex-wrap items-center gap-2">
                                    <p className="font-semibold">
                                      {isSessionEvent ? "session_event" : log.logLevel}
                                    </p>
                                    <Badge>
                                      {isSessionEvent ? "session" : log.logLevel}
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
                                {log.message}
                              </pre>
                            </div>
                          );
                        })
                      )}
                    </div>
                  </div>
                )}

                {/* Always-Visible Send Prompt / Safe Gate Section inside the card footer */}
                <div className="p-6 bg-[#11131c] border-t border-border/10">
                  {(() => {
                    const stepStatus = selectedStep.status?.toUpperCase();
                    const runStatus = detail.run.status?.toUpperCase();
                    const isWaiting = stepStatus === "WAITING_USER_APPROVAL";
                    const isDone = stepStatus === "DONE" || stepStatus === "COMPLETED";
                    const canContinue = (isWaiting || isDone) && runStatus !== "REJECTED" && runStatus !== "FAILED";

                    if (!canContinue) return null;

                    if (isWaiting) {
                      return (
                        <div className="border border-border/80 bg-card/45 p-6 rounded-2xl shadow-xl space-y-4">
                          <div className="flex items-center gap-2 pb-2 border-b border-border/30">
                            <ShieldAlert className="h-5 w-5 text-warning shrink-0" />
                            <span className="text-sm font-semibold text-warning">
                              Safe Gate: Approving compiles files & begins coding stage
                            </span>
                          </div>

                          <div className="space-y-3">
                            <textarea
                              className="w-full min-h-[70px] max-h-[200px] resize-none bg-[#090a0f] border border-border/60 rounded-xl px-4 py-3 text-sm focus:outline-none focus:ring-1 focus:ring-accent/50 placeholder:text-muted-foreground/60 text-[#eaeaea]"
                              placeholder="Provide revision notes for Reject & Retry..."
                              value={decisionComment}
                              onChange={(e) => {
                                setDecisionComment(e.target.value);
                              }}
                              rows={2}
                            />

                            <div className="flex flex-wrap items-center justify-end gap-3 pt-1">
                              <Button
                                variant="outline"
                                className="rounded-xl px-5 h-10 border-border/80 hover:bg-muted text-foreground"
                                disabled={submittingDecision || !decisionComment.trim()}
                                onClick={() => handleDecision(selectedStep.id, "changes_requested")}
                              >
                                {submittingDecision ? <RefreshCw className="h-4 w-4 animate-spin" /> : "Reject & Retry"}
                              </Button>
                              <Button
                                className="rounded-xl px-5 h-10 bg-emerald-600 text-white hover:bg-emerald-700 border-none flex items-center gap-1.5"
                                disabled={submittingDecision}
                                onClick={() => handleDecision(selectedStep.id, "approved")}
                              >
                                {submittingDecision ? <RefreshCw className="h-4 w-4 animate-spin" /> : <>Approve & Continue &rarr;</>}
                              </Button>
                            </div>
                          </div>
                        </div>
                      );
                    } else {
                      return (
                        <div className="relative border border-border/60 bg-[#090a0f] rounded-xl p-3 focus-within:border-emerald-500/50 transition-colors">
                          <textarea
                            className="w-full min-h-[60px] pb-12 resize-none bg-transparent text-sm text-foreground focus:outline-none placeholder:text-muted-foreground/60 leading-relaxed"
                            placeholder="Type chat for follow-up interactions..."
                            value={decisionComment}
                            onChange={(e) => {
                              setDecisionComment(e.target.value);
                            }}
                            rows={2}
                          />
                          <div className="absolute bottom-3 right-3">
                            <Button
                              size="sm"
                              className="rounded-lg px-4 h-8 bg-emerald-600 hover:bg-emerald-700 text-white font-semibold text-xs transition-colors"
                              disabled={submittingDecision || !decisionComment.trim()}
                              onClick={() => handleDecision(selectedStep.id, "changes_requested")}
                            >
                              {submittingDecision ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : "Send"}
                            </Button>
                          </div>
                        </div>
                      );
                    }
                  })()}
                </div>
              </div>
            </div>

          </div>
        ) : (
          <div className="rounded-[1.6rem] border border-border bg-background/50 p-6 text-center text-muted-foreground">
            Select a step from the pipeline steps list to view details.
          </div>
        )}
      </main>
    </div>
  );
}
