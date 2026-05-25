import Link from "next/link";
import { notFound } from "next/navigation";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetOutputDetailUseCase } from "@/domain/usecase/outputs/get-output-detail-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export default async function OutputDetailPage({
  params,
}: {
  params: Promise<{ outputId: string }>;
}) {
  const { outputId } = await params;
  const gateways = await createGatewayBundle();
  const detail = await new GetOutputDetailUseCase(
    gateways.workflowGateway,
  ).execute(outputId);

  if (!detail) {
    notFound();
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Output Detail
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            {detail.output.title}
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">
            {detail.project?.name ?? detail.output.projectId}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge tone={detail.output.isApproved ? "success" : "warning"}>
            {detail.output.isApproved ? "approved" : "pending"}
          </Badge>
          <Link
            className="rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold"
            href={`/api/outputs/${detail.output.id}/export`}
          >
            Export markdown
          </Link>
        </div>
      </header>
      <section className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <article className="rounded-[1.6rem] border border-border bg-background/70 p-6 whitespace-pre-wrap">
          {detail.output.contentMarkdown}
        </article>
        <aside className="space-y-4">
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-5">
            <h2 className="font-semibold">Metadata</h2>
            <dl className="mt-4 space-y-3 text-sm">
              <div>
                <dt className="text-muted-foreground">Run</dt>
                <dd>
                  {detail.run ? (
                    <Link className="text-accent" href={`/workflow-runs/${detail.run.id}`}>
                      {detail.run.id}
                    </Link>
                  ) : (
                    detail.output.workflowRunId
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Step</dt>
                <dd>{detail.step?.stepName ?? detail.output.workflowStepId}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Version</dt>
                <dd>{detail.output.version}</dd>
              </div>
            </dl>
          </div>
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-5">
            <h2 className="font-semibold">Decision history</h2>
            <div className="mt-4 space-y-3">
              {detail.approvalDecisions.length === 0 ? (
                <p className="text-sm text-muted-foreground">No decisions yet.</p>
              ) : (
                detail.approvalDecisions.map((decision) => (
                  <div key={decision.id} className="rounded-2xl border border-border bg-card p-3">
                    <Badge>{decision.decision}</Badge>
                    <p className="mt-2 text-sm text-muted-foreground">
                      {decision.comment ?? "No comment"}
                    </p>
                  </div>
                ))
              )}
            </div>
          </div>
        </aside>
      </section>
    </div>
  );
}
