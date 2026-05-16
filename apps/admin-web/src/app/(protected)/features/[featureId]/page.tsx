import { notFound } from "next/navigation";
import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetFeatureDetailUseCase } from "@/domain/usecase/features/get-feature-detail-usecase";
import { Button } from "@/presentation/components/ui/button";

export default async function FeatureDetailPage({
  params,
}: {
  params: Promise<{ featureId: string }>;
}) {
  const { featureId } = await params;
  const gateways = await createGatewayBundle();
  const detail = await new GetFeatureDetailUseCase(
    gateways.featureGateway,
    gateways.contextSourceGateway,
    gateways.workflowGateway,
  ).execute(featureId);

  if (!detail) {
    notFound();
  }

  const workflowDefinition = detail.definitions[0];

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-4 lg:flex-row lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Feature Detail
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            {detail.feature.title}
          </h1>
          <p className="mt-3 max-w-3xl text-muted-foreground">
            {detail.feature.expectedFlow}
          </p>
        </div>
        {workflowDefinition ? (
          <form action="/api/workflow-runs" method="post" className="self-start">
            <input name="featureId" type="hidden" value={detail.feature.id} />
            <input
              name="workflowDefinitionId"
              type="hidden"
              value={workflowDefinition.id}
            />
            <input
              name="contextSourceIds"
              type="hidden"
              value=""
            />
            <div className="mb-3 max-w-sm rounded-2xl border border-border bg-background/70 p-3">
              <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                Selected context
              </p>
              <div className="mt-2 space-y-2">
                {detail.contexts.map((context) => (
                  <label key={context.id} className="flex items-center gap-2 text-sm">
                    <input
                      defaultChecked
                      name="contextSourceIds"
                      type="checkbox"
                      value={context.id}
                    />
                    <span>{context.title}</span>
                  </label>
                ))}
              </div>
            </div>
            <Button type="submit">Start demo workflow</Button>
          </form>
        ) : null}
      </header>

      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Context Sources</h2>
          <div className="mt-4 space-y-3">
            {detail.contexts.map((context) => (
              <div
                key={context.id}
                className="rounded-2xl border border-border bg-card px-4 py-3"
              >
                <p className="font-semibold">{context.title}</p>
                <p className="mt-2 text-sm text-muted-foreground">{context.rawContent}</p>
              </div>
            ))}
          </div>
        </div>
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Related Runs</h2>
          <div className="mt-4 space-y-3">
            {detail.runs.length === 0 ? (
              <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
            ) : (
              detail.runs.map((run) => (
                <Link
                  key={run.id}
                  className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                  href={`/workflow-runs/${run.id}`}
                >
                  <span className="font-semibold">{run.id}</span>
                  <span className="text-sm text-muted-foreground">{run.status}</span>
                </Link>
              ))
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
