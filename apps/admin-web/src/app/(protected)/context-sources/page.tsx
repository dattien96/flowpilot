import { createGatewayBundle } from "@/data/repository/factory";
import { ListContextSourcesUseCase } from "@/domain/usecase/context-sources/list-context-sources-usecase";

export default async function ContextSourcesPage() {
  const gateways = await createGatewayBundle();
  const contexts = await new ListContextSourcesUseCase(
    gateways.contextSourceGateway,
  ).execute("project_meal_suggestion");

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Context Sources
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Attached workflow context</h1>
      </header>
      <div className="space-y-3">
        {contexts.map((context) => (
          <div
            key={context.id}
            className="rounded-[1.6rem] border border-border bg-background/70 p-5"
          >
            <p className="font-semibold">{context.title}</p>
            <p className="mt-2 text-sm text-muted-foreground">{context.rawContent}</p>
          </div>
        ))}
      </div>
    </div>
  );
}
