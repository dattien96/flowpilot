import { notFound } from "next/navigation";
import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { GetProjectDetailUseCase } from "@/domain/usecase/projects/get-project-detail-usecase";
import { Badge } from "@/presentation/components/ui/badge";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export default async function ProjectDetailPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  const gateways = await createGatewayBundle();
  const detail = await new GetProjectDetailUseCase(
    gateways.projectGateway,
    gateways.workflowGateway,
    gateways.teamGateway,
  ).execute(projectId);

  if (!detail) {
    notFound();
  }

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-3 lg:flex-row lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Project Detail
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            {detail.project.name}
          </h1>
          <p className="mt-3 max-w-2xl text-muted-foreground">
            {detail.project.description}
          </p>
        </div>
        <div className="flex flex-col items-start gap-2">
          <Badge>{detail.project.platform}</Badge>
          <Badge>{detail.project.status}</Badge>
        </div>
      </header>

      <ProjectSectionNav projectId={projectId} />

      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Workflow Runs</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Workflow runs are the execution sections inside this project.
          </p>
          <div className="mt-4 space-y-3">
            {detail.workflowRuns.length === 0 ? (
              <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
            ) : (
              detail.workflowRuns.map((run) => (
                <div
                  key={run.id}
                  className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                >
                  <span className="font-semibold">{run.id}</span>
                  <Badge>{run.status}</Badge>
                </div>
              ))
            )}
          </div>
        </div>
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Teams</h2>
          <div className="mt-4 space-y-3">
            {detail.teams.length === 0 ? (
              <p className="text-sm text-muted-foreground">No teams are linked to this project yet.</p>
            ) : (
              detail.teams.map((team) => (
                <div
                  key={team.id}
                  className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                >
                  <span className="font-semibold">{team.name}</span>
                  <Badge>linked</Badge>
                </div>
              ))
            )}
          </div>
        </div>
      </section>

      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <h2 className="text-xl font-semibold">Project members snapshot</h2>
        <div className="mt-4 space-y-3">
          {detail.members.length === 0 ? (
            <p className="text-sm text-muted-foreground">No members have been added yet.</p>
          ) : (
            detail.members.map((member) => (
              <div key={member.id} className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3">
                <div>
                  <p className="font-semibold">{member.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {member.role} • {member.levelLabel}
                  </p>
                </div>
                <Badge>{member.weeklyCapacityHours}h</Badge>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  );
}
