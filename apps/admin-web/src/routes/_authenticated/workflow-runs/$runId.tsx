import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  Play,
  X,
  ShieldAlert,
  Check,
  RefreshCw,
  FileText,
} from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";
import { WorkflowTimeline } from "@/presentation/components/workflow-runs/workflow-timeline";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GetWorkflowRunDetailUseCase } from "@/domain/usecase/workflow-runs/get-workflow-run-detail-usecase";
import { GetWorkflowRunDetailUseCase as GetWorkflowEngineRunDetailUseCase } from "@/domain/usecase/workflow-engine/get-workflow-run-detail-usecase";
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
import { statusTone } from "@/presentation/view-models/factories";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import { loadWorkflowRunPromptText } from "@/lib/workflow-run-prompt";

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
    <div className={`max-w-[85%] rounded-[1.6rem] px-6 py-4 shadow-sm ${
      isSecondary ? "bg-accent/90 text-accent-foreground" : "bg-accent text-accent-foreground"
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

const ArtifactContentViewer = ({ content, gateway }: { content: string, gateway: any }) => {
  const [expanded, setExpanded] = useState(false);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const TRUNCATE_LENGTH = 300;
  
  const handleLoadFile = async (path: string) => {
    setLoading(true);
    try {
      const data = await gateway.readFile(path);
      setFileContent(data);
    } catch (e) {
      window.alert("Failed to load file: " + (e instanceof Error ? e.message : "Unknown error"));
    } finally {
      setLoading(false);
    }
  };

  if (fileContent !== null) {
    return (
      <div className="space-y-4">
        <Button variant="secondary" onClick={() => setFileContent(null)}>
          &larr; Back to output
        </Button>
        <div className="rounded-2xl border border-border bg-card p-5 overflow-auto max-h-[600px] whitespace-pre-wrap font-mono text-sm">
          {fileContent}
        </div>
      </div>
    );
  }

  // Parse markdown link
  const linkRegex = /\[(.*?)\]\((.*?)\)/;
  const match = linkRegex.exec(content);

  const isLong = content.length > TRUNCATE_LENGTH;
  const displayContent = (!expanded && isLong) ? content.substring(0, TRUNCATE_LENGTH) + "..." : content;

  const onOpenClick = () => {
    if (!match) return;
    let path = match[2];
    if (path.startsWith("/abs/path/")) {
      path = path.substring(10);
    } else if (path.startsWith("file:///")) {
      path = path.substring(8);
    }
    handleLoadFile(path);
  };

  return (
    <div className="space-y-4">
      <article className="whitespace-pre-wrap text-foreground font-mono text-sm leading-relaxed">
        {displayContent}
      </article>
      
      <div className="flex flex-wrap gap-2 items-center pt-2">
        {isLong && (
          <Button variant="ghost" onClick={() => setExpanded(!expanded)} className="text-xs h-8">
            {expanded ? "Show less" : "Show more"}
          </Button>
        )}
        
        {match && (
          <Button 
            disabled={loading} 
            onClick={onOpenClick}
            className="text-xs h-8"
          >
            {loading ? <RefreshCw className="mr-2 h-3 w-3 animate-spin" /> : <FileText className="mr-2 h-3 w-3" />}
            View {match[1]}
          </Button>
        )}
      </div>
    </div>
  );
};

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
        setDetail(
          processed
            ? {
                ...processed,
                logs: engineDetail?.logs ?? [],
                sessions: engineDetail?.sessions ?? processed.sessions ?? [],
                runPromptText: promptText,
              }
            : processed,
        );
        setRunPromptText(promptText);
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
    let submitted = false;
    try {
      if (canOptimisticallyContinue) {
        const createdAt = new Date().toISOString();
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

  return (
    <PageFrame
      title={runTitle ?? `Run Detail: ${detail.run.id.slice(0, 8)}...`}
      description={`Workspace execution run of template definition.`}
      actions={
        <div className="flex flex-wrap items-center gap-2">
          <Link to="/workflow-runs">
            <Button variant="ghost">
              <ArrowLeft className="mr-2 h-4 w-4" /> History
            </Button>
          </Link>
          {detail.run.status !== "completed" &&
          detail.run.status !== "rejected" ? (
            <>
              <Button
                variant="secondary"
                disabled={processingAction || detail.run.status === "running"}
                onClick={handleResume}
              >
                <Play className="mr-2 h-4 w-4 fill-current" /> Resume
              </Button>
              <Button
                variant="ghost"
                disabled={processingAction}
                onClick={handleCancel}
              >
                <X className="mr-2 h-4 w-4" /> Cancel
              </Button>
            </>
          ) : null}
        </div>
      }
    >
      <div className="space-y-8">
        <header className="flex flex-col gap-3 rounded-[1.6rem] border border-border bg-background/40 p-6 md:flex-row md:items-center md:justify-between">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Execution Instance
            </p>
            <h1 className="mt-1 text-2xl font-bold tracking-tight md:text-3xl">
              {runTitle ?? detail.run.id}
            </h1>
            <p className="mt-1.5 break-all text-xs text-muted-foreground font-mono">
              Run ID: {detail.run.id}
            </p>
            <p className="mt-1 text-xs text-muted-foreground font-mono">
              Started At: {new Date(detail.run.startedAt).toLocaleString()}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Badge tone={statusTone(detail.run.status)}>
              {detail.run.status}
            </Badge>
            <div className="flex items-center gap-2 rounded-full border border-border bg-card/60 px-4 py-1.5 text-xs">
              <span className="font-medium text-muted-foreground">
                YOLO Mode
              </span>
              <button
                disabled={togglingYolo}
                onClick={handleToggleYolo}
                className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${
                  detail.run.yoloMode ? "bg-accent" : "bg-muted"
                }`}
              >
                <span
                  className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-background shadow ring-0 transition duration-200 ease-in-out ${
                    detail.run.yoloMode ? "translate-x-4" : "translate-x-0"
                  }`}
                />
              </button>
            </div>
          </div>
        </header>

        <section className="max-w-4xl mx-auto space-y-8 pb-32">
          
          {/* 1. Initial Prompt */}
          <div className="flex justify-end">
            <CollapsibleChatBubble
              title="Initial Prompt"
              content={runPromptText || "No initial prompt captured."}
            />
          </div>

          {/* 2. Step Outputs */}
          {detail.steps.map((step: any, index: number) => {
            const outputs =
              (outputsByStepId.get(step.id) as WorkflowOutputRecord[] | undefined) ?? [];
            const decisions = ((detail.approvalDecisions ?? []).filter(
              (d: any) => d.workflowStepId === step.id,
            ) as any[]).sort((left, right) =>
              left.createdAt.localeCompare(right.createdAt),
            );
            const timelineItems = buildWorkflowStepTimeline(outputs, decisions);
            const definitionStep = detail.definition?.steps?.find((ds: any) => ds.key === step.stepKey);
            const subagent = definitionStep?.subagent ?? null;
            const stepSession = (() => {
              if (!detail.sessions) return null;
              if (subagent) {
                return detail.sessions.find((s: any) => s.metadataJson?.step_run_id === step.id);
              } else {
                return detail.sessions.find((s: any) => s.metadataJson?.is_main === true || s.metadataJson?.is_main === "true");
              }
            })();
             
            return (
              <div key={step.id} className="space-y-4">
                {timelineItems.length === 0 ? (
                  <div className="flex justify-start">
                    <div className="max-w-[90%] w-full rounded-[1.6rem] border border-border/80 bg-card/40 p-6 shadow-sm backdrop-blur-sm">
                      <div className="flex flex-wrap items-center justify-between gap-3 mb-5 border-b border-border/40 pb-4">
                        <div>
                          <span className="font-bold text-foreground flex items-center gap-2">
                            {step.stepName}
                            <Badge tone={statusTone(step.status)} className="ml-2 px-2 py-0.5 text-[10px]">
                              {step.status}
                            </Badge>
                          </span>
                          <p className="mt-1 text-[11px] font-mono text-muted-foreground uppercase tracking-wider">
                            Step {index + 1}
                          </p>
                          {stepSession && (
                            <p className="mt-1 text-[10px] font-mono text-muted-foreground flex items-center gap-1.5 flex-wrap">
                              <span>Session:</span>
                              <span className="bg-accent/40 text-accent-foreground px-1.5 py-0.5 rounded font-bold uppercase tracking-wider">
                                {subagent ? `Isolated (${subagent})` : "Shared Main"}
                              </span>
                              <span>&bull;</span>
                              <span className="text-foreground/90 font-medium">
                                {stepSession.provider} ({stepSession.model})
                              </span>
                              <span>&bull;</span>
                              <span className={`font-semibold ${stepSession.status === "active" ? "text-success" : "text-muted-foreground"}`}>
                                {stepSession.status}
                              </span>
                              {stepSession.providerSessionId ? (
                                <>
                                  <span>&bull;</span>
                                  <span className="font-mono text-[10px] text-foreground/80">
                                    {stepSession.providerSessionId}
                                  </span>
                                </>
                              ) : null}
                            </p>
                          )}
                        </div>

                        <span className="rounded-full bg-muted border border-border/60 px-3 py-1 text-[10px] font-medium text-muted-foreground uppercase tracking-widest">
                          {step.status === "PENDING" || step.status === "RUNNING" ? "Processing..." : "No Artifact"}
                        </span>
                      </div>

                      {step.errorMessage ? (
                        <div className="rounded-2xl bg-destructive/10 border border-destructive/20 p-4 text-sm text-destructive">
                          {step.errorMessage}
                        </div>
                      ) : null}
                    </div>
                  </div>
                ) : (
                  timelineItems.map((item) =>
                    item.kind === "decision" ? (
                      <div key={item.key} className="flex justify-end mt-4">
                        <CollapsibleChatBubble
                          title="Follow-up"
                          time={new Date(item.decision.createdAt).toLocaleTimeString()}
                          content={item.decision.comment || `Decision: ${item.decision.decision}`}
                          isSecondary={true}
                        />
                      </div>
                    ) : (
                      <div key={item.key} className="flex justify-start">
                        <div className="max-w-[90%] w-full rounded-[1.6rem] border border-border/80 bg-card/40 p-6 shadow-sm backdrop-blur-sm transition-all hover:bg-card/60">
                          <div className="flex flex-wrap items-center justify-between gap-3 mb-5 border-b border-border/40 pb-4">
                            <div>
                              <span className="font-bold text-foreground flex items-center gap-2">
                                {step.stepName}
                                <Badge tone={statusTone(step.status)} className="ml-2 px-2 py-0.5 text-[10px]">
                                  {step.status}
                                </Badge>
                              </span>
                              <p className="mt-1 text-[11px] font-mono text-muted-foreground uppercase tracking-wider">
                                Step {index + 1}
                              </p>
                              {stepSession && (
                                <p className="mt-1 text-[10px] font-mono text-muted-foreground flex items-center gap-1.5 flex-wrap">
                                  <span>Session:</span>
                                  <span className="bg-accent/40 text-accent-foreground px-1.5 py-0.5 rounded font-bold uppercase tracking-wider">
                                    {subagent ? `Isolated (${subagent})` : "Shared Main"}
                                  </span>
                                  <span>&bull;</span>
                                  <span className="text-foreground/90 font-medium">
                                    {stepSession.provider} ({stepSession.model})
                                  </span>
                                  <span>&bull;</span>
                                  <span className={`font-semibold ${stepSession.status === "active" ? "text-success" : "text-muted-foreground"}`}>
                                    {stepSession.status}
                                  </span>
                                  {stepSession.providerSessionId ? (
                                    <>
                                      <span>&bull;</span>
                                      <span className="font-mono text-[10px] text-foreground/80">
                                        {stepSession.providerSessionId}
                                      </span>
                                    </>
                                  ) : null}
                                </p>
                              )}
                            </div>

                            <div className="text-right">
                              <span className="rounded-full bg-success/10 border border-success/20 px-3 py-1 text-[10px] font-medium text-success uppercase tracking-widest">
                                Artifact Generated
                              </span>
                            </div>
                          </div>

                          <div className="mb-6">
                            <ArtifactContentViewer
                              content={item.output.contentMarkdown || "No output content."}
                              gateway={gatewayBundle.current.localRunnerGateway}
                            />
                          </div>

                          <details className="mt-4 pt-4 border-t border-border/30">
                            <summary className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground cursor-pointer hover:text-foreground transition-colors inline-flex items-center gap-2">
                              Developer Diagnostics
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
                      </div>
                    ),
                  )
                )}
              </div>
            );
          })}

          <section className="max-w-4xl mx-auto">
            <CollapsibleSection
              title="Run Logs"
              subtitle="Use the filter to isolate runner session lifecycle events while testing provider reuse and restart recovery."
              defaultOpen={false}
            >
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="flex flex-wrap gap-2">
                  <Link
                    to="/workflow-runs/$runId"
                    params={{ runId }}
                    search={{ logView: undefined }}
                    className={`rounded-full border px-4 py-2 text-xs font-semibold uppercase tracking-[0.18em] transition-colors ${
                      !isSessionLogView
                        ? "border-accent bg-accent/10 text-accent"
                        : "border-border bg-card text-muted-foreground hover:text-foreground"
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
                    className={`rounded-full border px-4 py-2 text-xs font-semibold uppercase tracking-[0.18em] transition-colors ${
                      isSessionLogView
                        ? "border-accent bg-accent/10 text-accent"
                        : "border-border bg-card text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    Session events
                    <span className="ml-2 text-[10px] font-bold normal-case tracking-normal opacity-70">
                      {sessionEventLogs.length}
                    </span>
                  </Link>
                </div>
              </div>

              <div className="mt-4 space-y-3">
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
            </CollapsibleSection>
          </section>

          {/* 3. Follow-up Chat Input */}
          {(() => {
            const latestStep = detail.steps?.at(-1);
            if (!latestStep) return null;
            
            const stepStatus = latestStep.status?.toUpperCase();
            const runStatus = detail.run.status?.toUpperCase();
            const isWaiting = stepStatus === "WAITING_USER_APPROVAL";
            const isDone = stepStatus === "DONE" || stepStatus === "COMPLETED";
            const canContinue = (isWaiting || isDone) && runStatus !== "REJECTED" && runStatus !== "FAILED";

            if (!canContinue) return null;

            return (
              <div className="sticky bottom-6 mx-auto max-w-3xl mt-12 bg-card/90 backdrop-blur-xl p-3 rounded-[2rem] border border-border shadow-2xl transition-all">
                {isWaiting && (
                  <div className="px-4 pt-2 pb-3 mb-2 border-b border-border/50">
                    <p className="text-xs font-semibold text-warning flex items-center gap-2 uppercase tracking-wider">
                      <ShieldAlert className="h-3.5 w-3.5" /> Approval Required to proceed
                    </p>
                  </div>
                )}
                
                <div className="flex items-end gap-3 px-2 pb-1">
                  <textarea
                    className="flex-1 max-h-[200px] min-h-[50px] resize-none bg-transparent px-3 py-2 text-sm text-foreground focus:outline-none placeholder:text-muted-foreground/60"
                    placeholder={isWaiting ? "Provide revision notes..." : "Follow up with more instructions to refine this artifact..."}
                    value={decisionComment}
                    onChange={(e) => {
                      e.target.style.height = "auto";
                      e.target.style.height = `${e.target.scrollHeight}px`;
                      setDecisionComment(e.target.value);
                    }}
                    rows={1}
                  />
                  
                  <div className="flex flex-col gap-2 shrink-0">
                    <Button 
                      size="sm"
                      className="rounded-xl px-5 h-9"
                      disabled={submittingDecision || !decisionComment.trim()}
                      onClick={() => handleDecision(latestStep.id, "changes_requested")}
                    >
                      {submittingDecision ? <RefreshCw className="h-4 w-4 animate-spin" /> : "Send"}
                    </Button>
                    
                    {isWaiting && (
                      <Button 
                        size="sm"
                        variant="secondary"
                        className="rounded-xl px-5 h-9 bg-success/20 text-success hover:bg-success/30 border border-success/30"
                        disabled={submittingDecision}
                        onClick={() => handleDecision(latestStep.id, "approved")}
                      >
                        <Check className="mr-1.5 h-4 w-4" /> Approve
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            );
          })()}

        </section>
      </div>
    </PageFrame>
  );
}
