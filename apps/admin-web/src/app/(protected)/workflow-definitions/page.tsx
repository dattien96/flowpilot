import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { ListWorkflowDefinitionsUseCase } from "@/domain/usecase/workflow-definitions/list-workflow-definitions-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export default async function WorkflowDefinitionsPage() {
  const gateways = await createGatewayBundle();
  const definitions = await new ListWorkflowDefinitionsUseCase(
    gateways.workflowGateway,
  ).execute();

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Workflow Definitions
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">
          Versioned orchestration templates
        </h1>
      </header>
      <div className="grid gap-4 xl:grid-cols-2">
        {definitions.map((definition) => (
          <Link
            key={definition.id}
            className="rounded-[1.6rem] border border-border bg-background/70 p-6"
            href={`/workflow-definitions/${definition.id}`}
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <h2 className="text-2xl font-semibold">{definition.name}</h2>
                <p className="mt-2 text-sm text-muted-foreground">
                  {definition.description}
                </p>
              </div>
              <Badge>{definition.status}</Badge>
            </div>
            <p className="mt-5 text-sm text-muted-foreground">
              Version {definition.version} · {definition.steps.length} steps
            </p>
          </Link>
        ))}
      </div>
    </div>
  );
}
