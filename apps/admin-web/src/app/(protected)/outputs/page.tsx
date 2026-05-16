import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";
import { ListOutputsUseCase } from "@/domain/usecase/outputs/list-outputs-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export default async function OutputsPage({
  searchParams,
}: {
  searchParams: Promise<{
    projectId?: string;
    featureId?: string;
    workflowRunId?: string;
    outputType?: string;
    approvalState?: string;
  }>;
}) {
  const params = await searchParams;
  const outputType = [
    "business_summary",
    "product_spec",
    "android_tech_spec",
    "task_breakdown",
    "test_plan",
    "risk_report",
  ].includes(params.outputType ?? "")
    ? (params.outputType as ListOutputsFilters["outputType"])
    : undefined;
  const filters: ListOutputsFilters = {
    projectId: params.projectId || undefined,
    featureId: params.featureId || undefined,
    workflowRunId: params.workflowRunId || undefined,
    outputType,
    approvalState:
      params.approvalState === "approved" || params.approvalState === "pending"
        ? params.approvalState
        : undefined,
  };
  const gateways = await createGatewayBundle();
  const outputs = await new ListOutputsUseCase(gateways.workflowGateway).execute(filters);

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Output Library
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">
          Persisted workflow outputs
        </h1>
      </header>
      <form className="grid gap-3 rounded-[1.6rem] border border-border bg-background/70 p-5 lg:grid-cols-5">
        <input
          className="rounded-2xl border border-border bg-card px-4 py-3"
          defaultValue={filters.projectId ?? ""}
          name="projectId"
          placeholder="Project id"
        />
        <input
          className="rounded-2xl border border-border bg-card px-4 py-3"
          defaultValue={filters.featureId ?? ""}
          name="featureId"
          placeholder="Feature id"
        />
        <input
          className="rounded-2xl border border-border bg-card px-4 py-3"
          defaultValue={filters.workflowRunId ?? ""}
          name="workflowRunId"
          placeholder="Run id"
        />
        <select
          className="rounded-2xl border border-border bg-card px-4 py-3"
          defaultValue={filters.approvalState ?? ""}
          name="approvalState"
        >
          <option value="">Any approval</option>
          <option value="approved">approved</option>
          <option value="pending">pending</option>
        </select>
        <button className="rounded-full bg-accent px-4 py-3 text-sm font-semibold text-accent-foreground">
          Filter
        </button>
      </form>
      <div className="grid gap-4 xl:grid-cols-2">
        {outputs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No workflow outputs yet.</p>
        ) : (
          outputs.map((output) => (
            <Link
              key={output.id}
              className="rounded-[1.6rem] border border-border bg-background/70 p-5"
              href={`/outputs/${output.id}`}
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-semibold">{output.title}</h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    {output.outputType} · version {output.version}
                  </p>
                </div>
                <Badge tone={output.isApproved ? "success" : "warning"}>
                  {output.isApproved ? "approved" : "pending"}
                </Badge>
              </div>
              <p className="mt-4 line-clamp-4 whitespace-pre-wrap text-sm text-muted-foreground">
                {output.contentMarkdown}
              </p>
            </Link>
          ))
        )}
      </div>
    </div>
  );
}
