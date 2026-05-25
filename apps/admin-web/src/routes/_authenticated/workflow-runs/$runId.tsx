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
import { SubmitStepApprovalDecisionUseCase } from "@/domain/usecase/workflow-engine/submit-step-approval-decision-usecase";
import { createSupabaseBrowserClient } from "@/data/datasource/supabase/client";
import { statusTone } from "@/presentation/view-models/factories";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import { loadWorkflowRunPromptText } from "@/lib/workflow-run-prompt";

type WorkflowOutputRecord = {
  id: string;
  workflowRunId: string;
  workflowStepId: string;
  projectId: string;
  outputType: string;
  version: number;
  title: string;
  contentMarkdown: string;
  isApproved: boolean;
  createdAt: string;
  promptText?: string;
  stdoutText?: string;
  stderrText?: string;
  commandText?: string;
  localPath?: string;
};

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
  component: WorkflowRunDetailPage,
});

function WorkflowRunDetailPage() {
  const { runId } = Route.useParams();
  const gatewayBundle = useRef(createGatewayBundle());
  const getWorkflowRunDetailUseCase = useRef(
    new GetWorkflowRunDetailUseCase(gatewayBundle.current.workflowGateway),
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

      mappedOutputs = artifactDetails.map((art: LocalRunnerArtifact) => {
        const step = data.steps?.find(
          (s: any) =>
            s.stepKey?.toLowerCase() === art.workflowStepKey?.toLowerCase() ||
            s.stepType?.toLowerCase() === art.workflowStepKey?.toLowerCase(),
        );

        return {
          id: art.artifactId,
          workflowRunId: art.workflowRunId,
          workflowStepId: step ? step.id : art.workflowStepKey,
          projectId: art.projectId,
          outputType: "document",
          version: 1,
          title: art.title,
          contentMarkdown: art.contentMarkdown,
          isApproved: true,
          createdAt: art.createdAt,
          promptText: art.promptText,
          stdoutText: art.stdoutText,
          stderrText: art.stderrText,
          commandText: art.commandText,
          localPath: art.localPath,
        };
      });
    } catch (artifactErr) {
      console.warn(
        "Failed to fetch local runner artifacts, using DB outputs only:",
        artifactErr,
      );
    }

    const combinedOutputs = [...(data.outputs || [])];
    for (const localOut of mappedOutputs) {
      const exists = combinedOutputs.some(
        (out: any) =>
          out.id === localOut.id ||
          out.workflowStepId === localOut.workflowStepId,
      );
      if (!exists) {
        combinedOutputs.push(localOut);
      } else {
        const idx = combinedOutputs.findIndex(
          (out: any) =>
            out.id === localOut.id ||
            out.workflowStepId === localOut.workflowStepId,
        );
        if (idx !== -1) {
          combinedOutputs[idx] = { ...combinedOutputs[idx], ...localOut };
        }
      }
    }

    return {
      ...data,
      outputs: combinedOutputs,
    };
  };

  const loadData = async () => {
    if (!runId) return;
    try {
      const [data, promptText] = await Promise.all([
        getWorkflowRunDetailUseCase.current.execute(runId),
        loadWorkflowRunPromptText(
          gatewayBundle.current.localRunnerGateway,
          runId,
        ),
      ]);
      if (data) {
        const processed = await processDetailData(data);
        setDetail(processed ? { ...processed, runPromptText: promptText } : processed);
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

  const outputByStepId = useMemo(() => {
    if (!detail?.outputs) return new Map();
    return new Map(
      detail.outputs.map((output: WorkflowOutputRecord) => [
        output.workflowStepId,
        output,
      ]),
    );
  }, [detail?.outputs]);

  const latestOutput = useMemo<WorkflowOutputRecord | null>(() => {
    if (!detail?.outputs || detail.outputs.length === 0) {
      return null;
    }

    return detail.outputs.at(-1) ?? null;
  }, [detail?.outputs]);

  const runTitle = useMemo(
    () => summarizeRunPrompt(runPromptText ?? latestOutput?.promptText),
    [latestOutput?.promptText, runPromptText],
  );

  const pendingApproval = useMemo(() => {
    if (!detail?.approvals) return null;
    return detail.approvals.find(
      (approval: any) => approval.status === "pending",
    );
  }, [detail?.approvals]);

  const handleDecision = async (
    decision: "approved" | "changes_requested" | "rejected",
  ) => {
    if (!pendingApproval) return;
    setSubmittingDecision(true);
    try {
      if (decision === "approved" || decision === "changes_requested") {
        await submitStepApprovalDecisionUseCase.current.execute(
          pendingApproval.workflowStepId,
          decision === "approved",
          decisionComment || undefined,
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
      await loadData();
      setDecisionComment("");
    } catch (err: any) {
      alert(`Decision submission failed: ${err.message}`);
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

        <section className="grid gap-6 xl:grid-cols-[1fr_1.1fr]">
          {/* Left Column: Timeline & Step outputs */}
          <div className="space-y-6">
            <CollapsibleSection
              title="Step Timeline"
              subtitle="Expand each area below to inspect the prompt, output, and diagnostics for this run."
              defaultOpen
            >
              <WorkflowTimeline steps={detail.steps} />
            </CollapsibleSection>

            <div className="space-y-4">
              <h2 className="text-xl font-bold tracking-tight pl-2">
                Timeline Output Records
              </h2>
              {detail.steps.map((step: any) => {
                const output = outputByStepId.get(step.id) as
                  | WorkflowOutputRecord
                  | undefined;
                return (
                  <details
                    key={step.id}
                    className="rounded-[1.6rem] border border-border bg-background/50 shadow-sm transition-all hover:border-border/80"
                    open={Boolean(
                      output?.contentMarkdown ||
                      output?.promptText ||
                      step.errorMessage,
                    )}
                  >
                    <summary className="flex cursor-pointer list-none items-start justify-between gap-3 px-5 py-5">
                      <div>
                        <p className="font-bold text-foreground">
                          {step.stepName}
                        </p>
                        <p className="mt-1 text-xs font-mono text-muted-foreground">
                          {step.stepKey} · {step.status}
                        </p>
                      </div>
                      {output ? (
                        <span className="rounded-full bg-success/10 border border-success/20 px-2.5 py-0.5 text-xs font-medium text-success uppercase tracking-wider">
                          Output ready
                        </span>
                      ) : (
                        <span className="rounded-full bg-muted border border-border px-2.5 py-0.5 text-xs font-medium text-muted-foreground uppercase tracking-wider">
                          No output
                        </span>
                      )}
                    </summary>

                    <div className="space-y-4 border-t border-border/60 px-5 py-5">
                      {output?.localPath ? (
                        <div className="rounded-2xl border border-border/60 bg-card/40 px-4 py-3">
                          <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-muted-foreground">
                            Local Artifact Path
                          </p>
                          <p className="mt-2 break-words font-mono text-xs text-foreground">
                            {output.localPath}
                          </p>
                        </div>
                      ) : null}

                      <div className="space-y-3">
                        <CollapsibleTextBlock
                          title="Prompt Used"
                          value={output?.promptText}
                          emptyLabel="No prompt was captured for this step."
                          defaultOpen
                        />
                        <CollapsibleTextBlock
                          title="Generated Output"
                          value={output?.contentMarkdown}
                          emptyLabel="No output was generated for this step."
                          defaultOpen={Boolean(output?.contentMarkdown)}
                        />
                        <CollapsibleTextBlock
                          title="Error / Stderr"
                          value={output?.stderrText ?? step.errorMessage}
                          emptyLabel="No stderr was captured."
                        />
                        <CollapsibleTextBlock
                          title="Stdout"
                          value={output?.stdoutText}
                          emptyLabel="No stdout was captured."
                        />
                        <CollapsibleTextBlock
                          title="Command"
                          value={output?.commandText}
                          emptyLabel="No command was captured."
                        />
                      </div>
                    </div>
                  </details>
                );
              })}
            </div>
          </div>

          {/* Right Column: Dynamic Panel details */}
          <div className="space-y-6">
            {/* Selected Context Sources */}
            <CollapsibleSection title="Selected Context Sources" defaultOpen>
              <div className="space-y-3">
                {(detail.selectedContextSources ?? []).length === 0 ? (
                  <p className="text-sm text-muted-foreground italic">
                    No context selected.
                  </p>
                ) : (
                  detail.selectedContextSources?.map((context: any) => (
                    <div
                      key={context.id}
                      className="rounded-2xl border border-border/80 bg-card/40 p-4 shadow-sm"
                    >
                      <p className="font-bold text-foreground text-sm">
                        {context.title}
                      </p>
                      <p className="mt-2 text-xs leading-relaxed text-muted-foreground font-mono">
                        {context.summarizedContent ?? context.rawContent}
                      </p>
                    </div>
                  ))
                )}
              </div>
            </CollapsibleSection>

            <CollapsibleSection
              title="Prompt Used For This Run"
              subtitle="This is the latest captured step prompt sent to the local runner."
              defaultOpen
            >
              <CollapsibleTextBlock
                title="Latest Prompt"
                value={runPromptText ?? latestOutput?.promptText}
                emptyLabel="No prompt has been captured for this run yet."
                defaultOpen
              />
            </CollapsibleSection>

            <CollapsibleSection
              title="Provider Command"
              subtitle="The exact CLI command executed by the local runner."
              defaultOpen={false}
            >
              <CollapsibleTextBlock
                title="Execution Command"
                value={latestOutput?.commandText}
                emptyLabel="No command captured."
                defaultOpen
              />
            </CollapsibleSection>

            {/* Latest Output Panel */}
            <CollapsibleSection title="Latest Execution Output" defaultOpen>
              <div className="rounded-2xl border border-border/40 bg-card/60 p-5">
                {latestOutput ? (
                  <ArtifactContentViewer 
                    content={latestOutput.contentMarkdown || "No output generated yet."}
                    gateway={gatewayBundle.current.localRunnerGateway}
                  />
                ) : (
                  <p className="text-muted-foreground italic font-mono text-sm">
                    No output generated yet.
                  </p>
                )}
              </div>
            </CollapsibleSection>

            {/* Approval Gate Panel */}
            <CollapsibleSection title="Approval Panel" defaultOpen>
              {pendingApproval ? (
                <div className="space-y-4">
                  <div className="rounded-2xl border border-warning/20 bg-warning/5 p-4 text-sm text-warning/90">
                    <p className="font-semibold flex items-center gap-2">
                      <ShieldAlert className="h-4 w-4" /> Approval requested
                    </p>
                    <p className="mt-1 text-xs">
                      A step requires manual confirmation. Provide review notes
                      and authorize.
                    </p>
                  </div>
                  <textarea
                    className="min-h-24 w-full rounded-2xl border border-border bg-card/60 px-4 py-3 text-sm focus:border-accent focus:ring-1 focus:ring-accent outline-none"
                    value={decisionComment}
                    onChange={(e) => setDecisionComment(e.target.value)}
                    placeholder="Provide comments or revision notes..."
                  />
                  <div className="grid grid-cols-3 gap-2.5">
                    <Button
                      disabled={submittingDecision}
                      onClick={() => void handleDecision("approved")}
                      className="bg-success text-success-foreground hover:bg-success/90"
                    >
                      <Check className="mr-1.5 h-4 w-4" /> Approve
                    </Button>
                    <Button
                      variant="secondary"
                      disabled={submittingDecision}
                      onClick={() => void handleDecision("changes_requested")}
                    >
                      Request Changes
                    </Button>
                    <Button
                      variant="ghost"
                      disabled={submittingDecision}
                      onClick={() => void handleDecision("rejected")}
                      className="text-destructive hover:bg-destructive/10"
                    >
                      <X className="mr-1.5 h-4 w-4" /> Reject
                    </Button>
                  </div>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground italic">
                  No pending approval triggers found.
                </p>
              )}

              {/* Decision History */}
              {(detail.approvalDecisions ?? []).length > 0 ? (
                <div className="mt-6 border-t border-border pt-4 space-y-3">
                  <p className="text-sm font-bold text-foreground">
                    Decision Logs
                  </p>
                  {detail.approvalDecisions?.map((decision: any) => (
                    <div
                      key={decision.id}
                      className="rounded-2xl border border-border bg-card/35 p-3 text-xs shadow-sm"
                    >
                      <div className="flex items-center justify-between mb-2">
                        <Badge
                          tone={
                            decision.decision === "approved"
                              ? "success"
                              : decision.decision === "rejected"
                                ? "danger"
                                : "warning"
                          }
                        >
                          {decision.decision}
                        </Badge>
                        <span className="text-[10px] text-muted-foreground font-mono">
                          {new Date(decision.createdAt).toLocaleDateString()}
                        </span>
                      </div>
                      <p className="text-muted-foreground font-mono bg-background/25 p-2 rounded-xl border border-border/20">
                        {decision.comment ?? "No comment."}
                      </p>
                    </div>
                  ))}
                </div>
              ) : null}
            </CollapsibleSection>

            {/* AI Engine & Metrics Logs */}
            <CollapsibleSection title="Engine Calls & Metrics">
              <div className="space-y-3">
                {detail.logs.length === 0 ? (
                  <p className="text-sm text-muted-foreground italic">
                    No metrics reported yet.
                  </p>
                ) : (
                  detail.logs.map((log: any) => (
                    <div
                      key={log.id}
                      className="rounded-2xl border border-border bg-card/40 p-4 text-xs font-mono shadow-sm"
                    >
                      <div className="flex items-center justify-between font-bold text-foreground">
                        <span>
                          {log.provider}/{log.model}
                        </span>
                        <span className="text-success">
                          ${log.costEstimate.toFixed(4)}
                        </span>
                      </div>
                      <div className="mt-2 grid grid-cols-3 gap-2 text-muted-foreground text-[10px]">
                        <div>Tokens: {log.inputTokens + log.outputTokens}</div>
                        <div className="text-center">
                          Latency: {log.latencyMs}ms
                        </div>
                        <div className="text-right uppercase">
                          Status: {log.status}
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </CollapsibleSection>
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
