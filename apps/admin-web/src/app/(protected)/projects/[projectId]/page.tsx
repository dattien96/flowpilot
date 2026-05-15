import { notFound } from "next/navigation";
import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetProjectDetailUseCase } from "@/domain/usecase/projects/get-project-detail-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export default async function ProjectDetailPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  const gateways = await createGatewayBundle();
  const detail = await new GetProjectDetailUseCase(
    gateways.projectGateway,
    gateways.featureGateway,
    gateways.workflowGateway,
  ).execute(projectId);

  if (!detail) {
    notFound();
  }

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-3 lg:flex-row lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Project Detail
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            {detail.project.name}
          </h1>
          <p className="mt-3 max-w-2xl text-muted-foreground">
            {detail.project.description}
          </p>
        </div>
        <Badge>{detail.project.platform}</Badge>
      </header>

      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Features</h2>
          <div className="mt-4 space-y-3">
            {detail.features.map((feature) => (
              <Link
                key={feature.id}
                className="block rounded-2xl border border-border bg-card px-4 py-3"
                href={`/features/${feature.id}`}
              >
                <p className="font-semibold">{feature.title}</p>
                <p className="text-sm text-muted-foreground">{feature.businessGoal}</p>
              </Link>
            ))}
          </div>
        </div>
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Workflow Runs</h2>
          <div className="mt-4 space-y-3">
            {detail.workflowRuns.length === 0 ? (
              <p className="text-sm text-muted-foreground">No runs for this project yet.</p>
            ) : (
              detail.workflowRuns.map((run) => (
                <Link
                  key={run.id}
                  className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                  href={`/workflow-runs/${run.id}`}
                >
                  <span className="font-semibold">{run.id}</span>
                  <Badge>{run.status}</Badge>
                </Link>
              ))
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
