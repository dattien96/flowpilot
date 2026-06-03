import { createGatewayBundle } from "@/data/repository/factory";
import { Badge } from "@/presentation/components/ui/badge";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { updateArtifactStoragePreferenceAction, updateSessionTtlAction } from "./actions";
import { Button } from "@/presentation/components/ui/button";

export default async function ProjectSettingsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  const gateways = await createGatewayBundle();
  const project = await gateways.projectGateway.getProjectById(projectId);
  const teams = await gateways.teamGateway.listTeamsByProject(projectId);

  return (
    <div className="space-y-6">
      <ProjectSectionNav projectId={projectId} />
      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Storage Strategy
          </p>
          <h1 className="mt-3 text-3xl font-semibold tracking-tight">Artifact storage preference</h1>
          <p className="mt-3 text-sm text-muted-foreground">
            Choose the shared destination for synced artifact bytes. The current PC runner still
            owns local auth state and upload execution.
          </p>
          <form
            className="mt-4 flex flex-wrap items-center gap-3"
            action={async (formData: FormData) => {
              "use server";
              await updateArtifactStoragePreferenceAction(projectId, formData);
            }}
          >
            <select
              name="artifactStoragePreference"
              defaultValue={project?.artifactStoragePreference ?? "supabase"}
              className="min-w-52 rounded-lg border border-border bg-background px-3 py-2 text-sm"
            >
              <option value="supabase">supabase</option>
              <option value="google_drive">google_drive</option>
            </select>
            <Button type="submit" variant="secondary">
              Save
            </Button>
          </form>
          <div className="mt-4 flex gap-2">
            <Badge tone={(project?.artifactStoragePreference ?? "supabase") === "supabase" ? "success" : "neutral"}>supabase</Badge>
            <Badge tone={(project?.artifactStoragePreference ?? "supabase") === "google_drive" ? "success" : "neutral"}>google_drive</Badge>
          </div>
        </div>
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            MCP Contexts
          </p>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight">Status display only</h2>
          <p className="mt-3 text-sm text-muted-foreground">
            Integrations are handled in a later phase; this screen only exposes the current project linkage and future slots.
          </p>
          <div className="mt-4 space-y-2">
            {teams.length === 0 ? (
              <p className="text-sm text-muted-foreground">No configured MCP contexts yet.</p>
            ) : (
              teams.map((team) => (
                <div key={team.id} className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3">
                  <span className="font-medium">{team.name}</span>
                  <Badge>connected</Badge>
                </div>
              ))
            )}
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-2 mt-6">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Session Configuration
          </p>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight">Idle Timeout</h2>
          <p className="mt-3 text-sm text-muted-foreground">
            Configure how long a terminal session process should stay alive while waiting for follow-up prompts before being terminated (in minutes). Default is 120 (2 hours).
          </p>
          <form
            className="mt-4 flex gap-2 items-center"
            action={async (formData: FormData) => {
              "use server";
              await updateSessionTtlAction(projectId, formData);
            }}
          >
            <input
              name="sessionIdleTtlMinutes"
              type="number"
              defaultValue={project?.sessionIdleTtlMinutes ?? 120}
              className="w-24 rounded-lg border border-border bg-background px-3 py-2 text-sm"
              min="1"
            />
            <Button type="submit" variant="secondary">
              Save
            </Button>
          </form>
        </div>
      </section>
    </div>
  );
}
