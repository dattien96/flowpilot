import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/settings/runner")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const health = await new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute();
    return { health };
  },
  component: RunnerPage,
});

function RunnerPage() {
  const { health } = Route.useLoaderData();
  return <RunnerContent health={health} />;
}

export function RunnerContent({ health }: { health: { baseUrl: string; cwd: string | null; errorMessage: string | null; os: string | null; runnerVersion: string | null; startedAt: string | null; status: "online" | "offline" } }) {
  const online = health.status === "online";

  return (
    <PageFrame
      description="System-wide runner status and reachability for all local automation flows."
      title="Runner"
    >
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Runner Reachability
            </p>
            <h3 className="mt-3 text-2xl font-semibold tracking-tight">
              Local runner control plane
            </h3>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              This page is the single place to check whether the local runner is available for
              MCP installs, verification, and other system-wide tasks.
            </p>
          </div>
          <Badge tone={online ? "success" : "danger"}>{health.status}</Badge>
        </div>

        <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <DetailRow label="Base URL" value={health.baseUrl} />
          <DetailRow label="Version" value={health.runnerVersion ?? "Unavailable"} />
          <DetailRow label="Workspace" value={health.cwd ?? "Unavailable"} />
          <DetailRow label="Platform" value={health.os ?? "Unavailable"} />
        </div>

        <div
          className={`mt-4 rounded-2xl border px-4 py-3 text-sm ${online ? "border-success/30 bg-success/10 text-success" : "border-danger/30 bg-danger/10 text-danger"
            }`}
        >
          {health.errorMessage ??
            (online
              ? "The runner is online and ready for operations."
              : "The runner is offline, so operations will be blocked until it starts.")}
        </div>
      </section>
    </PageFrame>
  );
}

function DetailRow({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}
