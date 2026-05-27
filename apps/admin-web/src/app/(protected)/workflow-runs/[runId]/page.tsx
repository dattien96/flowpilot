import { notFound } from "next/navigation";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetWorkflowRunDetailUseCase } from "@/domain/usecase/workflow-runs/get-workflow-run-detail-usecase";
import { WorkflowTimeline } from "@/presentation/components/workflow-runs/workflow-timeline";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import { statusTone } from "@/presentation/view-models/factories";

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

export default async function WorkflowRunDetailPage({
  params,
}: {
  params: Promise<{ runId: string }>;
}) {
  const { runId } = await params;
  const gateways = await createGatewayBundle();
  const detail = await new GetWorkflowRunDetailUseCase(
    gateways.workflowGateway,
  ).execute(runId);

  if (!detail) {
    notFound();
  }

  let runTitle: string | null = null;
  try {
    const artifacts = await gateways.localRunnerGateway.listArtifacts();
    const runArtifacts = artifacts
      .filter(
        (artifact) =>
          artifact.workflowRunId === runId &&
          artifact.sourceKind === "workflow_output",
      )
      .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));

    const latestArtifact = runArtifacts[0];
    if (latestArtifact) {
      const artifactDetail =
        (await gateways.localRunnerGateway.getArtifactById(
          latestArtifact.artifactId,
        )) ?? latestArtifact;
      runTitle = summarizeRunPrompt(artifactDetail.promptText);
    }
  } catch {
    runTitle = null;
  }

  const pendingApproval = detail.approvals.find(
    (approval) => approval.status === "pending",
  );
  const outputByStepId = new Map(
    detail.outputs.map((output) => [output.workflowStepId, output]),
  );

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-3 lg:flex-row lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Workflow Run
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            {runTitle ?? detail.run.id}
          </h1>
          <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
            Run ID: {detail.run.id}
          </p>
        </div>
        <Badge tone={statusTone(detail.run.status)}>{detail.run.status}</Badge>
      </header>

      <section className="grid gap-4 xl:grid-cols-[0.95fr_1.05fr]">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-3">
            <h2 className="text-xl font-semibold">Step timeline</h2>
            <div className="flex gap-2">
              <form
                action={`/api/workflow-runs/${detail.run.id}/resume`}
                method="post"
              >
                <Button type="submit" variant="secondary">
                  Resume
                </Button>
              </form>
              <form
                action={`/api/workflow-runs/${detail.run.id}/terminate-session`}
                method="post"
              >
                <Button type="submit" variant="secondary">
                  Terminate session
                </Button>
              </form>
              <form
                action={`/api/workflow-runs/${detail.run.id}/cancel`}
                method="post"
              >
                <Button type="submit" variant="ghost">
                  Cancel
                </Button>
              </form>
            </div>
          </div>
          <div className="mt-4">
            <WorkflowTimeline steps={detail.steps} />
          </div>
          <div className="mt-6 space-y-3">
            {detail.steps.map((step) => {
              const output = outputByStepId.get(step.id);
              return (
                <div
                  key={step.id}
                  className="rounded-2xl border border-border bg-card p-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="font-semibold">{step.stepName}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {step.stepKey} · {step.status}
                      </p>
                    </div>
                    {output ? (
                      <a
                        className="text-sm font-medium text-accent"
                        href={`/outputs/${output.id}`}
                      >
                        Open output
                      </a>
                    ) : null}
                  </div>
                  {output ? (
                    <p className="mt-3 line-clamp-3 whitespace-pre-wrap text-sm text-muted-foreground">
                      {output.contentMarkdown}
                    </p>
                  ) : null}
                  {step.errorMessage ? (
                    <p className="mt-3 text-sm text-warning">
                      {step.errorMessage}
                    </p>
                  ) : null}
                </div>
              );
            })}
          </div>
        </div>
        <div className="space-y-4">
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-xl font-semibold">Selected context</h2>
            <div className="mt-4 space-y-3">
              {(detail.selectedContextSources ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No context selected.
                </p>
              ) : (
                detail.selectedContextSources?.map((context) => (
                  <div
                    key={context.id}
                    className="rounded-2xl border border-border bg-card p-4"
                  >
                    <p className="font-semibold">{context.title}</p>
                    <p className="mt-2 text-sm text-muted-foreground">
                      {context.summarizedContent ?? context.rawContent}
                    </p>
                  </div>
                ))
              )}
            </div>
          </div>
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-xl font-semibold">Latest output</h2>
            <article className="mt-4 whitespace-pre-wrap text-sm text-foreground">
              {detail.outputs.at(-1)?.contentMarkdown ??
                "No output generated yet."}
            </article>
          </div>
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-xl font-semibold">Approval panel</h2>
            {pendingApproval ? (
              <div className="mt-4 grid gap-3">
                <form
                  action={`/api/approvals/${pendingApproval.id}/decision`}
                  method="post"
                >
                  <textarea
                    className="mb-3 min-h-20 w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm"
                    name="comment"
                    placeholder="Decision comment"
                  />
                  <div className="flex flex-wrap gap-3">
                    <input name="decision" type="hidden" value="approved" />
                    <Button type="submit">Approve</Button>
                  </div>
                </form>
                <form
                  action={`/api/approvals/${pendingApproval.id}/decision`}
                  method="post"
                >
                  <input
                    name="decision"
                    type="hidden"
                    value="changes_requested"
                  />
                  <input
                    name="comment"
                    type="hidden"
                    value="Changes requested from run detail."
                  />
                  <Button type="submit" variant="secondary">
                    Request changes
                  </Button>
                </form>
                <form
                  action={`/api/approvals/${pendingApproval.id}/decision`}
                  method="post"
                >
                  <input name="decision" type="hidden" value="rejected" />
                  <Button type="submit" variant="ghost">
                    Reject
                  </Button>
                </form>
              </div>
            ) : (
              <p className="mt-4 text-sm text-muted-foreground">
                No pending approval for this run.
              </p>
            )}
            {(detail.approvalDecisions ?? []).length > 0 ? (
              <div className="mt-5 space-y-2">
                <p className="text-sm font-semibold">Decision history</p>
                {detail.approvalDecisions?.map((decision) => (
                  <div
                    key={decision.id}
                    className="rounded-2xl border border-border bg-card p-3 text-sm"
                  >
                    <Badge>{decision.decision}</Badge>
                    <p className="mt-2 text-muted-foreground">
                      {decision.comment ?? "No comment"}
                    </p>
                  </div>
                ))}
              </div>
            ) : null}
          </div>
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-xl font-semibold">Logs</h2>
            <div className="mt-4 space-y-2">
              {detail.logs.length === 0 ? (
                <p className="text-sm text-muted-foreground">No logs yet.</p>
              ) : (
                detail.logs.map((log) => (
                  <div
                    key={log.id}
                    className="rounded-2xl border border-border bg-card p-3 text-sm"
                  >
                    <p className="font-semibold">
                      {log.provider}/{log.model}
                    </p>
                    <p className="mt-1 text-muted-foreground">
                      {log.inputTokens + log.outputTokens} tokens · $
                      {log.costEstimate.toFixed(4)} · {log.latencyMs}ms
                    </p>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
