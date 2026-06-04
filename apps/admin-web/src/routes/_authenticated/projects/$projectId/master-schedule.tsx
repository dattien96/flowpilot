import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListOutputsUseCase } from "@/domain/usecase/outputs/list-outputs-usecase";
import { GenerationLaunchPanel } from "@/features/ai-orchestration/generation-launch-panel";
import { parseTaskBreakdownMarkdown } from "@/features/workflow-engine/task-breakdown-parser";
import { Badge } from "@/presentation/components/ui/badge";
import { Link } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/projects/$projectId/master-schedule")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const [bindings, taskBreakdownOutputs] = await Promise.all([
      gateways.projectGateway.listProjectWorkspaceBindings(params.projectId),
      new ListOutputsUseCase(gateways.workflowGateway).execute({
        projectId: params.projectId,
        outputType: "task_breakdown",
      }),
    ]);
    return { bindings, projectId: params.projectId, taskBreakdownOutputs };
  },
  component: ProjectMasterSchedulePage,
});

function ProjectMasterSchedulePage() {
  const { bindings, projectId, taskBreakdownOutputs } = Route.useLoaderData();
  const latestOutput = taskBreakdownOutputs[0] ?? null;
  const latestParsedOutput = latestOutput ? parseTaskBreakdownMarkdown(latestOutput.contentMarkdown) : null;

  return (
    <PageFrame
      actions={
        <Link
          className="rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold"
          to="/projects/$projectId/tasks"
          params={{ projectId }}
        >
          Open task board
        </Link>
      }
      description="Master schedule generation for project management."
      title="Master Schedule"
    >
      <ProjectSectionNav projectId={projectId} />
      <div className="mt-6">
        <GenerationLaunchPanel
          buttonLabel="Generate Schedule"
          description="Convert the coding plan into milestone slices and delivery checkpoints for the project schedule."
          promptHint="Include implementation order, checkpoints, and any delivery constraints that should shape the schedule."
          promptLabel="Coding plan source"
          promptPlaceholder="Paste the coding plan or the delivery assumptions that should be planned."
          bindings={bindings}
          projectId={projectId}
          stepType="task_breakdown"
          title="Generate master schedule"
        />
      </div>
      <section className="mt-6 rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Generated output
            </p>
            <h3 className="mt-3 text-2xl font-semibold tracking-tight">Recent task breakdown artifacts</h3>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              The task breakdown workflow step is the source artifact for the project schedule and the task board.
            </p>
          </div>
          <Badge tone="neutral">{taskBreakdownOutputs.length} artifacts</Badge>
        </div>

        {latestOutput && latestParsedOutput ? (
          <div className="mt-6 grid gap-4 lg:grid-cols-[1.4fr_0.6fr]">
            <article className="rounded-[1.4rem] border border-border bg-card p-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <h4 className="text-lg font-semibold">{latestOutput.title}</h4>
                <Badge tone={latestOutput.isApproved ? "success" : "warning"}>
                  {latestOutput.isApproved ? "approved" : "pending"}
                </Badge>
              </div>
              <p className="mt-3 text-sm text-muted-foreground">
                {latestParsedOutput.summary ?? "No summary lines were detected in this artifact."}
              </p>
              <p className="mt-4 text-sm text-muted-foreground">
                Parsed tasks: <span className="font-semibold text-foreground">{latestParsedOutput.tasks.length}</span>
              </p>
            </article>
            <aside className="rounded-[1.4rem] border border-border bg-card p-5">
              <h4 className="text-sm font-semibold">Artifact list</h4>
              <div className="mt-4 space-y-3">
                {taskBreakdownOutputs.slice(0, 5).map((output) => (
                  <div key={output.id} className="rounded-2xl border border-border bg-background/70 p-4">
                    <p className="text-sm font-medium">{output.title}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {new Intl.DateTimeFormat(undefined, {
                        dateStyle: "medium",
                        timeStyle: "short",
                      }).format(new Date(output.createdAt))}
                    </p>
                  </div>
                ))}
              </div>
            </aside>
          </div>
        ) : (
          <p className="mt-6 text-sm text-muted-foreground">
            No task breakdown artifacts have been generated yet.
          </p>
        )}
      </section>
    </PageFrame>
  );
}
