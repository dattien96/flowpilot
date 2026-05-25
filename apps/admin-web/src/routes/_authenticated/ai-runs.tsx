import type { ReactNode } from "react";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { AiRun, AiRunSummary, AiRunStatus } from "@/domain/model/entity/ai-orchestration";
import { ListAiRunsUseCase } from "@/domain/usecase/ai-orchestration/list-ai-runs-usecase";
import { Badge } from "@/presentation/components/ui/badge";

function providerForModel(modelName: string) {
  if (modelName.startsWith("gpt-")) {
    return "codex";
  }
  if (modelName.startsWith("gemini-")) {
    return "gemini";
  }
  if (modelName.startsWith("claude-")) {
    return "claude";
  }

  return "unknown";
}

export const Route = createFileRoute("/_authenticated/ai-runs")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const useCase = new ListAiRunsUseCase(gateways.aiOrchestrationGateway);
    const [runs, summary] = await Promise.all([useCase.execute(), useCase.summarize()]);
    return { runs, summary };
  },
  component: AiRunsPage,
});

function AiRunsPage() {
  const { runs, summary } = Route.useLoaderData();
  return <AiRunsContent runs={runs} summary={summary} />;
}

export function AiRunsContent({
  runs,
  summary,
}: {
  runs: AiRun[];
  summary: AiRunSummary;
}) {
  const [statusFilter, setStatusFilter] = useState<"all" | AiRunStatus>("all");
  const [providerFilter, setProviderFilter] = useState("all");
  const [projectFilter, setProjectFilter] = useState("all");
  const [modelFilter, setModelFilter] = useState("all");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  const projectOptions = Array.from(new Set(runs.map((run) => run.projectId))).sort();
  const providerOptions = Array.from(
    new Set(runs.map((run) => run.provider ?? providerForModel(run.modelName))),
  ).sort();
  const modelOptions = Array.from(new Set(runs.map((run) => run.modelName))).sort();
  const filteredRuns = runs
    .filter((run) => statusFilter === "all" || run.status === statusFilter)
    .filter(
      (run) => providerFilter === "all" || (run.provider ?? providerForModel(run.modelName)) === providerFilter,
    )
    .filter((run) => projectFilter === "all" || run.projectId === projectFilter)
    .filter((run) => modelFilter === "all" || run.modelName === modelFilter)
    .filter((run) => matchesDateRange(run.createdAt, dateFrom, dateTo));
  const filteredSummary = summarizeRuns(filteredRuns);

  return (
    <PageFrame
      title="AI Runs"
      description="Monitor lineage-backed AI executions, provider cost, and artifact outputs from browser-triggered and runner-triggered flows."
      actions={
        <div className="flex flex-wrap gap-2">
          <FilterButton
            active={statusFilter === "all"}
            label={`All (${summary.totalRuns})`}
            onClick={() => setStatusFilter("all")}
          />
          <FilterButton
            active={statusFilter === "running"}
            label={`Running (${summary.runningRuns})`}
            onClick={() => setStatusFilter("running")}
          />
          <FilterButton
            active={statusFilter === "success"}
            label={`Success (${summary.successfulRuns})`}
            onClick={() => setStatusFilter("success")}
          />
          <FilterButton
            active={statusFilter === "failed"}
            label={`Failed (${summary.failedRuns})`}
            onClick={() => setStatusFilter("failed")}
          />
        </div>
      }
    >
      <div className="grid gap-3 md:grid-cols-4">
        <MetricCard label="Visible Runs" value={String(filteredSummary.totalRuns)} />
        <MetricCard label="Input Tokens" value={formatNumber(filteredSummary.totalInputTokens)} />
        <MetricCard
          label="Output Tokens"
          value={formatNumber(filteredSummary.totalOutputTokens)}
        />
        <MetricCard label="Cost" value={formatCurrency(filteredSummary.totalCostUsd)} />
      </div>

      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Execution Ledger
            </p>
            <h3 className="mt-3 text-2xl font-semibold tracking-tight">
              Call-level AI telemetry
            </h3>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              Every durable generation should link back to a workflow run, workflow step,
              and output artifact run. Retry from this view starts a fresh single-step lineage.
            </p>
          </div>
          <div className="rounded-2xl border border-border bg-card/80 px-4 py-3 text-sm">
            <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">Filter</p>
            <p className="mt-2 font-semibold capitalize">{statusFilter}</p>
          </div>
        </div>

        <div className="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <FilterField label="Project">
            <select
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) => setProjectFilter(event.target.value)}
              value={projectFilter}
            >
              <option value="all">All projects</option>
              {projectOptions.map((projectId) => (
                <option key={projectId} value={projectId}>
                  {projectId}
                </option>
              ))}
            </select>
          </FilterField>
          <FilterField label="Provider">
            <select
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) => setProviderFilter(event.target.value)}
              value={providerFilter}
            >
              <option value="all">All providers</option>
              {providerOptions.map((provider) => (
                <option key={provider} value={provider}>
                  {provider}
                </option>
              ))}
            </select>
          </FilterField>
          <FilterField label="Model">
            <select
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) => setModelFilter(event.target.value)}
              value={modelFilter}
            >
              <option value="all">All models</option>
              {modelOptions.map((modelName) => (
                <option key={modelName} value={modelName}>
                  {modelName}
                </option>
              ))}
            </select>
          </FilterField>
          <FilterField label="Created On Or After">
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) => setDateFrom(event.target.value)}
              type="date"
              value={dateFrom}
            />
          </FilterField>
          <FilterField label="Created On Or Before">
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) => setDateTo(event.target.value)}
              type="date"
              value={dateTo}
            />
          </FilterField>
        </div>

        <div className="mt-6 space-y-4">
          {filteredRuns.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-border px-4 py-8 text-center text-sm text-muted-foreground">
              No AI runs matched the selected filter.
            </div>
          ) : null}

          {filteredRuns.map((run) => (
            <article
              key={run.id}
              className="rounded-[1.4rem] border border-border bg-card/80 p-5"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                <div className="flex flex-wrap items-center gap-2">
                  <h4 className="text-lg font-semibold">{run.runType}</h4>
                  <Badge tone={toneForStatus(run.status)}>{run.status}</Badge>
                  <Badge tone="neutral">{run.provider ?? providerForModel(run.modelName)}</Badge>
                  <Badge tone="neutral">{run.modelName}</Badge>
                  <Badge tone="neutral">{run.reasoningEffort ?? "default reasoning"}</Badge>
                </div>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Project <span className="font-medium text-foreground">{run.projectId}</span>
                    {" | "}Created {formatTimestamp(run.createdAt)}
                  </p>
                </div>
                <div className="text-right text-sm">
                  <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                    Cost
                  </p>
                  <p className="mt-2 font-semibold">{formatCurrency(run.costUsd ?? 0)}</p>
                </div>
              </div>

              <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <DetailCard label="Workflow Run" value={run.workflowRunId ?? "Not linked"} />
                <DetailCard label="Workflow Step" value={run.workflowRunStepId ?? "Not linked"} />
                <DetailCard label="Artifact Run" value={run.artifactRunId ?? "Pending"} />
                <DetailCard
                  label="Tokens"
                  value={`${formatNumber(run.tokensInput ?? 0)} in / ${formatNumber(
                    run.tokensOutput ?? 0
                  )} out`}
                />
              </div>

              <div className="mt-5 grid gap-3 md:grid-cols-2">
                <JsonPanel label="Input Payload" value={run.inputPayload} />
                <JsonPanel label="Output Payload" value={run.outputPayload ?? {}} />
              </div>

              {run.errorMessage ? (
                <div className="mt-4 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                  {run.errorMessage}
                </div>
              ) : null}
            </article>
          ))}
        </div>
      </section>
    </PageFrame>
  );
}

function FilterButton({
  active,
  label,
  onClick,
}: Readonly<{ active: boolean; label: string; onClick: () => void }>) {
  return (
    <Button onClick={onClick} variant={active ? "primary" : "secondary"}>
      {label}
    </Button>
  );
}

function FilterField({
  children,
  label,
}: Readonly<{ children: ReactNode; label: string }>) {
  return (
    <label className="space-y-2 text-sm">
      <span className="font-medium">{label}</span>
      {children}
    </label>
  );
}

function MetricCard({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-2xl font-semibold tracking-tight">{value}</p>
    </div>
  );
}

function DetailCard({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-background p-4">
      <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

function JsonPanel({
  label,
  value,
}: Readonly<{ label: string; value: Record<string, unknown> }>) {
  return (
    <div className="rounded-2xl border border-border bg-background p-4">
      <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">{label}</p>
      <pre className="mt-3 overflow-x-auto whitespace-pre-wrap break-words text-xs text-muted-foreground">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

function summarizeRuns(runs: AiRun[]): AiRunSummary {
  return {
    totalRuns: runs.length,
    runningRuns: runs.filter((run) => run.status === "running").length,
    successfulRuns: runs.filter((run) => run.status === "success").length,
    failedRuns: runs.filter((run) => run.status === "failed").length,
    totalInputTokens: runs.reduce((sum, run) => sum + (run.tokensInput ?? 0), 0),
    totalOutputTokens: runs.reduce((sum, run) => sum + (run.tokensOutput ?? 0), 0),
    totalCostUsd: runs.reduce((sum, run) => sum + (run.costUsd ?? 0), 0),
  };
}

function toneForStatus(status: AiRunStatus) {
  if (status === "success") return "success";
  if (status === "failed") return "danger";
  return "warning";
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("en-US").format(value);
}

function formatCurrency(value: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function formatTimestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }

  return parsed.toLocaleString("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

function matchesDateRange(value: string, from: string, to: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return !from && !to;
  }

  if (from) {
    const start = new Date(`${from}T00:00:00`);
    if (parsed < start) {
      return false;
    }
  }

  if (to) {
    const end = new Date(`${to}T23:59:59`);
    if (parsed > end) {
      return false;
    }
  }

  return true;
}
