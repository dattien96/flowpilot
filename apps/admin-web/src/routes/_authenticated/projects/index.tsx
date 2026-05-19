import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/projects/")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const projects = await gateways.projectGateway.listProjects();
    return { projects };
  },
  component: ProjectsPage,
});

function ProjectsPage() {
  const { projects } = Route.useLoaderData();

  return (
    <PageFrame
      actions={
        <Link
          className="inline-flex items-center justify-center rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold text-card-foreground transition-colors hover:bg-muted"
          to="/projects/create"
        >
          ADD project
        </Link>
      }
      description="Project registry with team and MCP entry points."
      title="Projects"
    >
      <div className="grid gap-4 xl:grid-cols-2">
        {projects.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground xl:col-span-2">
            No projects yet. Use ADD project to create the first one.
          </div>
        ) : null}
        {projects.map((project) => (
          <Link
            key={project.id}
            className="rounded-[1.5rem] border border-border bg-background/60 p-5 transition-transform hover:-translate-y-0.5"
            params={{ projectId: project.id }}
            to="/projects/$projectId"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <h3 className="text-xl font-semibold">{project.name}</h3>
                <p className="mt-2 text-sm text-muted-foreground">{project.description}</p>
              </div>
              <div className="flex flex-col items-end gap-2">
                <Badge>{project.platform}</Badge>
                <Badge>{project.status}</Badge>
              </div>
            </div>
            <p className="mt-4 text-xs text-muted-foreground">{project.repositoryUrl}</p>
            {project.directoryPath ? (
              <p className="mt-2 text-xs text-muted-foreground">
                Directory: {project.directoryPath}
              </p>
            ) : null}
            <span className="mt-4 inline-flex items-center justify-center rounded-full border border-border bg-card px-4 py-2 text-sm font-semibold text-card-foreground transition-colors hover:bg-muted">
              Open project
            </span>
          </Link>
        ))}
      </div>
    </PageFrame>
  );
}
