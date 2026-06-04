import type { AiOutput } from "@/domain/model/entity/workflow";
import type {
  ParsedTaskBreakdown,
  ParsedTaskBreakdownTask,
  TaskBoardStatus,
} from "@/features/workflow-engine/task-breakdown-parser";
import {
  formatTaskBoardStatus,
  groupTaskBreakdownTasksByStatus,
} from "@/features/workflow-engine/task-breakdown-parser";
import { Badge } from "@/presentation/components/ui/badge";

const boardColumns: Array<{
  status: TaskBoardStatus;
  tone: "neutral" | "success" | "warning" | "danger";
}> = [
  { status: "backlog", tone: "neutral" },
  { status: "ready", tone: "neutral" },
  { status: "in_progress", tone: "warning" },
  { status: "blocked", tone: "danger" },
  { status: "done", tone: "success" },
];

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function TaskCard({ task }: { task: ParsedTaskBreakdownTask }) {
  return (
    <article className="rounded-[1.25rem] border border-border bg-background/80 p-4">
      <div className="flex items-start justify-between gap-3">
        <h4 className="text-sm font-semibold leading-6">{task.title}</h4>
        <Badge tone="neutral">{formatTaskBoardStatus(task.status)}</Badge>
      </div>
      {task.milestone ? (
        <p className="mt-2 text-xs uppercase tracking-[0.2em] text-muted-foreground">
          {task.milestone}
        </p>
      ) : null}
      <dl className="mt-3 space-y-2 text-xs text-muted-foreground">
        <div className="flex flex-wrap gap-2">
          <span>{task.assignee ? `Assignee: ${task.assignee}` : "Unassigned"}</span>
          <span>{task.estimateHours !== null ? `Estimate: ${task.estimateHours}h` : "No estimate"}</span>
        </div>
        {task.dependencies.length > 0 ? (
          <div>Depends on: {task.dependencies.join(", ")}</div>
        ) : null}
      </dl>
      {task.rawText !== task.title ? (
        <p className="mt-3 line-clamp-4 whitespace-pre-wrap text-xs text-muted-foreground">
          {task.rawText}
        </p>
      ) : null}
    </article>
  );
}

export function TaskBreakdownBoard({
  output,
  parsed,
}: {
  output: Pick<AiOutput, "createdAt" | "id" | "isApproved" | "title" | "workflowRunId">;
  parsed: ParsedTaskBreakdown;
}) {
  const groupedTasks = groupTaskBreakdownTasksByStatus(parsed.tasks);

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Latest task breakdown
          </p>
          <h3 className="mt-3 text-2xl font-semibold tracking-tight">{parsed.title}</h3>
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
            Parsed from workflow output <span className="font-mono">{output.id}</span>
            {" "}for run <span className="font-mono">{output.workflowRunId}</span>.
          </p>
          {parsed.summary ? (
            <p className="mt-3 max-w-3xl text-sm text-muted-foreground">{parsed.summary}</p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge tone={output.isApproved ? "success" : "warning"}>
            {output.isApproved ? "approved" : "pending"}
          </Badge>
          <Badge tone="neutral">{formatDateTime(output.createdAt)}</Badge>
          <Badge tone="neutral">{parsed.tasks.length} tasks</Badge>
        </div>
      </div>

      <div className="mt-6 grid gap-4 xl:grid-cols-5">
        {boardColumns.map(({ status, tone }) => {
          const items = groupedTasks[status];
          return (
            <section key={status} className="rounded-[1.4rem] border border-border bg-card p-4">
              <div className="flex items-center justify-between gap-3">
                <h4 className="text-sm font-semibold">{formatTaskBoardStatus(status)}</h4>
                <Badge tone={tone}>{items.length}</Badge>
              </div>
              <div className="mt-4 space-y-3">
                {items.length === 0 ? (
                  <p className="text-xs text-muted-foreground">No tasks in this lane.</p>
                ) : (
                  items.map((task) => <TaskCard key={task.id} task={task} />)
                )}
              </div>
            </section>
          );
        })}
      </div>

      <div className="mt-6 rounded-[1.4rem] border border-border bg-card p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h4 className="text-sm font-semibold">Milestones</h4>
            <p className="mt-1 text-xs text-muted-foreground">
              Grouped by the headings found in the task breakdown artifact.
            </p>
          </div>
          <Badge tone="neutral">{parsed.sections.length} sections</Badge>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {parsed.sections.map((section) => (
            <div key={section.title} className="rounded-[1.1rem] border border-border bg-background/70 p-4">
              <h5 className="text-sm font-semibold">{section.title}</h5>
              <p className="mt-2 text-xs text-muted-foreground">{section.tasks.length} tasks</p>
            </div>
          ))}
        </div>
      </div>

      <details className="mt-6 rounded-[1.4rem] border border-border bg-card p-5">
        <summary className="cursor-pointer text-sm font-semibold">Raw artifact markdown</summary>
        <pre className="mt-4 overflow-x-auto whitespace-pre-wrap text-sm text-muted-foreground">
          {parsed.rawMarkdown}
        </pre>
      </details>
    </section>
  );
}
