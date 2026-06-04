import { createFileRoute } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListOutputsUseCase } from "@/domain/usecase/outputs/list-outputs-usecase";
import { TaskBreakdownBoard } from "@/components/project/task-breakdown-board";
import { Link } from "@tanstack/react-router";
import { parseTaskBreakdownMarkdown } from "@/features/workflow-engine/task-breakdown-parser";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/projects/$projectId/tasks")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const taskBreakdownOutputs = await new ListOutputsUseCase(gateways.workflowGateway).execute({
      projectId: params.projectId,
      outputType: "task_breakdown",
    });
    return { projectId: params.projectId, taskBreakdownOutputs };
  },
  component: ProjectTasksPage,
});

function ProjectTasksPage() {
  const { projectId, taskBreakdownOutputs } = Route.useLoaderData();
  const latestOutput = taskBreakdownOutputs[0] ?? null;
  const parsed = latestOutput ? parseTaskBreakdownMarkdown(latestOutput.contentMarkdown) : null;

  return (
    <PageFrame
      actions={
        <Link
          className="rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold"
          to="/projects/$projectId/master-schedule"
          params={{ projectId }}
        >
          Back to schedule
        </Link>
      }
      description="Task board derived from generated task breakdown artifacts."
      title="Tasks"
    >
      <ProjectSectionNav projectId={projectId} />
      {latestOutput && parsed ? (
        <div className="mt-6 space-y-6">
          <section className="rounded-[1.6rem] border border-border bg-background/70 p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                  Task board source
                </p>
                <h3 className="mt-3 text-2xl font-semibold tracking-tight">{latestOutput.title}</h3>
                <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
                  The latest `task_breakdown` artifact is parsed into milestone lanes and task status columns.
                </p>
              </div>
              <Badge tone={latestOutput.isApproved ? "success" : "warning"}>
                {latestOutput.isApproved ? "approved" : "pending"}
              </Badge>
            </div>
          </section>

          <TaskBreakdownBoard output={latestOutput} parsed={parsed} />
        </div>
      ) : (
        <section className="mt-6 rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h3 className="text-2xl font-semibold tracking-tight">No task breakdown artifacts yet</h3>
          <p className="mt-3 max-w-2xl text-sm text-muted-foreground">
            Generate a master schedule first, then return here to see the parsed task board.
          </p>
        </section>
      )}
    </PageFrame>
  );
}
