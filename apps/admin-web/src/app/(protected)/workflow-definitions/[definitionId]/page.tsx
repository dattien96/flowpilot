import { notFound } from "next/navigation";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetWorkflowDefinitionDetailUseCase } from "@/domain/usecase/workflow-definitions/get-workflow-definition-detail-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export default async function WorkflowDefinitionDetailPage({
  params,
}: {
  params: Promise<{ definitionId: string }>;
}) {
  const { definitionId } = await params;
  const gateways = await createGatewayBundle();
  const definition = await new GetWorkflowDefinitionDetailUseCase(
    gateways.workflowGateway,
  ).execute(definitionId);

  if (!definition) {
    notFound();
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Workflow Definition
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            {definition.name}
          </h1>
          <p className="mt-3 max-w-3xl text-muted-foreground">
            {definition.description}
          </p>
        </div>
        <Badge>{definition.status}</Badge>
      </header>
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <h2 className="text-xl font-semibold">Step contract</h2>
        <div className="mt-5 space-y-3">
          {definition.steps.map((step, index) => (
            <div
              key={step.key}
              className="grid gap-3 rounded-2xl border border-border bg-card px-4 py-4 lg:grid-cols-[70px_1fr_180px_180px]"
            >
              <span className="font-mono text-sm text-muted-foreground">
                #{index + 1}
              </span>
              <span className="font-semibold">{step.name}</span>
              <Badge>{step.type}</Badge>
              <span className="text-sm text-muted-foreground">
                {step.outputType ?? "no output"}
              </span>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
