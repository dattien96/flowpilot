import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-runs/list-workflow-runs-usecase";
import { Badge } from "@/presentation/components/ui/badge";
import { statusTone } from "@/presentation/view-models/factories";
import { loadWorkflowRunTitleMap } from "@/lib/workflow-run-title";

export default async function WorkflowRunsPage() {
  const gateways = await createGatewayBundle();
  const runs = await new ListWorkflowRunsUseCase(
    gateways.workflowGateway,
  ).execute();
  const runTitles = await loadWorkflowRunTitleMap(
    gateways.localRunnerGateway,
    runs.map((run) => run.id),
  );

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Workflow Runs
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">
          Execution timeline registry
        </h1>
      </header>
      <div className="space-y-3">
        {runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
        ) : (
          runs.map((run) => (
            <Link
              key={run.id}
              className="flex items-center justify-between rounded-[1.6rem] border border-border bg-background/70 p-5"
              href={`/workflow-runs/${run.id}`}
            >
              <div>
                <p className="font-semibold">
                  {runTitles.get(run.id) ?? run.id}
                </p>
                {runTitles.has(run.id) ? (
                  <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                    Run ID: {run.id}
                  </p>
                ) : null}
                <p className="mt-1 text-sm text-muted-foreground">
                  Started {new Date(run.startedAt).toLocaleString()}
                </p>
              </div>
              <Badge tone={statusTone(run.status)}>{run.status}</Badge>
            </Link>
          ))
        )}
      </div>
    </div>
  );
}
