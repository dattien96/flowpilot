import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { ListPendingApprovalsUseCase } from "@/domain/usecase/approvals/list-pending-approvals-usecase";
import { Button } from "@/presentation/components/ui/button";

export default async function ApprovalsPage() {
  const gateways = await createGatewayBundle();
  const approvals = await new ListPendingApprovalsUseCase(
    gateways.workflowGateway,
  ).execute();

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Approval Center
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Pending human decisions</h1>
      </header>
      <div className="space-y-3">
        {approvals.length === 0 ? (
          <p className="text-sm text-muted-foreground">No pending approvals.</p>
        ) : (
          approvals.map((item) => (
            <div
              key={item.approval.id}
              className="rounded-[1.6rem] border border-border bg-background/70 p-5"
            >
              <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <p className="font-semibold">
                    {item.step?.stepName ?? item.approval.workflowStepId}
                  </p>
                  <p className="mt-2 text-sm text-muted-foreground">
                    {item.project?.name ?? item.run.projectId}
                  </p>
                </div>
                <Link className="text-sm font-medium text-accent" href={`/workflow-runs/${item.run.id}`}>
                  Open run
                </Link>
              </div>
              <article className="mt-4 max-h-56 overflow-hidden rounded-2xl border border-border bg-card p-4 text-sm whitespace-pre-wrap">
                {item.output?.contentMarkdown ?? "No output attached to this approval."}
              </article>
              <form
                action={`/api/approvals/${item.approval.id}/decision`}
                method="post"
                className="mt-4 grid gap-3"
              >
                <textarea
                  className="min-h-20 rounded-2xl border border-border bg-card px-4 py-3 text-sm"
                  name="comment"
                  placeholder="Decision comment"
                />
                <div className="flex flex-wrap gap-2">
                  <Button name="decision" type="submit" value="approved">
                    Approve
                  </Button>
                  <Button
                    name="decision"
                    type="submit"
                    value="changes_requested"
                    variant="secondary"
                  >
                    Request changes
                  </Button>
                  <Button name="decision" type="submit" value="rejected" variant="ghost">
                    Reject
                  </Button>
                </div>
              </form>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
