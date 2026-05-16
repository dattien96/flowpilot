import { Activity, CheckCircle2, Clock3, FolderOpen } from "lucide-react";
import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetDashboardSummaryUseCase } from "@/domain/usecase/dashboard/get-dashboard-summary-usecase";
import { StatCard } from "@/presentation/components/dashboard/stat-card";
import { Badge } from "@/presentation/components/ui/badge";
import { statusTone } from "@/presentation/view-models/factories";

export default async function DashboardPage() {
  const gateways = await createGatewayBundle();
  const summary = await new GetDashboardSummaryUseCase(
    gateways.projectGateway,
    gateways.featureGateway,
    gateways.contextSourceGateway,
    gateways.workflowGateway,
  ).execute();

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Operational Status
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            Workflow control at a glance.
          </h1>
        </div>
        <p className="max-w-xl text-sm text-muted-foreground">
          Demo mode is enabled. The admin shell still uses the same clean
          contracts that the real Supabase-backed AI orchestration layer will use.
        </p>
      </header>

      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Running"
          value={summary.activeWorkflowCount}
          hint="Workflow runs progressing without a human gate."
          accent={<Activity className="size-6 text-accent" />}
        />
        <StatCard
          label="Approvals"
          value={summary.pendingApprovalCount}
          hint="Outputs paused for an explicit human decision."
          accent={<Clock3 className="size-6 text-warning" />}
        />
        <StatCard
          label="Outputs"
          value={summary.completedOutputCount}
          hint="Stored artifacts available for review and export."
          accent={<CheckCircle2 className="size-6 text-success" />}
        />
        <StatCard
          label="Projects"
          value={summary.projectCount}
          hint="Registered product scopes available for intake."
          accent={<FolderOpen className="size-6 text-accent" />}
        />
      </section>

      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex items-end justify-between">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Recent Runs
            </p>
            <h2 className="mt-3 text-2xl font-semibold tracking-tight">
              Latest workflow activity
            </h2>
          </div>
          <Link className="text-sm font-medium text-accent" href="/workflow-runs">
            View all runs
          </Link>
        </div>
        <div className="mt-6 space-y-3">
          {summary.recentRuns.length === 0 ? (
            <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
          ) : (
            summary.recentRuns.map((run) => (
              <Link
                key={run.id}
                className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3 transition-colors hover:bg-muted/50"
                href={`/workflow-runs/${run.id}`}
              >
                <div>
                  <p className="font-semibold">{run.id}</p>
                  <p className="text-sm text-muted-foreground">
                    Started {new Date(run.startedAt).toLocaleString()}
                  </p>
                </div>
                <Badge tone={statusTone(run.status)}>{run.status}</Badge>
              </Link>
            ))
          )}
        </div>
      </section>
    </div>
  );
}
