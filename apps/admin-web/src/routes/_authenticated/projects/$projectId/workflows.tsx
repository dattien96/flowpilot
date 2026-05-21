import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowRight, Clock3, FolderKanban, PlayCircle, Plus, Sparkles } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-engine/list-workflow-runs-usecase";
import { ListWorkflowsUseCase } from "@/domain/usecase/workflow-engine/list-workflows-usecase";
import type { Workflow, WorkflowRun } from "@/domain/model/entity/workflow-engine";

export const Route = createFileRoute("/_authenticated/projects/$projectId/workflows")({
  component: ProjectWorkflowsPage,
});

function ProjectWorkflowsPage() {
  const { projectId } = Route.useParams();
  const gatewayBundle = useRef(createGatewayBundle());
  const listWorkflowRunsUseCase = useRef(
    new ListWorkflowRunsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const listWorkflowsUseCase = useRef(
    new ListWorkflowsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [runRows, workflowRows] = await Promise.all([
          listWorkflowRunsUseCase.current.execute(projectId),
          listWorkflowsUseCase.current.execute(projectId),
        ]);
        setRuns(runRows);
        setWorkflows(workflowRows);
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [projectId]);

  const workflowNameById = useMemo(
    () => new Map(workflows.map((workflow) => [workflow.id, workflow.name])),
    [workflows]
  );

  return (
    <PageFrame
      title="Workflows"
      description="Use the dedicated workflow definition pages to browse, create, and compose workflows for this project."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflows" search={{ projectId }}>
            <Button variant="secondary">Browse definitions</Button>
          </Link>
          <Link to="/workflow-steps">
            <Button variant="secondary">Workflow steps</Button>
          </Link>
          <Link to="/workflows/create" search={{ projectId }}>
            <Button>Create workflow</Button>
          </Link>
        </div>
      }
    >
      <div className="space-y-6">
        <ProjectSectionNav projectId={projectId} />

        <div className="grid gap-4 lg:grid-cols-3">
          <Link
            className="rounded-[1.5rem] border border-border bg-background/60 p-5 transition-colors hover:bg-background"
            to="/workflows"
            search={{ projectId }}
          >
            <div className="flex items-center gap-3">
              <div className="rounded-2xl bg-accent/20 p-3 text-accent-foreground">
                <FolderKanban className="h-5 w-5" />
              </div>
              <div>
                <h3 className="font-semibold">Browse Definitions</h3>
                <p className="text-sm text-muted-foreground">
                  Open the workflow catalog and jump into a workflow builder page.
                </p>
              </div>
            </div>
            <div className="mt-4 flex items-center gap-2 text-sm font-medium text-primary">
              Open definitions <ArrowRight className="h-4 w-4" />
            </div>
          </Link>

          <Link
            className="rounded-[1.5rem] border border-border bg-background/60 p-5 transition-colors hover:bg-background"
            to="/workflows/create"
            search={{ projectId }}
          >
            <div className="flex items-center gap-3">
              <div className="rounded-2xl bg-emerald-500/15 p-3 text-emerald-500">
                <Plus className="h-5 w-5" />
              </div>
              <div>
                <h3 className="font-semibold">Create Private Workflow</h3>
                <p className="text-sm text-muted-foreground">
                  Create a project-owned workflow and then continue composing it on its own page.
                </p>
              </div>
            </div>
            <div className="mt-4 flex items-center gap-2 text-sm font-medium text-primary">
              Create new workflow <ArrowRight className="h-4 w-4" />
            </div>
          </Link>

          <Link
            className="rounded-[1.5rem] border border-border bg-background/60 p-5 transition-colors hover:bg-background"
            to="/workflow-runs"
          >
            <div className="flex items-center gap-3">
              <div className="rounded-2xl bg-amber-500/15 p-3 text-amber-500">
                <PlayCircle className="h-5 w-5" />
              </div>
              <div>
                <h3 className="font-semibold">View Run History</h3>
                <p className="text-sm text-muted-foreground">
                  Review all workflow runs and filter the history across projects and scopes.
                </p>
              </div>
            </div>
            <div className="mt-4 flex items-center gap-2 text-sm font-medium text-primary">
              Open run history <ArrowRight className="h-4 w-4" />
            </div>
          </Link>
        </div>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold">Recent Runs</h2>
              <p className="text-sm text-muted-foreground">
                Project-specific execution history stays here, while composing moves to dedicated workflow pages.
              </p>
            </div>
            <Link to="/workflow-runs">
              <Button variant="secondary">All run history</Button>
            </Link>
          </div>

          <div className="mt-5 space-y-3">
            {loading ? (
              <div className="rounded-2xl border border-dashed border-border bg-card px-4 py-8 text-sm text-muted-foreground">
                Loading recent runs...
              </div>
            ) : null}
            {!loading && runs.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-border bg-card px-4 py-8 text-sm text-muted-foreground">
                No runs yet for this project. Start from the workflow definition pages when you are ready.
              </div>
            ) : null}
            {runs.slice(0, 8).map((run) => (
              <div
                key={run.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-4"
              >
                <div>
                  <p className="font-medium">{workflowNameById.get(run.workflowId) ?? run.workflowId}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{run.status}</p>
                </div>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Clock3 className="h-4 w-4" />
                  <span>{new Date(run.startedAt).toLocaleString()}</span>
                </div>
              </div>
            ))}
          </div>
        </section>

        <section className="rounded-[1.5rem] border border-dashed border-border bg-background/40 p-5">
          <div className="flex items-start gap-3">
            <div className="rounded-2xl bg-cyan-500/15 p-3 text-cyan-500">
              <Sparkles className="h-5 w-5" />
            </div>
            <div>
              <h2 className="font-semibold">Builder Has Moved</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                The workflow composer now lives on each workflow’s own detail page so the list page can act as the source of truth for selecting what to edit or run.
              </p>
            </div>
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
