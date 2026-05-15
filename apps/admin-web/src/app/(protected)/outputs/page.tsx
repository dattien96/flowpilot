import { createGatewayBundle } from "@/data/repository/factory";
import { ListOutputsUseCase } from "@/domain/usecase/outputs/list-outputs-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export default async function OutputsPage() {
  const gateways = await createGatewayBundle();
  const outputs = await new ListOutputsUseCase(gateways.workflowGateway).execute();

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Outputs Library
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Stored workflow artifacts</h1>
      </header>
      <div className="space-y-3">
        {outputs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No outputs yet.</p>
        ) : (
          outputs.map((output) => (
            <div
              key={output.id}
              className="rounded-[1.6rem] border border-border bg-background/70 p-5"
            >
              <div className="flex items-center justify-between gap-4">
                <h2 className="text-xl font-semibold">{output.title}</h2>
                <Badge tone={output.isApproved ? "success" : "warning"}>
                  {output.outputType}
                </Badge>
              </div>
              <p className="mt-3 text-sm text-muted-foreground">
                {output.contentMarkdown.slice(0, 180)}...
              </p>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
