import { ProjectSectionNav } from "@/components/project/project-section-nav";

export default async function ProjectCodingPlanPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  return (
    <div className="space-y-6">
      <ProjectSectionNav projectId={projectId} />
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <h1 className="text-3xl font-semibold tracking-tight">Coding Plan</h1>
        <p className="mt-3 text-sm text-muted-foreground">Implementation slices for the project should be tracked in this tab.</p>
      </section>
    </div>
  );
}
