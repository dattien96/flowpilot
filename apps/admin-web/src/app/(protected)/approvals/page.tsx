import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { ListPendingApprovalsUseCase } from "@/domain/usecase/approvals/list-pending-approvals-usecase";

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
          approvals.map((approval) => (
            <Link
              key={approval.id}
              className="block rounded-[1.6rem] border border-border bg-background/70 p-5"
              href={`/workflow-runs/${approval.workflowRunId}`}
            >
              <p className="font-semibold">{approval.id}</p>
              <p className="mt-2 text-sm text-muted-foreground">
                Review step {approval.workflowStepId} in run {approval.workflowRunId}
              </p>
            </Link>
          ))
        )}
      </div>
    </div>
  );
}
