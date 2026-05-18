import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";

export const Route = createFileRoute("/_authenticated/projects/")({
  component: ProjectsPage,
});

function ProjectsPage() {
  const sampleProjects = [
    { id: "alpha", name: "Alpha Workspace" },
    { id: "beta", name: "Beta Workspace" },
  ];

  return (
    <PageFrame
      description="Project listing is scaffolded so nested project routes can be exercised before deeper data migration lands."
      title="Projects"
    >
      <div className="grid gap-4 md:grid-cols-2">
        {sampleProjects.map((project) => (
          <article
            key={project.id}
            className="rounded-[1.5rem] border border-border bg-background/60 p-5"
          >
            <h3 className="text-xl font-semibold">{project.name}</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              Route-ready project shell for `{project.id}`.
            </p>
            <Link params={{ projectId: project.id }} to="/projects/$projectId/business-logic">
              <Button className="mt-4">Open project</Button>
            </Link>
          </article>
        ))}
      </div>
    </PageFrame>
  );
}
