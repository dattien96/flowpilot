import { createGatewayBundle } from "@/data/repository/factory";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-runs/list-workflow-runs-usecase";
import { loadWorkflowRunTitleMap } from "@/lib/workflow-run-title";
import { WorkflowRunsList } from "./workflow-runs-list";

export default async function WorkflowRunsPage() {
  const gateways = await createGatewayBundle();
  const runs = await new ListWorkflowRunsUseCase(
    gateways.workflowGateway,
  ).execute();
  const runTitles = await loadWorkflowRunTitleMap(
    gateways.localRunnerGateway,
    runs.map((run) => run.id),
  );
  const titleByRunId = Object.fromEntries(runTitles);

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
      <WorkflowRunsList runs={runs} titleByRunId={titleByRunId} />
    </div>
  );
}
