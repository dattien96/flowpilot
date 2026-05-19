import { createGatewayBundle } from "@/data/repository/factory";
import { Badge } from "@/presentation/components/ui/badge";
import { ProjectSectionNav } from "@/components/project/project-section-nav";

export default async function ProjectMembersPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  const gateways = await createGatewayBundle();
  const teams = await gateways.teamGateway.listTeamsByProject(projectId);
  const members = teams.length === 0 ? [] : (await Promise.all(teams.map((team) => gateways.teamGateway.listMembersByTeam(team.id)))).flat();

  return (
    <div className="space-y-6">
      <ProjectSectionNav projectId={projectId} />
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Team Members
            </p>
            <h1 className="mt-3 text-3xl font-semibold tracking-tight">Project team roster</h1>
          </div>
          <Badge>{members.length} members</Badge>
        </div>
        <div className="mt-6 space-y-3">
          {members.length === 0 ? (
            <p className="text-sm text-muted-foreground">No members linked to this project yet.</p>
          ) : (
            members.map((member) => (
              <div key={member.id} className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3">
                <div>
                  <p className="font-semibold">{member.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {member.role} • {member.levelLabel}
                  </p>
                </div>
                <Badge>{member.weeklyCapacityHours}h/week</Badge>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  );
}
