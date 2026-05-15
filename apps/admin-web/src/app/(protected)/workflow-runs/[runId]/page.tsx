import { notFound } from "next/navigation";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetWorkflowRunDetailUseCase } from "@/domain/usecase/workflow-runs/get-workflow-run-detail-usecase";
import { WorkflowTimeline } from "@/presentation/components/workflow-runs/workflow-timeline";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import { statusTone } from "@/presentation/view-models/factories";

export default async function WorkflowRunDetailPage({
  params,
}: {
  params: Promise<{ runId: string }>;
}) {
  const { runId } = await params;
  const gateways = await createGatewayBundle();
  const detail = await new GetWorkflowRunDetailUseCase(
    gateways.workflowGateway,
  ).execute(runId);

  if (!detail) {
    notFound();
  }

  const pendingApproval = detail.approvals.find((approval) => approval.status === "pending");

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-3 lg:flex-row lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Workflow Run
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">{detail.run.id}</h1>
        </div>
        <Badge tone={statusTone(detail.run.status)}>{detail.run.status}</Badge>
      </header>

      <section className="grid gap-4 xl:grid-cols-[0.95fr_1.05fr]">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Step timeline</h2>
          <div className="mt-4">
            <WorkflowTimeline steps={detail.steps} />
          </div>
        </div>
        <div className="space-y-4">
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-xl font-semibold">Latest output</h2>
            <article className="mt-4 whitespace-pre-wrap text-sm text-foreground">
              {detail.outputs.at(-1)?.contentMarkdown ?? "No output generated yet."}
            </article>
          </div>
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-xl font-semibold">Approval panel</h2>
            {pendingApproval ? (
              <div className="mt-4 flex flex-wrap gap-3">
                <form action={`/api/approvals/${pendingApproval.id}/decision`} method="post">
                  <input name="decision" type="hidden" value="approved" />
                  <Button type="submit">Approve</Button>
                </form>
                <form action={`/api/approvals/${pendingApproval.id}/decision`} method="post">
                  <input name="decision" type="hidden" value="changes_requested" />
                  <Button type="submit" variant="secondary">
                    Request changes
                  </Button>
                </form>
                <form action={`/api/approvals/${pendingApproval.id}/decision`} method="post">
                  <input name="decision" type="hidden" value="rejected" />
                  <Button type="submit" variant="ghost">
                    Reject
                  </Button>
                </form>
              </div>
            ) : (
              <p className="mt-4 text-sm text-muted-foreground">
                No pending approval for this run.
              </p>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
