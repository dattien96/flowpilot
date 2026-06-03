import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { createGatewayBundle } from "@/data/repository/factory";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-runs/list-workflow-runs-usecase";
import { loadWorkflowRunTitleMap } from "@/lib/workflow-run-title";
import { WorkflowRunsList } from "./workflow-runs-list";

export default async function WorkflowRunsPage() {
  const gateways = await createGatewayBundle();
  const supabase = createSupabaseServerClient();
  const runs = await new ListWorkflowRunsUseCase(
    gateways.workflowGateway,
  ).execute();
  const runTitles = await loadWorkflowRunTitleMap(
    gateways.localRunnerGateway,
    runs.map((run) => run.id),
    {
      listArtifactRuns: () => gateways.workflowEngineGateway.listArtifactRuns(),
      loadArtifactContent: async (artifactRun) => {
        if (artifactRun.storageProvider !== "supabase" || !artifactRun.remotePath.trim()) {
          return null;
        }

        const { data, error } = await supabase.storage
          .from("flowpilot-artifacts")
          .download(artifactRun.remotePath.trim());
        if (error || !data) {
          return null;
        }

        return await data.text();
      },
    },
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
